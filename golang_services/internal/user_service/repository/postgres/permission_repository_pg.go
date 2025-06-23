package postgres

import (
	"context"
    "errors"
	"github.com/aradsms/golang_services/internal/user_service/domain"
	"github.com/aradsms/golang_services/internal/user_service/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrPermissionNotFound = errors.New("permission not found")
// ErrDuplicatePermission if names must be unique

type pgPermissionRepository struct {
	db *pgxpool.Pool
}

func NewPgPermissionRepository(db *pgxpool.Pool) repository.PermissionRepository {
	return &pgPermissionRepository{db: db}
}

// Implement PermissionRepository interface methods
func (r *pgPermissionRepository) Create(ctx context.Context, permission *domain.Permission) (*domain.Permission, error) { /* ... */ return nil, nil }
func (r *pgPermissionRepository) GetByID(ctx context.Context, id string) (*domain.Permission, error) { /* ... */ return nil, ErrPermissionNotFound }
func (r *pgPermissionRepository) GetByName(ctx context.Context, name string) (*domain.Permission, error) { /* ... */ return nil, ErrPermissionNotFound }
func (r *pgPermissionRepository) GetAll(ctx context.Context) ([]domain.Permission, error) { /* ... */ return nil, nil }
