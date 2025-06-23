package http

import (
	"time"
)

// --- Request DTOs ---

// CreateScheduledMessageRequestDTO is used for creating a new scheduled message.
type CreateScheduledMessageRequestDTO struct {
	JobType      string            `json:"job_type" validate:"required,alphanum,max=50"`
	Payload      map[string]string `json:"payload" validate:"required"`
	ScheduleTime time.Time         `json:"schedule_time" validate:"required"` // Should be in future
	UserID       string            `json:"user_id,omitempty" validate:"omitempty,uuid"` // Optional: if admin schedules for user or system schedules
}

// UpdateScheduledMessageRequestDTO is used for updating an existing scheduled message.
// All fields are optional. Use pointers for fields that can distinguish between empty and not provided (e.g. time.Time).
// However, for simple types like string, omitempty is often sufficient.
type UpdateScheduledMessageRequestDTO struct {
	Payload      map[string]string `json:"payload,omitempty"`
	ScheduleTime *time.Time        `json:"schedule_time,omitempty"` // Pointer to distinguish empty from not provided
	Status       string            `json:"status,omitempty" validate:"omitempty,oneof=pending cancelled"` // Example: only allow cancellation or re-pending by user
}

// --- Response DTOs ---

// ScheduledMessageDTO represents a scheduled message in API responses.
type ScheduledMessageDTO struct {
	ID           string            `json:"id"`
	JobType      string            `json:"job_type"`
	Payload      map[string]string `json:"payload,omitempty"` // omitempty if payload can be large or sensitive and not always needed in list views
	ScheduleTime time.Time         `json:"schedule_time"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	Status       string            `json:"status"`
	UserID       string            `json:"user_id,omitempty"`
}

// ListScheduledMessagesResponseDTO is the response for listing scheduled messages.
type ListScheduledMessagesResponseDTO struct {
	Messages   []ScheduledMessageDTO `json:"messages"`
	TotalCount int32                 `json:"total_count"`
	Page       int32                 `json:"page,omitempty"`
	PageSize   int32                 `json:"page_size,omitempty"`
}
