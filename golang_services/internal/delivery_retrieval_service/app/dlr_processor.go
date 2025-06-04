package app

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/your-repo/project/internal/delivery_retrieval_service/domain"
)

// DLRProcessor is responsible for processing delivery reports and updating the database.
type DLRProcessor struct {
	outboxRepo domain.OutboxRepository
	logger     *slog.Logger
}

// NewDLRProcessor creates a new DLRProcessor instance.
func NewDLRProcessor(outboxRepo domain.OutboxRepository, logger *slog.Logger) *DLRProcessor {
	return &DLRProcessor{
		outboxRepo: outboxRepo,
		logger:     logger,
	}
}

// ProcessDLRs iterates through a slice of DeliveryReport objects and updates
// their corresponding entries in the outbox_messages table.
func (p *DLRProcessor) ProcessDLRs(ctx context.Context, dlrs []domain.DeliveryReport) error {
	if len(dlrs) == 0 {
		p.logger.InfoContext(ctx, "No DLRs to process.")
		return nil
	}

	p.logger.InfoContext(ctx, "Starting to process DLRs", "count", len(dlrs))

	var processedCount int
	var errorCount int

	for _, dlr := range dlrs {
		p.logger.DebugContext(ctx, "Processing DLR",
			"message_id", dlr.MessageID,
			"provider_message_id", dlr.ProviderMessageID,
			"status", dlr.Status.String(),
			"provider_status", dlr.ProviderStatus,
		)

		// ProviderMessageID from DLR might be new or confirm an existing one
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
			p.logger.ErrorContext(ctx, "Failed to update outbox message status for DLR",
				"error", err,
				"message_id", dlr.MessageID,
				"provider_message_id", dlr.ProviderMessageID,
				"new_status", dlr.Status.String(),
			)
			// Decide if one error should stop processing others. For now, continue.
		} else {
			processedCount++
			p.logger.InfoContext(ctx, "Successfully processed DLR and updated outbox message",
				"message_id", dlr.MessageID,
				"new_status", dlr.Status.String(),
			)
		}
	}

	p.logger.InfoContext(ctx, "Finished processing DLRs",
		"total_received", len(dlrs),
		"successfully_processed", processedCount,
		"errors_encountered", errorCount,
	)

	// Depending on requirements, could return an aggregated error or a summary.
	// For now, just logging errors and returning nil unless a critical error occurs.
	return nil
}
