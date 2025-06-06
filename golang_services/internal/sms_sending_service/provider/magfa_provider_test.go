package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMagfaSMSProvider_GetName(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	provider := NewMagfaSMSProvider(logger, "url", "key", "sender", nil)
	assert.Equal(t, "magfa", provider.GetName())
}

func TestMagfaSMSProvider_Send_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		authHeader := r.Header.Get("Authorization")
		assert.Equal(t, "Bearer test-api-key", authHeader)
		contentType := r.Header.Get("Content-Type")
		assert.Equal(t, "application/json", contentType)

		bodyBytes, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var reqBody MagfaSendRequestBody
		err = json.Unmarshal(bodyBytes, &reqBody)
		require.NoError(t, err)
		assert.Len(t, reqBody.Messages, 1)
		assert.Equal(t, "test-sender-id", reqBody.Messages[0].Sender)
		assert.Equal(t, "Hello Magfa", reqBody.Messages[0].Body)
		assert.Contains(t, reqBody.Messages[0].Recipients, "1234567890")


		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // Magfa success is often 200 OK
		// Example Magfa success response structure
		successResp := MagfaSendSuccessResponse{
			Messages: []MagfaSentMessageDetail{
				{ID: 12345, Recipient: "1234567890", Status: 0, Message: "Sent"},
			},
			Status: 0, // Overall status
			Message: "Successfully sent",
		}
		json.NewEncoder(w).Encode(successResp)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	provider := NewMagfaSMSProvider(logger, server.URL, "test-api-key", "test-sender-id", server.Client())

	details := SendRequestDetails{
		InternalMessageID: "internal-id-1",
		Recipient:         "1234567890",
		Content:           "Hello Magfa",
		SenderID:          "test-sender-id", // This should match what's in the body if not taken from provider's default
	}

	resp, err := provider.Send(context.Background(), details)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.IsSuccess)
	assert.Equal(t, "12345", resp.ProviderMessageID) // Assuming we format the numeric ID as string
	assert.Contains(t, resp.ProviderStatus, "SENT_MAGFA_200")
	assert.Empty(t, resp.ErrorMessage)
}


func TestMagfaSMSProvider_Send_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest) // Example: 400 Bad Request
		// Example Magfa error response
		errorResp := MagfaErrorResponse{
			Status:  101, // Magfa specific error code
			Message: "Invalid recipient number",
		}
		json.NewEncoder(w).Encode(errorResp)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	provider := NewMagfaSMSProvider(logger, server.URL, "test-api-key", "test-sender-id", server.Client())

	details := SendRequestDetails{Recipient: "invalid", Content: "Test"}
	resp, err := provider.Send(context.Background(), details)

	require.Error(t, err) // Expect an error to be returned from Send method itself
	require.NotNil(t, resp)
	assert.False(t, resp.IsSuccess)
	assert.Contains(t, resp.ProviderStatus, "FAILED_MAGFA_400")
	assert.Contains(t, resp.ErrorMessage, "Invalid recipient number") // Check if Magfa's message is included
	assert.Contains(t, err.Error(), "Invalid recipient number")
}

func TestMagfaSMSProvider_Send_NetworkError(t *testing.T) {
	// Intentionally stop the server to simulate a network error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should not be called
	}))
	server.Close() // Close server immediately

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	provider := NewMagfaSMSProvider(logger, server.URL, "test-api-key", "test-sender-id", nil) // Use default client which will fail

	details := SendRequestDetails{Recipient: "123", Content: "Test"}
	resp, err := provider.Send(context.Background(), details)

	require.Error(t, err)
	assert.Nil(t, resp) // On direct network errors like connection refused, resp might be nil from the provider
	assert.True(t, strings.Contains(err.Error(), "connect: connection refused") || strings.Contains(err.Error(), "context deadline exceeded"))
}

func TestMagfaSMSProvider_Send_NonJSONErrorResponse(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/plain") // Not JSON
        w.WriteHeader(http.StatusInternalServerError)
        fmt.Fprint(w, "An unexpected server error occurred on Magfa side.")
    }))
    defer server.Close()

    logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
    provider := NewMagfaSMSProvider(logger, server.URL, "api-key", "sender", server.Client())

    details := SendRequestDetails{Recipient: "123", Content: "Test"}
    resp, err := provider.Send(context.Background(), details)

    require.Error(t, err)
    require.NotNil(t, resp)
    assert.False(t, resp.IsSuccess)
    assert.Contains(t, resp.ProviderStatus, "FAILED_MAGFA_500")
    assert.Contains(t, resp.ErrorMessage, "raw_body: An unexpected server error occurred on Magfa side.")
}

func TestMagfaSMSProvider_Send_SuccessNonJSONResponse(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/plain") // Not JSON
        w.WriteHeader(http.StatusOK)
        fmt.Fprint(w, "OK") // Simple non-JSON success
    }))
    defer server.Close()

    logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
    provider := NewMagfaSMSProvider(logger, server.URL, "api-key", "sender", server.Client())

    details := SendRequestDetails{Recipient: "123", Content: "Test"}
    resp, err := provider.Send(context.Background(), details)

    require.NoError(t, err) // HTTP 200 is success, even if response body is not as expected
    require.NotNil(t, resp)
    assert.True(t, resp.IsSuccess)
    assert.Equal(t, "", resp.ProviderMessageID) // No ID could be parsed
    assert.Contains(t, resp.ProviderStatus, "SENT_MAGFA_200_UNPARSED_RESP")
}
