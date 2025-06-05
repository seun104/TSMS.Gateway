package domain

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// ScheduledJobRepository defines the interface for managing ScheduledJob data.
type ScheduledJobRepository interface {
	Create(ctx context.Context, job *ScheduledJob) error
	GetByID(ctx context.Context, id uuid.UUID) (*ScheduledJob, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status JobStatus, eventTime time.Time, errorMessage sql.NullString, retryIncrement int) error
	// AcquireDueJobs selects 'pending' or 'retry' jobs that are due,
	// updates their status to 'processing', sets 'run_at', increments 'retry_count' (if applicable),
	// and returns them.
	AcquireDueJobs(ctx context.Context, dueTime time.Time, limit int) ([]*ScheduledJob, error)

	// MarkForRetry updates a job's status to 'retry', increments its retry count,
	// and sets a new scheduled_at time for the next attempt.
	MarkForRetry(ctx context.Context, id uuid.UUID, nextRetryTime time.Time, currentRetryCount int, errorMessage sql.NullString) error

	// ListByStatus retrieves jobs with a specific status, primarily for admin/monitoring.
	// ListByStatus(ctx context.Context, status JobStatus, offset, limit int) ([]*ScheduledJob, error)

	// DeleteOldJobs removes jobs older than a certain threshold with specified statuses (e.g., completed, failed).
	// DeleteOldJobs(ctx context.Context, olderThan time.Time, statuses []JobStatus) (int64, error) // Returns number of rows deleted
}
