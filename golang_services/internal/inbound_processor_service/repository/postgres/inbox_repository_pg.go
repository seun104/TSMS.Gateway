package postgres

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/your-repo/project/internal/inbound_processor_service/domain"
)

type PgInboxRepository struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

// NewPgInboxRepository creates a new PostgreSQL implementation of InboxRepository.
func NewPgInboxRepository(dbPool *pgxpool.Pool, logger *slog.Logger) *PgInboxRepository {
	return &PgInboxRepository{
		db:     dbPool,
		logger: logger,
	}
}

// Create inserts a new InboxMessage record into the inbox_messages table.
func (r *PgInboxRepository) Create(ctx context.Context, msg *domain.InboxMessage) error {
	query := `
		INSERT INTO inbox_messages (
			id, "from", "to", text_content, provider_message_id, provider_name,
			user_id, private_number_id,
			received_by_provider_at, received_by_gateway_at, processed_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
	`
	// Note: "text" is often a reserved keyword, so using "text_content" or similar for the column name.
	// Adjust column names as per the actual schema defined in migrations.
	// For this example, assuming:
	// - "from" column name in DB is "from_number" or just "from" (if quoted)
	// - "to" column name in DB is "to_number" or just "to" (if quoted)
	// - "text" column name in DB is "text_content"

	r.logger.DebugContext(ctx, "Attempting to insert inbox message",
		"query", query,
		"id", msg.ID,
		"from", msg.From,
		"to", msg.To,
		"provider_message_id", msg.ProviderMessageID,
		"provider_name", msg.ProviderName,
	)

	_, err := r.db.Exec(ctx, query,
		msg.ID,
		msg.From,
		msg.To,
		msg.Text,
		msg.ProviderMessageID,
		msg.ProviderName,
		msg.UserID,             // uuid.NullUUID is handled correctly by pgx
		msg.PrivateNumberID,    // uuid.NullUUID is handled correctly by pgx
		msg.ReceivedByProviderAt, // sql.NullTime is handled correctly by pgx
		msg.ReceivedByGatewayAt,
		msg.ProcessedAt,
	)

	if err != nil {
		r.logger.ErrorContext(ctx, "Error inserting inbox message into database",
			"error", err,
			"message_id", msg.ID,
			"provider_message_id", msg.ProviderMessageID,
		)
		// TODO: Check for unique constraint violation errors (e.g., on provider_message_id)
		// and potentially return a domain-specific error like domain.ErrDuplicateMessage.
		return err
	}

	r.logger.InfoContext(ctx, "Successfully inserted inbox message", "id", msg.ID, "provider_message_id", msg.ProviderMessageID)
	return nil
}

/*
// GetByProviderMessageID example implementation (if added to interface)
func (r *PgInboxRepository) GetByProviderMessageID(ctx context.Context, providerName string, providerMessageID string) (*domain.InboxMessage, error) {
	query := `
		SELECT id, "from", "to", text_content, provider_message_id, provider_name,
		       user_id, private_number_id,
		       received_by_provider_at, received_by_gateway_at, processed_at
		FROM inbox_messages
		WHERE provider_name = $1 AND provider_message_id = $2
	`
	var msg domain.InboxMessage
	row := r.db.QueryRow(ctx, query, providerName, providerMessageID)
	err := row.Scan(
		&msg.ID,
		&msg.From,
		&msg.To,
		&msg.Text,
		&msg.ProviderMessageID,
		&msg.ProviderName,
		&msg.UserID,
		&msg.PrivateNumberID,
		&msg.ReceivedByProviderAt,
		&msg.ReceivedByGatewayAt,
		&msg.ProcessedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Standard practice to return nil, nil when not found
		}
		r.logger.ErrorContext(ctx, "Error getting inbox message by provider ID", "error", err, "provider_name", providerName, "provider_message_id", providerMessageID)
		return nil, err
	}
	return &msg, nil
}
*/
