package domain

import "time"

// ProviderIncomingSMSRequest mirrors the DTO used by public-api-service
// for receiving raw incoming SMS data from a provider via NATS.
// This struct is used for deserializing the NATS message payload.
type ProviderIncomingSMSRequest struct {
	From                 string                 `json:"from"`
	To                   string                 `json:"to"`
	Text                 string                 `json:"text"`
	MessageID            string                 `json:"message_id"` // Provider's ID for this incoming message
	Timestamp            time.Time              `json:"timestamp"`  // When the message was received by the provider
	ProviderSpecificData map[string]interface{} `json:"provider_specific_data,omitempty"`
	// Note: The 'ProviderName' will be part of the NATS subject, not typically in the payload itself from public-api.
}
