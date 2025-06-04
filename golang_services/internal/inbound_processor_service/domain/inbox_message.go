package domain

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// InboxMessage represents an SMS message received from an external provider.
type InboxMessage struct {
	ID                   uuid.UUID     `json:"id"`
	From                 string        `json:"from"`
	To                   string        `json:"to"`
	Text                 string        `json:"text"`
	ProviderMessageID    string        `json:"provider_message_id"` // Unique ID from the provider
	ProviderName         string        `json:"provider_name"`       // Name of the provider (e.g., "twilio", "infobip")
	UserID               uuid.NullUUID `json:"user_id,omitempty"`   // If associated with a user
	PrivateNumberID      uuid.NullUUID `json:"private_number_id,omitempty"` // If associated with a specific private number
	ReceivedByProviderAt sql.NullTime  `json:"received_by_provider_at,omitempty"` // When the provider originally received it
	ReceivedByGatewayAt  time.Time     `json:"received_by_gateway_at"`            // When our public API received the callback
	ProcessedAt          time.Time     `json:"processed_at"`                      // When this inbound-processor-service processed it
	// Could add fields like: IsRead, ParsedData (map[string]interface{}), etc. in future
}

// NewInboxMessage creates a new InboxMessage instance.
// The ID is typically generated before calling this.
// ReceivedByGatewayAt is the timestamp from the incoming NATS message (originally from provider callback).
// ProcessedAt is set at the time of processing by this service.
func NewInboxMessage(
	id uuid.UUID,
	from string,
	to string,
	text string,
	providerMessageID string,
	providerName string,
	receivedByProviderAt sql.NullTime,
	receivedByGatewayAt time.Time,
) *InboxMessage {
	return &InboxMessage{
		ID:                   id,
		From:                 from,
		To:                   to,
		Text:                 text,
		ProviderMessageID:    providerMessageID,
		ProviderName:         providerName,
		ReceivedByProviderAt: receivedByProviderAt,
		ReceivedByGatewayAt:  receivedByGatewayAt,
		ProcessedAt:          time.Now().UTC(), // Set by this service upon processing
		// UserID and PrivateNumberID would be set later after association logic
	}
}
