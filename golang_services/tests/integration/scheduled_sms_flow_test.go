package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AradIT/aradsms/golang_services/internal/public_api_service/transport/http/dto" // For auth DTOs
	"github.com/AradIT/aradsms/golang_services/internal/public_api_service/transport/http/scheduler_dtos"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5" // For pgx.ErrNoRows check
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb" // For converting time.Time to timestamppb.Timestamp
)

// Constants from user_auth_flow_test.go (or a shared test util)
// const (
// 	defaultPublicApiServiceURL = "http://localhost:8080"
// 	defaultPostgresDSN         = "postgres://smsuser:smspassword@localhost:5432/sms_gateway_db?sslmode=disable"
// )

// Expected statuses (should match domain values)
const (
	statusScheduled           = "scheduled"            // Initial status in outbox_messages for a scheduled SMS
	statusPending             = "pending"              // Initial status in scheduled_jobs
	statusProcessed           = "processed"            // Status in scheduled_jobs after scheduler picks it up
	statusSubmittedToProvider = "submitted_to_provider" // Final status in outbox_messages after sms_sending_service
)

// getEnv (copied for standalone execution if needed)
// func getEnv(key, fallback string) string { ... }


// registerAndLoginUserIntegrationTest (copied/adapted from user_auth_flow_test.go)
func registerAndLoginUserIntegrationTest_ScheduledSMS(t *testing.T, ctx context.Context, uniqueSuffix string) (accessToken string, userID string) {
	publicAPIURL := getEnv("PUBLIC_API_URL", defaultPublicApiServiceURL)
	username := "scheduled_user_" + uniqueSuffix
	email := "scheduled_" + uniqueSuffix + "@example.com"
	password := "Password123!"

	registerPayload := dto.RegisterRequest{Username: username, Email: email, Password: password}
	regPayloadBytes, err := json.Marshal(registerPayload)
	require.NoError(t, err)
	regReq, err := http.NewRequestWithContext(ctx, "POST", publicAPIURL+"/api/v1/auth/register", bytes.NewBuffer(regPayloadBytes))
	require.NoError(t, err)
	regReq.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	regResp, err := httpClient.Do(regReq)
	require.NoError(t, err)
	defer regResp.Body.Close()
	if regResp.StatusCode != http.StatusCreated && regResp.StatusCode != http.StatusConflict { // Allow conflict if user already exists from previous failed run
		require.Equal(t, http.StatusCreated, regResp.StatusCode, "Registration failed")
	}

	loginPayload := dto.LoginRequest{Username: username, Password: password}
	logPayloadBytes, err := json.Marshal(loginPayload)
	require.NoError(t, err)
	logReq, err := http.NewRequestWithContext(ctx, "POST", publicAPIURL+"/api/v1/auth/login", bytes.NewBuffer(logPayloadBytes))
	require.NoError(t, err)
	logReq.Header.Set("Content-Type", "application/json")

	logResp, err := httpClient.Do(logReq)
	require.NoError(t, err)
	defer logResp.Body.Close()
	require.Equal(t, http.StatusOK, logResp.StatusCode, "Login failed")

	var loginResponse dto.LoginResponse
	err = json.NewDecoder(logResp.Body).Decode(&loginResponse)
	require.NoError(t, err)
	return loginResponse.AccessToken, loginResponse.UserID
}

// makeAuthenticatedRequest (copied/adapted from phonebook_flow_test.go)
func makeAuthenticatedRequest_ScheduledSMS(t *testing.T, ctx context.Context, method, url, accessToken string, body io.Reader) *http.Response {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	return resp
}

// getScheduledJobStatus queries the scheduled_jobs table for the status and outbox_message_id.
func getScheduledJobStatus(ctx context.Context, dbPool *pgxpool.Pool, jobID string) (status string, outboxMessageID string, err error) {
	query := "SELECT status, outbox_message_id FROM scheduled_jobs WHERE id = $1"
	var outboxMsgID uuid.UUID // Assuming outbox_message_id is UUID in DB
	err = dbPool.QueryRow(ctx, query, jobID).Scan(&status, &outboxMsgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", fmt.Errorf("scheduled job with ID '%s' not found: %w", jobID, err)
		}
		return "", "", fmt.Errorf("error querying scheduled job status for ID '%s': %w", jobID, err)
	}
	return status, outboxMsgID.String(), nil
}

