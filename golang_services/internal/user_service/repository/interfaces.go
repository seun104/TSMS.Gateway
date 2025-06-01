package repository

import (
	"context"
	"time" // Added for UpdateLoginInfo

	"github.com/aradsms/golang_services/internal/user_service/domain"
    "github.com/jackc/pgx/v5"       // For pgx.Tx
    "github.com/jackc/pgx/v5/pgconn"
)

type Querier interface {
    Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) (*domain.User, error)
	GetByID(ctx context.Context, id string) (*domain.User, error)
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByAPIKeyHash(ctx context.Context, apiKeyHash string) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
    UpdateLoginInfo(ctx context.Context, id string, loginTime time.Time, failedAttempts int, lockoutUntil *time.Time) error
	Delete(ctx context.Context, id string) error
    // Methods for billing service interaction
    GetByIDForUpdate(ctx context.Context, querier Querier, id string) (*domain.User, error) // Accepts Querier for transaction
    UpdateCreditBalance(ctx context.Context, querier Querier, id string, newBalance float64) error // Accepts Querier
}

// ... (RoleRepository, PermissionRepository, RefreshTokenRepository interfaces remain the same) ...
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
type PermissionRepository interface {
    Create(ctx context.Context, permission *domain.Permission) (*domain.Permission, error)
    GetByID(ctx context.Context, id string) (*domain.Permission, error)
    GetByName(ctx context.Context, name string) (*domain.Permission, error)
    GetAll(ctx context.Context) ([]domain.Permission, error)
}
type RefreshTokenRepository interface {
    Create(ctx context.Context, token *domain.RefreshToken) error
    GetByID(ctx context.Context, id string) (*domain.RefreshToken, error) // Added GetByID
    GetByTokenHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error)
    Delete(ctx context.Context, id string) error
    DeleteByUserID(ctx context.Context, userID string) error
}
