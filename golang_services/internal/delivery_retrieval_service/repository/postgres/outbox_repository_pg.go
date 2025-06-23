package postgres

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/your-repo/project/internal/delivery_retrieval_service/domain"
)

type pgOutboxRepository struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

// NewPgOutboxRepository creates a new PostgreSQL implementation of OutboxRepository.
func NewPgOutboxRepository(db *pgxpool.Pool, logger *slog.Logger) domain.OutboxRepository {
	return &pgOutboxRepository{
		db:     db,
		logger: logger,
	}
}

// GetByMessageID retrieves a minimal OutboxMessage by its primary ID.
// Assumes table `outbox_messages` with columns: id, user_id, status, provider_msg_id, created_at, updated_at
func (r *pgOutboxRepository) GetByMessageID(ctx context.Context, messageID uuid.UUID) (*domain.OutboxMessage, error) {
	query := `
		SELECT id, user_id, status, provider_msg_id, created_at, updated_at
		FROM outbox_messages
		WHERE id = $1
	`
	// Note: The 'status' column in the DB needs to be mapped to domain.DeliveryStatus.
	// This might require careful handling if DB stores as string vs. int.
	// For this example, assuming it's an int that maps directly to DeliveryStatus iota.
	// If it's a string, a conversion function would be needed here or at the domain level.

	var msg domain.OutboxMessage
	var statusInt int // Assuming status is stored as int in DB

	row := r.db.QueryRow(ctx, query, messageID)
	err := row.Scan(
		&msg.ID,
		&msg.UserID,
		&statusInt, // Scan into int first
		&msg.ProviderMSGID,
		&msg.CreatedAt,
		&msg.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			r.logger.WarnContext(ctx, "Outbox message not found", "message_id", messageID)
			return nil, nil // Or a specific domain.ErrNotFound
		}
		r.logger.ErrorContext(ctx, "Error scanning outbox message", "error", err, "message_id", messageID)
		return nil, err
	}

	msg.Status = domain.DeliveryStatus(statusInt) // Convert int to DeliveryStatus

	return &msg, nil
}

// UpdateStatus updates the delivery status information for a given message ID.
// Assumes table `outbox_messages` with columns: status, provider_status, delivered_at, error_code, error_description, provider_msg_id, updated_at.
func (r *pgOutboxRepository) UpdateStatus(
	ctx context.Context,
	messageID uuid.UUID,
	status domain.DeliveryStatus,
	providerStatus string,
	deliveredAt sql.NullTime,
	errorCode sql.NullString,
	errorDescription sql.NullString,
	providerMessageID sql.NullString,
) error {
	query := `
		UPDATE outbox_messages
		SET
			status = $2,             -- Normalized status
			provider_status = $3,    -- Raw status from provider
			delivered_at = $4,
			error_code = $5,
			error_description = $6,
			provider_msg_id = COALESCE($7, provider_msg_id), -- Update provider_msg_id if new one is provided
			updated_at = $8
		WHERE id = $1
	`
	// Storing status as int, matching DeliveryStatus iota.
	// If DB stores status as string, $2 would be status.String().
	statusInt := int(status)
	now := time.Now().UTC()

	r.logger.DebugContext(ctx, "Attempting to update outbox message status",
		"query", query,
		"message_id", messageID,
		"status", status.String(), "status_int", statusInt,
		"provider_status", providerStatus,
		"delivered_at", deliveredAt,
		"error_code", errorCode,
		"error_description", errorDescription,
		"provider_message_id", providerMessageID,
		"updated_at", now,
	)

	commandTag, err := r.db.Exec(ctx, query,
		messageID,
		statusInt, // Storing normalized status as int
		providerStatus,
		deliveredAt,
		errorCode,
		errorDescription,
		providerMessageID,
		now,
	)

	if err != nil {
		r.logger.ErrorContext(ctx, "Error updating outbox message status", "error", err, "message_id", messageID)
		return err
	}

	if commandTag.RowsAffected() == 0 {
		r.logger.WarnContext(ctx, "No rows affected when updating outbox message status", "message_id", messageID)
		// This could mean the message ID didn't exist. Depending on requirements,
		// this might be an error or just a warning.
		// return domain.ErrNotFound // Or a similar specific error
	} else {
		r.logger.InfoContext(ctx, "Successfully updated outbox message status", "message_id", messageID, "new_status", status.String())
	}

	return nil
}
