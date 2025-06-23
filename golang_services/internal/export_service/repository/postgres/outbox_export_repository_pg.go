package postgres

import (
	"context"
	"database/sql" // For sql.NullString and sql.NullTime
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	coreSmsDomain "github.com/AradIT/aradsms/golang_services/internal/core_sms/domain"
	exportDomain "github.com/AradIT/aradsms/golang_services/internal/export_service/domain"
)

type PgOutboxExportRepository struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

func NewPgOutboxExportRepository(db *pgxpool.Pool, logger *slog.Logger) exportDomain.OutboxExportRepository {
	return &PgOutboxExportRepository{db: db, logger: logger.With("component", "outbox_export_repository_pg")}
}

func (r *PgOutboxExportRepository) GetOutboxMessagesForUser(ctx context.Context, userID uuid.UUID, filters map[string]string) ([]*exportDomain.ExportedOutboxMessage, error) {
	// Basic query, can be expanded with filters (e.g., date range from filters map)
	// For now, filters are ignored for simplicity in this "basic" step.
	// TODO: Implement filtering based on the 'filters' map (e.g., date range, status)
	query := `SELECT id, user_id, sender_id, recipient, content, status, segments,
	                 provider_message_id, scheduled_at, sent_to_provider_at, delivered_at, created_at
	          FROM outbox_messages
	          WHERE user_id = $1 ORDER BY created_at DESC`
	// Add LIMIT for safety in basic version. This should be configurable or part of pagination.
	query += " LIMIT 1000" // Placeholder limit

	r.logger.DebugContext(ctx, "Fetching outbox messages for export", "user_id", userID, "query", query)
	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error querying outbox messages for export", "user_id", userID, "error", err)
		return nil, fmt.Errorf("querying outbox_messages: %w", err)
	}
	defer rows.Close()

	var messages []*exportDomain.ExportedOutboxMessage
	for rows.Next() {
		var msg exportDomain.ExportedOutboxMessage
		var providerMsgID sql.NullString
		var scheduledFor, sentToProviderAt, deliveredAt sql.NullTime

		// Ensure the scan order matches the SELECT query columns
		err := rows.Scan(
			&msg.ID, &msg.UserID, &msg.SenderID, &msg.Recipient, &msg.Content, &msg.Status, &msg.Segments,
			&providerMsgID, &scheduledFor, &sentToProviderAt, &deliveredAt, &msg.CreatedAt,
		)
		if err != nil {
			r.logger.ErrorContext(ctx, "Error scanning outbox message row for export", "error", err)
			continue // Skip bad rows, log and continue
		}
		if providerMsgID.Valid {
			msg.ProviderMessageID = &providerMsgID.String
		}
		if scheduledFor.Valid {
			msg.ScheduledFor = &scheduledFor.Time
		}
		if sentToProviderAt.Valid {
			msg.SentToProviderAt = &sentToProviderAt.Time
		}
		if deliveredAt.Valid {
			msg.DeliveredAt = &deliveredAt.Time
		}
		messages = append(messages, &msg)
	}
	if err = rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "Error after iterating outbox message rows for export", "error", err)
		return nil, fmt.Errorf("iterating outbox_messages: %w", err)
	}

	r.logger.InfoContext(ctx, "Fetched outbox messages for export", "user_id", userID, "count", len(messages))
	return messages, nil
}
