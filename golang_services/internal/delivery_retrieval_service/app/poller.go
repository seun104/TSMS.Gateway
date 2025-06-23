package app

import (
	"context"
	"database/sql"
	"log/slog"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/your-repo/project/internal/delivery_retrieval_service/domain"
)

// DLRPoller is responsible for polling SMS providers for delivery reports.
type DLRPoller struct {
	logger *slog.Logger
}

// NewDLRPoller creates a new DLRPoller instance.
func NewDLRPoller(logger *slog.Logger) *DLRPoller {
	return &DLRPoller{
		logger: logger,
	}
}

// PollProvider simulates polling an SMS provider for delivery reports.
// In a real implementation, this method would make HTTP calls to provider APIs.
func (p *DLRPoller) PollProvider(ctx context.Context) ([]domain.DeliveryReport, error) {
	p.logger.InfoContext(ctx, "Polling provider for DLRs...")

	// Simulate network latency
	select {
	case <-time.After(time.Duration(rand.Intn(500)+100) * time.Millisecond):
	case <-ctx.Done():
		p.logger.InfoContext(ctx, "Polling cancelled")
		return nil, ctx.Err()
	}

	var reports []domain.DeliveryReport
	numReports := rand.Intn(4) // Generate 0 to 3 reports

	for i := 0; i < numReports; i++ {
		msgID := uuid.New()
		providerMsgID := uuid.New().String()
		now := time.Now().UTC()

		var status domain.DeliveryStatus
		var deliveredAt sql.NullTime
		var errorCode sql.NullString
		var errorDesc sql.NullString

		// Randomize status
		switch rand.Intn(5) {
		case 0:
			status = domain.DeliveryStatusDelivered
			deliveredAt = sql.NullTime{Time: now.Add(-time.Duration(rand.Intn(60)) * time.Minute), Valid: true}
		case 1:
			status = domain.DeliveryStatusFailed
			errorCode = sql.NullString{String: "P001", Valid: true}
			errorDesc = sql.NullString{String: "Provider general failure", Valid: true}
		case 2:
			status = domain.DeliveryStatusExpired
			errorCode = sql.NullString{String: "P005", Valid: true}
			errorDesc = sql.NullString{String: "Message expired", Valid: true}
		case 3:
			status = domain.DeliveryStatusRejected
			errorCode = sql.NullString{String: "P010", Valid: true}
			errorDesc = sql.NullString{String: "Rejected by carrier", Valid: true}
		default:
			status = domain.DeliveryStatusSent // Still marked as sent, no final DLR yet
		}

		report := domain.NewDeliveryReport(
			msgID,
			providerMsgID,
			status,
			status.String()+"_RAW_FROM_PROVIDER", // Example provider status
			deliveredAt,
			errorCode,
			errorDesc,
		)
		// Override ReceivedAt for more deterministic testing if needed, but NewDeliveryReport sets it.
		// report.ReceivedAt = now

		reports = append(reports, *report)
		p.logger.InfoContext(ctx, "Generated mock DLR", "message_id", report.MessageID, "status", report.Status.String())
	}

	if len(reports) == 0 {
		p.logger.InfoContext(ctx, "No new DLRs found in this poll.")
	}

	return reports, nil
}
