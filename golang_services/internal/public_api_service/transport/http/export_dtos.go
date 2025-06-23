package http // public_api_service/transport/http

// CreateExportRequestDTO is used by clients to request a data export.
type CreateExportRequestDTO struct {
	EntityType string            `json:"entity_type" validate:"required,eq=outbox_messages"` // For now, only "outbox_messages" is supported
	Filters    map[string]string `json:"filters,omitempty"`                                // Optional filters like "date_from", "date_to", "status"
	// RequesterEmail string `json:"requester_email,omitempty"` // Email for notifications will be taken from auth context
}

// CreateExportResponseDTO is returned to the client after successfully queueing an export request.
type CreateExportResponseDTO struct {
	Message    string `json:"message"`              // e.g., "Export request accepted and is being processed."
	RequestID  string `json:"request_id,omitempty"` // Optional: an ID for tracking this specific export request batch
}
