package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/AradIT/Arad.SMS.Gateway/golang_services/internal/scheduler_service/domain"
	"strings"
	"fmt"
)

type PgScheduledJobRepository struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

func NewPgScheduledJobRepository(db *pgxpool.Pool, logger *slog.Logger) *PgScheduledJobRepository {
	return &PgScheduledJobRepository{db: db, logger: logger}
}

func (r *PgScheduledJobRepository) Create(ctx context.Context, job *domain.ScheduledJob) error {
	query := `
		INSERT INTO scheduled_jobs (id, user_id, job_type, payload, scheduled_at, status, retry_count, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := r.db.Exec(ctx, query,
		job.ID, job.UserID, job.JobType, job.Payload, job.ScheduledAt, job.Status,
		job.RetryCount, job.CreatedAt, job.UpdatedAt,
	)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error creating scheduled job", "error", err, "job_id", job.ID)
		return err
	}
	r.logger.InfoContext(ctx, "Scheduled job created successfully", "job_id", job.ID)
	return nil
}

func (r *PgScheduledJobRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.ScheduledJob, error) {
	query := `
		SELECT id, user_id, job_type, payload, scheduled_at, status, run_at, processed_at, error_message, retry_count, created_at, updated_at
		FROM scheduled_jobs
		WHERE id = $1
	`
	job := &domain.ScheduledJob{}
	var payloadJSON []byte
	err := r.db.QueryRow(ctx, query, id).Scan(
		&job.ID, &job.UserID, &job.JobType, &payloadJSON, &job.ScheduledAt, &job.Status,
		&job.RunAt, &job.ProcessedAt, &job.Error, &job.RetryCount, &job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.WarnContext(ctx, "Scheduled job not found", "job_id", id)
			return nil, domain.ErrNotFound
		}
		r.logger.ErrorContext(ctx, "Error getting scheduled job by ID", "error", err, "job_id", id)
		return nil, err
	}
	job.Payload = json.RawMessage(payloadJSON)
	return job, nil
}

// UpdateStatus updates a job's status, error message, and relevant timestamps (run_at or processed_at).
// retryIncrement is added to current retry_count.
func (r *PgScheduledJobRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.JobStatus, eventTime time.Time, errorMessage sql.NullString, retryIncrement int) error {
	var query string
	var tag pgx.CommandTag
	var err error

	updatedAt := time.Now().UTC()

	switch status {
	case domain.StatusProcessing:
		query = `
			UPDATE scheduled_jobs
			SET status = $1, run_at = $2, retry_count = retry_count + $3, updated_at = $4
			WHERE id = $5
		`
		tag, err = r.db.Exec(ctx, query, status, eventTime, retryIncrement, updatedAt, id)
	case domain.StatusCompleted, domain.StatusFailed:
		query = `
			UPDATE scheduled_jobs
			SET status = $1, processed_at = $2, error_message = $3, updated_at = $4
			WHERE id = $5
		`
		tag, err = r.db.Exec(ctx, query, status, eventTime, errorMessage, updatedAt, id)
	default: // For other statuses like pending, retry (where only status and maybe error changes)
		query = `
			UPDATE scheduled_jobs
			SET status = $1, error_message = $2, updated_at = $3
			WHERE id = $4
		`
		tag, err = r.db.Exec(ctx, query, status, errorMessage, updatedAt, id)
	}

	if err != nil {
		r.logger.ErrorContext(ctx, "Error updating scheduled job status", "error", err, "job_id", id, "new_status", status)
		return err
	}
	if tag.RowsAffected() == 0 {
		r.logger.WarnContext(ctx, "Scheduled job not found for status update", "job_id", id)
		return domain.ErrNotFound
	}
	r.logger.InfoContext(ctx, "Scheduled job status updated", "job_id", id, "new_status", status)
	return nil
}

func (r *PgScheduledJobRepository) AcquireDueJobs(ctx context.Context, dueTime time.Time, limit int) ([]*domain.ScheduledJob, error) {
	query := `
		WITH due_job_ids AS (
			SELECT id
			FROM scheduled_jobs
			WHERE (status = $1 OR status = $2) AND scheduled_at <= $3
			ORDER BY scheduled_at ASC, retry_count ASC
			LIMIT $4
			FOR UPDATE SKIP LOCKED
		)
		UPDATE scheduled_jobs sj
		SET status = $5, run_at = $6, retry_count = sj.retry_count + CASE WHEN sj.status = $2 THEN 1 ELSE 0 END, updated_at = $6
		FROM due_job_ids dj
		WHERE sj.id = dj.id
		RETURNING sj.id, sj.user_id, sj.job_type, sj.payload, sj.scheduled_at, sj.status, sj.run_at, sj.processed_at, sj.error_message, sj.retry_count, sj.created_at, sj.updated_at;
	`
	// $1 = StatusPending, $2 = StatusRetry, $3 = dueTime, $4 = limit, $5 = StatusProcessing, $6 = now()
	now := time.Now().UTC()
	rows, err := r.db.Query(ctx, query, domain.StatusPending, domain.StatusRetry, dueTime, limit, domain.StatusProcessing, now)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error acquiring due jobs", "error", err)
		return nil, err
	}
	defer rows.Close()

	var jobs []*domain.ScheduledJob
	for rows.Next() {
		job := &domain.ScheduledJob{}
		var payloadJSON []byte
		if err := rows.Scan(
			&job.ID, &job.UserID, &job.JobType, &payloadJSON, &job.ScheduledAt, &job.Status,
			&job.RunAt, &job.ProcessedAt, &job.Error, &job.RetryCount, &job.CreatedAt, &job.UpdatedAt,
		); err != nil {
			r.logger.ErrorContext(ctx, "Error scanning acquired job row", "error", err)
			// Continue to scan other rows if possible, or return error immediately
			return nil, err
		}
		job.Payload = json.RawMessage(payloadJSON)
		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "Error iterating acquired job rows", "error", err)
		return nil, err
	}

	if len(jobs) == 0 {
		return nil, domain.ErrNoDueJobs // Specific error if no jobs were acquired
	}

	r.logger.InfoContext(ctx, "Acquired jobs for processing", "count", len(jobs))
	return jobs, nil
}

// MarkForRetry updates a job's status to 'retry', increments its retry count,
// and sets a new scheduled_at time for the next attempt.
func (r *PgScheduledJobRepository) MarkForRetry(ctx context.Context, id uuid.UUID, nextRetryTime time.Time, currentRetryCount int, errorMessage sql.NullString) error {
	query := `
		UPDATE scheduled_jobs
		SET status = $1, scheduled_at = $2, retry_count = $3, error_message = $4, updated_at = $5, run_at = NULL, processed_at = NULL
		WHERE id = $6
	`
	// run_at and processed_at are NULLed out for the retry
	updatedAt := time.Now().UTC()
	tag, err := r.db.Exec(ctx, query,
		domain.StatusRetry,
		nextRetryTime,
		currentRetryCount+1, // Incrementing retry count
		errorMessage,
		updatedAt,
		id,
	)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error marking job for retry", "error", err, "job_id", id)
		return err
	}
	if tag.RowsAffected() == 0 {
		r.logger.WarnContext(ctx, "Scheduled job not found for marking as retry", "job_id", id)
		return domain.ErrNotFound
	}
	r.logger.InfoContext(ctx, "Scheduled job marked for retry", "job_id", id, "next_retry_at", nextRetryTime)
	return nil
}

func (r *PgScheduledJobRepository) Update(ctx context.Context, job *domain.ScheduledJob) error {
	query := `
		UPDATE scheduled_jobs
		SET user_id = $1, job_type = $2, payload = $3, scheduled_at = $4, status = $5,
		    retry_count = $6, error_message = $7, run_at = $8, processed_at = $9, updated_at = $10
		WHERE id = $11
	`
	job.UpdatedAt = time.Now().UTC()
	_, err := r.db.Exec(ctx, query,
		job.UserID, job.JobType, job.Payload, job.ScheduledAt, job.Status,
		job.RetryCount, job.Error, job.RunAt, job.ProcessedAt, job.UpdatedAt,
		job.ID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.WarnContext(ctx, "Scheduled job not found for update", "job_id", job.ID)
			return domain.ErrNotFound
		}
		r.logger.ErrorContext(ctx, "Error updating scheduled job", "error", err, "job_id", job.ID)
		return err
	}
	r.logger.InfoContext(ctx, "Scheduled job updated successfully", "job_id", job.ID)
	return nil
}

func (r *PgScheduledJobRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM scheduled_jobs WHERE id = $1`
	tag, err := r.db.Exec(ctx, query, id)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error deleting scheduled job", "error", err, "job_id", id)
		return err
	}
	if tag.RowsAffected() == 0 {
		r.logger.WarnContext(ctx, "Scheduled job not found for deletion", "job_id", id)
		return domain.ErrNotFound
	}
	r.logger.InfoContext(ctx, "Scheduled job deleted successfully", "job_id", id)
	return nil
}

