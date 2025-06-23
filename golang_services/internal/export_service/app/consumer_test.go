package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	exportDomain "github.com/AradIT/aradsms/golang_services/internal/export_service/domain"
	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockExportService for testing NATSConsumer
type MockExportService struct {
	mock.Mock
}

func (m *MockExportService) ExportOutboxMessagesToCSV(ctx context.Context, userID uuid.UUID, filters map[string]string) (filePath string, err error) {
	args := m.Called(ctx, userID, filters)
	return args.String(0), args.Error(1)
}

// MockNATSClient for testing NATSConsumer
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


func TestNATSConsumer_HandleExportRequest_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockExportService := new(MockExportService)
	mockNatsClient := new(MockNATSClient)

	consumer := NewNATSConsumer(mockExportService, mockNatsClient, logger)

	userID := uuid.New()
	requesterEmail := "test@example.com"
	requestEvent := exportDomain.ExportRequestEvent{
		UserID:         userID,
		Filters:        map[string]string{"date_from": "2023-01-01"},
		RequesterEmail: requesterEmail,
	}
	requestPayload, _ := json.Marshal(requestEvent)
	expectedFilePath := "/tmp/exports/export_file.csv"

	mockExportService.On("ExportOutboxMessagesToCSV", mock.Anything, userID, requestEvent.Filters).
		Return(expectedFilePath, nil).Once()

	mockNatsClient.On("Publish", mock.Anything, exportDomain.NATSExportCompletedOutboxV1, mock.MatchedBy(func(data []byte) bool {
		var completedEvent exportDomain.ExportCompletedEvent
		json.Unmarshal(data, &completedEvent)
		return completedEvent.UserID == userID &&
			completedEvent.FilePath == expectedFilePath &&
			completedEvent.FileName == "export_file.csv" && // filepath.Base(expectedFilePath)
			completedEvent.RequesterEmail == requesterEmail
	})).Return(nil).Once()

	consumer.HandleExportRequest(context.Background(), exportDomain.NATSExportRequestOutboxV1, requestPayload)

	mockExportService.AssertExpectations(t)
	mockNatsClient.AssertExpectations(t)
}

func TestNATSConsumer_HandleExportRequest_ExportServiceError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockExportService := new(MockExportService)
	mockNatsClient := new(MockNATSClient)
	consumer := NewNATSConsumer(mockExportService, mockNatsClient, logger)

	userID := uuid.New()
	requestEvent := exportDomain.ExportRequestEvent{UserID: userID, RequesterEmail: "fail@example.com"}
	requestPayload, _ := json.Marshal(requestEvent)
	exportError := errors.New("failed to generate CSV")

	mockExportService.On("ExportOutboxMessagesToCSV", mock.Anything, userID, requestEvent.Filters).
		Return("", exportError).Once()

	mockNatsClient.On("Publish", mock.Anything, exportDomain.NATSExportFailedOutboxV1, mock.MatchedBy(func(data []byte) bool {
		var failedEvent exportDomain.ExportFailedEvent
		json.Unmarshal(data, &failedEvent)
		return failedEvent.UserID == userID && failedEvent.ErrorMessage == exportError.Error() && failedEvent.RequesterEmail == "fail@example.com"
	})).Return(nil).Once()

	consumer.HandleExportRequest(context.Background(), exportDomain.NATSExportRequestOutboxV1, requestPayload)

	mockExportService.AssertExpectations(t)
	mockNatsClient.AssertExpectations(t)
}

