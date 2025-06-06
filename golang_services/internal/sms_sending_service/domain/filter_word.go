package domain // sms_sending_service/domain

import (
	"context"
	"github.com/google/uuid"
	"time"
)

// FilterWord represents a word that should be filtered from messages.
type FilterWord struct {
	ID        uuid.UUID `json:"id"`
	Word      string    `json:"word"`       // The actual word/phrase to filter
	IsActive  bool      `json:"is_active"`  // Whether this filter word is currently active
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FilterWordRepository defines the interface for accessing filter word data.
type FilterWordRepository interface {
	// GetActiveFilterWords fetches all active filter words.
	// Returns a slice of strings (the words themselves, typically normalized like lowercase)
	// and an error if the query fails.
	GetActiveFilterWords(ctx context.Context) ([]string, error)
}
