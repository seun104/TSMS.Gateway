package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings" // For normalizing status strings
	"time"    // For ProcessedTimestamp
	"encoding/json"

	"github.com/google/uuid"
	"github.com/AradIT/aradsms/golang_services/internal/delivery_retrieval_service/domain" // Corrected
	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker" // Corrected
)

// DLRProcessor is responsible for processing delivery reports, updating the database,
// and publishing a processed DLR event.
type DLRProcessor struct {
	outboxRepo domain.OutboxRepository
	natsClient *messagebroker.NATSClient // NATS client for publishing processed events
	logger     *slog.Logger
}

// NewDLRProcessor creates a new DLRProcessor instance.
func NewDLRProcessor(outboxRepo domain.OutboxRepository, natsClient *messagebroker.NATSClient, logger *slog.Logger) *DLRProcessor {
	return &DLRProcessor{
		outboxRepo: outboxRepo,
		natsClient: natsClient,
		logger:     logger,
	}
}

// ProcessDLRs iterates through a slice of DeliveryReport objects and updates (This method can be deprecated or removed if only NATS events are used)
// their corresponding entries in the outbox_messages table.
func (p *DLRProcessor) ProcessDLRs(ctx context.Context, dlrs []domain.DeliveryReport) error {
	// Create a logger for this batch operation, could include a batch ID if available/generated
	batchLogger := p.logger.With("batch_dlr_process_id", uuid.NewString().String()) // Example batch ID
	if len(dlrs) == 0 {
		batchLogger.InfoContext(ctx, "No DLRs to process in batch.")
		return nil
	}

	batchLogger.InfoContext(ctx, "Starting to process DLRs batch", "count", len(dlrs))

	var processedCount int
	var errorCount int

	for _, dlr := range dlrs {
		// Create a logger specific to this DLR within the batch
		dlrLogger := batchLogger.With(
			"internal_message_id", dlr.MessageID,
			"provider_message_id", dlr.ProviderMessageID,
		)
		dlrLogger.DebugContext(ctx, "Processing DLR from batch",
			"status", dlr.Status.String(),
			"provider_status", dlr.ProviderStatus,
		)

		var providerMsgIDForUpdate sql.NullString
		if dlr.ProviderMessageID != "" {
			providerMsgIDForUpdate = sql.NullString{String: dlr.ProviderMessageID, Valid: true}
		}

		err := p.outboxRepo.UpdateStatus(
			ctx,
			dlr.MessageID,
			dlr.Status,
			dlr.ProviderStatus,
			dlr.DeliveredAt,
			dlr.ErrorCode,
			dlr.ErrorDescription,
			providerMsgIDForUpdate,
		)

		if err != nil {
			errorCount++
			dlrLogger.ErrorContext(ctx, "Failed to update outbox message status for DLR from batch", // Use dlrLogger
				"error", err,
				// message_id and provider_message_id are already in dlrLogger context
				"new_status", dlr.Status.String(),
			)
		} else {
			processedCount++
			dlrLogger.InfoContext(ctx, "Successfully processed DLR from batch and updated outbox message", // Use dlrLogger
				"new_status", dlr.Status.String(),
			)
		}
	}

	batchLogger.InfoContext(ctx, "Finished processing DLRs batch", // Use batchLogger
		"total_received", len(dlrs),
		"successfully_processed", processedCount,
		"errors_encountered", errorCount,
	)

	return nil
}

// normalizeProviderStatus converts a provider's raw status string to a domain.DeliveryStatus.
// This needs to be customized based on the actual status strings returned by each provider.
func normalizeProviderStatus(providerName, providerStatus string, logger *slog.Logger) domain.DeliveryStatus {
	// Example normalization logic - THIS IS HIGHLY PROVIDER-SPECIFIC
	// In a real system, this might involve a lookup table or more complex rules per provider.
	s := strings.ToUpper(strings.TrimSpace(providerStatus))
	switch providerName {
	// case "provider_A": // Example for a specific provider
	// 	switch s {
	// 	case "DELIVRD":
	// 		return domain.DeliveryStatusDelivered
	// 	// ... other statuses for provider_A
	// 	}
	default: // Generic fallback
		switch s {
		case "DELIVERED", "DELIVRD", "SUCCESS":
			return domain.DeliveryStatusDelivered
		case "FAILED", "UNDELIVERABLE", "UNDELIV":
			return domain.DeliveryStatusFailed
		case "EXPIRED":
			return domain.DeliveryStatusExpired
		case "REJECTED", "REJCTED":
			return domain.DeliveryStatusRejected
		case "SENT", "ACCEPTED", "BUFFERED": // "Sent" might mean "sent to provider" or "sent from provider to carrier"
			return domain.DeliveryStatusSent
		case "QUEUED":
			return domain.DeliveryStatusQueued
		default:
			logger.Warn("Unknown provider status received", "provider", providerName, "status", providerStatus)
			return domain.DeliveryStatusUnknown
		}
	}
}

