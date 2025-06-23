package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
	"errors" // Added for errors.Is check

	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker" // Corrected path
	"github.com/AradIT/aradsms/golang_services/internal/scheduler_service/domain"   // Corrected path
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	jobsProcessedCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "scheduler",
			Name:      "jobs_processed_total",
			Help:      "Total number of jobs processed by the scheduler.",
		},
		[]string{"job_type", "status"}, // e.g., job_type="sms", status="success"
	)
	jobProcessingDurationHist = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "scheduler",
			Name:      "job_processing_duration_seconds",
			Help:      "Duration of job processing.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"job_type"},
	)
)

// PollerConfig holds configuration specific to the JobPoller.
type PollerConfig struct {
	PollingInterval time.Duration `mapstructure:"SCHEDULER_POLLING_INTERVAL"`
	JobBatchSize    int           `mapstructure:"SCHEDULER_JOB_BATCH_SIZE"`
	MaxRetry        int           `mapstructure:"SCHEDULER_MAX_RETRY"`
}

// JobPoller is responsible for acquiring due scheduled jobs and dispatching them.
type JobPoller struct {
	repo       domain.ScheduledJobRepository
	natsClient *messagebroker.NATSClient
	logger     *slog.Logger
	config     PollerConfig
}

// NewJobPoller creates a new JobPoller instance.
func NewJobPoller(
	repo domain.ScheduledJobRepository,
	nc *messagebroker.NATSClient,
	logger *slog.Logger,
	cfg PollerConfig,
) *JobPoller {
	return &JobPoller{
		repo:       repo,
		natsClient: nc,
		logger:     logger,
		config:     cfg,
	}
}

