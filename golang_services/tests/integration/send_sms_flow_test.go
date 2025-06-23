package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// Assuming public_api_service is mapped to port 8080 in docker-compose
	publicApiServiceURLDefault = "http://localhost:8080"
	// Standard DSN for connecting to the Dockerized PostgreSQL
	postgresDSNDefault = "postgres://smsuser:smspassword@localhost:5432/sms_gateway_db?sslmode=disable"

	// Expected message statuses (these should match your domain.MessageStatus values)
	statusQueued             = "queued"              // Initial status after Public API receives it
	statusSubmittedToProvider = "submitted_to_provider" // Status after SMS Sending Service (mock provider) processes it
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// Helper function to get message status from the database
func getOutboxMessageStatus(ctx context.Context, dbPool *pgxpool.Pool, messageID string) (string, error) {
	var status string
	// Assuming message_id in the table is compatible with string comparison (e.g., TEXT, VARCHAR, or UUID that pgx handles)
	err := dbPool.QueryRow(ctx, "SELECT status FROM outbox_messages WHERE message_id = $1", messageID).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("message with ID '%s' not found: %w", messageID, err)
		}
		return "", fmt.Errorf("error querying message status for ID '%s': %w", messageID, err)
	}
	return status, nil
}

// TestSendSMSFlow_Success verifies the end-to-end flow of sending an SMS
// from Public API, through NATS, processed by SMS Sending Service, and status updated in DB.
func TestSendSMSFlow_Success(t *testing.T) {
	if os.Getenv("INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration tests: INTEGRATION_TESTS env var not set.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // Overall test timeout
	defer cancel()

	publicApiURL := getEnv("PUBLIC_API_URL", publicApiServiceURLDefault)
	postgresDSN := getEnv("POSTGRES_DSN", postgresDSNDefault)

	// a. Setup DB Connection
	dbPool, err := pgxpool.New(ctx, postgresDSN)
	require.NoError(t, err, "Failed to connect to PostgreSQL database")
	defer dbPool.Close()

	// b. Prepare HTTP Request
	payload := map[string]string{
		"recipient": "1234567890",                                 // Using a dummy recipient
		"message":   "Test SMS via integration test - " + time.Now().String(), // Unique message
		"sender_id": "TestSender",                                 // Assuming this is accepted or mocked
	}
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err, "Failed to marshal JSON payload")

	req, err := http.NewRequestWithContext(ctx, "POST", publicApiURL+"/api/v1/messages/send", bytes.NewBuffer(payloadBytes))
	require.NoError(t, err, "Failed to create HTTP request")
	req.Header.Set("Content-Type", "application/json")
	// TODO: Add Authorization header if authentication is enforced by public_api_service
	// For now, assuming no auth or a test setup where auth is bypassed/mocked for this endpoint.
	// Example: req.Header.Set("Authorization", "Bearer your_test_token")

	// c. Send HTTP Request
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	require.NoError(t, err, "Failed to send HTTP request to Public API")
	defer resp.Body.Close()

	require.Contains(t, []int{http.StatusOK, http.StatusAccepted}, resp.StatusCode, "Expected OK or Accepted status from Public API")

	var apiResponse map[string]interface{} // More specific struct could be used
	err = json.NewDecoder(resp.Body).Decode(&apiResponse)
	require.NoError(t, err, "Failed to decode JSON response from Public API")

	messageID, ok := apiResponse["message_id"].(string)
	require.True(t, ok, "Response from Public API did not contain a string message_id")
	require.NotEmpty(t, messageID, "message_id from Public API is empty")
	t.Logf("Received message_id: %s", messageID)

	// d. Verify Initial DB Record (Queued by Public API)
	initialStatus, err := getOutboxMessageStatus(ctx, dbPool, messageID)
	require.NoError(t, err, "Failed to get initial message status from DB")
	assert.Equal(t, statusQueued, initialStatus, "Initial message status should be '%s'", statusQueued)
	t.Logf("Initial message status: %s", initialStatus)

	// e. Wait for Processing by sms_sending_service (using polling)
	var finalStatus string
	var pollError error
	processed := false
	pollingDuration := 20 * time.Second // Max time to wait for processing
	pollInterval := 1 * time.Second    // Interval between checks

	t.Logf("Polling for final status for up to %s...", pollingDuration)
	for i := 0; i < int(pollingDuration/pollInterval); i++ {
		select {
		case <-ctx.Done(): // Test timeout exceeded
			t.Fatalf("Test context timed out while polling for final status: %v", ctx.Err())
			return
		default:
		}

		finalStatus, pollError = getOutboxMessageStatus(ctx, dbPool, messageID)
		if pollError != nil {
			// Log error but continue polling as message might not exist yet on first few tries if processing is very fast
			t.Logf("Polling: error getting status (try %d): %v", i+1, pollError)
		} else if finalStatus == statusSubmittedToProvider {
			processed = true
			break
		}
		time.Sleep(pollInterval)
	}

	// f. Verify Final DB Record
	require.NoError(t, pollError, "Error during final poll for message status") // Check error from the last poll attempt
	require.True(t, processed, "Message did not reach expected status '%s' in time. Last status: '%s'", statusSubmittedToProvider, finalStatus)
	assert.Equal(t, statusSubmittedToProvider, finalStatus, "Final message status should be '%s'", statusSubmittedToProvider)
	t.Logf("Final message status: %s", finalStatus)

	t.Log("Send SMS flow test completed successfully.")
}
