package postgres

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/your-repo/project/internal/phonebook_service/domain"
)

type PgPhonebookRepository struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

func NewPgPhonebookRepository(db *pgxpool.Pool, logger *slog.Logger) *PgPhonebookRepository {
	return &PgPhonebookRepository{db: db, logger: logger}
}

func (r *PgPhonebookRepository) Create(ctx context.Context, pb *domain.Phonebook) error {
	query := `
		INSERT INTO phonebooks (id, user_id, name, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.db.Exec(ctx, query, pb.ID, pb.UserID, pb.Name, pb.Description, pb.CreatedAt, pb.UpdatedAt)
	if err != nil {
		// TODO: Check for unique constraint violations if 'name' and 'user_id' should be unique together
		r.logger.ErrorContext(ctx, "Error creating phonebook", "error", err, "phonebook_id", pb.ID, "user_id", pb.UserID)
		return err
	}
	r.logger.InfoContext(ctx, "Phonebook created successfully", "phonebook_id", pb.ID, "user_id", pb.UserID)
	return nil
}

func (r *PgPhonebookRepository) GetByID(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*domain.Phonebook, error) {
	query := `
		SELECT id, user_id, name, description, created_at, updated_at
		FROM phonebooks
		WHERE id = $1 AND user_id = $2
	`
	pb := &domain.Phonebook{}
	err := r.db.QueryRow(ctx, query, id, userID).Scan(
		&pb.ID, &pb.UserID, &pb.Name, &pb.Description, &pb.CreatedAt, &pb.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.WarnContext(ctx, "Phonebook not found or access denied", "phonebook_id", id, "user_id", userID)
			return nil, domain.ErrNotFound // Or ErrAccessDenied if we want to distinguish
		}
		r.logger.ErrorContext(ctx, "Error getting phonebook by ID", "error", err, "phonebook_id", id, "user_id", userID)
		return nil, err
	}
	return pb, nil
}

func (r *PgPhonebookRepository) ListByUserID(ctx context.Context, userID uuid.UUID, offset, limit int) ([]*domain.Phonebook, error) {
	query := `
		SELECT id, user_id, name, description, created_at, updated_at
		FROM phonebooks
		WHERE user_id = $1
		ORDER BY name ASC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.Query(ctx, query, userID, limit, offset)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error listing phonebooks by user ID", "error", err, "user_id", userID)
		return nil, err
	}
	defer rows.Close()

	var phonebooks []*domain.Phonebook
	for rows.Next() {
		pb := &domain.Phonebook{}
		if err := rows.Scan(&pb.ID, &pb.UserID, &pb.Name, &pb.Description, &pb.CreatedAt, &pb.UpdatedAt); err != nil {
			r.logger.ErrorContext(ctx, "Error scanning phonebook row", "error", err, "user_id", userID)
			// Decide if we should return partial results or fail all
			return nil, err
		}
		phonebooks = append(phonebooks, pb)
	}
	if err := rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "Error iterating phonebook rows", "error", err, "user_id", userID)
		return nil, err
	}
	return phonebooks, nil
}

func (r *PgPhonebookRepository) Update(ctx context.Context, pb *domain.Phonebook) error {
	query := `
		UPDATE phonebooks
		SET name = $1, description = $2, updated_at = $3
		WHERE id = $4 AND user_id = $5
	`
	pb.UpdatedAt = time.Now().UTC() // Ensure updated_at is set
	tag, err := r.db.Exec(ctx, query, pb.Name, pb.Description, pb.UpdatedAt, pb.ID, pb.UserID)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error updating phonebook", "error", err, "phonebook_id", pb.ID, "user_id", pb.UserID)
		return err
	}
	if tag.RowsAffected() == 0 {
		r.logger.WarnContext(ctx, "Phonebook not found or access denied for update", "phonebook_id", pb.ID, "user_id", pb.UserID)
		return domain.ErrNotFound // Or ErrAccessDenied
	}
	r.logger.InfoContext(ctx, "Phonebook updated successfully", "phonebook_id", pb.ID, "user_id", pb.UserID)
	return nil
}

func (r *PgPhonebookRepository) Delete(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	query := `DELETE FROM phonebooks WHERE id = $1 AND user_id = $2`
	tag, err := r.db.Exec(ctx, query, id, userID)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error deleting phonebook", "error", err, "phonebook_id", id, "user_id", userID)
		return err
	}
	if tag.RowsAffected() == 0 {
		r.logger.WarnContext(ctx, "Phonebook not found or access denied for delete", "phonebook_id", id, "user_id", userID)
		return domain.ErrNotFound // Or ErrAccessDenied
	}
	r.logger.InfoContext(ctx, "Phonebook deleted successfully", "phonebook_id", id, "user_id", userID)
	return nil
}
