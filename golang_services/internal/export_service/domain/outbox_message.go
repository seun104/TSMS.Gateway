package domain // export_service/domain

import (
	"context"
	"time"
	"github.com/google/uuid"
	// Assuming core_sms/domain/MessageStatus is accessible or redefine simplified status here
	coreSmsDomain "github.com/AradIT/aradsms/golang_services/internal/core_sms/domain"
)

// ExportedOutboxMessage is a simplified version of OutboxMessage for export purposes.
type ExportedOutboxMessage struct {
	ID                uuid.UUID
	UserID            uuid.UUID
	SenderID          string
	Recipient         string
	Content           string // May need truncation or sanitization for CSV
	Status            coreSmsDomain.MessageStatus // Use existing status type
	Segments          int
	ProviderMessageID *string    // Nullable
	ScheduledFor      *time.Time // Nullable
	SentToProviderAt  *time.Time // Nullable
	DeliveredAt       *time.Time // Nullable
	CreatedAt         time.Time
}

// OutboxExportRepository defines the interface for fetching outbox messages for export.
type OutboxExportRepository interface {
	// GetOutboxMessagesForUser fetches messages for a specific user, potentially with filters.
	// Filters map can contain keys like "start_date", "end_date", "status", etc.
	GetOutboxMessagesForUser(ctx context.Context, userID uuid.UUID, filters map[string]string) ([]*ExportedOutboxMessage, error)
}
