-- Migration: Create scheduled_jobs table
-- Direction: Down

DROP TABLE IF EXISTS scheduled_jobs;

-- Indexes (idx_scheduled_jobs_status_scheduled_at, idx_scheduled_jobs_user_id, idx_scheduled_jobs_job_type)
-- will be dropped automatically when the table is dropped.
