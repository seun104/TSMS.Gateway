package app

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/your-repo/project/internal/inbound_processor_service/domain"
)

// SMSProcessor is responsible for processing validated incoming SMS messages
// and storing them in the database, potentially associating them with users/private numbers.
type SMSProcessor struct {
	inboxRepo      domain.InboxRepository
	privateNumRepo domain.PrivateNumberRepository // Added repository for private numbers
	logger         *slog.Logger
}

// NewSMSProcessor creates a new SMSProcessor instance.
func NewSMSProcessor(
	inboxRepo domain.InboxRepository,
	privateNumRepo domain.PrivateNumberRepository,
	logger *slog.Logger,
) *SMSProcessor {
	return &SMSProcessor{
		inboxRepo:      inboxRepo,
		privateNumRepo: privateNumRepo,
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
	return nil
}
