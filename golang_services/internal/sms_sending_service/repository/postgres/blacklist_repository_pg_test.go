package postgres

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/AradIT/aradsms/golang_services/internal/sms_sending_service/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5" // For pgx.ErrNoRows
	"github.com/pashagolub/pgxmock/v3" // Using pgxmock
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPgBlacklistRepository_IsBlacklisted(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	userID := uuid.New()
	validUserID := uuid.NullUUID{UUID: userID, Valid: true}
	invalidUserID := uuid.NullUUID{Valid: false} // Represents global check or user not specified

	phoneNumber := "1234567890"
	reasonText := "Test reason"

	t.Run("UserSpecific_Blacklisted", func(t *testing.T) {
		mockPool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mockPool.Close()

		repo := NewPgBlacklistRepository(mockPool, logger)

		rows := mockPool.NewRows([]string{"reason"}).AddRow(sql.NullString{String: reasonText, Valid: true})
		mockPool.ExpectQuery(`SELECT reason FROM blacklisted_numbers WHERE phone_number = \$1 AND user_id = \$2 LIMIT 1`).
			WithArgs(phoneNumber, userID).
			WillReturnRows(rows)

		isBlacklisted, reason, err := repo.IsBlacklisted(context.Background(), phoneNumber, validUserID)
		assert.NoError(t, err)
		assert.True(t, isBlacklisted)
		assert.Equal(t, reasonText, reason)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("UserSpecific_NotBlacklisted_ThenGlobal_Blacklisted", func(t *testing.T) {
		mockPool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mockPool.Close()
		repo := NewPgBlacklistRepository(mockPool, logger)

		// Expect user-specific query, return no rows
		mockPool.ExpectQuery(`SELECT reason FROM blacklisted_numbers WHERE phone_number = \$1 AND user_id = \$2 LIMIT 1`).
			WithArgs(phoneNumber, userID).
			WillReturnError(pgx.ErrNoRows)

		// Expect global query, return a row
		globalRows := mockPool.NewRows([]string{"reason"}).AddRow(sql.NullString{String: "Global reason", Valid: true})
		mockPool.ExpectQuery(`SELECT reason FROM blacklisted_numbers WHERE phone_number = \$1 AND user_id IS NULL LIMIT 1`).
			WithArgs(phoneNumber).
			WillReturnRows(globalRows)

		isBlacklisted, reason, err := repo.IsBlacklisted(context.Background(), phoneNumber, validUserID)
		assert.NoError(t, err)
		assert.True(t, isBlacklisted)
		assert.Equal(t, "Global reason", reason)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("GlobalOnly_Blacklisted", func(t *testing.T) {
		mockPool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mockPool.Close()
		repo := NewPgBlacklistRepository(mockPool, logger)

		// Expect global query (since UserID.Valid is false), return a row
		globalRows := mockPool.NewRows([]string{"reason"}).AddRow(sql.NullString{String: "Global only reason", Valid: true})
		mockPool.ExpectQuery(`SELECT reason FROM blacklisted_numbers WHERE phone_number = \$1 AND user_id IS NULL LIMIT 1`).
			WithArgs(phoneNumber).
			WillReturnRows(globalRows)

		isBlacklisted, reason, err := repo.IsBlacklisted(context.Background(), phoneNumber, invalidUserID)
		assert.NoError(t, err)
		assert.True(t, isBlacklisted)
		assert.Equal(t, "Global only reason", reason)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("NotBlacklisted_UserSpecific_And_Global", func(t *testing.T) {
		mockPool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mockPool.Close()
		repo := NewPgBlacklistRepository(mockPool, logger)

		mockPool.ExpectQuery(`SELECT reason FROM blacklisted_numbers WHERE phone_number = \$1 AND user_id = \$2 LIMIT 1`).
			WithArgs(phoneNumber, userID).
			WillReturnError(pgx.ErrNoRows)

		mockPool.ExpectQuery(`SELECT reason FROM blacklisted_numbers WHERE phone_number = \$1 AND user_id IS NULL LIMIT 1`).
			WithArgs(phoneNumber).
			WillReturnError(pgx.ErrNoRows)

		isBlacklisted, reason, err := repo.IsBlacklisted(context.Background(), phoneNumber, validUserID)
		assert.NoError(t, err)
		assert.False(t, isBlacklisted)
		assert.Equal(t, "", reason)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("NotBlacklisted_GlobalOnlyCheck", func(t *testing.T) {
		mockPool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mockPool.Close()
		repo := NewPgBlacklistRepository(mockPool, logger)

		mockPool.ExpectQuery(`SELECT reason FROM blacklisted_numbers WHERE phone_number = \$1 AND user_id IS NULL LIMIT 1`).
			WithArgs(phoneNumber).
			WillReturnError(pgx.ErrNoRows)

		isBlacklisted, reason, err := repo.IsBlacklisted(context.Background(), phoneNumber, invalidUserID)
		assert.NoError(t, err)
		assert.False(t, isBlacklisted)
		assert.Equal(t, "", reason)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("QueryError_UserSpecific", func(t *testing.T) {
		mockPool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mockPool.Close()
		repo := NewPgBlacklistRepository(mockPool, logger)

		expectedError := errors.New("DB error")
		mockPool.ExpectQuery(`SELECT reason FROM blacklisted_numbers WHERE phone_number = \$1 AND user_id = \$2 LIMIT 1`).
			WithArgs(phoneNumber, userID).
			WillReturnError(expectedError)

		isBlacklisted, reason, err := repo.IsBlacklisted(context.Background(), phoneNumber, validUserID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), expectedError.Error())
		assert.False(t, isBlacklisted)
		assert.Equal(t, "", reason)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("QueryError_Global", func(t *testing.T) {
		mockPool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mockPool.Close()
		repo := NewPgBlacklistRepository(mockPool, logger)

		expectedError := errors.New("DB error global")
		mockPool.ExpectQuery(`SELECT reason FROM blacklisted_numbers WHERE phone_number = \$1 AND user_id IS NULL LIMIT 1`).
			WithArgs(phoneNumber).
			WillReturnError(expectedError)

		isBlacklisted, reason, err := repo.IsBlacklisted(context.Background(), phoneNumber, invalidUserID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), expectedError.Error())
		assert.False(t, isBlacklisted)
		assert.Equal(t, "", reason)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})
}
