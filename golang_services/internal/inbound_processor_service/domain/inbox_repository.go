package domain

import (
	"context"
)

// InboxRepository defines the interface for interacting with the storage
// of incoming SMS messages (inbox_messages table).
type InboxRepository interface {
	// Create inserts a new InboxMessage record into the database.
	Create(ctx context.Context, msg *InboxMessage) error

	// GetByProviderMessageID checks if a message with the given provider_message_id already exists.
	// This can be useful to prevent processing duplicate callbacks from providers.
	// Returns (nil, nil) if not found, (*InboxMessage, nil) if found, or (nil, error) on error.
	// GetByProviderMessageID(ctx context.Context, providerName string, providerMessageID string) (*InboxMessage, error)
	// TODO: Decide if GetByProviderMessageID is needed or if unique constraints on DB are sufficient.
	// For now, focusing on Create.
}
