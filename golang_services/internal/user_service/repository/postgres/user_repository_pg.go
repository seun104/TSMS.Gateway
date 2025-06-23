package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/aradsms/golang_services/internal/user_service/domain"
	"github.com/aradsms/golang_services/internal/user_service/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUserNotFound = errors.New("user not found")
var ErrDuplicateUser = errors.New("username or email already exists")

type pgUserRepository struct {
	db *pgxpool.Pool
}

func NewPgUserRepository(db *pgxpool.Pool) repository.UserRepository {
	return &pgUserRepository{db: db}
}

// ... (Create, GetByID, GetByUsername, GetByEmail, GetByAPIKeyHash, Update, UpdateLoginInfo, Delete methods from previous steps) ...
func (r *pgUserRepository) Create(ctx context.Context, user *domain.User) (*domain.User, error) {
	user.ID = uuid.NewString()
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now
	query := `
		INSERT INTO users (id, username, email, hashed_password, first_name, last_name, phone_number,
		                 credit_balance, currency_code, role_id, parent_user_id, is_active, is_admin,
		                 api_key_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`
	_, err := r.db.Exec(ctx, query,
		user.ID, user.Username, user.Email, user.HashedPassword, user.FirstName, user.LastName, user.PhoneNumber,
		user.CreditBalance, user.CurrencyCode, user.RoleID, user.ParentUserID, user.IsActive, user.IsAdmin,
		user.APIKey,
		user.CreatedAt, user.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrDuplicateUser
		}
		return nil, err
	}
	return user, nil
}

func (r *pgUserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	user := &domain.User{}
	query := `
        SELECT id, username, email, hashed_password, first_name, last_name, phone_number,
               credit_balance, currency_code, role_id, parent_user_id, is_active, is_admin,
               api_key_hash, last_login_at, failed_login_attempts, lockout_until, created_at, updated_at
        FROM users WHERE id = $1
    `
	err := r.db.QueryRow(ctx, query, id).Scan(
		&user.ID, &user.Username, &user.Email, &user.HashedPassword, &user.FirstName, &user.LastName, &user.PhoneNumber,
		&user.CreditBalance, &user.CurrencyCode, &user.RoleID, &user.ParentUserID, &user.IsActive, &user.IsAdmin,
		&user.APIKey, &user.LastLoginAt, &user.FailedLoginAttempts, &user.LockoutUntil, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) { return nil, ErrUserNotFound }
		return nil, err
	}
	return user, nil
}
func (r *pgUserRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
    user := &domain.User{}
    query := `
        SELECT id, username, email, hashed_password, first_name, last_name, phone_number,
               credit_balance, currency_code, role_id, parent_user_id, is_active, is_admin,
               api_key_hash, last_login_at, failed_login_attempts, lockout_until, created_at, updated_at
        FROM users WHERE username = $1
    `
    err := r.db.QueryRow(ctx, query, username).Scan(
        &user.ID, &user.Username, &user.Email, &user.HashedPassword, &user.FirstName, &user.LastName, &user.PhoneNumber,
        &user.CreditBalance, &user.CurrencyCode, &user.RoleID, &user.ParentUserID, &user.IsActive, &user.IsAdmin,
        &user.APIKey, &user.LastLoginAt, &user.FailedLoginAttempts, &user.LockoutUntil, &user.CreatedAt, &user.UpdatedAt,
    )
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) { return nil, ErrUserNotFound }
        return nil, err
    }
    return user, nil
}
func (r *pgUserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
    user := &domain.User{}
    query := "SELECT id, username, email, hashed_password, first_name, last_name, phone_number, credit_balance, currency_code, role_id, parent_user_id, is_active, is_admin, api_key_hash, last_login_at, failed_login_attempts, lockout_until, created_at, updated_at FROM users WHERE email = $1"
    err := r.db.QueryRow(ctx, query, email).Scan(
        &user.ID, &user.Username, &user.Email, &user.HashedPassword, &user.FirstName, &user.LastName, &user.PhoneNumber,
        &user.CreditBalance, &user.CurrencyCode, &user.RoleID, &user.ParentUserID, &user.IsActive, &user.IsAdmin,
        &user.APIKey, &user.LastLoginAt, &user.FailedLoginAttempts, &user.LockoutUntil, &user.CreatedAt, &user.UpdatedAt,
    )
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) { return nil, ErrUserNotFound }
        return nil, err
    }
    return user, nil
}
func (r *pgUserRepository) GetByAPIKeyHash(ctx context.Context, apiKeyHash string) (*domain.User, error) {
    user := &domain.User{}
    query := "SELECT id, username, email, hashed_password, first_name, last_name, phone_number, credit_balance, currency_code, role_id, parent_user_id, is_active, is_admin, api_key_hash, last_login_at, failed_login_attempts, lockout_until, created_at, updated_at FROM users WHERE api_key_hash = $1"
    err := r.db.QueryRow(ctx, query, apiKeyHash).Scan(
        &user.ID, &user.Username, &user.Email, &user.HashedPassword, &user.FirstName, &user.LastName, &user.PhoneNumber,
        &user.CreditBalance, &user.CurrencyCode, &user.RoleID, &user.ParentUserID, &user.IsActive, &user.IsAdmin,
        &user.APIKey, &user.LastLoginAt, &user.FailedLoginAttempts, &user.LockoutUntil, &user.CreatedAt, &user.UpdatedAt,
    )
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) { return nil, ErrUserNotFound }
        return nil, err
    }
    return user, nil
}
func (r *pgUserRepository) Update(ctx context.Context, user *domain.User) error {
	user.UpdatedAt = time.Now()
	query := `
		UPDATE users SET
			username = $2, email = $3, hashed_password = $4, first_name = $5, last_name = $6,
			phone_number = $7, credit_balance = $8, currency_code = $9, role_id = $10,
			parent_user_id = $11, is_active = $12, is_admin = $13, api_key_hash = $14,
			updated_at = $15
		WHERE id = $1
	`
	tag, err := r.db.Exec(ctx, query,
		user.ID, user.Username, user.Email, user.HashedPassword, user.FirstName, user.LastName, user.PhoneNumber,
		user.CreditBalance, user.CurrencyCode, user.RoleID, user.ParentUserID, user.IsActive, user.IsAdmin,
		user.APIKey, user.UpdatedAt,
	)
    if err != nil {
        var pgErr *pgconn.PgError
        if errors.As(err, &pgErr) && pgErr.Code == "23505" { return ErrDuplicateUser }
        return err
    }
    if tag.RowsAffected() == 0 {
        return ErrUserNotFound // Or a more specific "update affected no rows" error
    }
	return nil
}
func (r *pgUserRepository) UpdateLoginInfo(ctx context.Context, id string, loginTime time.Time, failedAttempts int, lockoutUntil *time.Time) error {
    query := `UPDATE users SET last_login_at = $2, failed_login_attempts = $3, lockout_until = $4, updated_at = $5 WHERE id = $1`
    _, err := r.db.Exec(ctx, query, id, loginTime, failedAttempts, lockoutUntil, time.Now())
    return err
}
func (r *pgUserRepository) Delete(ctx context.Context, id string) error {
    query := "DELETE FROM users WHERE id = $1"
	_, err := r.db.Exec(ctx, query, id)
	return err
}


