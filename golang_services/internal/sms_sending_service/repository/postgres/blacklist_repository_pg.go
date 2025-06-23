package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5" // Required for pgx.ErrNoRows
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AradIT/aradsms/golang_services/internal/sms_sending_service/domain"
)

type PgBlacklistRepository struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

func NewPgBlacklistRepository(db *pgxpool.Pool, logger *slog.Logger) domain.BlacklistRepository {
	return &PgBlacklistRepository{db: db, logger: logger.With("component", "blacklist_repository_pg")}
}

func (r *PgBlacklistRepository) IsBlacklisted(ctx context.Context, phoneNumber string, userID uuid.NullUUID) (isBlacklisted bool, reason string, err error) {
	r.logger.DebugContext(ctx, "Checking blacklist", "phone_number", phoneNumber, "user_id", userID)

	var nullableReason sql.NullString

	// 1. Check for user-specific blacklist if UserID is provided and valid
	if userID.Valid {
		userSpecificQuery := `SELECT reason FROM blacklisted_numbers WHERE phone_number = $1 AND user_id = $2 LIMIT 1`
		err = r.db.QueryRow(ctx, userSpecificQuery, phoneNumber, userID.UUID).Scan(&nullableReason)
		if err == nil {
			// Found a user-specific entry
			if nullableReason.Valid {
				reason = nullableReason.String
			}
			r.logger.InfoContext(ctx, "Number found in user-specific blacklist", "phone_number", phoneNumber, "user_id", userID.UUID)
			return true, reason, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) { // pgx.ErrNoRows is the correct error for pgx v5
			r.logger.ErrorContext(ctx, "Error checking user-specific blacklist", "phone_number", phoneNumber, "user_id", userID.UUID, "error", err)
			return false, "", fmt.Errorf("checking user-specific blacklist: %w", err)
		}
		// If pgx.ErrNoRows, proceed to check global blacklist
		r.logger.DebugContext(ctx, "Number not found in user-specific blacklist, checking global", "phone_number", phoneNumber, "user_id", userID.UUID)
	}

	// 2. Check for global blacklist (user_id IS NULL)
	// This check is performed if no UserID was provided, or if a UserID was provided but no user-specific entry was found.
	globalQuery := `SELECT reason FROM blacklisted_numbers WHERE phone_number = $1 AND user_id IS NULL LIMIT 1`
	err = r.db.QueryRow(ctx, globalQuery, phoneNumber).Scan(&nullableReason)
	if err == nil {
		// Found a global entry
		if nullableReason.Valid {
			reason = nullableReason.String
		}
		r.logger.InfoContext(ctx, "Number found in global blacklist", "phone_number", phoneNumber)
		return true, reason, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		r.logger.ErrorContext(ctx, "Error checking global blacklist", "phone_number", phoneNumber, "error", err)
		return false, "", fmt.Errorf("checking global blacklist: %w", err)
	}

	// If not found in user-specific (if applicable) and not in global, then it's not blacklisted.
	r.logger.DebugContext(ctx, "Number not found in any blacklist", "phone_number", phoneNumber, "user_id", userID)
	return false, "", nil
}
