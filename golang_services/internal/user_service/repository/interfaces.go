package repository

import (
	"context"
	"time" // Added for UpdateLoginInfo

	// Adjust import path to your domain models
	"github.com/aradsms/golang_services/internal/user_service/domain"

	// These will be needed by implementations.
	// Add them to go.mod via `go get github.com/jackc/pgx/v5`
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Querier defines common methods for DB interaction, implemented by *pgxpool.Pool and pgx.Tx
// This allows repositories to be used with or without transactions.
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	// Add other pgx/pgxpool methods if needed, or use a more specific interface from pgx itself
}

// UserRepository defines the interface for user data persistence.
type UserRepository interface {
	Create(ctx context.Context, user *domain.User) (*domain.User, error)
	GetByID(ctx context.Context, id string) (*domain.User, error)
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByAPIKeyHash(ctx context.Context, apiKeyHash string) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
	UpdateLoginInfo(ctx context.Context, id string, loginTime time.Time, failedAttempts int, lockoutUntil *time.Time) error
	Delete(ctx context.Context, id string) error
	// Add other methods like List, Count, etc. as needed
}

// RoleRepository defines the interface for role data persistence.
type RoleRepository interface {
	Create(ctx context.Context, role *domain.Role) (*domain.Role, error)
	GetByID(ctx context.Context, id string) (*domain.Role, error)
	GetByName(ctx context.Context, name string) (*domain.Role, error)
	GetAll(ctx context.Context) ([]domain.Role, error)
	Update(ctx context.Context, role *domain.Role) error
	Delete(ctx context.Context, id string) error
	AddPermissionToRole(ctx context.Context, roleID string, permissionID string) error
	RemovePermissionFromRole(ctx context.Context, roleID string, permissionID string) error
	GetPermissionsForRole(ctx context.Context, roleID string) ([]domain.Permission, error)
}

// PermissionRepository defines the interface for permission data persistence.
type PermissionRepository interface {
	Create(ctx context.Context, permission *domain.Permission) (*domain.Permission, error)
	GetByID(ctx context.Context, id string) (*domain.Permission, error)
	GetByName(ctx context.Context, name string) (*domain.Permission, error)
	GetAll(ctx context.Context) ([]domain.Permission, error)
	// Delete is often not needed for permissions if they are static, but can be added
}

// RefreshTokenRepository defines the interface for refresh token persistence.
type RefreshTokenRepository interface {
	Create(ctx context.Context, token *domain.RefreshToken) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error)
	Delete(ctx context.Context, id string) error
	DeleteByUserID(ctx context.Context, userID string) error // For logging out user from all sessions
}