// ProcessDLREvent processes a single DLR event received from NATS.
func (p *DLRProcessor) ProcessDLREvent(ctx context.Context, event DLREvent) error {
	p.logger.InfoContext(ctx, "Processing DLR event from NATS",
		"provider_name", event.ProviderName,
		"provider_message_id", event.RequestData.ProviderMessageID,
		"original_message_id", event.RequestData.MessageID,
		"raw_status", event.RequestData.Status,
	)

	// Attempt to parse the original MessageID (our system's ID)
	messageID, err := uuid.Parse(event.RequestData.MessageID)
	if err != nil {
		// If ProviderMessageID is present and MessageID is not a UUID,
		// it might be that MessageID from DTO is actually the Provider's ID,
		// and we need a lookup mechanism to find our internal MessageID.
		// This scenario is more complex and depends on how IDs are handled with providers.
		// For now, we assume event.RequestData.MessageID is our internal UUID.
		p.logger.ErrorContext(ctx, "Failed to parse MessageID from DLR event as UUID",
			"error", err,
			"message_id_string", event.RequestData.MessageID,
			"provider_name", event.ProviderName,
		)
		// Depending on policy, either error out or try to use ProviderMessageID for a lookup.
		// For now, we'll error out if our primary MessageID is not a valid UUID.
		return fmt.Errorf("invalid MessageID format: %w", err)
	}

	// Fetch UserID before updating, as UpdateStatus doesn't return it.
	// This adds an extra DB call, but is simpler than modifying UpdateStatus to return data
	// if that method is used by other components that don't need the returned data.
	outboxMsg, repoErr := p.outboxRepo.GetByMessageID(ctx, messageID)
	if repoErr != nil {
		p.logger.ErrorContext(ctx, "Failed to get outbox message by ID before DLR processing",
			"error", repoErr, "message_id", messageID)
		// Continue processing the DLR update even if we can't get UserID for the event,
		// but log that UserID will be missing in the published event.
		// Alternatively, could return an error here if UserID is critical for the event.
	} else if outboxMsg == nil {
		p.logger.WarnContext(ctx, "Outbox message not found for DLR, cannot get UserID", "message_id", messageID)
		// Similar to above, UserID will be missing.
	}


	// Normalize the provider's status string to our internal domain.DeliveryStatus
	normalizedStatus := normalizeProviderStatus(event.ProviderName, event.RequestData.Status, p.logger)

	var deliveredAt sql.NullTime
	if normalizedStatus == domain.DeliveryStatusDelivered && !event.RequestData.Timestamp.IsZero() {
		deliveredAt = sql.NullTime{Time: event.RequestData.Timestamp, Valid: true}
	} else if !event.RequestData.Timestamp.IsZero() {
		// If not delivered but timestamp is present, it's the time of this status update.
		// For simplicity, we're only storing it in delivered_at for "DELIVERED".
		// The `outbox_messages.updated_at` will reflect when we processed this DLR.
	}


	var errorCode sql.NullString
	if event.RequestData.ErrorCode != "" {
		errorCode = sql.NullString{String: event.RequestData.ErrorCode, Valid: true}
	}

	var errorDescription sql.NullString
	if event.RequestData.ErrorDescription != "" {
		errorDescription = sql.NullString{String: event.RequestData.ErrorDescription, Valid: true}
	}

	var providerMessageIDForUpdate sql.NullString
	if event.RequestData.ProviderMessageID != "" {
		providerMessageIDForUpdate = sql.NullString{String: event.RequestData.ProviderMessageID, Valid: true}
	}


	// Call the existing OutboxRepository method to update the status
	err = p.outboxRepo.UpdateStatus(
		ctx,
		messageID,
		normalizedStatus,
		event.RequestData.Status, // Store raw provider status
		deliveredAt,
		errorCode,
		errorDescription,
		providerMessageIDForUpdate, // Update/set the provider's message ID if available
	)

	if err != nil {
		p.logger.ErrorContext(ctx, "Failed to update outbox message status from DLR event",
			"error", err,
			"internal_message_id", messageID,
			"provider_message_id", event.RequestData.ProviderMessageID,
			"new_status", normalizedStatus.String(),
		)
		return err
	}

	p.logger.InfoContext(ctx, "Successfully processed DLR event and updated outbox message",
		"internal_message_id", messageID,
		"provider_name", event.ProviderName,
		"new_status", normalizedStatus.String(),
	)

	// If DB update was successful, publish a ProcessedDLREvent
	if err == nil {
		processedEvent := domain.ProcessedDLREvent{
			MessageID:          messageID,
			Status:             normalizedStatus.String(),
			ProviderName:       event.ProviderName,
			ProviderStatus:     event.RequestData.Status,
			DeliveredAt:        deliveredAt,
			ErrorCode:          errorCode,
			ErrorDescription:   errorDescription,
			ProcessedTimestamp: time.Now().UTC(),
		}
		if outboxMsg != nil { // Populate UserID if fetched successfully
			processedEvent.UserID = outboxMsg.UserID
		}

		eventData, marshalErr := json.Marshal(processedEvent)
		if marshalErr != nil {
			p.logger.ErrorContext(ctx, "Failed to marshal ProcessedDLREvent for NATS",
				"error", marshalErr, "message_id", messageID)
			// Log error but don't let it fail the DLR processing itself,
			// as DB update was successful.
		} else {
			natsSubject := fmt.Sprintf("dlr.processed.v1.%s", event.ProviderName) // Example subject
			if pubErr := p.natsClient.Publish(ctx, natsSubject, eventData); pubErr != nil {
				p.logger.ErrorContext(ctx, "Failed to publish ProcessedDLREvent to NATS",
					"error", pubErr, "subject", natsSubject, "message_id", messageID)
				// Log error, but DLR processing is still considered successful at DB level.
			} else {
				p.logger.InfoContext(ctx, "Successfully published ProcessedDLREvent to NATS",
					"subject", natsSubject, "message_id", messageID)
			}
		}
	}

	return nil // Return nil because the primary operation (DB update) was successful.
}
