package paymentgateway

import (
	"context"
	"encoding/json" // For simulating webhook payload parsing
	"fmt"
	"time"

	"github.com/google/uuid"
	// Corrected import path for the billing service's domain package
	"github.com/AradIT/aradsms/golang_services/internal/billing_service/domain"
	"log/slog" // It's good practice to have a logger, even in mocks for diagnostics
)

type MockPaymentGatewayAdapter struct {
	logger                         *slog.Logger
	SimulateCreateFailure          bool
	SimulateHandleWebhookFailure   bool // General failure in handling
	SimulateWebhookVerificationError bool
	SimulatePaymentSuccessEvent    bool // If true, HandleWebhookEvent returns a success event, else failure event
}

func NewMockPaymentGatewayAdapter(logger *slog.Logger, createFail, handleFail, verificationFail, paymentSuccess bool) domain.PaymentGatewayAdapter {
	if logger == nil {
		logger = slog.Default() // Fallback, though ideally always provided
	}
	return &MockPaymentGatewayAdapter{
		logger:                         logger.With("adapter", "mock_payment_gateway"),
		SimulateCreateFailure:          createFail,
		SimulateHandleWebhookFailure:   handleFail,
		SimulateWebhookVerificationError: verificationFail,
		SimulatePaymentSuccessEvent:    paymentSuccess,
	}
}

func (m *MockPaymentGatewayAdapter) CreatePaymentIntent(ctx context.Context, req domain.CreateIntentRequest) (*domain.CreateIntentResponse, error) {
	m.logger.InfoContext(ctx, "MockPaymentGatewayAdapter: CreatePaymentIntent called", "amount", req.Amount, "currency", req.Currency, "user_id", req.UserID)

	if m.SimulateCreateFailure {
		errMsg := "mock gateway simulated CreatePaymentIntent failure"
		m.logger.WarnContext(ctx, errMsg)
		// Note: The adapter should not generate OUR internal PaymentIntent ID.
		// It returns info from the gateway.
		return &domain.CreateIntentResponse{
			// PaymentIntentID field is removed from adapter's CreateIntentResponse as it's our internal ID
			GatewayPaymentIntentID: "failed_pi_" + uuid.New().String(), // Still, gateway might return an ID even on failure
			Status:                 domain.PaymentIntentStatusFailed,
			ErrorMessage:           &errMsg,
		}, fmt.Errorf(errMsg)
	}

	gatewayID := "mock_pi_" + uuid.New().String()
	clientSecret := "mock_client_secret_" + uuid.New().String()
	nextActionURL := "https://mockgateway.dev/action/" + uuid.New().String()

	m.logger.InfoContext(ctx, "MockPaymentGatewayAdapter: CreatePaymentIntent success (simulated)", "gateway_id", gatewayID)
	return &domain.CreateIntentResponse{
		GatewayPaymentIntentID: gatewayID,
		ClientSecret:           &clientSecret,
		NextActionURL:          &nextActionURL,
		Status:                 domain.PaymentIntentStatusRequiresAction, // Or Pending, depending on mock's typical flow
	}, nil
}

func (m *MockPaymentGatewayAdapter) HandleWebhookEvent(ctx context.Context, rawPayload []byte, signature string) (*domain.PaymentGatewayEvent, error) {
	m.logger.InfoContext(ctx, "MockPaymentGatewayAdapter: HandleWebhookEvent called", "signature", signature, "payload_len", len(rawPayload))

	if m.SimulateHandleWebhookFailure {
		errMsg := "mock gateway simulated HandleWebhookEvent general failure"
		m.logger.WarnContext(ctx, errMsg)
		return nil, fmt.Errorf(errMsg)
	}

	if m.SimulateWebhookVerificationError && signature == "invalid_signature" {
		errMsg := "mock gateway webhook signature verification failed"
		m.logger.WarnContext(ctx, errMsg)
		return nil, fmt.Errorf(errMsg)
	}
	m.logger.InfoContext(ctx, "Mock webhook signature verification passed (simulated)")

	// Simulate parsing payload to determine event type
	// In a real adapter, this would parse rawPayload.
	// For mock, we'll use the SimulatePaymentSuccessEvent flag.
	var eventDataMap map[string]interface{}
	_ = json.Unmarshal(rawPayload, &eventDataMap) // Try to unmarshal, ignore error for mock simplicity
	if eventDataMap == nil {
		eventDataMap = make(map[string]interface{})
	}
	eventDataMap["received_at"] = time.Now()
	if signature != "" {
		eventDataMap["received_signature"] = signature
	}


	gatewayPaymentIntentID := "mock_pi_webhook_" + uuid.New().String()
	// Try to extract from payload if it's there, for more realistic mock
	if idFromPayload, ok := eventDataMap["gateway_payment_intent_id"].(string); ok && idFromPayload != "" {
		gatewayPaymentIntentID = idFromPayload
	}


	amountReceived := int64(1000) // Example amount
	currency := "USD"
	eventType := domain.PaymentIntentStatusFailed // Default to failed unless specified

	if m.SimulatePaymentSuccessEvent {
		eventType = domain.PaymentIntentStatusSucceeded
		eventDataMap["status"] = "succeeded"
		eventDataMap["amount_details"] = map[string]int64{"gross": amountReceived, "fee": 0, "net": amountReceived}
	} else {
		eventDataMap["status"] = "failed"
		eventDataMap["failure_reason"] = "mock_payment_failed_reason"
	}


	m.logger.InfoContext(ctx, "MockPaymentGatewayAdapter: HandleWebhookEvent processed (simulated)", "event_type", eventType, "gateway_pi_id", gatewayPaymentIntentID)
	return &domain.PaymentGatewayEvent{
		GatewayPaymentIntentID: gatewayPaymentIntentID,
		Type:                   string(eventType), // Use PaymentIntentStatus as type for this mock
		AmountReceived:         amountReceived,
		Currency:               currency,
		Data:                   eventDataMap,
		OccurredAt:             time.Now(), // Or extract from payload if available
	}, nil
}
