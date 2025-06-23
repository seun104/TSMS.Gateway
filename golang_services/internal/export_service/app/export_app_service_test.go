package app

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	exportDomain "github.com/AradIT/aradsms/golang_services/internal/export_service/domain"
	coreSmsDomain "github.com/AradIT/aradsms/golang_services/internal/core_sms/domain" // For MessageStatus
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockOutboxExportRepository is a mock implementation of OutboxExportRepository
type MockOutboxExportRepository struct {
	mock.Mock
}

func (m *MockOutboxExportRepository) GetOutboxMessagesForUser(ctx context.Context, userID uuid.UUID, filters map[string]string) ([]*exportDomain.ExportedOutboxMessage, error) {
	args := m.Called(ctx, userID, filters)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*exportDomain.ExportedOutboxMessage), args.Error(1)
}

func setupExportServiceTest(t *testing.T) (*ExportService, *MockOutboxExportRepository, string) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil)) // Discard logs for cleaner test output
	mockRepo := new(MockOutboxExportRepository)

	// Create a temporary directory for export files for this test run
	// This makes tests self-contained and cleans up afterwards.
	exportTestPath, err := os.MkdirTemp("", "export_test_")
	require.NoError(t, err)

	service := NewExportService(mockRepo, logger, exportTestPath)
	return service, mockRepo, exportTestPath
}

func TestExportService_ExportOutboxMessagesToCSV_Success(t *testing.T) {
	service, mockRepo, exportTestPath := setupExportServiceTest(t)
	defer os.RemoveAll(exportTestPath) // Clean up the temp directory

	userID := uuid.New()
	now := time.Now().UTC()
	msgTime1 := now.Add(-1 * time.Hour)
	msgTime2 := now.Add(-2 * time.Hour)

	mockMessages := []*exportDomain.ExportedOutboxMessage{
		{
			ID: uuid.New(), UserID: userID, SenderID: "Sender1", Recipient: "Rec1", Content: "Hello",
			Status: coreSmsDomain.MessageStatusDelivered, Segments: 1, CreatedAt: msgTime1, DeliveredAt: &now,
		},
		{
			ID: uuid.New(), UserID: userID, SenderID: "Sender2", Recipient: "Rec2", Content: "World, with comma",
			Status: coreSmsDomain.MessageStatusSentToProvider, Segments: 1, CreatedAt: msgTime2, SentToProviderAt: &msgTime1,
			ProviderMessageID: strPtr("pid-123"),
		},
	}

	mockRepo.On("GetOutboxMessagesForUser", mock.Anything, userID, mock.AnythingOfType("map[string]string")).
		Return(mockMessages, nil).Once()

	filePath, err := service.ExportOutboxMessagesToCSV(context.Background(), userID, nil)
	require.NoError(t, err)
	require.NotEmpty(t, filePath)
	assert.Contains(t, filePath, exportTestPath) // Ensure file is in the test export path

	// Verify CSV content
	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	require.NoError(t, err)

	require.Len(t, records, 3) // Header + 2 data rows

	// Check header
	expectedHeader := []string{
		"ID", "UserID", "SenderID", "Recipient", "Content", "Status", "Segments",
		"ProviderMessageID", "ScheduledFor", "SentToProviderAt", "DeliveredAt", "CreatedAt",
	}
	assert.Equal(t, expectedHeader, records[0])

	// Check first data row (loosely, just check a few fields)
	assert.Equal(t, mockMessages[0].ID.String(), records[1][0])
	assert.Equal(t, "Hello", records[1][4])
	assert.Equal(t, string(coreSmsDomain.MessageStatusDelivered), records[1][5])
	assert.Equal(t, ptrToTimeToString(&now), records[1][10]) // DeliveredAt

	// Check second data row
	assert.Equal(t, mockMessages[1].ID.String(), records[2][0])
	assert.Equal(t, "World, with comma", records[2][4])
	assert.Equal(t, "pid-123", records[2][7]) // ProviderMessageID
	assert.Equal(t, ptrToTimeToString(&msgTime1), records[2][9]) // SentToProviderAt

	mockRepo.AssertExpectations(t)
}

func TestExportService_ExportOutboxMessagesToCSV_NoMessages(t *testing.T) {
	service, mockRepo, exportTestPath := setupExportServiceTest(t)
	defer os.RemoveAll(exportTestPath)

	userID := uuid.New()
	mockRepo.On("GetOutboxMessagesForUser", mock.Anything, userID, mock.AnythingOfType("map[string]string")).
		Return([]*exportDomain.ExportedOutboxMessage{}, nil).Once()

	filePath, err := service.ExportOutboxMessagesToCSV(context.Background(), userID, nil)
	require.NoError(t, err)
	assert.Empty(t, filePath) // Expect empty path as no data to export

	mockRepo.AssertExpectations(t)
}

func TestExportService_ExportOutboxMessagesToCSV_RepositoryError(t *testing.T) {
	service, mockRepo, exportTestPath := setupExportServiceTest(t)
	defer os.RemoveAll(exportTestPath)

	userID := uuid.New()
	dbError := errors.New("database error")
	mockRepo.On("GetOutboxMessagesForUser", mock.Anything, userID, mock.AnythingOfType("map[string]string")).
		Return(nil, dbError).Once()

	filePath, err := service.ExportOutboxMessagesToCSV(context.Background(), userID, nil)
	require.Error(t, err)
	assert.Empty(t, filePath)
	assert.Contains(t, err.Error(), dbError.Error())

	mockRepo.AssertExpectations(t)
}

func TestExportService_ExportOutboxMessagesToCSV_CannotCreateDir(t *testing.T) {
	// This test is a bit tricky as it involves file system permissions.
	// For simplicity, we'll make the exportPath invalid to simulate os.MkdirAll failure.
	// Note: This might not work on all OSes or if the test runner has broad permissions.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockRepo := new(MockOutboxExportRepository)

	// Using a path that's likely not creatable by a normal user, or a path that's a file.
	// On Unix, "/dev/null/exports" or similar could work.
	// For more robust testing, os.MkdirAll would need to be mockable, which is harder.
	// Let's use a path that's typically a file to cause MkdirAll to fail.
	// Create a dummy file to make MkdirAll fail
	dummyFilePath := filepath.Join(os.TempDir(), "dummy_file_for_export_test_"+uuid.NewString())
	f, err := os.Create(dummyFilePath)
	require.NoError(t, err)
	f.Close()
	defer os.Remove(dummyFilePath)

	service := NewExportService(mockRepo, logger, filepath.Join(dummyFilePath, "exports")) // Path is now parented by a file

	userID := uuid.New()
	// Repo call doesn't matter here as it should fail before that.
	// But if GetOutboxMessagesForUser is called first, then mock it to return some data.
	mockMessages := []*exportDomain.ExportedOutboxMessage{{ID: uuid.New()}}
	mockRepo.On("GetOutboxMessagesForUser", mock.Anything, userID, mock.AnythingOfType("map[string]string")).
		Return(mockMessages, nil).Once()


	filePath, err := service.ExportOutboxMessagesToCSV(context.Background(), userID, nil)
	require.Error(t, err)
	assert.Empty(t, filePath)
	assert.Contains(t, err.Error(), "could not create export directory")
	// mockRepo.AssertExpectations(t) // Might not be called if MkdirAll fails early
}


// Helper function to get a pointer to a string, useful for mock data
func strPtr(s string) *string {
	return &s
}
// ptrToTimeToString is already defined in app/export_app_service.go, not needed here if testing compiled package.
// But for whitebox testing or if it were private, it might be redefined or exported.
```
