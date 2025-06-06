package http_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	// Use an alias for the app package to avoid conflict if this test package was 'app_test'
	app_billing "github.com/AradIT/aradsms/golang_services/internal/billing_service/app"
	adapter_http "github.com/AradIT/aradsms/golang_services/internal/billing_service/adapters/http"
)

// MockPaymentWebhookProcessor provides a mock implementation of the PaymentWebhookProcessor interface.
type MockPaymentWebhookProcessor struct {
	mock.Mock
}

func (m *MockPaymentWebhookProcessor) HandlePaymentWebhook(ctx context.Context, rawPayload []byte, signature string) error {
	args := m.Called(ctx, rawPayload, signature)
	return args.Error(0)
}

func TestWebhookHandler_HandlePaymentWebhook_Success(t *testing.T) {
	mockAppService := new(MockPaymentWebhookProcessor)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := adapter_http.NewWebhookHandler(mockAppService, logger)

	payload := []byte(`{"event_type":"payment_intent.succeeded"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/payments", bytes.NewBuffer(payload))
	req.Header.Set("X-Payment-Signature", "valid_signature")
	rr := httptest.NewRecorder()

	mockAppService.On("HandlePaymentWebhook", mock.Anything, payload, "valid_signature").Return(nil).Once()

	handler.HandlePaymentWebhook(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "Webhook received successfully", rr.Body.String())
	mockAppService.AssertExpectations(t)
}

func TestWebhookHandler_HandlePaymentWebhook_MethodNotAllowed(t *testing.T) {
	mockAppService := new(MockPaymentWebhookProcessor) // Not called
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := adapter_http.NewWebhookHandler(mockAppService, logger)

	req := httptest.NewRequest(http.MethodGet, "/webhooks/payments", nil)
	rr := httptest.NewRecorder()

	handler.HandlePaymentWebhook(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	assert.Contains(t, rr.Body.String(), "Method not allowed")
}

func TestWebhookHandler_HandlePaymentWebhook_BodyTooLarge(t *testing.T) {
	mockAppService := new(MockPaymentWebhookProcessor) // Not called
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := adapter_http.NewWebhookHandler(mockAppService, logger)

	largePayload := make([]byte, adapter_http.MaxRequestBodySize+1)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/payments", bytes.NewBuffer(largePayload))
	rr := httptest.NewRecorder()

	handler.HandlePaymentWebhook(rr, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
	assert.Contains(t, rr.Body.String(), "Request body too large")
}

type errorReader struct{}

func (er *errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("simulated read error")
}

func TestWebhookHandler_HandlePaymentWebhook_ErrorReadingBody(t *testing.T) {
	mockAppService := new(MockPaymentWebhookProcessor) // Not called
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := adapter_http.NewWebhookHandler(mockAppService, logger)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/payments", &errorReader{})
	rr := httptest.NewRecorder()

	handler.HandlePaymentWebhook(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "Error reading request body")
}


func TestWebhookHandler_HandlePaymentWebhook_AppServiceError_Signature(t *testing.T) {
	mockAppService := new(MockPaymentWebhookProcessor)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := adapter_http.NewWebhookHandler(mockAppService, logger)

	payload := []byte(`{"event_type":"payment_intent.failed"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/payments", bytes.NewBuffer(payload))
	req.Header.Set("X-Payment-Signature", "invalid_signature")
	rr := httptest.NewRecorder()

	// Mock app service to return a specific error that the handler should map to 400
	appServiceError := errors.New("mock gateway webhook signature verification failed")
	mockAppService.On("HandlePaymentWebhook", mock.Anything, payload, "invalid_signature").Return(appServiceError).Once()

	handler.HandlePaymentWebhook(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "Webhook signature verification failed")
	mockAppService.AssertExpectations(t)
}

func TestWebhookHandler_HandlePaymentWebhook_AppServiceError_Internal(t *testing.T) {
	mockAppService := new(MockPaymentWebhookProcessor)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := adapter_http.NewWebhookHandler(mockAppService, logger)

	payload := []byte(`{"event_type":"payment_intent.succeeded"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/payments", bytes.NewBuffer(payload))
	req.Header.Set("X-Payment-Signature", "valid_signature")
	rr := httptest.NewRecorder()

	appServiceError := errors.New("some internal processing error")
	mockAppService.On("HandlePaymentWebhook", mock.Anything, payload, "valid_signature").Return(appServiceError).Once()

	handler.HandlePaymentWebhook(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "Internal server error processing webhook")
	mockAppService.AssertExpectations(t)
}
