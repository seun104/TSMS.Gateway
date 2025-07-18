package domain // billing_service/domain

import (
	"context"
	"time"
	"github.com/google/uuid"
)

// PaymentIntentStatus defines the possible statuses for a payment intent.
type PaymentIntentStatus string

const (
	PaymentIntentStatusPending        PaymentIntentStatus = "pending"
	PaymentIntentStatusRequiresAction PaymentIntentStatus = "requires_action" // e.g. 3DS authentication
	PaymentIntentStatusSucceeded      PaymentIntentStatus = "succeeded"
	PaymentIntentStatusFailed         PaymentIntentStatus = "failed"
	PaymentIntentStatusCancelled      PaymentIntentStatus = "cancelled"
)

// PaymentIntent represents a charge attempt or payment process.
type PaymentIntent struct {
	ID                     uuid.UUID
	UserID                 uuid.UUID
	Amount                 int64 // In smallest currency unit (e.g., cents, rials)
	Currency               string
	Status                 PaymentIntentStatus
	GatewayPaymentIntentID *string // ID from the payment gateway, nullable
	GatewayClientSecret    *string // Secret for client-side actions (e.g., Stripe client secret), nullable
	ErrorMessage           *string // Error message from gateway or internal processing, nullable
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// PaymentIntentRepository defines the interface for database operations on PaymentIntent.
type PaymentIntentRepository interface {
	Create(ctx context.Context, pi *PaymentIntent) error
	GetByID(ctx context.Context, id uuid.UUID) (*PaymentIntent, error)
	GetByGatewayPaymentIntentID(ctx context.Context, gatewayPaymentIntentID string) (*PaymentIntent, error)
	Update(ctx context.Context, pi *PaymentIntent) error
	// ListByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]*PaymentIntent, error) // Optional for later
}

// --- Payment Gateway Adapter related structures ---

// CreateIntentRequest is used by the application service to request payment intent creation from a gateway adapter.
type CreateIntentRequest struct {
	Amount            int64
	Currency          string
	UserID            uuid.UUID
	Email             string // Often required/useful for gateways
	Description       string // Optional description for the payment
	ReturnURL         string // Optional: URL to redirect after payment attempt for some gateways
	InternalRequestID string // Optional: for idempotency or logging
}

// CreateIntentResponse is returned by the gateway adapter after attempting to create a payment intent.
type CreateIntentResponse struct {
	// Our internal PaymentIntent ID is NOT set by the adapter, but by the service that calls the adapter.
	GatewayPaymentIntentID string              // The ID generated by the payment gateway.
	ClientSecret           *string             // If the gateway uses a client secret for front-end confirmation (e.g., Stripe).
	NextActionURL          *string             // If the user needs to be redirected to a URL for authentication/completion.
	Status                 PaymentIntentStatus // The initial status of the intent at the gateway.
	ErrorMessage           *string             // Any error message from the gateway during creation.
	RawGatewayResponse     map[string]interface{} // Optional: For storing raw response from gateway if needed for debugging
}

// PaymentGatewayEvent represents a normalized event received from a payment gateway webhook.
type PaymentGatewayEvent struct {
	GatewayPaymentIntentID string                 // The gateway's ID for the payment intent.
	GatewayTransactionID   *string                // Optional: The gateway's specific transaction/charge ID related to this event (e.g., Stripe's ch_ or py_ id)
	Type                   string                 // Gateway-specific event type (e.g., "payment_intent.succeeded").
	AmountReceived         int64                  // Amount received, for reconciliation.
	Currency               string                 // Currency of the amount received.
	Data                   map[string]interface{} // Raw event data from the gateway, for auditing or detailed processing.
	OccurredAt             time.Time              // Timestamp of when the event occurred at the gateway.
}

// PaymentGatewayAdapter defines the interface for interacting with a payment gateway.
type PaymentGatewayAdapter interface {
	// CreatePaymentIntent initiates a payment with the gateway.
	CreatePaymentIntent(ctx context.Context, req CreateIntentRequest) (*CreateIntentResponse, error)

	// HandleWebhookEvent processes an incoming webhook event from the gateway.
	// It should verify the event's authenticity (e.g., using the signature) and then parse it
	// into a normalized PaymentGatewayEvent.
	HandleWebhookEvent(ctx context.Context, rawPayload []byte, signature string) (*PaymentGatewayEvent, error)

	// GetName returns the name of the gateway adapter (e.g., "stripe", "mock").
	// GetName() string // Optional, but good for managing multiple gateways
}
