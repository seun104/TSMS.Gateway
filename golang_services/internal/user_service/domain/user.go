package domain

import (
	"time"
	// "github.com/google/uuid" // Will be needed if using UUIDs directly in domain models
)

// User represents a user in the system.
type User struct {
	ID                string    `json:"id"` // Using string for UUID representation
	Username          string    `json:"username"`
	Email             string    `json:"email,omitempty"`
	HashedPassword    string    `json:"-"` // Never expose
	FirstName         string    `json:"first_name,omitempty"`
	LastName          string    `json:"last_name,omitempty"`
	PhoneNumber       string    `json:"phone_number,omitempty"`
	CreditBalance     float64   `json:"credit_balance"` // Consider using a decimal type for precision
	CurrencyCode      string    `json:"currency_code"`
	RoleID            string    `json:"role_id,omitempty"` // Foreign key to Role
	ParentUserID      string    `json:"parent_user_id,omitempty"` // Nullable foreign key to User
	IsActive          bool      `json:"is_active"`
	IsAdmin           bool      `json:"is_admin"`
	APIKey            string    `json:"-"` // Store hashed or manage separately; never expose directly
	LastLoginAt       *time.Time `json:"last_login_at,omitempty"`
	FailedLoginAttempts int       `json:"-"`
	LockoutUntil      *time.Time `json:"-"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Role represents a user role.
type Role struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Permissions []Permission `json:"permissions,omitempty"` // Often loaded separately
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Permission represents an action that can be performed.
type Permission struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"` // e.g., "sms:send", "users:create"
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RefreshToken stores information about a user's refresh token.
type RefreshToken struct {
    ID        string    `json:"id"`
    UserID    string    `json:"user_id"`
    TokenHash string    `json:"-"` // Store the hash, not the token itself
    ExpiresAt time.Time `json:"expires_at"`
    CreatedAt time.Time `json:"created_at"`
}
