package http

import "time"

// ProviderDLRCallbackRequest represents the expected structure of a delivery report
// callback received from an external SMS provider.
type ProviderDLRCallbackRequest struct {
	// MessageID is typically the ID our system initially assigned or the one we received
	// from the provider upon message submission.
	MessageID string `json:"message_id" validate:"required"`

	// ProviderMessageID is an optional field for cases where the provider uses a different ID
	// for the message than the one initially exchanged or if they provide multiple IDs.
	ProviderMessageID string `json:"provider_message_id,omitempty"`

	// Status indicates the delivery status of the message (e.g., "DELIVERED", "FAILED", "EXPIRED").
	// Providers might have their own specific status codes/texts.
	Status string `json:"status" validate:"required"`

	// ErrorCode is an optional code provided by the provider if the message delivery failed or was rejected.
	ErrorCode string `json:"error_code,omitempty"`

	// ErrorDescription provides a human-readable description of the error, if any.
	ErrorDescription string `json:"error_description,omitempty"`

	// Timestamp indicates when the delivery report event occurred.
	// Using time.Time for automatic parsing if RFC3339 format is sent.
	// If providers use custom formats, string + custom parsing might be needed.
	Timestamp time.Time `json:"timestamp" validate:"required"`

	// ProviderSpecificData can capture any additional, non-standard fields
	// sent by the provider in the DLR callback.
	ProviderSpecificData map[string]interface{} `json:"provider_specific_data,omitempty"`
}

// ProviderIncomingSMSRequest represents the expected structure of an incoming SMS message
// callback received from an external SMS provider.
type ProviderIncomingSMSRequest struct {
	// From is the sender's phone number (MSISDN).
	From string `json:"from" validate:"required"`

	// To is the recipient number on our platform, e.g., a shortcode or long number owned by the service.
	To string `json:"to" validate:"required"`

	// Text contains the content of the SMS message.
	Text string `json:"text" validate:"required_without_all=BinaryData"` // Example: text or binary data

	// MessageID is the unique ID assigned by the provider for this incoming message.
	MessageID string `json:"message_id" validate:"required"`

	// Timestamp indicates when the message was received by the provider.
	Timestamp time.Time `json:"timestamp" validate:"required"`

	// ProviderSpecificData can capture any additional, non-standard fields
	// sent by the provider for the incoming SMS.
	ProviderSpecificData map[string]interface{} `json:"provider_specific_data,omitempty"`

	// Some providers might send content in binary form (e.g., UDH for concatenated SMS, WAP Push)
	// BinaryData []byte `json:"binary_data,omitempty"`
}
