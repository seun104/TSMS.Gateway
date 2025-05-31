package postgres

import (
	"context"
    "errors"
	"github.com/aradsms/golang_services/internal/user_service/domain"
	"github.com/aradsms/golang_services/internal/user_service/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrRefreshTokenNotFound = errors.New("refresh token not found")

type pgRefreshTokenRepository struct {
	db *pgxpool.Pool
}

func NewPgRefreshTokenRepository(db *pgxpool.Pool) repository.RefreshTokenRepository {
	return &pgRefreshTokenRepository{db: db}
}

// Implement RefreshTokenRepository interface methods
func (r *pgRefreshTokenRepository) Create(ctx context.Context, token *domain.RefreshToken) error { /* ... */ return nil }
func (r *pgRefreshTokenRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) { /* ... */ return nil, ErrRefreshTokenNotFound }
func (r *pgRefreshTokenRepository) Delete(ctx context.Context, id string) error { /* ... */ return nil }
func (r *pgRefreshTokenRepository) DeleteByUserID(ctx context.Context, userID string) error { /* ... */ return nil }
