-- Migration: Create scheduled_jobs table
-- Direction: Up

CREATE TABLE IF NOT EXISTS scheduled_jobs (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    job_type VARCHAR(50) NOT NULL, -- e.g., 'sms', 'email_campaign'
    payload JSONB NOT NULL,        -- Job-specific parameters
    scheduled_at TIMESTAMPTZ NOT NULL, -- When the job is intended to run
    status VARCHAR(50) NOT NULL,   -- e.g., 'pending', 'processing', 'completed', 'failed', 'retry'
    run_at TIMESTAMPTZ NULL,       -- Actual time when the job processing started
    processed_at TIMESTAMPTZ NULL, -- Time when processing completed or terminally failed
    error_message TEXT NULL,       -- Error message if the job failed
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Index for efficiently querying jobs that are due to run:
-- Query would typically be: WHERE status IN ('pending', 'retry') AND scheduled_at <= NOW()
CREATE INDEX IF NOT EXISTS idx_scheduled_jobs_status_scheduled_at ON scheduled_jobs (status, scheduled_at);

-- Index for users to list their scheduled jobs
CREATE INDEX IF NOT EXISTS idx_scheduled_jobs_user_id ON scheduled_jobs (user_id);

-- Index on job_type if there's a need to query or manage jobs by type
CREATE INDEX IF NOT EXISTS idx_scheduled_jobs_job_type ON scheduled_jobs (job_type);


COMMENT ON TABLE scheduled_jobs IS 'Stores tasks that are scheduled to be executed at a future time.';
COMMENT ON COLUMN scheduled_jobs.user_id IS 'Identifier of the user who created or owns the scheduled job.';
COMMENT ON COLUMN scheduled_jobs.job_type IS 'Type of the job, e.g., "sms" for sending an SMS, "email" for sending an email.';
COMMENT ON COLUMN scheduled_jobs.payload IS 'JSONB data containing the specific parameters for the job, e.g., recipients and message for an SMS job.';
COMMENT ON COLUMN scheduled_jobs.scheduled_at IS 'The timestamp when the job is scheduled to be executed.';
COMMENT ON COLUMN scheduled_jobs.status IS 'Current status of the job (e.g., pending, processing, completed, failed).';
COMMENT ON COLUMN scheduled_jobs.run_at IS 'The timestamp when a worker picked up this job for execution.';
COMMENT ON COLUMN scheduled_jobs.processed_at IS 'The timestamp when the job execution was finished (either completed or failed).';
COMMENT ON COLUMN scheduled_jobs.error_message IS 'Stores any error message if the job execution failed.';
COMMENT ON COLUMN scheduled_jobs.retry_count IS 'Number of times this job has been attempted.';
