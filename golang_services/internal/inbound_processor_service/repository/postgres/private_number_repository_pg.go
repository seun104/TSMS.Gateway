package postgres

import (
	"context"
	"errors" // For pgx.ErrNoRows
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4" // For pgx.ErrNoRows
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/your-repo/project/internal/inbound_processor_service/domain"
)

type PgPrivateNumberRepository struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

// NewPgPrivateNumberRepository creates a new PostgreSQL implementation of PrivateNumberRepository.
func NewPgPrivateNumberRepository(dbPool *pgxpool.Pool, logger *slog.Logger) *PgPrivateNumberRepository {
	return &PgPrivateNumberRepository{
		db:     dbPool,
		logger: logger,
	}
}

// FindByNumber searches for a PrivateNumber record matching the given phone number string.
// Assumes a 'private_numbers' table with columns: id, number, user_id, service_id.
func (r *PgPrivateNumberRepository) FindByNumber(ctx context.Context, number string) (*domain.PrivateNumber, error) {
	query := `
		SELECT id, number, user_id, service_id
		FROM private_numbers
		WHERE number = $1
		LIMIT 1
	`
	// LIMIT 1 is added as a safeguard, though 'number' should ideally be unique.

	var pn domain.PrivateNumber
	var serviceID sql.NullString // Handle nullable service_id from DB

	r.logger.DebugContext(ctx, "Querying private number by number", "number", number)

	err := r.db.QueryRow(ctx, query, number).Scan(
		&pn.ID,
		&pn.Number,
		&pn.UserID,
		&serviceID, // Scan into sql.NullString
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.InfoContext(ctx, "Private number not found", "number", number)
			return nil, nil // Not found is not an application error in this context
		}
		r.logger.ErrorContext(ctx, "Error querying private number by number", "error", err, "number", number)
		return nil, err
	}

	if serviceID.Valid {
		pn.ServiceID = serviceID.String
	}


	r.logger.InfoContext(ctx, "Private number found", "number", number, "id", pn.ID, "user_id", pn.UserID)
	return &pn, nil
}
