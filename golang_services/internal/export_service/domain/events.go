package domain // export_service/domain

import "github.com/google/uuid"

const (
	NATSExportRequestOutboxV1   = "export.request.outbox.v1"
	NATSExportCompletedOutboxV1 = "export.completed.outbox.v1"
	NATSExportFailedOutboxV1    = "export.failed.outbox.v1"
)

// ExportRequestEvent is published by public-api-service to request a data export.
type ExportRequestEvent struct {
	UserID         uuid.UUID         `json:"user_id"`
	Filters        map[string]string `json:"filters,omitempty"` // e.g., "date_from", "date_to", "status"
	RequesterEmail string            `json:"requester_email,omitempty"` // For sending notifications upon completion/failure
}

// ExportCompletedEvent is published by export-service when an export is successfully completed.
type ExportCompletedEvent struct {
	UserID         uuid.UUID `json:"user_id"`
	FilePath       string    `json:"file_path"` // Path where the file is stored (local to export-service or a shared storage URL)
	FileName       string    `json:"file_name"` // Just the name of the file
	RequesterEmail string    `json:"requester_email,omitempty"`
	// Could add FileSize, RecordCount etc.
}

// ExportFailedEvent is published by export-service when an export attempt fails.
type ExportFailedEvent struct {
	UserID          uuid.UUID         `json:"user_id"`
	ErrorMessage    string            `json:"error_message"`
	OriginalFilters map[string]string `json:"original_filters,omitempty"`
	RequesterEmail  string            `json:"requester_email,omitempty"`
}
