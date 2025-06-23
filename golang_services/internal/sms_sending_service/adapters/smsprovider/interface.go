package smsprovider

import (
	"context"
	// "github.com/aradsms/golang_services/internal/core_domain" // If OutboxMessage is used directly
)

// SMSRequestData holds the necessary data for sending an SMS via a provider.
type SMSRequestData struct {
	InternalMessageID string // Our system's message ID from OutboxMessage
	SenderID          string
	Recipient         string
	Content           string
	// Add other fields like UserData, IsUnicode, etc., if needed by adapters
}

// SMSResponseData holds the outcome of a send attempt from a provider.
type SMSResponseData struct {
	ProviderMessageID string // The ID returned by the provider
	Success           bool   // True if successfully submitted to provider
	StatusCode        int    // Provider's status code or HTTP status code
	ErrorMessage      string // Error message if not successful
	ProviderName      string // Name of the provider that handled this
}

// Adapter defines the interface for an SMS provider adapter.
type Adapter interface {
	Send(ctx context.Context, request SMSRequestData) (*SMSResponseData, error)
	GetName() string // Returns the name of the provider (e.g., "mock", "arad", "magfa")
    // HealthCheck(ctx context.Context) error // Optional: to check provider connectivity
}
