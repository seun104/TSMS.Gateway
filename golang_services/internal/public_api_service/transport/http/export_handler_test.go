package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	adapter_http "github.com/AradIT/aradsms/golang_services/internal/public_api_service/transport/http"
	exportDomain "github.com/AradIT/aradsms/golang_services/internal/export_service/domain"
	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker"
	"github.com/AradIT/aradsms/golang_services/internal/public_api_service/middleware"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockNATSClient for testing ExportHandler
type MockNATSClient struct {
	mock.Mock
}

func (m *MockNATSClient) Publish(ctx context.Context, subject string, data []byte) error {
	args := m.Called(ctx, subject, data)
	return args.Error(0)
}
func (m *MockNATSClient) Subscribe(ctx context.Context, subject string, queueGroup string, handler func(msg messagebroker.Message)) (messagebroker.Subscription, error) {
	args := m.Called(ctx, subject, queueGroup, handler)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(messagebroker.Subscription), args.Error(1)
}
func (m *MockNATSClient) Close() {
	m.Called()
}


func TestExportHandler_RequestExportOutboxMessages_Success(t *testing.T) {
	mockNatsClient := new(MockNATSClient)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	validate := validator.New()
	handler := adapter_http.NewExportHandler(mockNatsClient, logger, validate)

	userID := uuid.New()
	userEmail := "user@example.com"
	authDetails := &middleware.AuthenticatedUser{UserID: userID.String(), Email: userEmail, IsAdmin: false} // Ensure pointer type

	reqDTO := adapter_http.CreateExportRequestDTO{
		EntityType: "outbox_messages",
		Filters:    map[string]string{"status": "delivered"},
	}
	payloadBytes, _ := json.Marshal(reqDTO)

	req := httptest.NewRequest(http.MethodPost, "/v1/exports/outbox-messages", bytes.NewBuffer(payloadBytes))
	// Add auth details to context
	ctx := context.WithValue(req.Context(), middleware.AuthenticatedUserContextKey, authDetails)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	// Expect NATS publish
	mockNatsClient.On("Publish", mock.Anything, exportDomain.NATSExportRequestOutboxV1, mock.MatchedBy(func(data []byte) bool {
		var event exportDomain.ExportRequestEvent
		json.Unmarshal(data, &event)
		return event.UserID == userID &&
		       event.RequesterEmail == userEmail &&
			   event.Filters["status"] == "delivered"
	})).Return(nil).Once()

	handler.RequestExportOutboxMessages(rr, req)

	assert.Equal(t, http.StatusAccepted, rr.Code)
	var respBody map[string]string
	err := json.Unmarshal(rr.Body.Bytes(), &respBody)
	require.NoError(t, err)
	assert.Equal(t, "Export request accepted and is being processed.", respBody["message"])
	mockNatsClient.AssertExpectations(t)
}

func TestExportHandler_RequestExportOutboxMessages_Unauthorized(t *testing.T) {
	mockNatsClient := new(MockNATSClient) // Not called
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	validate := validator.New()
	handler := adapter_http.NewExportHandler(mockNatsClient, logger, validate)

	reqDTO := adapter_http.CreateExportRequestDTO{EntityType: "outbox_messages"}
	payloadBytes, _ := json.Marshal(reqDTO)
	req := httptest.NewRequest(http.MethodPost, "/v1/exports/outbox-messages", bytes.NewBuffer(payloadBytes))
	// No auth details in context
	rr := httptest.NewRecorder()

	handler.RequestExportOutboxMessages(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestExportHandler_RequestExportOutboxMessages_InvalidDTO(t *testing.T) {
	mockNatsClient := new(MockNATSClient)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	validate := validator.New()
	handler := adapter_http.NewExportHandler(mockNatsClient, logger, validate)

	authDetails := &middleware.AuthenticatedUser{UserID: uuid.NewString(), Email: "user@example.com"}
	ctx := context.WithValue(context.Background(), middleware.AuthenticatedUserContextKey, authDetails)

	// Invalid: EntityType is empty, but required
	reqDTO := adapter_http.CreateExportRequestDTO{EntityType: ""}
	payloadBytes, _ := json.Marshal(reqDTO)
	req := httptest.NewRequest(http.MethodPost, "/v1/exports/outbox-messages", bytes.NewBuffer(payloadBytes))
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.RequestExportOutboxMessages(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "Validation failed")
}

func TestExportHandler_RequestExportOutboxMessages_NatsPublishError(t *testing.T) {
	mockNatsClient := new(MockNATSClient)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	validate := validator.New()
	handler := adapter_http.NewExportHandler(mockNatsClient, logger, validate)

	userID := uuid.New()
	authDetails := &middleware.AuthenticatedUser{UserID: userID.String(), Email: "user@example.com"}
	ctx := context.WithValue(context.Background(), middleware.AuthenticatedUserContextKey, authDetails)

	reqDTO := adapter_http.CreateExportRequestDTO{EntityType: "outbox_messages"}
	payloadBytes, _ := json.Marshal(reqDTO)
	req := httptest.NewRequest(http.MethodPost, "/v1/exports/outbox-messages", bytes.NewBuffer(payloadBytes))
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	natsError := errors.New("NATS publish failed")
	mockNatsClient.On("Publish", mock.Anything, exportDomain.NATSExportRequestOutboxV1, mock.Anything).Return(natsError).Once()

	handler.RequestExportOutboxMessages(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "Failed to request export")
	mockNatsClient.AssertExpectations(t)
}
