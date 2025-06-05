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
	"github.com/your-repo/project/internal/scheduler_service/domain"
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