// getOutboxMessageStatus (copied/adapted from send_sms_flow_test.go)
func getOutboxMessageStatus_ScheduledSMS(ctx context.Context, dbPool *pgxpool.Pool, messageID string) (string, error) {
	var status string
	err := dbPool.QueryRow(ctx, "SELECT status FROM outbox_messages WHERE message_id = $1", messageID).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("outbox message with ID '%s' not found: %w", messageID, err)
		}
		return "", fmt.Errorf("error querying outbox message status for ID '%s': %w", messageID, err)
	}
	return status, nil
}


func TestScheduledSMSFlow_Success(t *testing.T) {
	if os.Getenv("INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration tests: INTEGRATION_TESTS env var not set.")
	}

	overallCtx, overallCancel := context.WithTimeout(context.Background(), 90*time.Second) // Overall test timeout
	defer overallCancel()

	publicAPIURL := getEnv("PUBLIC_API_URL", defaultPublicApiServiceURL)
	postgresDSN := getEnv("POSTGRES_DSN", defaultPostgresDSN)
	uniqueSuffix := uuid.New().String()[:8]

	// a. Setup
	dbPool, err := pgxpool.New(overallCtx, postgresDSN)
	require.NoError(t, err, "Failed to connect to PostgreSQL database")
	defer dbPool.Close()

	accessToken, userID := registerAndLoginUserIntegrationTest_ScheduledSMS(t, overallCtx, "scheduled_"+uniqueSuffix)
	require.NotEmpty(t, accessToken)
	require.NotEmpty(t, userID)
	t.Logf("Test user registered and logged in for scheduled SMS. UserID: %s", userID)

	// Defer cleanup of the test user
	defer func() {
		usernameToDelete := "scheduled_user_" + "scheduled_" + uniqueSuffix
		// Use a background context for cleanup in case overallCtx is done.
		_, delErr := dbPool.Exec(context.Background(), "DELETE FROM users WHERE username = $1", usernameToDelete)
		if delErr != nil {
			t.Logf("Failed to clean up test user %s: %v", usernameToDelete, delErr)
		} else {
			t.Logf("Cleaned up test user %s.", usernameToDelete)
		}
	}()


	// b. Create Scheduled SMS
	runAt := time.Now().Add(15 * time.Second) // Schedule for 15 seconds in the future
	createScheduledPayload := scheduler_dtos.CreateScheduledMessageRequest{
		Recipient: "0987654321",
		Message:   "Scheduled SMS Test - " + uniqueSuffix,
		SenderId:  "SchedSender",
		RunAt:     timestamppb.New(runAt),
	}
	payloadBytes, err := json.Marshal(createScheduledPayload)
	require.NoError(t, err)

	resp := makeAuthenticatedRequest_ScheduledSMS(t, overallCtx, "POST", publicAPIURL+"/api/v1/scheduled_messages", accessToken, bytes.NewBuffer(payloadBytes))
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Failed to create scheduled message")

	var createSchedResp scheduler_dtos.ScheduledMessageResponse
	err = json.NewDecoder(resp.Body).Decode(&createSchedResp)
	require.NoError(t, err)
	require.NotEmpty(t, createSchedResp.ID, "Scheduled job ID should not be empty")
	scheduledJobID := createSchedResp.ID
	t.Logf("Scheduled message created. Job ID: %s", scheduledJobID)

	// c. Verify Initial State in DB
	// Give a moment for the transaction in public_api to complete
	time.Sleep(2 * time.Second)

	jobStatus, outboxMessageIDFromJob, err := getScheduledJobStatus(overallCtx, dbPool, scheduledJobID)
	require.NoError(t, err, "Failed to get initial scheduled job status from DB")
	assert.Equal(t, statusPending, jobStatus, "Initial scheduled_jobs status should be '%s'", statusPending)
	require.NotEmpty(t, outboxMessageIDFromJob, "OutboxMessageID from scheduled_jobs should not be empty")
	t.Logf("Scheduled job initial status: %s, OutboxMessageID: %s", jobStatus, outboxMessageIDFromJob)

	outboxStatus, err := getOutboxMessageStatus_ScheduledSMS(overallCtx, dbPool, outboxMessageIDFromJob)
	require.NoError(t, err, "Failed to get initial outbox message status from DB")
	assert.Equal(t, statusScheduled, outboxStatus, "Initial outbox_messages status should be '%s'", statusScheduled)
	t.Logf("Outbox message initial status: %s", outboxStatus)

	// Defer cleanup of scheduled_job and outbox_message
	defer func() {
		_, _ = dbPool.Exec(context.Background(), "DELETE FROM scheduled_jobs WHERE id = $1", scheduledJobID)
		_, _ = dbPool.Exec(context.Background(), "DELETE FROM outbox_messages WHERE message_id = $1", outboxMessageIDFromJob)
		t.Logf("Cleaned up scheduled job %s and outbox message %s", scheduledJobID, outboxMessageIDFromJob)
	}()


	// d. Wait for Scheduled Time & Initial Processing
	// Wait until just after the message should have been processed by scheduler
	waitUntil := runAt.Add(5 * time.Second) // Wait 5s past scheduled time for scheduler to pick up
	t.Logf("Waiting until %s for scheduler to process the job...", waitUntil.Format(time.RFC3339))
	time.Sleep(time.Until(waitUntil))


	// e. Verify Job Processed by Scheduler
	var finalScheduledJobStatus string
	processedByScheduler := false
	for i := 0; i < 10; i++ { // Poll for up to 10 seconds
		select {
		case <-overallCtx.Done():
			t.Fatalf("Test context timed out while polling for scheduled job status: %v", overallCtx.Err())
		default:
		}
		finalScheduledJobStatus, _, err = getScheduledJobStatus(overallCtx, dbPool, scheduledJobID)
		if err == nil && finalScheduledJobStatus == statusProcessed {
			processedByScheduler = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	require.True(t, processedByScheduler, "Scheduled job did not reach status '%s' in time. Last status: '%s'", statusProcessed, finalScheduledJobStatus)
	t.Logf("Scheduled job final status: %s", finalScheduledJobStatus)

	// f. Verify Final Outbox Message Status (Processed by SMS Sending Service)
	var finalOutboxStatus string
	processedBySMSSender := false
	// Wait a bit longer for NATS message to be picked up by sms_sending_service and processed
	for i := 0; i < 20; i++ { // Poll for up to 20 seconds
		select {
		case <-overallCtx.Done():
			t.Fatalf("Test context timed out while polling for outbox message status: %v", overallCtx.Err())
		default:
		}
		finalOutboxStatus, err = getOutboxMessageStatus_ScheduledSMS(overallCtx, dbPool, outboxMessageIDFromJob)
		if err == nil && finalOutboxStatus == statusSubmittedToProvider {
			processedBySMSSender = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	require.True(t, processedBySMSSender, "Outbox message did not reach status '%s' in time. Last status: '%s'", statusSubmittedToProvider, finalOutboxStatus)
	t.Logf("Outbox message final status: %s", finalOutboxStatus)

	t.Log("Scheduled SMS flow test completed successfully.")
}

// Re-define getEnv if not in a shared utility, assuming it's defined in user_auth_flow_test.go
// For this file to be self-contained for tool execution:
// func getEnv(key, fallback string) string {
// 	if value, ok := os.LookupEnv(key); ok {
// 		return value
// 	}
// 	return fallback
// }
```

**Notes during generation:**
*   **Helper Functions:** Re-implemented simplified versions of `registerAndLoginUserIntegrationTest_ScheduledSMS` and `makeAuthenticatedRequest_ScheduledSMS` (suffixed to avoid collision if these files are ever merged into one package, though they are separate here). The `getEnv` function is assumed to be available (e.g., copied from other test files or defined in a shared utility).
*   **Status Constants:** Defined string constants for expected statuses to improve readability.
*   **`getScheduledJobStatus`:** This helper now also returns `outboxMessageID` as it's crucial for linking the scheduled job to the outbox message. Assumed `outbox_message_id` in `scheduled_jobs` is of type UUID.
*   **Scheduling Time:** Scheduled the SMS for 15 seconds in the future to allow time for the test setup and for the scheduler to pick it up within a reasonable polling window. The wait time is set to 20 seconds (15s + 5s buffer).
*   **Polling:** Implemented polling loops for checking both `scheduled_jobs` and `outbox_messages` statuses.
*   **Cleanup:** Added `defer` calls to clean up the created scheduled job, outbox message, and the test user.
*   **Error Handling for `checkUserExists`:** Corrected `pgx.ErrNoRows` check for `pgxpool`.
*   **DTOs:** Using `scheduler_dtos` for requests/responses related to scheduled messages.
*   **Timestamppb:** Imported `google.golang.org/protobuf/types/known/timestamppb` for `timestamppb.New(runAt)`.
*   **Initial DB State Check Delay:** Added a small `time.Sleep(2 * time.Second)` after creating the scheduled SMS to give the `public_api_service` time to commit its transaction before the test queries the DB. This can make tests more robust against very fast test execution environments.

The file is ready to be created.
