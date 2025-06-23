package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5" // For pgx.ErrNoRows and pgx.Row interface
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AradIT/aradsms/golang_services/internal/billing_service/domain"
)

type PgTariffRepository struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

func NewPgTariffRepository(db *pgxpool.Pool, logger *slog.Logger) domain.TariffRepository {
	return &PgTariffRepository{db: db, logger: logger.With("component", "tariff_repository_pg")}
}

// scanTariff is a helper function to scan a single tariff row.
func scanTariff(row pgx.Row) (*domain.Tariff, error) {
	var t domain.Tariff
	var description sql.NullString // Use sql.NullString for Description

	err := row.Scan(
		&t.ID,
		&t.Name,
		&t.PricePerSMS,
		&t.Currency,
		&t.IsActive,
		&description, // Scan into sql.NullString
		&t.CreatedAt,
		&t.UpdatedAt,
	)
	if err != nil {
		return nil, err // Let caller handle pgx.ErrNoRows
	}
	t.Description = description // Assign the scanned sql.NullString
	return &t, nil
}

func (r *PgTariffRepository) GetTariffByID(ctx context.Context, id uuid.UUID) (*domain.Tariff, error) {
	query := `SELECT id, name, price_per_sms, currency, is_active, description, created_at, updated_at
	          FROM tariffs WHERE id = $1 AND is_active = TRUE` // Typically, GetByID might not check is_active, but for pricing, often only active matters.
														  // Or, remove is_active here and let service layer decide. For now, keeping it to fetch active.
	r.logger.DebugContext(ctx, "Getting tariff by ID", "query", query, "tariff_id", id)
	row := r.db.QueryRow(ctx, query, id)
	tariff, err := scanTariff(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.InfoContext(ctx, "Tariff not found by ID or not active", "tariff_id", id)
			return nil, nil // Not found is nil, nil
		}
		r.logger.ErrorContext(ctx, "Error scanning tariff by ID", "tariff_id", id, "error", err)
		return nil, fmt.Errorf("scanning tariff by ID %s: %w", id, err)
	}
	return tariff, nil
}

func (r *PgTariffRepository) GetActiveUserTariff(ctx context.Context, userID uuid.UUID) (*domain.Tariff, error) {
	query := `SELECT t.id, t.name, t.price_per_sms, t.currency, t.is_active, t.description, t.created_at, t.updated_at
	          FROM tariffs t
	          JOIN user_tariffs ut ON t.id = ut.tariff_id
	          WHERE ut.user_id = $1 AND t.is_active = TRUE`
	r.logger.DebugContext(ctx, "Getting active user tariff", "query", query, "user_id", userID)

	row := r.db.QueryRow(ctx, query, userID)
	tariff, err := scanTariff(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.InfoContext(ctx, "No active tariff assigned or found for user", "user_id", userID)
			return nil, nil // No active tariff assigned or found for user
		}
		r.logger.ErrorContext(ctx, "Error scanning active user tariff", "user_id", userID, "error", err)
		return nil, fmt.Errorf("scanning active user tariff for user %s: %w", userID, err)
	}
	return tariff, nil
}

func (r *PgTariffRepository) GetDefaultActiveTariff(ctx context.Context) (*domain.Tariff, error) {
	// Assumption: The default tariff is named 'Default'.
	// This should be made more robust, e.g., by a specific flag or configuration.
	defaultTariffName := "Default"
	query := `SELECT id, name, price_per_sms, currency, is_active, description, created_at, updated_at
	          FROM tariffs WHERE name = $1 AND is_active = TRUE LIMIT 1`
	r.logger.DebugContext(ctx, "Getting default active tariff", "query", query, "default_tariff_name", defaultTariffName)

	row := r.db.QueryRow(ctx, query, defaultTariffName)
	tariff, err := scanTariff(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.WarnContext(ctx, "Default active tariff not found in database", "default_tariff_name", defaultTariffName)
			return nil, nil
		}
		r.logger.ErrorContext(ctx, "Error scanning default active tariff", "error", err, "default_tariff_name", defaultTariffName)
		return nil, fmt.Errorf("scanning default active tariff: %w", err)
	}
	return tariff, nil
}
