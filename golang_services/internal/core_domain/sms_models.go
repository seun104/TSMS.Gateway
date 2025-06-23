package core_domain

import (
	"time"
	// "github.com/google/uuid" // If using UUID type directly
)

// MessageStatus defines the possible statuses of an SMS message.
type MessageStatus string

const (
	StatusQueued        MessageStatus = "queued"
	StatusProcessing    MessageStatus = "processing"
	StatusSent          MessageStatus = "sent"          // Dispatched to provider
	StatusFailedToSend  MessageStatus = "failed_to_send" // Failed to dispatch to provider
	StatusDelivered     MessageStatus = "delivered"
	StatusUndelivered   MessageStatus = "undelivered"
	StatusExpired       MessageStatus = "expired"
	StatusRejected      MessageStatus = "rejected"
)

// OutboxMessage represents an outgoing SMS message.
type OutboxMessage struct {
	ID                string        `json:"id"` // UUID
	UserID            string        `json:"user_id"` // UUID of the user who sent it
	BatchID           *string       `json:"batch_id,omitempty"` // Optional UUID for grouping bulk messages
	SenderID          string        `json:"sender_id"`
	Recipient         string        `json:"recipient"`
	Content           string        `json:"content"`
	Status            MessageStatus `json:"status"`
	Segments          int           `json:"segments"`
	ProviderMessageID *string       `json:"provider_message_id,omitempty"` // ID from the SMS provider
	ProviderStatusCode*string       `json:"provider_status_code,omitempty"`
	ErrorMessage      *string       `json:"error_message,omitempty"`
	UserData          *string       `json:"user_data,omitempty"` // Optional user-defined data
	ScheduledFor      *time.Time    `json:"scheduled_for,omitempty"`
	ProcessedAt       *time.Time    `json:"processed_at,omitempty"` // When sending service picked it up
	SentAt            *time.Time    `json:"sent_at,omitempty"`      // When provider confirmed sending
	DeliveredAt       *time.Time    `json:"delivered_at,omitempty"` // When DLR confirmed delivery
	LastStatusUpdateAt *time.Time   `json:"last_status_update_at,omitempty"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
    SMSProviderID     *string       `json:"sms_provider_id,omitempty"` // UUID of the provider used
    RouteID           *string       `json:"route_id,omitempty"`      // UUID of the route used
}

// InboxMessage represents an incoming SMS message.
type InboxMessage struct {
	ID                string     `json:"id"` // UUID
	UserID            *string    `json:"user_id,omitempty"` // User associated with the private number (if any)
	PrivateNumberID   string     `json:"private_number_id"` // UUID of the private number it was sent to
	ProviderMessageID *string    `json:"provider_message_id,omitempty"` // ID from SMS provider
	SenderNumber      string     `json:"sender_number"`
	RecipientNumber   string     `json:"recipient_number"` // The private number
	Content           string     `json:"content"`
	ReceivedAt        time.Time  `json:"received_at"` // Timestamp from provider or when ingested
	IsParsed          bool       `json:"is_parsed"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// SMSProvider represents an SMS service provider configuration.
type SMSProvider struct {
	ID                             string    `json:"id"` // UUID
	Name                           string    `json:"name"`
	APIConfig                      string    `json:"-"` // JSONB in DB, string for Go struct (to be marshalled/unmarshalled)
	IsActive                       bool      `json:"is_active"`
	SupportsDeliveryReportPolling  bool      `json:"supports_delivery_report_polling"`
	SupportsDeliveryReportCallback bool      `json:"supports_delivery_report_callback"`
	CreatedAt                      time.Time `json:"created_at"`
	UpdatedAt                      time.Time `json:"updated_at"`
}

// PrivateNumber represents a sender ID or number owned/assigned by the system.
type PrivateNumber struct {
	ID            string     `json:"id"` // UUID
	UserID        *string    `json:"user_id,omitempty"` // User who owns this number (if not shared)
	NumberValue   string     `json:"number_value"`
	SMSProviderID *string    `json:"sms_provider_id,omitempty"` // Default provider for this number
	IsShared      bool       `json:"is_shared"`
	Capabilities  string     `json:"-"` // JSONB in DB, string for Go struct (e.g., {"can_receive_sms": true})
	IsActive      bool       `json:"is_active"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// Route defines rules for routing messages to SMS providers.
type Route struct {
	ID            string    `json:"id"` // UUID
	Name          string    `json:"name"`
	Priority      int       `json:"priority"` // Lower is higher
	Criteria      string    `json:"-"` // JSONB in DB, string for Go struct (e.g., {"country_code": "98"})
	SMSProviderID string    `json:"sms_provider_id"` // UUID of the provider to route to
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
