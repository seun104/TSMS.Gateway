package domain // sms_sending_service/domain

import (
	"context"
	"database/sql"
	"time" // Added for time.Time
	"github.com/google/uuid"
)

// BlacklistedNumber represents a blacklisted phone number.
type BlacklistedNumber struct {
	ID          uuid.UUID
	PhoneNumber string
	UserID      uuid.NullUUID // If NULL, it's a global blacklist entry
	Reason      sql.NullString
	CreatedAt   time.Time // Changed from sql.NullTime, as created_at usually NOT NULL
}

// BlacklistRepository defines the interface for accessing blacklist data.
type BlacklistRepository interface {
	// IsBlacklisted checks if a phone number is blacklisted either globally
	// or for a specific user (if userID.Valid is true).
	// It returns true if blacklisted, the reason for the blacklist (if any),
	// and an error if the query fails.
	IsBlacklisted(ctx context.Context, phoneNumber string, userID uuid.NullUUID) (isBlacklisted bool, reason string, err error)
}
