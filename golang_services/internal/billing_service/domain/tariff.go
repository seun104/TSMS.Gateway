package domain // billing_service/domain

import (
	"context"
	"database/sql"
	"time"
	"github.com/google/uuid"
)

// Tariff represents a pricing plan for SMS messages.
type Tariff struct {
	ID          uuid.UUID      `json:"id"`
	Name        string         `json:"name"`
	PricePerSMS int64          `json:"price_per_sms"` // Price in smallest currency unit (e.g., cents, rials)
	Currency    string         `json:"currency"`      // e.g., "USD", "EUR", "IRR"
	IsActive    bool           `json:"is_active"`
	Description sql.NullString `json:"description,omitempty"` // Optional description
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// UserTariff represents the assignment of a tariff to a user.
// This struct might not be directly returned by the repository if GetActiveUserTariff directly returns *Tariff.
// However, it's good to have if we ever manage UserTariff assignments explicitly.
// For now, the repository methods focus on directly fetching the applicable Tariff.
/*
type UserTariff struct {
    UserID     uuid.UUID
    TariffID   uuid.UUID
    AssignedAt time.Time
    UpdatedAt  time.Time
}
*/

// TariffRepository defines the interface for accessing tariff data.
type TariffRepository interface {
	// GetTariffByID retrieves a specific tariff by its ID.
	// Returns nil, nil if not found.
	GetTariffByID(ctx context.Context, id uuid.UUID) (*Tariff, error)

	// GetActiveUserTariff retrieves the currently active tariff assigned to a specific user.
	// Returns nil, nil if the user has no active tariff assigned or the tariff itself is inactive.
	GetActiveUserTariff(ctx context.Context, userID uuid.UUID) (*Tariff, error)

	// GetDefaultActiveTariff retrieves a system-wide default active tariff.
	// This is used if a user doesn't have a specific tariff assigned.
	// Returns nil, nil if no default active tariff is configured or found.
	GetDefaultActiveTariff(ctx context.Context) (*Tariff, error)

	// Future methods for tariff management:
	// CreateTariff(ctx context.Context, tariff *Tariff) error
	// UpdateTariff(ctx context.Context, tariff *Tariff) error
	// AssignTariffToUser(ctx context.Context, userID uuid.UUID, tariffID uuid.UUID) error
	// ListTariffs(ctx context.Context, page, pageSize int) ([]*Tariff, int, error)
}