// GetByIDForUpdate fetches a user by ID and locks the row for update (within a transaction).
func (r *pgUserRepository) GetByIDForUpdate(ctx context.Context, querier repository.Querier, id string) (*domain.User, error) {
	user := &domain.User{}
	query := `
		SELECT id, username, email, hashed_password, first_name, last_name, phone_number,
		       credit_balance, currency_code, role_id, parent_user_id, is_active, is_admin,
		       api_key_hash, last_login_at, failed_login_attempts, lockout_until, created_at, updated_at
		FROM users WHERE id = $1 FOR UPDATE
	`
	// The querier is pgx.Tx in this case, passed from the service layer
	err := querier.QueryRow(ctx, query, id).Scan(
		&user.ID, &user.Username, &user.Email, &user.HashedPassword, &user.FirstName, &user.LastName, &user.PhoneNumber,
		&user.CreditBalance, &user.CurrencyCode, &user.RoleID, &user.ParentUserID, &user.IsActive, &user.IsAdmin,
		&user.APIKey, &user.LastLoginAt, &user.FailedLoginAttempts, &user.LockoutUntil, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}

// UpdateCreditBalance updates only the credit_balance and updated_at fields for a user.
// It expects to be run within a transaction (via Querier).
func (r *pgUserRepository) UpdateCreditBalance(ctx context.Context, querier repository.Querier, id string, newBalance float64) error {
	query := `UPDATE users SET credit_balance = $2, updated_at = $3 WHERE id = $1`
	tag, err := querier.Exec(ctx, query, id, newBalance, time.Now())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound // Or a more specific error indicating the user was not found for update
	}
	return nil
}
