package app

import (
	"context"
	"database/sql"
	"log/slog"
	"time"
	"strings" // For keyword parsing
	"encoding/json" // For NATS event payload

	"github.com/google/uuid"
	"github.com/AradIT/aradsms/golang_services/internal/inbound_processor_service/domain" // Corrected path
	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker"
)

var keywordActions = map[string]string{
	"STOP": "inbound.keyword.stop",
	"HELP": "inbound.keyword.help",
	"START": "inbound.keyword.start", // Example: Re-subscribe
	"INFO": "inbound.keyword.info",
}

// SMSProcessor is responsible for processing validated incoming SMS messages
// and storing them in the database, potentially associating them with users/private numbers,
// and acting on keywords.
type SMSProcessor struct {
	inboxRepo      domain.InboxRepository
	privateNumRepo domain.PrivateNumberRepository
	natsClient     messagebroker.NATSClient // Added NATS client
	logger         *slog.Logger
}

// NewSMSProcessor creates a new SMSProcessor instance.
func NewSMSProcessor(
	inboxRepo domain.InboxRepository,
	privateNumRepo domain.PrivateNumberRepository,
	natsClient messagebroker.NATSClient, // Added
	logger *slog.Logger,
) *SMSProcessor {
	return &SMSProcessor{
		inboxRepo:      inboxRepo,
		privateNumRepo: privateNumRepo,
		natsClient:     natsClient, // Added
		logger:         logger,
	}
}

// ProcessMessage takes an InboundSMSEvent (which contains the raw provider data and provider name),
// transforms it into a domain.InboxMessage, and saves it to the database.
func (s *SMSProcessor) ProcessMessage(ctx context.Context, event InboundSMSEvent) error {
	s.logger.InfoContext(ctx, "Processing message from provider",
		"provider_name", event.ProviderName,
		"provider_message_id", event.Data.MessageID,
		"from", event.Data.From,
		"to", event.Data.To,
	)

	// Transform ProviderIncomingSMSRequest (from NATS) to domain.InboxMessage
	// Generate a new UUID for the InboxMessage ID.
	messageID := uuid.New()

	// Convert event.Data.Timestamp (when provider received it) to sql.NullTime
	var receivedByProviderAt sql.NullTime
	if !event.Data.Timestamp.IsZero() {
		receivedByProviderAt = sql.NullTime{Time: event.Data.Timestamp, Valid: true}
	}

	// Create the domain model for InboxMessage
	// ReceivedByGatewayAt is not directly available in ProviderIncomingSMSRequest,
	// it would have been set when the public-api first received the callback.
	// For now, we'll use the event.Data.Timestamp also as a proxy for ReceivedByGatewayAt,
	// or assume it's very close to ReceivedByProviderAt.
	// A more robust solution might involve adding ReceivedByGatewayAt to the NATS message payload.
	// For this iteration, let's assume event.Data.Timestamp is what we have for gateway receipt time.
	inboxMsg := domain.NewInboxMessage(
		messageID,
		event.Data.From,
		event.Data.To,
		event.Data.Text,
		event.Data.MessageID, // This is ProviderMessageID
		event.ProviderName,
		receivedByProviderAt,
		event.Data.Timestamp, // Using provider's timestamp as ReceivedByGatewayAt for now
	)
	// UserID and PrivateNumberID are left as default (uuid.NullUUID) for now.
	// ProcessedAt is set by NewInboxMessage().

	// Attempt to associate with a private number and user
	privateNum, pnErr := s.privateNumRepo.FindByNumber(ctx, inboxMsg.To)
	if pnErr != nil {
		// Log the error but don't fail the entire message processing for this.
		// The message will be saved without association.
		s.logger.ErrorContext(ctx, "Failed to query private number for association",
			"error", pnErr,
			"recipient_number", inboxMsg.To,
			"inbox_message_id", inboxMsg.ID,
		)
	} else if privateNum != nil {
		inboxMsg.UserID = uuid.NullUUID{UUID: privateNum.UserID, Valid: true}
		inboxMsg.PrivateNumberID = uuid.NullUUID{UUID: privateNum.ID, Valid: true}
		s.logger.InfoContext(ctx, "Associated incoming SMS with private number and user",
			"inbox_message_id", inboxMsg.ID,
			"private_number_id", privateNum.ID,
			"user_id", privateNum.UserID,
		)
	} else {
		s.logger.InfoContext(ctx, "No private number found for association",
			"recipient_number", inboxMsg.To,
			"inbox_message_id", inboxMsg.ID,
		)
	}

	s.logger.DebugContext(ctx, "Final InboxMessage before saving", "inbox_message_id", inboxMsg.ID, "details", inboxMsg)

	// Save to database
	err := s.inboxRepo.Create(ctx, inboxMsg)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to save inbox message to database",
			"error", err,
			"inbox_message_id", inboxMsg.ID,
			"provider_message_id", inboxMsg.ProviderMessageID,
		)
		// TODO: Consider retry logic or dead-lettering for certain types of errors.
		return err
	}

	s.logger.InfoContext(ctx, "Successfully processed and saved inbox message",
		"inbox_message_id", inboxMsg.ID,
		"provider_name", event.ProviderName,
		"provider_message_id", event.Data.MessageID,
		"associated_user_id", inboxMsg.UserID,
		"associated_private_number_id", inboxMsg.PrivateNumberID,
	)

	// After successfully saving, parse for keywords and act
	s.parseAndActOnKeywords(ctx, inboxMsg)

	return nil
}

