package smsprovider

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/google/uuid"
)

// MockProvider is a simulated SMS provider for testing and development.
type MockProvider struct {
	logger      *slog.Logger
	name        string
	failRate    float64 // Chance to simulate failure (0.0 to 1.0)
	minLatencyMs int
	maxLatencyMs int
}

// NewMockProvider creates a new MockProvider.
func NewMockProvider(logger *slog.Logger, name string, failRate float64, minLatencyMs int, maxLatencyMs int) Adapter {
	if name == "" {
		name = "mock-provider"
	}
	return &MockProvider{
		logger: logger.With("provider", name),
		name:   name,
		failRate: failRate,
		minLatencyMs: minLatencyMs,
		maxLatencyMs: maxLatencyMs,
	}
}

func (p *MockProvider) GetName() string {
	return p.name
}

func (p *MockProvider) Send(ctx context.Context, request SMSRequestData) (*SMSResponseData, error) {
	latency := p.minLatencyMs + rand.Intn(p.maxLatencyMs-p.minLatencyMs+1)
	time.Sleep(time.Duration(latency) * time.Millisecond)

	p.logger.InfoContext(ctx, "MockProvider: Send called",
		"message_id", request.InternalMessageID,
		"sender", request.SenderID,
		"recipient", request.Recipient,
		"content_len", len(request.Content))

	if rand.Float64() < p.failRate {
		errMsg := fmt.Sprintf("MockProvider simulated failure for recipient %s", request.Recipient)
		p.logger.WarnContext(ctx, errMsg, "message_id", request.InternalMessageID)
		return &SMSResponseData{
			Success:           false,
			StatusCode:        500, // Simulate a generic provider error
			ErrorMessage:      errMsg,
			ProviderMessageID: "",
			ProviderName:      p.name,
		}, nil // Or return an actual error: errors.New(errMsg)
	}

	providerMsgID := uuid.NewString()
	p.logger.InfoContext(ctx, "MockProvider: SMS sent successfully (simulated)",
		"message_id", request.InternalMessageID,
		"provider_message_id", providerMsgID)

	return &SMSResponseData{
		Success:           true,
		StatusCode:        200, // Simulate success
		ProviderMessageID: providerMsgID,
		ProviderName:      p.name,
	}, nil
}
