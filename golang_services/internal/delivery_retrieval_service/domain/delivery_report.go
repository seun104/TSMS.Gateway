package domain

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// DeliveryStatus represents the normalized status of an SMS delivery report.
type DeliveryStatus int

const (
	// DeliveryStatusUnknown indicates an undetermined status.
	DeliveryStatusUnknown DeliveryStatus = iota
	// DeliveryStatusQueued means the message is queued for delivery by the provider.
	DeliveryStatusQueued
	// DeliveryStatusSent means the message has been sent by the provider but not yet confirmed delivered.
	DeliveryStatusSent
	// DeliveryStatusDelivered means the message was successfully delivered to the handset.
	DeliveryStatusDelivered
	// DeliveryStatusFailed means the message delivery failed.
	DeliveryStatusFailed
	// DeliveryStatusExpired means the message expired before delivery could be completed.
	DeliveryStatusExpired
	// DeliveryStatusRejected means the message was rejected by the provider or carrier.
	DeliveryStatusRejected
	// DeliveryStatusUndeliverable means the message could not be delivered (e.g., invalid number).
	DeliveryStatusUndeliverable
)

// String returns the string representation of the DeliveryStatus.
func (s DeliveryStatus) String() string {
	switch s {
	case DeliveryStatusQueued:
		return "Queued"
	case DeliveryStatusSent:
		return "Sent"
	case DeliveryStatusDelivered:
		return "Delivered"
	case DeliveryStatusFailed:
		return "Failed"
	case DeliveryStatusExpired:
		return "Expired"
	case DeliveryStatusRejected:
		return "Rejected"
	case DeliveryStatusUndeliverable:
		return "Undeliverable"
	case DeliveryStatusUnknown:
		return "Unknown"
	default:
		return "Unknown"
	}
}

// DeliveryReport represents the information received from an SMS provider
// about the status of a previously sent message.
type DeliveryReport struct {
	// Our system's internal message ID, linking to an OutboxMessage.
	MessageID uuid.UUID `json:"message_id"`
	// The ID assigned by the SMS provider for this message.
	ProviderMessageID string `json:"provider_message_id"`
	// Normalized status of the delivery.
	Status DeliveryStatus `json:"status"`
	// Raw status code or text received from the provider.
	ProviderStatus string `json:"provider_status"`
	// Timestamp when the message was confirmed delivered to the handset.
	// Nullable, as it's only present for successful deliveries.
	DeliveredAt sql.NullTime `json:"delivered_at,omitempty"`
	// Error code from the provider, if any.
	ErrorCode sql.NullString `json:"error_code,omitempty"`
	// Textual description of the error, if any.
	ErrorDescription sql.NullString `json:"error_description,omitempty"`
	// Timestamp when this delivery report was received and processed by our system.
	ReceivedAt time.Time `json:"received_at"`
	// Could also include:
	// ProviderSpecificData map[string]interface{} `json:"provider_specific_data,omitempty"`
	// SegmentCount int `json:"segment_count,omitempty"` // If the provider gives this info
}

// NewDeliveryReport creates a new DeliveryReport instance.
// This can be expanded with validation or default setting logic.
func NewDeliveryReport(
	messageID uuid.UUID,
	providerMessageID string,
	status DeliveryStatus,
	providerStatus string,
	deliveredAt sql.NullTime,
	errorCode sql.NullString,
	errorDescription sql.NullString,
) *DeliveryReport {
	return &DeliveryReport{
		MessageID:         messageID,
		ProviderMessageID: providerMessageID,
		Status:            status,
		ProviderStatus:    providerStatus,
		DeliveredAt:       deliveredAt,
		ErrorCode:         errorCode,
		ErrorDescription:  errorDescription,
		ReceivedAt:        time.Now().UTC(), // Default to current time in UTC
	}
}
