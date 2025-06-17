package provider

import (
	"context"
	"fmt"
	"log/slog" // Or your platform logger
	"time"

	"github.com/AradIT/aradsms/golang_services/internal/sms_sending_service/app" // For metrics
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

// MockSMSProvider is a test implementation of SMSSenderProvider.
type MockSMSProvider struct {
	logger         *slog.Logger
    FailSend       bool // Control whether Send should simulate failure
    SimulatedDelay time.Duration // To simulate network latency
}

// NewMockSMSProvider creates a new MockSMSProvider.
func NewMockSMSProvider(logger *slog.Logger, failSend bool, delay time.Duration) *MockSMSProvider {
	return &MockSMSProvider{
		logger: logger.With("provider", "mock"),
        FailSend: failSend,
        SimulatedDelay: delay,
	}
}

// Send simulates sending an SMS.
func (p *MockSMSProvider) Send(ctx context.Context, details SendRequestDetails) (*SendResponseDetails, error) {
	providerTimer := prometheus.NewTimer(app.SmsProviderRequestDurationHist.WithLabelValues(p.GetName()))
	defer providerTimer.ObserveDuration()

	p.logger.InfoContext(ctx, "MockSMSProvider: Send called",
		"internal_message_id", details.InternalMessageID,
		"sender_id", details.SenderID,
		"recipient", details.Recipient,
		"content_length", len(details.Content))

    if p.SimulatedDelay > 0 {
        time.Sleep(p.SimulatedDelay)
    }

    if p.FailSend {
        errMsg := "mock provider simulated send failure"
        p.logger.WarnContext(ctx, errMsg, "recipient", details.Recipient)
        return &SendResponseDetails{
            ProviderMessageID: "",
            IsSuccess:         false,
            ProviderStatus:    "FAILED_MOCK",
            ErrorMessage:      errMsg,
        }, fmt.Errorf(errMsg) // Can also return a specific error type
    }

	providerMsgID := "mock-" + uuid.NewString()
	p.logger.InfoContext(ctx, "MockSMSProvider: SMS sent successfully (simulated)", "recipient", details.Recipient, "provider_msg_id", providerMsgID)

	return &SendResponseDetails{
		ProviderMessageID: providerMsgID,
		IsSuccess:         true,
		ProviderStatus:    "SENT_MOCK_OK",
	}, nil
}

// GetName returns the name of the provider.
func (p *MockSMSProvider) GetName() string {
	return "MockSMSProvider"
}
