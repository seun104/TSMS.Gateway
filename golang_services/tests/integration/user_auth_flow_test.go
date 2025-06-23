package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AradIT/aradsms/golang_services/internal/public_api_service/transport/http/dto" // Adjusted path
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	defaultPublicApiServiceURL = "http://localhost:8080"
	defaultPostgresDSN         = "postgres://smsuser:smspassword@localhost:5432/sms_gateway_db?sslmode=disable"
)

// getEnv reads an environment variable or returns a fallback value.
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// checkUserExists checks if a user exists in the database by username or email.
func checkUserExists(ctx context.Context, dbPool *pgxpool.Pool, username string, email string) (bool, string, error) {
	var userID string
	query := "SELECT id FROM users WHERE username = $1 OR email = $2"
	err := dbPool.QueryRow(ctx, query, username, email).Scan(&userID)
	if err != nil {
		if strings.Contains(err.Error(), "no rows in result set") { // pgx.ErrNoRows is not directly exported by pgxpool for ErrNoRows
			return false, "", nil
		}
		return false, "", fmt.Errorf("error querying user: %w", err)
	}
	return true, userID, nil
}

func TestUserRegistrationLoginAndProfile_Success(t *testing.T) {
	if os.Getenv("INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration tests: INTEGRATION_TESTS env var not set.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second) // Increased timeout for full flow
	defer cancel()

	publicAPIURL := getEnv("PUBLIC_API_URL", defaultPublicApiServiceURL)
	postgresDSN := getEnv("POSTGRES_DSN", defaultPostgresDSN)

	// a. Setup DB Connection (for verification and cleanup)
	dbPool, err := pgxpool.New(ctx, postgresDSN)
	require.NoError(t, err, "Failed to connect to PostgreSQL database")
	defer dbPool.Close()

	// b. Generate Unique User Data
	uniqueSuffix := uuid.New().String()
	username := "integ_user_" + uniqueSuffix[:8]
	email := "integ_user_" + uniqueSuffix[:8] + "@example.com"
	password := "Password123!"

	// Ensure user does not exist before registration (optional cleanup from previous failed runs)
	// For CI, a clean DB state is preferred. Locally, this helps.
	var initialUserID string
	exists, initialUserID, err := checkUserExists(ctx, dbPool, username, email)
	require.NoError(t, err, "Pre-check for user existence failed")
	if exists {
		_, err = dbPool.Exec(ctx, "DELETE FROM users WHERE id = $1", initialUserID)
		t.Logf("Pre-existing test user %s/%s (ID: %s) deleted before test run.", username, email, initialUserID)
		require.NoError(t, err, "Failed to delete pre-existing test user")
	}


	// c. Register User
	registerPayload := dto.RegisterRequest{
		Username:    username,
		Email:       email,
		Password:    password,
		PhoneNumber: "1234567890", // Optional, but good to include
	}
	registerPayloadBytes, err := json.Marshal(registerPayload)
	require.NoError(t, err, "Failed to marshal register request")

	registerReq, err := http.NewRequestWithContext(ctx, "POST", publicAPIURL+"/api/v1/auth/register", bytes.NewBuffer(registerPayloadBytes))
	require.NoError(t, err, "Failed to create register request")
	registerReq.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 15 * time.Second}
	registerResp, err := httpClient.Do(registerReq)
	require.NoError(t, err, "Failed to send register request")
	defer registerResp.Body.Close()

	require.Equal(t, http.StatusCreated, registerResp.StatusCode, "Expected HTTP 201 Created for registration")
	t.Logf("User %s registered successfully.", username)

	// d. Verify User in DB
	var createdUserID string // To store the ID for potential cleanup
	userNowExists, createdUserID, err := checkUserExists(ctx, dbPool, username, email)
	require.NoError(t, err, "Checking user existence after registration failed")
	assert.True(t, userNowExists, "User should exist in DB after registration")
	t.Logf("User %s verified in DB with ID: %s.", username, createdUserID)

	// Optional: Defer cleanup of the created user
	if createdUserID != "" {
		defer func() {
			_, delErr := dbPool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", createdUserID)
			if delErr != nil {
				t.Logf("Failed to clean up user %s (ID: %s): %v", username, createdUserID, delErr)
			} else {
				t.Logf("Cleaned up user %s (ID: %s)", username, createdUserID)
			}
		}()
	}


	// e.Login User
	loginPayload := dto.LoginRequest{
		Username: username,
		Password: password,
	}
	loginPayloadBytes, err := json.Marshal(loginPayload)
	require.NoError(t, err, "Failed to marshal login request")

	loginReq, err := http.NewRequestWithContext(ctx, "POST", publicAPIURL+"/api/v1/auth/login", bytes.NewBuffer(loginPayloadBytes))
	require.NoError(t, err, "Failed to create login request")
	loginReq.Header.Set("Content-Type", "application/json")

	loginResp, err := httpClient.Do(loginReq)
	require.NoError(t, err, "Failed to send login request")
	defer loginResp.Body.Close()

	require.Equal(t, http.StatusOK, loginResp.StatusCode, "Expected HTTP 200 OK for login")

	var loginResponse dto.LoginResponse
	err = json.NewDecoder(loginResp.Body).Decode(&loginResponse)
	require.NoError(t, err, "Failed to decode login response")
	assert.NotEmpty(t, loginResponse.AccessToken, "Access token should not be empty")
	assert.NotEmpty(t, loginResponse.RefreshToken, "Refresh token should not be empty")
	t.Logf("User %s logged in successfully. Access token received.", username)


	// f. Access Protected Route (/users/me)
	profileReq, err := http.NewRequestWithContext(ctx, "GET", publicAPIURL+"/api/v1/users/me", nil)
	require.NoError(t, err, "Failed to create /users/me request")
	profileReq.Header.Set("Authorization", "Bearer "+loginResponse.AccessToken)

	profileResp, err := httpClient.Do(profileReq)
	require.NoError(t, err, "Failed to send /users/me request")
	defer profileResp.Body.Close()

	require.Equal(t, http.StatusOK, profileResp.StatusCode, "Expected HTTP 200 OK for /users/me")

	var userProfile dto.UserProfileResponse
	err = json.NewDecoder(profileResp.Body).Decode(&userProfile)
	require.NoError(t, err, "Failed to decode /users/me response")

	assert.Equal(t, createdUserID, userProfile.ID, "User ID in profile should match created user ID")
	assert.Equal(t, username, userProfile.Username, "Username in profile should match registered username")
	assert.Equal(t, email, userProfile.Email, "Email in profile should match registered email")
	// Assert other fields as necessary, e.g., IsActive should likely be true by default
	assert.True(t, userProfile.IsActive, "User should be active by default")

	t.Logf("Successfully fetched user profile for %s.", username)
}

// Note on DTOs:
// The test assumes that DTOs from `github.com/AradIT/aradsms/golang_services/internal/public_api_service/transport/http/dto`
// are accessible. If they are in an `internal` sub-package of `public_api_service` that `tests/integration`
// cannot import, these DTO struct definitions would need to be duplicated in the test file
// or moved to a shared location. For this implementation, direct import is assumed.
// The UserProfileResponse fields like RoleID, Permissions are not asserted as their JSON tags might be missing or values not set in this basic flow.
```

**Correction during thought process:**
*   The DTO import path in the generated code was `github.com/AradIT/aradsms/golang_services/internal/public_api_service/transport/http/dto`. This is good.
*   The `checkUserExists` function's error handling for "no rows" was adapted to check for string containment as `pgx.ErrNoRows` is not directly exported by `pgxpool` in a way that `errors.Is` works easily across driver versions or without importing `github.com/jackc/pgx/v5`.
*   Added pre-check and deferred cleanup for the user to make tests more robust when run locally multiple times. In a CI environment with a fresh DB for each run, this is less critical but good practice.
*   Asserted `userProfile.IsActive` as it's a common default.

The test covers the full E2E flow for user registration, login, and fetching a protected profile.