// PollAndProcessJobs acquires due jobs and attempts to process them.
// It returns the number of jobs attempted in this poll cycle and any critical error.
func (p *JobPoller) PollAndProcessJobs(ctx context.Context) (processedInLoop int, criticalErr error) {
	p.logger.InfoContext(ctx, "Polling for due jobs...", "due_time", time.Now().UTC().Format(time.RFC3339), "batch_size", p.config.JobBatchSize)

	acquiredJobs, err := p.repo.AcquireDueJobs(ctx, time.Now().UTC(), p.config.JobBatchSize)
	if err != nil {
		if errors.Is(err, domain.ErrNoDueJobs) {
			p.logger.InfoContext(ctx, "No due jobs found in this poll cycle.")
			return 0, nil // Not an error, just no work
		}
		p.logger.ErrorContext(ctx, "Failed to acquire due jobs", "error", err)
		return 0, fmt.Errorf("failed to acquire due jobs: %w", err) // Critical error for the poller
	}

	if len(acquiredJobs) == 0 {
		p.logger.InfoContext(ctx, "No due jobs were acquired despite no error.")
		return 0, nil
	}

	p.logger.InfoContext(ctx, "Acquired jobs for processing", "count", len(acquiredJobs))

	for _, job := range acquiredJobs {
		processedInLoop++
		timer := prometheus.NewTimer(jobProcessingDurationHist.WithLabelValues(string(job.JobType))) // Cast job.JobType
		jobStatus := "success" // Assume success initially

		p.logger.InfoContext(ctx, "Processing job", "job_id", job.ID, "job_type", job.JobType, "retry_count", job.RetryCount)

		var processingError error
		var natsPublishSuccess bool

		switch job.JobType {
		case domain.JobTypeSMS: // Use domain constant
			var smsPayload domain.SMSJobPayload
			if err := json.Unmarshal(job.Payload, &smsPayload); err != nil {
				p.logger.ErrorContext(ctx, "Failed to deserialize SMSJobPayload", "error", err, "job_id", job.ID, "payload", string(job.Payload))
				processingError = fmt.Errorf("deserialize payload: %w", err)
				// Mark as failed immediately, this is unlikely to succeed on retry without code change
				err = p.repo.UpdateStatus(ctx, job.ID, domain.StatusFailed, time.Now().UTC(), sql.NullString{String: "Payload deserialization failed: " + err.Error(), Valid: true}, 0)
				if err != nil {
					p.logger.ErrorContext(ctx, "Failed to update job status to Failed after deserialization error", "job_id", job.ID, "update_error", err)
				}
				continue // Move to next job
			}

			natsSubject := "sms.jobs.send" // Central subject for sending SMS jobs
			jsonData, err := json.Marshal(smsPayload)
			if err != nil {
				p.logger.ErrorContext(ctx, "Failed to marshal SMSJobPayload for NATS", "error", err, "job_id", job.ID)
				processingError = fmt.Errorf("marshal payload for NATS: %w", err)
				// This is an internal error, likely persistent
				err = p.repo.UpdateStatus(ctx, job.ID, domain.StatusFailed, time.Now().UTC(), sql.NullString{String: "Payload marshal for NATS failed: " + err.Error(), Valid: true}, 0)
				if err != nil {
					p.logger.ErrorContext(ctx, "Failed to update job status to Failed after NATS marshal error", "job_id", job.ID, "update_error", err)
				}
				continue
			}

			p.logger.InfoContext(ctx, "Publishing job to NATS", "job_id", job.ID, "subject", natsSubject)
			if err := p.natsClient.Publish(ctx, natsSubject, jsonData); err != nil {
				p.logger.ErrorContext(ctx, "Failed to publish job to NATS", "error", err, "job_id", job.ID, "subject", natsSubject)
				processingError = fmt.Errorf("NATS publish: %w", err)
				natsPublishSuccess = false
			} else {
				p.logger.InfoContext(ctx, "Successfully published job to NATS", "job_id", job.ID, "subject", natsSubject)
				natsPublishSuccess = true
			}

		default:
			p.logger.WarnContext(ctx, "Unknown job type encountered", "job_id", job.ID, "job_type", job.JobType)
			processingError = fmt.Errorf("unknown job type: %s", job.JobType)
			// Mark as failed, as we don't know how to process it
			err = p.repo.UpdateStatus(ctx, job.ID, domain.StatusFailed, time.Now().UTC(), sql.NullString{String: "Unknown job type: " + job.JobType, Valid: true}, 0)
			if err != nil {
				p.logger.ErrorContext(ctx, "Failed to update job status to Failed for unknown job type", "job_id", job.ID, "update_error", err)
			}
			continue
		}

		// Post-processing status update based on natsPublishSuccess or other processingError
		if natsPublishSuccess { // For "sms" type, this means success for now
			err = p.repo.UpdateStatus(ctx, job.ID, domain.StatusCompleted, time.Now().UTC(), sql.NullString{}, 0)
			if err != nil {
				jobStatus = "error_update_status_completed" // Specific error status for metrics
				p.logger.ErrorContext(ctx, "Failed to update job status to Completed", "job_id", job.ID, "update_error", err)
			}
		} else if processingError != nil { // Handle failures that occurred before or during NATS publish
			jobStatus = "error_processing"
			if job.RetryCount < p.config.MaxRetry {
				nextRetryTime := time.Now().UTC().Add(calculateBackoff(job.RetryCount + 1))
				p.logger.InfoContext(ctx, "Scheduling job for retry", "job_id", job.ID, "next_retry_at", nextRetryTime, "current_retries", job.RetryCount)
				err = p.repo.MarkForRetry(ctx, job.ID, nextRetryTime, job.RetryCount+1, sql.NullString{String: processingError.Error(), Valid: true}) // Increment retry count
				if err != nil {
					jobStatus = "error_mark_for_retry"
					p.logger.ErrorContext(ctx, "Failed to mark job for retry", "job_id", job.ID, "update_error", err)
				}
			} else {
				jobStatus = "error_max_retries_reached"
				p.logger.WarnContext(ctx, "Job failed after max retries", "job_id", job.ID, "error", processingError.Error(), "max_retries", p.config.MaxRetry)
				err = p.repo.UpdateStatus(ctx, job.ID, domain.StatusFailed, time.Now().UTC(), sql.NullString{String: "Failed after max retries: " + processingError.Error(), Valid: true}, 0)
				if err != nil {
					jobStatus = "error_update_status_failed"
					p.logger.ErrorContext(ctx, "Failed to update job status to Failed after max retries", "job_id", job.ID, "update_error", err)
				}
			}
		}
		timer.ObserveDuration()
		jobsProcessedCounter.WithLabelValues(string(job.JobType), jobStatus).Inc()
	} // end for loop

	return processedInLoop, nil
}

// calculateBackoff determines the next retry time.
// Example: exponential backoff with some jitter, or fixed delay.
func calculateBackoff(retryNum int) time.Duration {
	// Simple fixed delay for now, can be made more sophisticated
	// e.g., (2^retryNum) * base_interval + jitter
	return time.Minute * time.Duration(retryNum*2) // e.g. 2m, 4m, 6m...
}
