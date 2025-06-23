package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"strings" // For potential normalization if needed, e.g. ToLower

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/AradIT/aradsms/golang_services/internal/sms_sending_service/domain"
)

type PgFilterWordRepository struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

func NewPgFilterWordRepository(db *pgxpool.Pool, logger *slog.Logger) domain.FilterWordRepository {
	return &PgFilterWordRepository{db: db, logger: logger.With("component", "filter_word_repository_pg")}
}

func (r *PgFilterWordRepository) GetActiveFilterWords(ctx context.Context) ([]string, error) {
	query := `SELECT word FROM filter_words WHERE is_active = TRUE`
	r.logger.DebugContext(ctx, "Fetching active filter words", "query", query)

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error querying active filter words", "error", err)
		return nil, fmt.Errorf("querying active filter words: %w", err)
	}
	defer rows.Close()

	var words []string
	for rows.Next() {
		var word string
		if err := rows.Scan(&word); err != nil {
			r.logger.ErrorContext(ctx, "Error scanning filter word row", "error", err)
			// Continue to collect other words, but log this error.
			// Depending on requirements, might choose to return error and no words.
			continue
		}
		// The words are stored in the DB. Normalization (e.g., to lowercase) should happen
		// during insertion into the DB (via unique index on lower(word)) and during checking.
		// Here, we return them as they are stored. The checking logic will handle normalization.
		words = append(words, word)
	}

	if err = rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "Error after iterating filter word rows", "error", err)
		return nil, fmt.Errorf("iterating filter word rows: %w", err)
	}

	r.logger.InfoContext(ctx, "Fetched active filter words", "count", len(words))
	return words, nil
}
