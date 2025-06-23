package domain

import (
	"time"

	"github.com/google/uuid"
)

// OutboxMessage represents a message that has been sent or is queued for sending.
// This is a minimal representation for the delivery_retrieval_service,
// focusing on fields relevant to DLR processing.
type OutboxMessage struct {
	ID            uuid.UUID      `json:"id"`
	UserID        uuid.UUID      `json:"user_id"` // For context, logging, or potential future rules
	Status        DeliveryStatus `json:"status"`  // Current normalized status in our system
	ProviderMSGID sql.NullString `json:"provider_msg_id"` // Provider's message ID
	// Other fields like Sender, PhoneNumber, Content, etc., are omitted for DLR processing focus.
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
