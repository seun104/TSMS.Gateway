package domain

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
)

// OutboxRepository defines the interface for interacting with the storage
// of outbox messages, specifically for updating their delivery status.
type OutboxRepository interface {
	// GetByMessageID retrieves a minimal OutboxMessage by its primary ID.
	// This might be used to check current status or for context before updating,
	// though for DLR processing, direct update is often sufficient if DLRs are trusted.
	GetByMessageID(ctx context.Context, messageID uuid.UUID) (*OutboxMessage, error)

	// UpdateStatus updates the delivery status information for a given message ID.
	// It should reflect the information received in a DeliveryReport.
	UpdateStatus(
		ctx context.Context,
		messageID uuid.UUID,
		status DeliveryStatus,
		providerStatus string,
		deliveredAt sql.NullTime,
		errorCode sql.NullString,
		errorDescription sql.NullString,
		providerMessageID sql.NullString, // Added to update provider_msg_id if it wasn't known at send time or changes
	) error
}
