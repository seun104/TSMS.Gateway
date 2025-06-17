package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/AradIT/aradsms/golang_services/api/proto/billingservice"
	coreSmsDomain "github.com/AradIT/aradsms/golang_services/internal/core_sms/domain"
	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker"
	blacklistDomain "github.com/AradIT/aradsms/golang_services/internal/sms_sending_service/domain"
	"github.com/AradIT/aradsms/golang_services/internal/sms_sending_service/provider"
	"github.com/AradIT/aradsms/golang_services/internal/sms_sending_service/repository" // Assuming this is where the interface is
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	// grpc "google.golang.org/grpc" // Not needed directly in this file for client mock
)

// --- Mocks ---

type MockOutboxRepository struct {
	mock.Mock
}

func (m *MockOutboxRepository) GetByID(ctx context.Context, querier pgx.Tx, id string) (*coreSmsDomain.OutboxMessage, error) {
	args := m.Called(ctx, querier, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*coreSmsDomain.OutboxMessage), args.Error(1)
}

func (m *MockOutboxRepository) UpdateStatus(ctx context.Context, querier pgx.Tx, id string, newStatus coreSmsDomain.MessageStatus, processedAt *time.Time, segments *int, errMessage *string) error {
	args := m.Called(ctx, querier, id, newStatus, processedAt, segments, errMessage)
	return args.Error(0)
}
func (m *MockOutboxRepository) UpdatePostSendInfo(ctx context.Context, querier pgx.Tx, id string, status coreSmsDomain.MessageStatus, providerMsgID *string, providerStatus *string, sentToProviderAt time.Time, errorMessage *string) error {
	args := m.Called(ctx, querier, id, status, providerMsgID, providerStatus, sentToProviderAt, errorMessage)
	return args.Error(0)
}


type MockBillingServiceClient struct {
	mock.Mock
}

func (m *MockBillingServiceClient) DeductCredit(ctx context.Context, in *billingservice.DeductCreditRequest, opts ...interface{}) (*billingservice.DeductCreditResponse, error) {
	var callOpts []interface{}
	callOpts = append(callOpts, ctx, in)
	for _, o := range opts {
		callOpts = append(callOpts, o)
	}
	args := m.Called(callOpts...)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*billingservice.DeductCreditResponse), args.Error(1)
}
func (m *MockBillingServiceClient) HasSufficientCredit(ctx context.Context, in *billingservice.CreditCheckRequest, opts ...interface{}) (*billingservice.CreditCheckResponse, error) {
	args := m.Called(ctx, in) // Simplified for this example
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*billingservice.CreditCheckResponse), args.Error(1)
}


type MockSMSSenderProvider struct {
	mock.Mock
	ProviderName string
}

func (m *MockSMSSenderProvider) Send(ctx context.Context, details provider.SendRequestDetails) (*provider.SendResponseDetails, error) {
	args := m.Called(ctx, details)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*provider.SendResponseDetails), args.Error(1)
}
func (m *MockSMSSenderProvider) GetName() string {
	// Return the ProviderName set in the mock instance for more dynamic mock naming
	if m.ProviderName == "" {
		return "mock_provider_from_getname" // Fallback if not set
	}
	return m.ProviderName
}


type MockBlacklistRepository struct {
	mock.Mock
}

func (m *MockBlacklistRepository) IsBlacklisted(ctx context.Context, phoneNumber string, userID uuid.NullUUID) (isBlacklisted bool, reason string, err error) {
	args := m.Called(ctx, phoneNumber, userID)
	return args.Bool(0), args.String(1), args.Error(2)
}

type MockFilterWordRepository struct {
	mock.Mock
}

