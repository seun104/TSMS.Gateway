package domain

import "time"

// ProviderDLRCallbackRequest mirrors the DTO used by public-api-service
// for receiving raw DLR data from a provider via NATS.
// This struct is used for deserializing the NATS message payload.
type ProviderDLRCallbackRequest struct {
	MessageID            string                 `json:"message_id"` // Original MessageID (our system or provider's)
	ProviderMessageID    string                 `json:"provider_message_id,omitempty"`
	Status               string                 `json:"status"` // Provider's raw status string
	ErrorCode            string                 `json:"error_code,omitempty"`
	ErrorDescription     string                 `json:"error_description,omitempty"`
	Timestamp            time.Time              `json:"timestamp"` // Time of DLR event from provider
	ProviderSpecificData map[string]interface{} `json:"provider_specific_data,omitempty"`
}
