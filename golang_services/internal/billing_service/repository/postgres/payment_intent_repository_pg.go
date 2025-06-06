package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	// Corrected import path
	"github.com/AradIT/aradsms/golang_services/internal/billing_service/domain"
)

type PgPaymentIntentRepository struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

func NewPgPaymentIntentRepository(db *pgxpool.Pool, logger *slog.Logger) domain.PaymentIntentRepository {
	return &PgPaymentIntentRepository{db: db, logger: logger.With("component", "payment_intent_repository_pg")}
}

func (r *PgPaymentIntentRepository) Create(ctx context.Context, pi *domain.PaymentIntent) error {
	query := `
		INSERT INTO payment_intents (
			id, user_id, amount, currency, status,
			gateway_payment_intent_id, gateway_client_secret, error_message,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := r.db.Exec(ctx, query,
		pi.ID, pi.UserID, pi.Amount, pi.Currency, pi.Status,
		pi.GatewayPaymentIntentID, pi.GatewayClientSecret, pi.ErrorMessage,
		pi.CreatedAt, pi.UpdatedAt,
	)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error creating payment intent", "error", err, "payment_intent_id", pi.ID)
		return fmt.Errorf("creating payment intent: %w", err)
	}
	r.logger.InfoContext(ctx, "Payment intent created successfully", "payment_intent_id", pi.ID)
	return nil
}

func (r *PgPaymentIntentRepository) scanPaymentIntent(row pgx.Row) (*domain.PaymentIntent, error) {
	var pi domain.PaymentIntent
	var gatewayPaymentIntentID sql.NullString
	var gatewayClientSecret sql.NullString
	var errorMessage sql.NullString

	err := row.Scan(
		&pi.ID, &pi.UserID, &pi.Amount, &pi.Currency, &pi.Status,
		&gatewayPaymentIntentID, &gatewayClientSecret, &errorMessage,
		&pi.CreatedAt, &pi.UpdatedAt,
	)
	if err != nil {
		return nil, err // Handle pgx.ErrNoRows in the calling method
	}

	if gatewayPaymentIntentID.Valid {
		pi.GatewayPaymentIntentID = &gatewayPaymentIntentID.String
	}
	if gatewayClientSecret.Valid {
		pi.GatewayClientSecret = &gatewayClientSecret.String
	}
	if errorMessage.Valid {
		pi.ErrorMessage = &errorMessage.String
	}
	return &pi, nil
}

func (r *PgPaymentIntentRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.PaymentIntent, error) {
	query := `
		SELECT id, user_id, amount, currency, status,
		       gateway_payment_intent_id, gateway_client_secret, error_message,
		       created_at, updated_at
		FROM payment_intents
		WHERE id = $1
	`
	row := r.db.QueryRow(ctx, query, id)
	pi, err := r.scanPaymentIntent(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.InfoContext(ctx, "Payment intent not found by ID", "id", id)
			return nil, domain.ErrNotFound // Assuming ErrNotFound is defined in domain package or a common error package
		}
		r.logger.ErrorContext(ctx, "Error getting payment intent by ID", "error", err, "id", id)
		return nil, fmt.Errorf("getting payment intent by ID: %w", err)
	}
	return pi, nil
}

func (r *PgPaymentIntentRepository) GetByGatewayPaymentIntentID(ctx context.Context, gatewayPaymentIntentID string) (*domain.PaymentIntent, error) {
	query := `
		SELECT id, user_id, amount, currency, status,
		       gateway_payment_intent_id, gateway_client_secret, error_message,
		       created_at, updated_at
		FROM payment_intents
		WHERE gateway_payment_intent_id = $1
	`
	row := r.db.QueryRow(ctx, query, gatewayPaymentIntentID)
	pi, err := r.scanPaymentIntent(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.InfoContext(ctx, "Payment intent not found by GatewayPaymentIntentID", "gateway_payment_intent_id", gatewayPaymentIntentID)
			return nil, domain.ErrNotFound // Assuming ErrNotFound
		}
		r.logger.ErrorContext(ctx, "Error getting payment intent by GatewayPaymentIntentID", "error", err, "gateway_payment_intent_id", gatewayPaymentIntentID)
		return nil, fmt.Errorf("getting payment intent by GatewayPaymentIntentID: %w", err)
	}
	return pi, nil
}

func (r *PgPaymentIntentRepository) Update(ctx context.Context, pi *domain.PaymentIntent) error {
	query := `
		UPDATE payment_intents SET
			user_id = $1,
			amount = $2,
			currency = $3,
			status = $4,
			gateway_payment_intent_id = $5,
			gateway_client_secret = $6,
			error_message = $7,
			updated_at = $8
		WHERE id = $9
	`
	pi.UpdatedAt = time.Now().UTC() // Ensure UpdatedAt is set

	tag, err := r.db.Exec(ctx, query,
		pi.UserID, pi.Amount, pi.Currency, pi.Status,
		pi.GatewayPaymentIntentID, pi.GatewayClientSecret, pi.ErrorMessage,
		pi.UpdatedAt, pi.ID,
	)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error updating payment intent", "error", err, "payment_intent_id", pi.ID)
		return fmt.Errorf("updating payment intent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		r.logger.WarnContext(ctx, "No payment intent found for update", "payment_intent_id", pi.ID)
		return domain.ErrNotFound // Or a more specific error like "update failed due to no rows affected"
	}
	r.logger.InfoContext(ctx, "Payment intent updated successfully", "payment_intent_id", pi.ID)
	return nil
}
