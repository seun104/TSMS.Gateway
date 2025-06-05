package domain

import "github.com/google/uuid"

// PrivateNumber represents a dedicated number assigned to a user or service.
// This is a minimal representation for the inbound-processor-service,
// focusing on fields relevant for associating an incoming SMS.
type PrivateNumber struct {
	ID        uuid.UUID `json:"id"`
	Number    string    `json:"number"`     // The actual phone number string
	UserID    uuid.UUID `json:"user_id"`    // The user this private number is assigned to
	ServiceID string    `json:"service_id"` // Optional: an identifier for a specific service using this number
	// Other fields like ProviderID, Capabilities, CreatedAt, UpdatedAt are omitted for this context.
}
