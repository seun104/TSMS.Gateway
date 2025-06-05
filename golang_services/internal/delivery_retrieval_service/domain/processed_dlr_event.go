package domain

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// ProcessedDLREvent represents a delivery report that has been processed
// by the delivery_retrieval_service and is being published for other services to consume.
type ProcessedDLREvent struct {
	MessageID          uuid.UUID     `json:"message_id"`           // Our internal message ID (from outbox_messages.id)
	UserID             uuid.NullUUID `json:"user_id,omitempty"`    // User associated with the message
	Status             string        `json:"status"`               // Normalized status (e.g., "Delivered", "Failed")
	ProviderName       string        `json:"provider_name"`        // Name of the SMS provider
	ProviderStatus     string        `json:"provider_status"`      // Raw status text from the provider
	DeliveredAt        sql.NullTime  `json:"delivered_at,omitempty"` // Timestamp of actual delivery
	ErrorCode          sql.NullString `json:"error_code,omitempty"`   // Error code from provider, if any
	ErrorDescription   sql.NullString `json:"error_description,omitempty"` // Error description from provider, if any
	ProcessedTimestamp time.Time     `json:"processed_timestamp"`  // When this DLR was processed by delivery_retrieval_service
}
