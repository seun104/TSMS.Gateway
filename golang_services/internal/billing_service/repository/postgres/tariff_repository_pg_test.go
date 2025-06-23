package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/AradIT/aradsms/golang_services/internal/billing_service/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTariffTest(t *testing.T) (domain.TariffRepository, pgxmock.PgxPoolIface) {
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := NewPgTariffRepository(mockPool, logger)
	return repo, mockPool
}

func TestPgTariffRepository_GetTariffByID(t *testing.T) {
	repo, mockPool := setupTariffTest(t)
	defer mockPool.Close()

	tariffID := uuid.New()
	expectedTariff := &domain.Tariff{
		ID:          tariffID,
		Name:        "Test Tariff",
		PricePerSMS: 100,
		Currency:    "USD",
		IsActive:    true,
		Description: sql.NullString{String: "A test tariff", Valid: true},
		CreatedAt:   time.Now().Add(-24 * time.Hour),
		UpdatedAt:   time.Now().Add(-1 * time.Hour),
	}

	t.Run("Found", func(t *testing.T) {
		rows := mockPool.NewRows([]string{"id", "name", "price_per_sms", "currency", "is_active", "description", "created_at", "updated_at"}).
			AddRow(expectedTariff.ID, expectedTariff.Name, expectedTariff.PricePerSMS, expectedTariff.Currency, expectedTariff.IsActive, expectedTariff.Description, expectedTariff.CreatedAt, expectedTariff.UpdatedAt)

		mockPool.ExpectQuery(`SELECT id, name, price_per_sms, currency, is_active, description, created_at, updated_at FROM tariffs WHERE id = \$1 AND is_active = TRUE`).
			WithArgs(tariffID).
			WillReturnRows(rows)

		tariff, err := repo.GetTariffByID(context.Background(), tariffID)
		require.NoError(t, err)
		require.NotNil(t, tariff)
		assert.Equal(t, expectedTariff.ID, tariff.ID)
		assert.Equal(t, expectedTariff.Name, tariff.Name)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("NotFound", func(t *testing.T) {
		mockPool.ExpectQuery(`SELECT id, name, price_per_sms, currency, is_active, description, created_at, updated_at FROM tariffs WHERE id = \$1 AND is_active = TRUE`).
			WithArgs(tariffID).
			WillReturnError(pgx.ErrNoRows)

		tariff, err := repo.GetTariffByID(context.Background(), tariffID)
		require.NoError(t, err) // pgx.ErrNoRows should be translated to nil, nil by the repo method
		assert.Nil(t, tariff)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		dbErr := errors.New("database error")
		mockPool.ExpectQuery(`SELECT id, name, price_per_sms, currency, is_active, description, created_at, updated_at FROM tariffs WHERE id = \$1 AND is_active = TRUE`).
			WithArgs(tariffID).
			WillReturnError(dbErr)

		tariff, err := repo.GetTariffByID(context.Background(), tariffID)
		require.Error(t, err)
		assert.Nil(t, tariff)
		assert.Contains(t, err.Error(), dbErr.Error())
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})
}

func TestPgTariffRepository_GetActiveUserTariff(t *testing.T) {
	repo, mockPool := setupTariffTest(t)
	defer mockPool.Close()

	userID := uuid.New()
	expectedTariff := &domain.Tariff{ID: uuid.New(), Name: "User Specific Tariff", PricePerSMS: 120, Currency: "USD", IsActive: true}

	query := `SELECT t.id, t.name, t.price_per_sms, t.currency, t.is_active, t.description, t.created_at, t.updated_at
	          FROM tariffs t
	          JOIN user_tariffs ut ON t.id = ut.tariff_id
	          WHERE ut.user_id = \$1 AND t.is_active = TRUE`

	t.Run("Found", func(t *testing.T) {
		rows := mockPool.NewRows([]string{"id", "name", "price_per_sms", "currency", "is_active", "description", "created_at", "updated_at"}).
			AddRow(expectedTariff.ID, expectedTariff.Name, expectedTariff.PricePerSMS, expectedTariff.Currency, expectedTariff.IsActive, expectedTariff.Description, time.Now(), time.Now())

		mockPool.ExpectQuery(query).WithArgs(userID).WillReturnRows(rows)

		tariff, err := repo.GetActiveUserTariff(context.Background(), userID)
		require.NoError(t, err)
		require.NotNil(t, tariff)
		assert.Equal(t, expectedTariff.Name, tariff.Name)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("NotFound", func(t *testing.T) {
		mockPool.ExpectQuery(query).WithArgs(userID).WillReturnError(pgx.ErrNoRows)
		tariff, err := repo.GetActiveUserTariff(context.Background(), userID)
		require.NoError(t, err)
		assert.Nil(t, tariff)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		dbErr := errors.New("db error on user tariff")
		mockPool.ExpectQuery(query).WithArgs(userID).WillReturnError(dbErr)
		tariff, err := repo.GetActiveUserTariff(context.Background(), userID)
		require.Error(t, err)
		assert.Nil(t, tariff)
		assert.Contains(t, err.Error(), dbErr.Error())
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})
}

func TestPgTariffRepository_GetDefaultActiveTariff(t *testing.T) {
	repo, mockPool := setupTariffTest(t)
	defer mockPool.Close()

	expectedTariff := &domain.Tariff{ID: uuid.New(), Name: "Default", PricePerSMS: 150, Currency: "USD", IsActive: true}
	query := `SELECT id, name, price_per_sms, currency, is_active, description, created_at, updated_at
	          FROM tariffs WHERE name = \$1 AND is_active = TRUE LIMIT 1`

	t.Run("Found", func(t *testing.T) {
		rows := mockPool.NewRows([]string{"id", "name", "price_per_sms", "currency", "is_active", "description", "created_at", "updated_at"}).
			AddRow(expectedTariff.ID, expectedTariff.Name, expectedTariff.PricePerSMS, expectedTariff.Currency, expectedTariff.IsActive, expectedTariff.Description, time.Now(), time.Now())

		mockPool.ExpectQuery(query).WithArgs("Default").WillReturnRows(rows)

		tariff, err := repo.GetDefaultActiveTariff(context.Background())
		require.NoError(t, err)
		require.NotNil(t, tariff)
		assert.Equal(t, "Default", tariff.Name)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("NotFound", func(t *testing.T) {
		mockPool.ExpectQuery(query).WithArgs("Default").WillReturnError(pgx.ErrNoRows)
		tariff, err := repo.GetDefaultActiveTariff(context.Background())
		require.NoError(t, err)
		assert.Nil(t, tariff)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		dbErr := errors.New("db error on default tariff")
		mockPool.ExpectQuery(query).WithArgs("Default").WillReturnError(dbErr)
		tariff, err := repo.GetDefaultActiveTariff(context.Background())
		require.Error(t, err)
		assert.Nil(t, tariff)
		assert.Contains(t, err.Error(), dbErr.Error())
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})
}
