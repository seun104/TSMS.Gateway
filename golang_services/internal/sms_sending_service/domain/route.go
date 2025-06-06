package domain // sms_sending_service/domain

import (
	"context"
	"time"
	"github.com/google/uuid"
)

// RouteCriteria defines the conditions for a route to be matched.
// All fields are optional. A more specific route (more matching criteria)
// should typically have higher priority.
type RouteCriteria struct {
	CountryCode    *string `json:"country_code,omitempty"`    // e.g., "98" for Iran, "1" for USA
	OperatorPrefix *string `json:"operator_prefix,omitempty"` // e.g., "912" for MCI, "935" for Irancell (after country code)
	UserID         *string `json:"user_id,omitempty"`         // Specific user ID to apply this route for
	// GroupID        *string `json:"group_id,omitempty"`     // Future: Specific group ID
	// SenderID       *string `json:"sender_id,omitempty"`    // Future: If route depends on requested sender ID
}

// Route defines a routing rule for SMS messages.
type Route struct {
	ID              uuid.UUID     `json:"id"`
	Name            string        `json:"name"`            // Descriptive name for the route
	Priority        int           `json:"priority"`        // Lower number means higher priority
	Criteria        RouteCriteria `json:"criteria"`        // Parsed criteria
	CriteriaJSON    string        `json:"criteria_json"`   // Raw JSON string of criteria from DB
	SmsProviderID   uuid.UUID     `json:"sms_provider_id"` // FK to sms_providers table
	SmsProviderName string        `json:"sms_provider_name"`// Name of the SMS provider (e.g., "magfa", "mock")
	IsActive        bool          `json:"is_active"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

// RouteRepository defines the interface for fetching route data.
type RouteRepository interface {
	// GetActiveRoutesOrderedByPriority fetches all active routes,
	// ordered by priority (ascending, so lower number is higher priority).
	GetActiveRoutesOrderedByPriority(ctx context.Context) ([]*Route, error)
	// Potentially other methods like GetByID, Create, Update, Delete for route management later.
}
