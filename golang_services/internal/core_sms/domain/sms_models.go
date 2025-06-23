package domain

import (
	"time"
	// "github.com/google/uuid" // If using UUIDs directly
	"database/sql/driver" // For custom ENUM Scan/Value
    "fmt"
)

// MessageStatus defines the possible states of an SMS message.
type MessageStatus string

const (
	MessageStatusQueued            MessageStatus = "queued"
	MessageStatusProcessing        MessageStatus = "processing"
	MessageStatusSentToProvider    MessageStatus = "sent_to_provider" // Renamed from "sent" to be more specific
	MessageStatusFailedProviderSubmission MessageStatus = "failed_provider_submission" // Renamed from "failed_to_send"
	MessageStatusDelivered         MessageStatus = "delivered"
	MessageStatusUndelivered       MessageStatus = "undelivered"
	MessageStatusExpired           MessageStatus = "expired"
	MessageStatusRejected          MessageStatus = "rejected"
    MessageStatusAccepted          MessageStatus = "accepted" // Accepted by provider, awaiting DLR
    MessageStatusUnknown           MessageStatus = "unknown"  // Default or error state for DLR
)

// Value implements the driver.Valuer interface for MessageStatus.
func (ms MessageStatus) Value() (driver.Value, error) {
	return string(ms), nil
}

// Scan implements the sql.Scanner interface for MessageStatus.
func (ms *MessageStatus) Scan(value interface{}) error {
	strVal, ok := value.(string)
	if !ok {
		bytesVal, ok := value.([]byte)
        if !ok {
            return fmt.Errorf("failed to scan MessageStatus: value is not string or []byte, it is %T", value)
        }
        strVal = string(bytesVal)
	}
	*ms = MessageStatus(strVal)
	// Optional: Validate if strVal is one of the known enum values
	switch *ms {
    case MessageStatusQueued, MessageStatusProcessing, MessageStatusSentToProvider, MessageStatusFailedProviderSubmission, MessageStatusDelivered, MessageStatusUndelivered, MessageStatusExpired, MessageStatusRejected, MessageStatusAccepted, MessageStatusUnknown:
        return nil
    default:
        // Optionally, set to a default like MessageStatusUnknown or return an error
        // *ms = MessageStatusUnknown
        return fmt.Errorf("unknown MessageStatus value: %s", strVal)
    }
}


// OutboxMessage represents an outgoing SMS message.
type OutboxMessage struct {
	ID                  string        `json:"id"` // UUID
	UserID              string        `json:"user_id"` // UUID of the user who sent it
	BatchID             *string       `json:"batch_id,omitempty"` // Optional UUID for bulk messages
	SenderID            string        `json:"sender_id"`
	Recipient           string        `json:"recipient"`
	Content             string        `json:"content"`
	Status              MessageStatus `json:"status"`
	Segments            int           `json:"segments"`
	ProviderMessageID   *string       `json:"provider_message_id,omitempty"` // ID from the SMS provider
	ProviderStatusCode  *string       `json:"provider_status_code,omitempty"`
	ErrorMessage        *string       `json:"error_message,omitempty"`
	UserData            *string       `json:"user_data,omitempty"` // Optional user-defined data
	ScheduledFor        *time.Time    `json:"scheduled_for,omitempty"`
	ProcessedAt         *time.Time    `json:"processed_at,omitempty"` // When sms-sending-service picked it up
	SentToProviderAt    *time.Time    `json:"sent_to_provider_at,omitempty"` // When provider confirmed acceptance or message sent
	DeliveredAt         *time.Time    `json:"delivered_at,omitempty"` // When DLR confirmed delivery
	LastStatusUpdateAt  *time.Time    `json:"last_status_update_at,omitempty"`
	SmsProviderID       *string       `json:"sms_provider_id,omitempty"` // UUID of the provider used
	RouteID             *string       `json:"route_id,omitempty"`      // UUID of the route used
	CreatedAt           time.Time     `json:"created_at"`
	UpdatedAt           time.Time     `json:"updated_at"`
}

// InboxMessage represents an incoming SMS message.
type InboxMessage struct {
	ID                string     `json:"id"` // UUID
	UserID            *string    `json:"user_id,omitempty"` // User associated with the private number (if any)
	PrivateNumberID   string     `json:"private_number_id"` // UUID of the private number it was sent to
	ProviderMessageID *string    `json:"provider_message_id,omitempty"`
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
	Name                           string    `json:"name"` // e.g., "Arad", "Magfa"
	ApiConfigJSON                  string    `json:"-"` // JSONB in DB, string representation of JSON for Go struct. Or map[string]interface{}
	IsActive                       bool      `json:"is_active"`
	SupportsDeliveryReportPolling  bool      `json:"supports_delivery_report_polling"`
	SupportsDeliveryReportCallback bool      `json:"supports_delivery_report_callback"`
	CreatedAt                      time.Time `json:"created_at"`
	UpdatedAt                      time.Time `json:"updated_at"`
}

// PrivateNumber represents a sender ID or number owned/used by users.
type PrivateNumber struct {
	ID            string     `json:"id"` // UUID
	UserID        *string    `json:"user_id,omitempty"` // User who owns/registered this number, can be null for system/shared numbers
	NumberValue   string     `json:"number_value"` // The actual sender ID/number
	SmsProviderID *string    `json:"sms_provider_id,omitempty"` // Provider through which this number operates (if specific)
	IsShared      bool       `json:"is_shared"`
	CapabilitiesJSON string    `json:"-"` // JSONB in DB, e.g., {"can_receive_sms": true, "is_alphanumeric": false}
	IsActive      bool       `json:"is_active"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// Route represents rules for routing SMS messages to different SMS providers.
type Route struct {
	ID            string    `json:"id"` // UUID
	Name          string    `json:"name"`
	Priority      int       `json:"priority"` // Lower is higher priority
	CriteriaJSON  string    `json:"-"` // JSONB in DB, e.g., {"country_code": "98", "operator_prefix": "912"}
	SmsProviderID string    `json:"sms_provider_id"` // UUID of the provider to route to
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
