package app // export_service/app

import (
	"context"
	"encoding/json"
	// "fmt" // Not directly used, but could be for more complex error messages
	"log/slog"
	"path/filepath" // For getting filename from path

	// "github.com/google/uuid" // Not directly used in this file, but in domain events
	exportDomain "github.com/AradIT/aradsms/golang_services/internal/export_service/domain"
	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker"
)

// NATSConsumer handles NATS messages for the export service.
type NATSConsumer struct {
	exportService *ExportService // Assumes ExportService has ExportOutboxMessagesToCSV method
	natsClient    messagebroker.NATSClient
	logger        *slog.Logger
}

// NewNATSConsumer creates a new NATSConsumer.
func NewNATSConsumer(exportService *ExportService, natsClient messagebroker.NATSClient, logger *slog.Logger) *NATSConsumer {
	return &NATSConsumer{
		exportService: exportService,
		natsClient:    natsClient,
		logger:        logger.With("component", "nats_consumer"),
	}
}

// HandleExportRequest processes incoming export requests from NATS.
// The actual message type for NATS will be messagebroker.Message, which has Data() []byte and Subject() string.
// The handler signature for natsClient.Subscribe should match this.
// Assuming natsClient.Subscribe provides a handler like: func(ctx context.Context, subject string, data []byte)
// or func(msg messagebroker.Message) which then extracts ctx, subject, data.
// For this example, using the simpler func(ctx context.Context, subject string, data []byte) for clarity.
func (c *NATSConsumer) HandleExportRequest(ctx context.Context, subject string, data []byte) {
	// Increment NATS messages received counter
	natsExportRequestsReceived.WithLabelValues(subject).Inc()

	c.logger.InfoContext(ctx, "Received export request on NATS", "subject", subject, "data_len", len(data))
	var reqEvent exportDomain.ExportRequestEvent
	if err := json.Unmarshal(data, &reqEvent); err != nil {
		c.logger.ErrorContext(ctx, "Failed to unmarshal export request event", "error", err, "data", string(data))
		// Not publishing a "bad request" event here to avoid potential loops if this message itself is malformed.
		// Dead-lettering or just logging might be appropriate.
		return
	}

	// Add a sub-logger with event details for all subsequent logs for this request
	requestLogger := c.logger.With("user_id", reqEvent.UserID.String(), "requester_email", reqEvent.RequesterEmail)
	requestLogger.InfoContext(ctx, "Processing export for user", "filters", reqEvent.Filters)

	// The context passed to ExportOutboxMessagesToCSV should ideally be a new context
	// derived from the initial context, perhaps with a timeout specific to export processing.
	// exportCtx, cancel := context.WithTimeout(ctx, 5*time.Minute) // Example timeout
	// defer cancel()
	// For now, passing through the consumer's context.
	filePath, err := c.exportService.ExportOutboxMessagesToCSV(ctx, reqEvent.UserID, reqEvent.Filters)
	if err != nil {
		requestLogger.ErrorContext(ctx, "Failed to export outbox messages", "error", err)
		failEvent := exportDomain.ExportFailedEvent{
			UserID:          reqEvent.UserID,
			ErrorMessage:    err.Error(),
			OriginalFilters: reqEvent.Filters,
			RequesterEmail:  reqEvent.RequesterEmail,
		}
		failPayload, marshalErr := json.Marshal(failEvent)
		if marshalErr != nil {
			requestLogger.ErrorContext(ctx, "Failed to marshal export failed event payload", "error", marshalErr)
			return // Cannot publish if marshalling fails
		}
		if pubErr := c.natsClient.Publish(context.Background(), exportDomain.NATSExportFailedOutboxV1, failPayload); pubErr != nil { // Use fresh context for publish
			requestLogger.ErrorContext(ctx, "Failed to publish export failed event", "error", pubErr)
		}
		return
	}

	if filePath == "" { // No data to export, treated as a "failure" to produce a file
		requestLogger.InfoContext(ctx, "No data to export for user, publishing 'no data' failed event")
		failEvent := exportDomain.ExportFailedEvent{
			UserID:          reqEvent.UserID,
			ErrorMessage:    "No data found to export for the given criteria.",
			OriginalFilters: reqEvent.Filters,
			RequesterEmail:  reqEvent.RequesterEmail,
		}
		failPayload, marshalErr := json.Marshal(failEvent)
		if marshalErr != nil {
			requestLogger.ErrorContext(ctx, "Failed to marshal export 'no data' event payload", "error", marshalErr)
			return
		}
		if pubErr := c.natsClient.Publish(context.Background(), exportDomain.NATSExportFailedOutboxV1, failPayload); pubErr != nil {
			requestLogger.ErrorContext(ctx, "Failed to publish export 'no data' event", "error", pubErr)
		}
		return
	}

	requestLogger.InfoContext(ctx, "Successfully exported data, publishing completion event", "file_path", filePath)
	completedEvent := exportDomain.ExportCompletedEvent{
		UserID:         reqEvent.UserID,
		FilePath:       filePath, // This path is local to export-service.
		FileName:       filepath.Base(filePath),
		RequesterEmail: reqEvent.RequesterEmail,
	}
	completedPayload, marshalErr := json.Marshal(completedEvent)
	if marshalErr != nil {
		requestLogger.ErrorContext(ctx, "Failed to marshal export completed event payload", "error", marshalErr)
		return
	}
	if pubErr := c.natsClient.Publish(context.Background(), exportDomain.NATSExportCompletedOutboxV1, completedPayload); pubErr != nil {
		requestLogger.ErrorContext(ctx, "Failed to publish export completed event", "error", pubErr)
	}
}