// KeywordDetectedEvent is the payload for NATS events when a keyword is detected.
type KeywordDetectedEvent struct {
	InboxMessageID    uuid.UUID     `json:"inbox_message_id"`
	ProviderMessageID string        `json:"provider_message_id"` // The ID from the provider
	SenderNumber      string        `json:"sender_number"`       // From number
	RecipientNumber   string        `json:"recipient_number"`    // To number (usually the private number)
	MatchedKeyword    string        `json:"matched_keyword"`
	OriginalText      string        `json:"original_text"`
	ReceivedAt        time.Time     `json:"received_at"`         // Timestamp from InboxMessage (e.g., ReceivedByGatewayAt)
	UserID            uuid.NullUUID `json:"user_id,omitempty"`   // Associated UserID, if any
}

func (s *SMSProcessor) parseAndActOnKeywords(ctx context.Context, msg *domain.InboxMessage) {
	if s.natsClient == nil {
		s.logger.ErrorContext(ctx, "NATS client is nil in SMSProcessor, cannot publish keyword events", "message_id", msg.ID)
		return
	}

	normalizedText := strings.ToUpper(strings.TrimSpace(msg.Text))
	s.logger.DebugContext(ctx, "Parsing for keywords", "message_id", msg.ID, "normalized_text", normalizedText)

	for keyword, natsSubject := range keywordActions {
		// Check if the normalized text *is* the keyword, or *starts with* the keyword followed by a space or end of string.
		if normalizedText == keyword || strings.HasPrefix(normalizedText, keyword+" ") {
			s.logger.InfoContext(ctx, "Keyword detected",
				"keyword", keyword,
				"message_id", msg.ID,
				"nats_subject", natsSubject)

			eventPayload := KeywordDetectedEvent{
				InboxMessageID:    msg.ID,
				ProviderMessageID: msg.ProviderMessageID,
				SenderNumber:      msg.From,
				RecipientNumber:   msg.To,
				MatchedKeyword:    keyword,
				OriginalText:      msg.Text,
				ReceivedAt:        msg.ReceivedByGatewayAt, // Assuming this is the relevant timestamp
				UserID:            msg.UserID,
			}
			payloadBytes, err := json.Marshal(eventPayload)
			if err != nil {
				s.logger.ErrorContext(ctx, "Failed to marshal keyword event payload", "error", err, "keyword", keyword, "message_id", msg.ID)
				continue // Log error and try next keyword or stop? For now, continue.
			}

			if err := s.natsClient.Publish(ctx, natsSubject, payloadBytes); err != nil {
				s.logger.ErrorContext(ctx, "Failed to publish keyword detected event to NATS",
					"error", err,
					"subject", natsSubject,
					"keyword", keyword,
					"message_id", msg.ID)
				// TODO: Consider retry logic or dead-lettering for NATS publish failures.
			} else {
				s.logger.InfoContext(ctx, "Successfully published keyword event", "subject", natsSubject, "keyword", keyword, "message_id", msg.ID)
			}
			// If a message can trigger multiple keywords (e.g. "HELP STOP"), this loop will publish for each.
			// If only the first match should trigger, add 'return' here.
		}
	}
}