func (m *MockFilterWordRepository) GetActiveFilterWords(ctx context.Context) ([]string, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

type MockPgxPool struct {
    mock.Mock
}

func (m *MockPgxPool) BeginFunc(ctx context.Context, f func(pgx.Tx) error) error {
    args := m.Called(ctx, f)
    // Simulate transaction execution: call f with a nil Tx.
    // Repo mocks should be set up to expect nil for the pgx.Tx querier argument.
    errFromF := f(nil)

    // If BeginFunc itself is mocked to return an error (e.g., commit fails), use that.
    if overallErr := args.Error(0); overallErr != nil {
        return overallErr
    }
    // Otherwise, propagate the error from the function f.
    return errFromF
}
func (m *MockPgxPool) Close() { m.Called() }


type MockRouter struct {
    mock.Mock
}
func (m *MockRouter) SelectProvider(ctx context.Context, recipient string, userIDStr string) (provider.SMSSenderProvider, error) {
    args := m.Called(ctx, recipient, userIDStr)
    if args.Get(0) == nil {
        // This covers returning (nil, error) or (nil, nil)
        return nil, args.Error(1)
    }
    return args.Get(0).(provider.SMSSenderProvider), args.Error(1)
}


// --- Test Setup ---
type testAppServiceComponents struct {
	service         *SMSSendingAppService
	mockOutboxRepo  *MockOutboxRepository
	mockBilling     *MockBillingServiceClient
	mockProviders   map[string]provider.SMSSenderProvider
	mockRouter      *MockRouter
	mockBlacklist   *MockBlacklistRepository
	mockFilterWord  *MockFilterWordRepository
	mockNats        *messagebroker.NatsClient // Keep as concrete type if SMSSendingAppService uses it directly
	mockDbPool      *MockPgxPool
	logger          *slog.Logger
    // Specific provider mocks if needed for certain tests
    testSpecificProvider *MockSMSSenderProvider

}

func setupSMSSendingAppTest(t *testing.T) testAppServiceComponents {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockOutboxRepo := new(MockOutboxRepository)
	mockBilling := new(MockBillingServiceClient)

	mockDefaultProvider := &MockSMSSenderProvider{ProviderName: "mock_default"}
	// mockDefaultProvider.On("GetName").Return("mock_default").Maybe() // Setup in test if needed

	testSpecificProvider := &MockSMSSenderProvider{ProviderName: "specific_provider"}
	// testSpecificProvider.On("GetName").Return("specific_provider").Maybe()


	providersMap := map[string]provider.SMSSenderProvider{
		"mock_default": mockDefaultProvider,
		"specific_provider": testSpecificProvider,
	}

	mockRouter := new(MockRouter)
	mockBlacklistRepo := new(MockBlacklistRepository)
	mockFilterWordRepo := new(MockFilterWordRepository)
	mockDbPool := new(MockPgxPool)

	// SMSSendingAppService.natsClient is used for subscribing, not in processSMSJob for publishing.
	// Pass a nil or a minimally viable NatsClient if it's not used by the functions under test here.
	// If it's a concrete struct, initialize it minimally.
	var dummyNatsClient *messagebroker.NatsClient // SMSSendingAppService expects *messagebroker.NatsClient

	service := NewSMSSendingAppService(
		mockOutboxRepo,
		providersMap,
		"mock_default", // Default provider name
		mockBilling,
		dummyNatsClient,
		mockDbPool,
		logger,
		mockRouter,
		mockBlacklistRepo,
		mockFilterWordRepo,
	)

	return testAppServiceComponents{
		service:         service,
		mockOutboxRepo:  mockOutboxRepo,
		mockBilling:     mockBilling,
		mockProviders:   providersMap, // Give access to the map for provider expectations
		mockRouter:      mockRouter,
		mockBlacklist:   mockBlacklistRepo,
		mockFilterWord:  mockFilterWordRepo,
		mockDbPool:      mockDbPool,
		logger:          logger,
		testSpecificProvider: testSpecificProvider, // Keep a reference if needed for specific provider tests
	}
}


func TestSMSSendingAppService_ProcessSMSJob(t *testing.T) {
	ctx := context.Background()
	outboxID := uuid.New().String()
	userID := uuid.New().String()

	baseJob := NATSJobPayload{OutboxMessageID: outboxID}

	baseMockOutboxMsg := &coreSmsDomain.OutboxMessage{
		ID:        outboxID,
		UserID:    userID,
		Recipient: "1234567890",
		Content:   "Test message",
		Status:    coreSmsDomain.MessageStatusQueued,
		Segments:  1,
		SenderID: "TestSender",
	}

	t.Run("SuccessfulSendingFlow", func(t *testing.T) {
		comps := setupSMSSendingAppTest(t)
		mockProvider := comps.mockProviders["mock_default"].(*MockSMSSenderProvider)
		mockProvider.On("GetName").Return("mock_default").Maybe()


		comps.mockDbPool.On("BeginFunc", ctx, mock.AnythingOfType("func(pgx.Tx) error")).Return(nil).Run(func(args mock.Arguments) {
			fn := args.Get(1).(func(pgx.Tx) error)
			_ = fn(nil)
		}).Once()

		comps.mockOutboxRepo.On("GetByID", ctx, mock.Anything, outboxID).Return(baseMockOutboxMsg, nil).Once()
		comps.mockBlacklist.On("IsBlacklisted", ctx, baseMockOutboxMsg.Recipient, mock.AnythingOfType("uuid.NullUUID")).Return(false, "", nil).Once()
		comps.mockFilterWord.On("GetActiveFilterWords", ctx).Return([]string{}, nil).Once()
		comps.mockOutboxRepo.On("UpdateStatus", ctx, mock.Anything, outboxID, coreSmsDomain.MessageStatusProcessing, mock.AnythingOfType("*time.Time"), nil, nil).Return(nil).Once()
		comps.mockBilling.On("DeductCredit", mock.Anything, mock.AnythingOfType("*billingservice.DeductCreditRequest")).Return(&billingservice.DeductCreditResponse{Success: true}, nil).Once()
		comps.mockRouter.On("SelectProvider", ctx, baseMockOutboxMsg.Recipient, baseMockOutboxMsg.UserID).Return(mockProvider, nil).Once()
		mockProvider.On("Send", mock.Anything, mock.AnythingOfType("provider.SendRequestDetails")).Return(&provider.SendResponseDetails{IsSuccess: true, ProviderMessageID: "pid_123", ProviderStatus: "SENT"}, nil).Once()
		comps.mockOutboxRepo.On("UpdatePostSendInfo", ctx, mock.Anything, outboxID, coreSmsDomain.MessageStatusSentToProvider, mock.AnythingOfType("*string"), mock.AnythingOfType("*string"), mock.AnythingOfType("time.Time"), (*string)(nil)).Return(nil).Once()

		err := comps.service.processSMSJob(ctx, baseJob)

		require.NoError(t, err)
		comps.mockOutboxRepo.AssertExpectations(t); comps.mockBilling.AssertExpectations(t); comps.mockRouter.AssertExpectations(t); mockProvider.AssertExpectations(t); comps.mockBlacklist.AssertExpectations(t); comps.mockFilterWord.AssertExpectations(t); comps.mockDbPool.AssertExpectations(t)
	})

	t.Run("BillingDeductionFails", func(t *testing.T) {
		comps := setupSMSSendingAppTest(t)
		billingError := errors.New("insufficient funds")

		comps.mockDbPool.On("BeginFunc", ctx, mock.AnythingOfType("func(pgx.Tx) error")).Return(billingError).Run(func(args mock.Arguments) { // Simulate tx failing due to error from f
			fn := args.Get(1).(func(pgx.Tx) error)
			comps.mockOutboxRepo.On("GetByID", ctx, mock.Anything, outboxID).Return(baseMockOutboxMsg, nil).Once()
			comps.mockBlacklist.On("IsBlacklisted", ctx, baseMockOutboxMsg.Recipient, mock.AnythingOfType("uuid.NullUUID")).Return(false, "", nil).Once()
			comps.mockFilterWord.On("GetActiveFilterWords", ctx).Return([]string{}, nil).Once()
			comps.mockOutboxRepo.On("UpdateStatus", ctx, mock.Anything, outboxID, coreSmsDomain.MessageStatusProcessing, mock.AnythingOfType("*time.Time"), nil, nil).Return(nil).Once()
			comps.mockBilling.On("DeductCredit", mock.Anything, mock.AnythingOfType("*billingservice.DeductCreditRequest")).Return(nil, billingError).Once()
			comps.mockOutboxRepo.On("UpdatePostSendInfo", ctx, mock.Anything, outboxID, coreSmsDomain.MessageStatusFailedProviderSubmission, (*string)(nil), (*string)(nil), mock.AnythingOfType("time.Time"), mock.MatchedBy(func(s *string)bool{ return *s == "Billing error: insufficient funds"})).Return(nil).Once()

			err := fn(nil)
			assert.ErrorIs(t, err, billingError)
		}).Once()

		err := comps.service.processSMSJob(ctx, baseJob)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "credit deduction failed")
		comps.mockOutboxRepo.AssertExpectations(t); comps.mockBilling.AssertExpectations(t); comps.mockRouter.AssertNotCalled(t, "SelectProvider"); comps.mockDbPool.AssertExpectations(t)
	})

	t.Run("MessageBlacklisted", func(t *testing.T) {
		comps := setupSMSSendingAppTest(t)
		comps.mockDbPool.On("BeginFunc", ctx, mock.AnythingOfType("func(pgx.Tx) error")).Return(nil).Run(func(args mock.Arguments) {
			fn := args.Get(1).(func(pgx.Tx) error)
			comps.mockOutboxRepo.On("GetByID", ctx, mock.Anything, outboxID).Return(baseMockOutboxMsg, nil).Once()
			comps.mockBlacklist.On("IsBlacklisted", ctx, baseMockOutboxMsg.Recipient, mock.AnythingOfType("uuid.NullUUID")).Return(true, "User-level blacklist", nil).Once()
			comps.mockOutboxRepo.On("UpdatePostSendInfo", ctx, mock.Anything, outboxID, coreSmsDomain.MessageStatusRejected, (*string)(nil), (*string)(nil), mock.AnythingOfType("time.Time"), mock.MatchedBy(func(s *string) bool { return *s == "Recipient blacklisted: User-level blacklist" })).Return(nil).Once()

			err := fn(nil)
			assert.NoError(t, err)
		}).Once()

		err := comps.service.processSMSJob(ctx, baseJob)
		require.NoError(t, err)
		comps.mockOutboxRepo.AssertExpectations(t); comps.mockBlacklist.AssertExpectations(t); comps.mockFilterWord.AssertNotCalled(t, "GetActiveFilterWords"); comps.mockBilling.AssertNotCalled(t, "DeductCredit"); comps.mockDbPool.AssertExpectations(t)
	})

	t.Run("MessageActuallyFiltered", func(t *testing.T) {
		comps := setupSMSSendingAppTest(t)
		filteredMessageContent := "This message contains forbiddenword."
		mockOutboxMsgFiltered := &coreSmsDomain.OutboxMessage{
			ID: outboxID, UserID: userID, Recipient: "1234567890", Content: filteredMessageContent,
			Status: coreSmsDomain.MessageStatusQueued, Segments: 1, SenderID: "TestSender",
		}

		comps.mockDbPool.On("BeginFunc", ctx, mock.AnythingOfType("func(pgx.Tx) error")).Return(nil).Run(func(args mock.Arguments) {
			fn := args.Get(1).(func(pgx.Tx) error)
			comps.mockOutboxRepo.On("GetByID", ctx, mock.Anything, outboxID).Return(mockOutboxMsgFiltered, nil).Once()
			comps.mockBlacklist.On("IsBlacklisted", ctx, mockOutboxMsgFiltered.Recipient, mock.AnythingOfType("uuid.NullUUID")).Return(false, "", nil).Once()
			comps.mockFilterWord.On("GetActiveFilterWords", ctx).Return([]string{"forbiddenword"}, nil).Once()
			comps.mockOutboxRepo.On("UpdatePostSendInfo", ctx, mock.Anything, outboxID, coreSmsDomain.MessageStatusRejected, (*string)(nil), (*string)(nil), mock.AnythingOfType("time.Time"), mock.MatchedBy(func(s *string) bool { return *s == "Message content rejected due to filter word: forbiddenword" })).Return(nil).Once()

			err := fn(nil)
			assert.NoError(t, err)
		}).Once()

		err := comps.service.processSMSJob(ctx, baseJob) // baseJob's content doesn't have "forbiddenword"
                                                    // but GetByID returns mockOutboxMsgFiltered with it.
		require.NoError(t, err)
		comps.mockOutboxRepo.AssertExpectations(t); comps.mockBlacklist.AssertExpectations(t); comps.mockFilterWord.AssertExpectations(t); comps.mockBilling.AssertNotCalled(t, "DeductCredit"); comps.mockDbPool.AssertExpectations(t)
	})

	t.Run("RoutingErrorNoProviderFound", func(t *testing.T) {
		comps := setupSMSSendingAppTest(t)
		routingErr := errors.New("no provider route found") // Error returned by SelectProvider

		comps.mockDbPool.On("BeginFunc", ctx, mock.AnythingOfType("func(pgx.Tx) error")).Return(routingErr).Run(func(args mock.Arguments) {
			fn := args.Get(1).(func(pgx.Tx) error)
			comps.mockOutboxRepo.On("GetByID", ctx, mock.Anything, outboxID).Return(baseMockOutboxMsg, nil).Once()
			comps.mockBlacklist.On("IsBlacklisted", ctx, baseMockOutboxMsg.Recipient, mock.AnythingOfType("uuid.NullUUID")).Return(false, "", nil).Once()
			comps.mockFilterWord.On("GetActiveFilterWords", ctx).Return([]string{}, nil).Once()
			comps.mockOutboxRepo.On("UpdateStatus", ctx, mock.Anything, outboxID, coreSmsDomain.MessageStatusProcessing, mock.AnythingOfType("*time.Time"), nil, nil).Return(nil).Once()
			comps.mockBilling.On("DeductCredit", mock.Anything, mock.AnythingOfType("*billingservice.DeductCreditRequest")).Return(&billingservice.DeductCreditResponse{Success: true}, nil).Once()
			comps.mockRouter.On("SelectProvider", ctx, baseMockOutboxMsg.Recipient, baseMockOutboxMsg.UserID).Return(nil, routingErr).Once() // Router returns error
			comps.mockOutboxRepo.On("UpdatePostSendInfo", ctx, mock.Anything, outboxID, coreSmsDomain.MessageStatusFailedConfiguration, (*string)(nil), (*string)(nil), mock.AnythingOfType("time.Time"), mock.MatchedBy(func(s *string) bool { return *s == "Routing error: no provider route found" })).Return(nil).Once()

			err := fn(nil)
			assert.ErrorIs(t, err, routingErr)
		}).Once()

		err := comps.service.processSMSJob(ctx, baseJob)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Routing error")
		comps.mockRouter.AssertExpectations(t)
		comps.mockProviders["mock_default"].(*MockSMSSenderProvider).AssertNotCalled(t, "Send")
	})

	t.Run("ProviderSendError", func(t *testing.T) {
		comps := setupSMSSendingAppTest(t)
		providerError := errors.New("provider send failed")
		mockProvider := comps.mockProviders["mock_default"].(*MockSMSSenderProvider)
		mockProvider.On("GetName").Return("mock_default").Maybe()

		comps.mockDbPool.On("BeginFunc", ctx, mock.AnythingOfType("func(pgx.Tx) error")).Return(providerError).Run(func(args mock.Arguments) {
			fn := args.Get(1).(func(pgx.Tx) error)
			comps.mockOutboxRepo.On("GetByID", ctx, mock.Anything, outboxID).Return(baseMockOutboxMsg, nil).Once()
			comps.mockBlacklist.On("IsBlacklisted", ctx, baseMockOutboxMsg.Recipient, mock.AnythingOfType("uuid.NullUUID")).Return(false, "", nil).Once()
			comps.mockFilterWord.On("GetActiveFilterWords", ctx).Return([]string{}, nil).Once()
			comps.mockOutboxRepo.On("UpdateStatus", ctx, mock.Anything, outboxID, coreSmsDomain.MessageStatusProcessing, mock.AnythingOfType("*time.Time"), nil, nil).Return(nil).Once()
			comps.mockBilling.On("DeductCredit", mock.Anything, mock.AnythingOfType("*billingservice.DeductCreditRequest")).Return(&billingservice.DeductCreditResponse{Success: true}, nil).Once()
			comps.mockRouter.On("SelectProvider", ctx, baseMockOutboxMsg.Recipient, baseMockOutboxMsg.UserID).Return(mockProvider, nil).Once()
			mockProvider.On("Send", mock.Anything, mock.AnythingOfType("provider.SendRequestDetails")).Return(nil, providerError).Once()
			comps.mockOutboxRepo.On("UpdatePostSendInfo", ctx, mock.Anything, outboxID, coreSmsDomain.MessageStatusFailedProviderSubmission, (*string)(nil), mock.AnythingOfType("*string"), mock.AnythingOfType("time.Time"), mock.MatchedBy(func(s *string) bool { return *s == "provider send failed" })).Return(nil).Once()

			err := fn(nil)
			assert.ErrorIs(t, err, providerError)
		}).Once()

		err := comps.service.processSMSJob(ctx, baseJob)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider send error")
		mockProvider.AssertExpectations(t)
	})

	t.Run("OutboxMessageNotFoundInitial", func(t *testing.T) {
		comps := setupSMSSendingAppTest(t)
		// Use repository.ErrOutboxMessageNotFound if it's defined and returned by your repo
		notFoundError := errors.New("outbox message not found") // Or repository.ErrOutboxMessageNotFound

		comps.mockDbPool.On("BeginFunc", ctx, mock.AnythingOfType("func(pgx.Tx) error")).Return(notFoundError).Run(func(args mock.Arguments) {
			fn := args.Get(1).(func(pgx.Tx) error)
			comps.mockOutboxRepo.On("GetByID", ctx, mock.Anything, outboxID).Return(nil, notFoundError).Once()
			err := fn(nil)
			assert.ErrorIs(t, err, notFoundError)
		}).Once()

		err := comps.service.processSMSJob(ctx, baseJob)
		require.Error(t, err)
		assert.ErrorIs(t, err, notFoundError)
		comps.mockOutboxRepo.AssertExpectations(t)
		comps.mockBilling.AssertNotCalled(t, "DeductCredit", mock.Anything, mock.Anything)
	})

	t.Run("MessageAlreadyProcessed_NotInQueuedState", func(t *testing.T) {
		comps := setupSMSSendingAppTest(t)
		alreadySentMsg := &coreSmsDomain.OutboxMessage{
			ID: outboxID, UserID: userID, Recipient: "1234567890", Content: "Test message",
			Status: coreSmsDomain.MessageStatusSentToProvider, // Not Queued
			Segments: 1, SenderID: "TestSender",
		}
		comps.mockDbPool.On("BeginFunc", ctx, mock.AnythingOfType("func(pgx.Tx) error")).Return(nil).Run(func(args mock.Arguments) {
			fn := args.Get(1).(func(pgx.Tx) error)
			comps.mockOutboxRepo.On("GetByID", ctx, mock.Anything, outboxID).Return(alreadySentMsg, nil).Once()
			// No further calls should be made after this check
			err := fn(nil)
			assert.NoError(t, err) // The transaction itself doesn't fail, just no processing
		}).Once()

		err := comps.service.processSMSJob(ctx, baseJob)
		require.NoError(t, err) // processSMSJob returns nil for already processed messages
		comps.mockOutboxRepo.AssertExpectations(t)
		comps.mockBlacklist.AssertNotCalled(t, "IsBlacklisted")
		comps.mockFilterWord.AssertNotCalled(t, "GetActiveFilterWords")
		comps.mockBilling.AssertNotCalled(t, "DeductCredit")
		comps.mockRouter.AssertNotCalled(t, "SelectProvider")
		comps.mockProviders["mock_default"].(*MockSMSSenderProvider).AssertNotCalled(t, "Send")
	})

	t.Run("DBErrorOnUpdatePostSendInfo_AfterSuccessfulSend", func(t *testing.T) {
		comps := setupSMSSendingAppTest(t)
		mockProvider := comps.mockProviders["mock_default"].(*MockSMSSenderProvider)
		mockProvider.On("GetName").Return("mock_default").Maybe()
		dbUpdateError := errors.New("db error updating post send info")

		comps.mockDbPool.On("BeginFunc", ctx, mock.AnythingOfType("func(pgx.Tx) error")).Return(dbUpdateError).Run(func(args mock.Arguments) {
			fn := args.Get(1).(func(pgx.Tx) error)
			comps.mockOutboxRepo.On("GetByID", ctx, mock.Anything, outboxID).Return(baseMockOutboxMsg, nil).Once()
			comps.mockBlacklist.On("IsBlacklisted", ctx, baseMockOutboxMsg.Recipient, mock.AnythingOfType("uuid.NullUUID")).Return(false, "", nil).Once()
			comps.mockFilterWord.On("GetActiveFilterWords", ctx).Return([]string{}, nil).Once()
			comps.mockOutboxRepo.On("UpdateStatus", ctx, mock.Anything, outboxID, coreSmsDomain.MessageStatusProcessing, mock.AnythingOfType("*time.Time"), nil, nil).Return(nil).Once()
			comps.mockBilling.On("DeductCredit", mock.Anything, mock.AnythingOfType("*billingservice.DeductCreditRequest")).Return(&billingservice.DeductCreditResponse{Success: true}, nil).Once()
			comps.mockRouter.On("SelectProvider", ctx, baseMockOutboxMsg.Recipient, baseMockOutboxMsg.UserID).Return(mockProvider, nil).Once()
			mockProvider.On("Send", mock.Anything, mock.AnythingOfType("provider.SendRequestDetails")).Return(&provider.SendResponseDetails{IsSuccess: true, ProviderMessageID: "pid_123", ProviderStatus: "SENT"}, nil).Once()
			comps.mockOutboxRepo.On("UpdatePostSendInfo", ctx, mock.Anything, outboxID, coreSmsDomain.MessageStatusSentToProvider, mock.AnythingOfType("*string"), mock.AnythingOfType("*string"), mock.AnythingOfType("time.Time"), (*string)(nil)).Return(dbUpdateError).Once()

			err := fn(nil)
			assert.ErrorIs(t, err, dbUpdateError)
		}).Once()

		err := comps.service.processSMSJob(ctx, baseJob)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db update (post send info) failed")
		comps.mockOutboxRepo.AssertExpectations(t)
		mockProvider.AssertExpectations(t)
	})

}

// Note: Need to ensure all repository interfaces are correctly mocked.
>>>>>>> REPLACE