func (r *PgScheduledJobRepository) List(ctx context.Context, userID uuid.NullUUID, status string, jobType string, pageSize int, pageNumber int) ([]*domain.ScheduledJob, int, error) {
	var baseQuery strings.Builder
	baseQuery.WriteString("SELECT id, user_id, job_type, payload, scheduled_at, status, run_at, processed_at, error_message, retry_count, created_at, updated_at FROM scheduled_jobs")

	var countQueryBuilder strings.Builder
	countQueryBuilder.WriteString("SELECT COUNT(*) FROM scheduled_jobs")

	var conditions []string
	var args []interface{}
	argCounter := 1

	if userID.Valid {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", argCounter))
		args = append(args, userID.UUID)
		argCounter++
	}
	if status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argCounter))
		args = append(args, status)
		argCounter++
	}
	if jobType != "" {
		conditions = append(conditions, fmt.Sprintf("job_type = $%d", argCounter))
		args = append(args, jobType)
		argCounter++
	}

	if len(conditions) > 0 {
		whereClause := " WHERE " + strings.Join(conditions, " AND ")
		baseQuery.WriteString(whereClause)
		countQueryBuilder.WriteString(whereClause)
	}

	// Get total count
	var totalCount int
	err := r.db.QueryRow(ctx, countQueryBuilder.String(), args...).Scan(&totalCount)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error counting scheduled jobs", "error", err)
		return nil, 0, err
	}

	if totalCount == 0 {
		return []*domain.ScheduledJob{}, 0, nil
	}

	// Add sorting, pagination for the actual list query
	baseQuery.WriteString(" ORDER BY created_at DESC") // Default sort
	if pageSize > 0 && pageNumber > 0 {
		offset := (pageNumber - 1) * pageSize
		baseQuery.WriteString(fmt.Sprintf(" LIMIT $%d OFFSET $%d", argCounter, argCounter+1))
		args = append(args, pageSize, offset)
	} else if pageSize > 0 { // if only pageSize is provided, treat as limit for first page
		baseQuery.WriteString(fmt.Sprintf(" LIMIT $%d", argCounter))
		args = append(args, pageSize)
	}


	rows, err := r.db.Query(ctx, baseQuery.String(), args...)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error listing scheduled jobs", "error", err)
		return nil, 0, err
	}
	defer rows.Close()

	var jobs []*domain.ScheduledJob
	for rows.Next() {
		job := &domain.ScheduledJob{}
		var payloadJSON []byte
		if err := rows.Scan(
			&job.ID, &job.UserID, &job.JobType, &payloadJSON, &job.ScheduledAt, &job.Status,
			&job.RunAt, &job.ProcessedAt, &job.Error, &job.RetryCount, &job.CreatedAt, &job.UpdatedAt,
		); err != nil {
			r.logger.ErrorContext(ctx, "Error scanning scheduled job row during list", "error", err)
			return nil, 0, err
		}
		job.Payload = json.RawMessage(payloadJSON)
		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "Error iterating scheduled job rows during list", "error", err)
		return nil, 0, err
	}

	return jobs, totalCount, nil
}