func TestNATSConsumer_HandleExportRequest_NoDataToExport(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockExportService := new(MockExportService)
	mockNatsClient := new(MockNATSClient)
	consumer := NewNATSConsumer(mockExportService, mockNatsClient, logger)

	userID := uuid.New()
	requestEvent := exportDomain.ExportRequestEvent{UserID: userID, RequesterEmail: "nodata@example.com"}
	requestPayload, _ := json.Marshal(requestEvent)

	mockExportService.On("ExportOutboxMessagesToCSV", mock.Anything, userID, requestEvent.Filters).
		Return("", nil).Once() // Empty path, no error signifies no data

	mockNatsClient.On("Publish", mock.Anything, exportDomain.NATSExportFailedOutboxV1, mock.MatchedBy(func(data []byte) bool {
		var failedEvent exportDomain.ExportFailedEvent
		json.Unmarshal(data, &failedEvent)
		return failedEvent.UserID == userID && failedEvent.ErrorMessage == "No data found to export for the given criteria." && failedEvent.RequesterEmail == "nodata@example.com"
	})).Return(nil).Once()

	consumer.HandleExportRequest(context.Background(), exportDomain.NATSExportRequestOutboxV1, requestPayload)

	mockExportService.AssertExpectations(t)
	mockNatsClient.AssertExpectations(t)
}


func TestNATSConsumer_HandleExportRequest_UnmarshalError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockExportService := new(MockExportService) // Not called
	mockNatsClient := new(MockNATSClient)       // Not called for publish
	consumer := NewNATSConsumer(mockExportService, mockNatsClient, logger)

	invalidPayload := []byte(`{"user_id":"not-a-uuid",`) // Malformed JSON

	// No mocks expected to be called for ExportService or NATSClient.Publish
	consumer.HandleExportRequest(context.Background(), exportDomain.NATSExportRequestOutboxV1, invalidPayload)

	mockExportService.AssertNotCalled(t, "ExportOutboxMessagesToCSV", mock.Anything, mock.Anything, mock.Anything)
	mockNatsClient.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything, mock.Anything)
}

func TestNATSConsumer_HandleExportRequest_PublishCompletedError(t *testing.T) {
    logger := slog.New(slog.NewTextHandler(io.Discard, nil))
    mockExportService := new(MockExportService)
    mockNatsClient := new(MockNATSClient)
    consumer := NewNATSConsumer(mockExportService, mockNatsClient, logger)

    userID := uuid.New()
    requestEvent := exportDomain.ExportRequestEvent{UserID: userID}
    requestPayload, _ := json.Marshal(requestEvent)
    expectedFilePath := "/tmp/export.csv"

    mockExportService.On("ExportOutboxMessagesToCSV", mock.Anything, userID, requestEvent.Filters).
        Return(expectedFilePath, nil).Once()
    mockNatsClient.On("Publish", mock.Anything, exportDomain.NATSExportCompletedOutboxV1, mock.Anything).
        Return(errors.New("NATS publish failed")).Once()

    consumer.HandleExportRequest(context.Background(), exportDomain.NATSExportRequestOutboxV1, requestPayload)

    mockExportService.AssertExpectations(t)
    mockNatsClient.AssertExpectations(t) // Verify Publish was called
}

func TestNATSConsumer_HandleExportRequest_PublishFailedError(t *testing.T) {
    logger := slog.New(slog.NewTextHandler(io.Discard, nil))
    mockExportService := new(MockExportService)
    mockNatsClient := new(MockNATSClient)
    consumer := NewNATSConsumer(mockExportService, mockNatsClient, logger)

    userID := uuid.New()
    requestEvent := exportDomain.ExportRequestEvent{UserID: userID}
    requestPayload, _ := json.Marshal(requestEvent)
    exportError := errors.New("CSV generation error")

    mockExportService.On("ExportOutboxMessagesToCSV", mock.Anything, userID, requestEvent.Filters).
        Return("", exportError).Once()

    // First Publish (for failure event) also fails
    mockNatsClient.On("Publish", mock.Anything, exportDomain.NATSExportFailedOutboxV1, mock.Anything).
        Return(errors.New("NATS publish failed")).Once()

    consumer.HandleExportRequest(context.Background(), exportDomain.NATSExportRequestOutboxV1, requestPayload)

    mockExportService.AssertExpectations(t)
    mockNatsClient.AssertExpectations(t)
}
