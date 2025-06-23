package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/aradsms/golang_services/internal/core_domain"
	"github.com/aradsms/golang_services/internal/sms_sending_service/repository" // Interfaces for this service
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	// "github.com/jackc/pgx/v5/pgconn" // Not directly used in this version of the file
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrOutboxMessageNotFound = errors.New("outbox message not found")
// No unique constraint errors typically other than PK for outbox on create.

type pgOutboxMessageRepository struct {
	db *pgxpool.Pool
}

// NewPgOutboxMessageRepository creates a new instance for PostgreSQL.
func NewPgOutboxMessageRepository(db *pgxpool.Pool) repository.OutboxMessageRepository {
	return &pgOutboxMessageRepository{db: db}
}

func (r *pgOutboxMessageRepository) Create(ctx context.Context, msg *core_domain.OutboxMessage) (*core_domain.OutboxMessage, error) {
	if msg.ID == "" { // Ensure ID is set if not already
		msg.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	msg.CreatedAt = now
	msg.UpdatedAt = now
    if msg.Status == "" { // Default status if not set
        msg.Status = core_domain.StatusQueued
    }


	query := `
		INSERT INTO outbox_messages (
			id, user_id, batch_id, sender_id_text, private_number_id, recipient, content, status,
			segments, provider_message_id, provider_status_code, error_message, user_data,
			scheduled_for, processed_at, sent_at, delivered_at, last_status_update_at,
			created_at, updated_at, sms_provider_id, route_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22
		)
	`
	_, err := r.db.Exec(ctx, query,
		msg.ID, msg.UserID, msg.BatchID, msg.SenderID, msg.PrivateNumberID, msg.Recipient, msg.Content, msg.Status,
		msg.Segments, msg.ProviderMessageID, msg.ProviderStatusCode, msg.ErrorMessage, msg.UserData,
		msg.ScheduledFor, msg.ProcessedAt, msg.SentAt, msg.DeliveredAt, msg.LastStatusUpdateAt,
		msg.CreatedAt, msg.UpdatedAt, msg.SMSProviderID, msg.RouteID,
	)

	if err != nil {
		// Handle potential errors, e.g., foreign key violations if IDs are incorrect
		return nil, err
	}
	return msg, nil
}

func (r *pgOutboxMessageRepository) GetByID(ctx context.Context, id string) (*core_domain.OutboxMessage, error) {
	msg := &core_domain.OutboxMessage{}
	query := `
		SELECT id, user_id, batch_id, sender_id_text, private_number_id, recipient, content, status,
		       segments, provider_message_id, provider_status_code, error_message, user_data,
		       scheduled_for, processed_at, sent_at, delivered_at, last_status_update_at,
		       created_at, updated_at, sms_provider_id, route_id
		FROM outbox_messages WHERE id = $1
	`
	err := r.db.QueryRow(ctx, query, id).Scan(
		&msg.ID, &msg.UserID, &msg.BatchID, &msg.SenderID, &msg.PrivateNumberID, &msg.Recipient, &msg.Content, &msg.Status,
		&msg.Segments, &msg.ProviderMessageID, &msg.ProviderStatusCode, &msg.ErrorMessage, &msg.UserData,
		&msg.ScheduledFor, &msg.ProcessedAt, &msg.SentAt, &msg.DeliveredAt, &msg.LastStatusUpdateAt,
		&msg.CreatedAt, &msg.UpdatedAt, &msg.SMSProviderID, &msg.RouteID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOutboxMessageNotFound
		}
		return nil, err
	}
	return msg, nil
}

func (r *pgOutboxMessageRepository) UpdateStatus(ctx context.Context, id string, status core_domain.MessageStatus,
    providerMsgID *string, providerStatusCode *string, errorMessage *string, sentAt *time.Time) error {
	now := time.Now().UTC()
	// Base query
	baseQuery := `
		UPDATE outbox_messages
		SET status = $2, provider_message_id = COALESCE($3, provider_message_id),
            provider_status_code = COALESCE($4, provider_status_code), error_message = COALESCE($5, error_message),
            sent_at = COALESCE($6, sent_at), updated_at = $7, last_status_update_at = $7`

	// Conditionally add processed_at update
	if status == core_domain.StatusSent || status == core_domain.StatusFailedToSend {
		baseQuery += `, processed_at = COALESCE(processed_at, $7)`
	}

	finalQuery := baseQuery + ` WHERE id = $1`

	tag, err := r.db.Exec(ctx, finalQuery, id, status, providerMsgID, providerStatusCode, errorMessage, sentAt, now)
    if err != nil {
        return err
    }
    if tag.RowsAffected() == 0 {
        return ErrOutboxMessageNotFound
    }
	return nil
}

func (r *pgOutboxMessageRepository) UpdateDeliveryStatus(ctx context.Context, id string, status core_domain.MessageStatus,
    deliveredAt *time.Time, providerStatusCode *string) error {
    now := time.Now().UTC()
    query := `
        UPDATE outbox_messages
        SET status = $2, delivered_at = COALESCE($3, delivered_at),
            provider_status_code = COALESCE($4, provider_status_code),
            updated_at = $5, last_status_update_at = $5
        WHERE id = $1
    `
    tag, err := r.db.Exec(ctx, query, id, status, deliveredAt, providerStatusCode, now)
    if err != nil {
        return err
    }
    if tag.RowsAffected() == 0 {
        return ErrOutboxMessageNotFound
    }
    return nil
}

func (r *pgOutboxMessageRepository) GetMessagesByStatus(ctx context.Context, status core_domain.MessageStatus, limit int) ([]*core_domain.OutboxMessage, error) {
    query := `
        SELECT id, user_id, batch_id, sender_id_text, private_number_id, recipient, content, status,
               segments, provider_message_id, provider_status_code, error_message, user_data,
               scheduled_for, processed_at, sent_at, delivered_at, last_status_update_at,
               created_at, updated_at, sms_provider_id, route_id
        FROM outbox_messages
        WHERE status = $1
        ORDER BY created_at ASC
        LIMIT $2
        FOR UPDATE SKIP LOCKED
    ` // FOR UPDATE SKIP LOCKED for job queue pattern

    rows, err := r.db.Query(ctx, query, status, limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var messages []*core_domain.OutboxMessage
    for rows.Next() {
        var msg core_domain.OutboxMessage
        err := rows.Scan(
            &msg.ID, &msg.UserID, &msg.BatchID, &msg.SenderID, &msg.PrivateNumberID, &msg.Recipient, &msg.Content, &msg.Status,
		    &msg.Segments, &msg.ProviderMessageID, &msg.ProviderStatusCode, &msg.ErrorMessage, &msg.UserData,
		    &msg.ScheduledFor, &msg.ProcessedAt, &msg.SentAt, &msg.DeliveredAt, &msg.LastStatusUpdateAt,
		    &msg.CreatedAt, &msg.UpdatedAt, &msg.SMSProviderID, &msg.RouteID,
        )
        if err != nil {
            return nil, err
        }
        messages = append(messages, &msg)
    }
    if err = rows.Err(); err != nil {
        return nil, err
    }
    return messages, nil
}
