package repository

import (
	"context"
	"time" // Added for UpdateStatus method signature
	"github.com/aradsms/golang_services/internal/core_domain" // Using shared core domain models
)

// OutboxMessageRepository defines the interface for outbox message data persistence.
type OutboxMessageRepository interface {
	Create(ctx context.Context, msg *core_domain.OutboxMessage) (*core_domain.OutboxMessage, error)
	GetByID(ctx context.Context, id string) (*core_domain.OutboxMessage, error)
	UpdateStatus(ctx context.Context, id string, status core_domain.MessageStatus, providerMsgID *string, providerStatusCode *string, errorMessage *string, sentAt *time.Time) error
    UpdateDeliveryStatus(ctx context.Context, id string, status core_domain.MessageStatus, deliveredAt *time.Time, providerStatusCode *string) error
    GetMessagesByStatus(ctx context.Context, status core_domain.MessageStatus, limit int) ([]*core_domain.OutboxMessage, error)
    // Add other methods as needed, e.g., for querying by user, recipient, etc.
}
