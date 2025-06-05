package domain

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// JobStatus represents the status of a scheduled job.
type JobStatus string

const (
	StatusPending    JobStatus = "pending"
	StatusProcessing JobStatus = "processing" // Job picked up by a worker
	StatusCompleted  JobStatus = "completed"  // Job finished successfully
	StatusFailed     JobStatus = "failed"     // Job failed and will not be retried (or exhausted retries)
	StatusRetry      JobStatus = "retry"      // Job failed, but is scheduled for a retry
)

// ScheduledJob represents a task that is scheduled to be executed at a later time.
type ScheduledJob struct {
	ID          uuid.UUID       `json:"id"`
	UserID      uuid.UUID       `json:"user_id"`    // User who owns/created this job
	JobType     string          `json:"job_type"`   // e.g., "sms", "email_campaign", "webhook_call"
	Payload     json.RawMessage `json:"payload"`    // Job-specific parameters, stored as JSONB
	ScheduledAt time.Time       `json:"scheduled_at"` // When the job is intended to run
	Status      JobStatus       `json:"status"`
	RunAt       sql.NullTime    `json:"run_at,omitempty"`       // Actual time when the job was picked up for processing
	ProcessedAt sql.NullTime    `json:"processed_at,omitempty"` // Time when processing completed/failed
	Error       sql.NullString  `json:"error,omitempty"`        // Error message if the job failed
	RetryCount  int             `json:"retry_count"`            // Number of times this job has been retried
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// NewScheduledJob creates a new ScheduledJob instance.
// ID is typically generated before calling this.
// Payload should be a marshaled JSON byte slice.
func NewScheduledJob(
	id uuid.UUID,
	userID uuid.UUID,
	jobType string,
	payload json.RawMessage,
	scheduledAt time.Time,
) *ScheduledJob {
	now := time.Now().UTC()
	return &ScheduledJob{
		ID:          id,
		UserID:      userID,
		JobType:     jobType,
		Payload:     payload,
		ScheduledAt: scheduledAt,
		Status:      StatusPending, // Initial status
		RetryCount:  0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// --- Example Payload Structures ---

// SMSJobPayload is an example structure for the Payload field when JobType is "sms".
type SMSJobPayload struct {
	Recipients   []string `json:"recipients"`
	Message      string   `json:"message"`
	SenderID     string   `json:"sender_id,omitempty"` // Optional, system might have a default
	IsUnicode    bool     `json:"is_unicode,omitempty"`
	IsFlash      bool     `json:"is_flash,omitempty"`
	// DeliveryCallbackURL string   `json:"delivery_callback_url,omitempty"` // Optional callback for this specific job
}

// ToJSON marshals the SMSJobPayload to json.RawMessage.
func (p *SMSJobPayload) ToJSON() (json.RawMessage, error) {
	return json.Marshal(p)
}

// FromJSON unmarshals json.RawMessage into SMSJobPayload.
func (p *SMSJobPayload) FromJSON(data json.RawMessage) error {
	return json.Unmarshal(data, p)
}

// Example of another job type payload
// type EmailCampaignPayload struct {
// 	CampaignID  uuid.UUID `json:"campaign_id"`
// 	SegmentID   uuid.UUID `json:"segment_id,omitempty"`
// 	Subject     string    `json:"subject"`
// }
