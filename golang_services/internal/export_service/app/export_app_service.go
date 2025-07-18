package app

import (
	"context"
	"encoding/csv"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"
	exportDomain "github.com/AradIT/aradsms/golang_services/internal/export_service/domain"
)

// ExportService handles the business logic for exporting data.
type ExportService struct {
	repo       exportDomain.OutboxExportRepository
	logger     *slog.Logger
	exportPath string // Base path where export files will be stored
}

// NewExportService creates a new ExportService.
func NewExportService(repo exportDomain.OutboxExportRepository, logger *slog.Logger, exportPath string) *ExportService {
	if exportPath == "" {
		exportPath = "/tmp/exports" // Default export path if not configured
		logger.Warn("Export path not configured, using default", "path", exportPath)
	}
	return &ExportService{
		repo:       repo,
		logger:     logger.With("service_component", "ExportService"),
		exportPath: exportPath,
	}
}

// ExportOutboxMessagesToCSV fetches outbox messages for a user and writes them to a CSV file.
// It returns the full path to the generated CSV file or an error.
func (s *ExportService) ExportOutboxMessagesToCSV(ctx context.Context, userID uuid.UUID, filters map[string]string) (filePath string, err error) {
	const exportType = "outbox_csv"
	timer := prometheus.NewTimer(exportJobProcessingDurationHist.WithLabelValues(exportType))
	defer timer.ObserveDuration()

	jobStatus := "success" // Assume success initially
	var rowCount int = 0

	// Defer the counter update for job status
	defer func() {
		exportJobsProcessedCounter.WithLabelValues(exportType, jobStatus).Inc()
		if jobStatus == "success" && rowCount > 0 {
			exportedRowsCounter.WithLabelValues(exportType).Add(float64(rowCount))
		}
	}()

	s.logger.InfoContext(ctx, "Starting CSV export for outbox messages", "user_id", userID, "filters", filters)

	messages, err := s.repo.GetOutboxMessagesForUser(ctx, userID, filters)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to get outbox messages for CSV export", "user_id", userID, "error", err)
		jobStatus = "error_fetch_data"
		return "", fmt.Errorf("fetching messages for export failed: %w", err)
	}

	rowCount = len(messages) // Set rowCount here

	if rowCount == 0 {
		s.logger.InfoContext(ctx, "No outbox messages found for user to export", "user_id", userID)
		// This is not an error from the system's perspective, but might be a "success_no_data" for metrics.
		// For now, if no file is produced, it might not be counted as a "success" for job processed if filePath is empty.
		// Let's consider this a success for processing, but exportedRows will be 0.
		// The defer func will handle jobStatus="success" and exportedRows=0.
		return "", nil
	}

	// Ensure export directory exists
	if err := os.MkdirAll(s.exportPath, 0750); err != nil { // 0750 permissions: rwxr-x---
		s.logger.ErrorContext(ctx, "Failed to create export directory", "path", s.exportPath, "error", err)
		jobStatus = "error_create_dir"
		return "", fmt.Errorf("could not create export directory: %w", err)
	}

	// Generate a unique file name
	fileName := fmt.Sprintf("outbox_export_%s_%s.csv", userID.String(), time.Now().UTC().Format("20060102T150405Z"))
	fullPath := filepath.Join(s.exportPath, fileName)

	file, err := os.Create(fullPath)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to create CSV export file", "path", fullPath, "error", err)
		jobStatus = "error_create_file"
		return "", fmt.Errorf("creating CSV file failed: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush() // Ensure all buffered data is written to the file

	// Write CSV header
	headers := []string{
		"ID", "UserID", "SenderID", "Recipient", "Content", "Status", "Segments",
		"ProviderMessageID", "ScheduledFor", "SentToProviderAt", "DeliveredAt", "CreatedAt",
	}
	if err := writer.Write(headers); err != nil {
		s.logger.ErrorContext(ctx, "Failed to write CSV header", "path", fullPath, "error", err)
		_ = os.Remove(fullPath) // Attempt to remove partially created file on error
		jobStatus = "error_write_csv_header"
		return "", fmt.Errorf("writing CSV header failed: %w", err)
	}

	// Write data rows
	for _, msg := range messages {
		row := []string{
			msg.ID.String(),
			msg.UserID.String(),
			msg.SenderID,
			msg.Recipient,
			msg.Content,        // Content sanitization (e.g., for newlines, quotes) might be needed for robust CSV.
			string(msg.Status), // Assuming MessageStatus is a string or has a String() method.
			strconv.Itoa(msg.Segments),
			ptrToString(msg.ProviderMessageID),
			ptrToTimeToString(msg.ScheduledFor),
			ptrToTimeToString(msg.SentToProviderAt),
			ptrToTimeToString(msg.DeliveredAt),
			msg.CreatedAt.Format(time.RFC3339Nano), // Using RFC3339Nano for more precision
		}
		if err := writer.Write(row); err != nil {
			s.logger.ErrorContext(ctx, "Failed to write CSV row", "path", fullPath, "message_id", msg.ID.String(), "error", err)
			// Decide if one bad row should stop the whole export. For now, log and continue.
			// If it's critical all rows are written or none, then return error here and delete file.
		}
	}

	if err := writer.Error(); err != nil {
        s.logger.ErrorContext(ctx, "CSV writer error after writing rows", "path", fullPath, "error", err)
        _ = os.Remove(fullPath)
		jobStatus = "error_write_csv_rows"
        return "", fmt.Errorf("csv writer error: %w", err)
    }

	s.logger.InfoContext(ctx, "Successfully exported outbox messages to CSV", "user_id", userID, "file_path", fullPath, "num_records", rowCount)
	// jobStatus remains "success" if no errors by this point
	return fullPath, nil
}

// ptrToString converts a *string to string, returning empty if nil.
func ptrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ptrToTimeToString converts a *time.Time to string (RFC3339Nano), returning empty if nil.
func ptrToTimeToString(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339Nano)
}
