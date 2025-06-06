package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/aradsms/golang_services/api/proto/billingservice"
	coreSmsDomain "github.com/aradsms/golang_services/internal/core_sms/domain"
	"github.com/aradsms/golang_services/internal/platform/messagebroker"
	blacklistDomain "github.com/AradIT/aradsms/golang_services/internal/sms_sending_service/domain" // Blacklist domain
	"github.com/aradsms/golang_services/internal/sms_sending_service/provider"
	"github.com/aradsms/golang_services/internal/sms_sending_service/repository" // For OutboxRepository
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool" // For *pgxpool.Pool if needed by mocks
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/jackc/pgx/v5"       // For pgx.Tx if used in mocks
)

// --- Mocks ---

type MockOutboxRepository struct {
	mock.Mock
}

func (m *MockOutboxRepository) GetByID(ctx context.Context, querier repository.Querier, id string) (*coreSmsDomain.OutboxMessage, error) {
	args := m.Called(ctx, querier, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*coreSmsDomain.OutboxMessage), args.Error(1)
}

func (m *MockOutboxRepository) UpdateStatus(ctx context.Context, querier repository.Querier, id string, newStatus coreSmsDomain.MessageStatus, processedAt *time.Time, providerMsgID *string, providerStatus *string) error {
	args := m.Called(ctx, querier, id, newStatus, processedAt, providerMsgID, providerStatus)
	return args.Error(0)
}
func (m *MockOutboxRepository) UpdatePostSendInfo(ctx context.Context, querier repository.Querier, id string, status coreSmsDomain.MessageStatus, providerMsgID *string, providerStatus *string, sentToProviderAt time.Time, errorMessage *string) error {
	args := m.Called(ctx, querier, id, status, providerMsgID, providerStatus, sentToProviderAt, errorMessage)
	return args.Error(0)
}
// Add other methods if needed by tests, like Create.


type MockBillingServiceClient struct {
	mock.Mock
}

func (m *MockBillingServiceClient) DeductCredit(ctx context.Context, in *billingservice.DeductCreditRequest, opts ...grpc.CallOption) (*billingservice.DeductCreditResponse, error) {
	args := m.Called(ctx, in)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*billingservice.DeductCreditResponse), args.Error(1)
}
func (m *MockBillingServiceClient) HasSufficientCredit(ctx context.Context, in *billingservice.CreditCheckRequest, opts ...grpc.CallOption) (*billingservice.CreditCheckResponse, error) {
	// This method might not be used in the current flow but good to have for completeness if billingClient is mocked.
	args := m.Called(ctx, in)
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


// MockPgxPool to mock BeginFunc behavior
type MockPgxPool struct {
    mock.Mock
    // Store a reference to the test to fail it if BeginFunc is not expected
    t *testing.T
}

func NewMockPgxPool(t *testing.T) *MockPgxPool {
    return &MockPgxPool{t: t}
}

func (m *MockPgxPool) BeginFunc(ctx context.Context, f func(pgx.Tx) error) error {
    // This mock implementation of BeginFunc calls the provided function `f` with a nil pgx.Tx.
    // It's crucial that the code under test (inside `f`) doesn't actually try to use the tx object
    // for database operations if it's nil. This setup is for testing the logic flow around the transaction,
    // assuming individual repository methods (which would use the tx) are mocked separately.

    // Check if this call was expected.
    // Note: This is a simplified check. For more robust mock verification,
    // you'd use `m.Called(ctx, f)` and set up expectations with `m.On(...)`.
    // However, `m.Called` with a function argument can be tricky for matching.
    // A common pattern is to have `BeginFunc` itself be an expectation if you are using `m.On`.

    // For now, just execute f and return its error.
    // This means the test relies on the mocks of repository methods passed to `f`
    // to behave as expected when they receive a nil pgx.Tx.
    // If repository methods are not mocked to handle nil Tx, this will panic.
    // A safer mock for BeginFunc might involve a more complex setup or ensuring
    // that the transaction function `f` is testable in isolation with a mocked Tx.

    args := m.Called(ctx, f) // This allows setting expectations on BeginFunc itself.

    // If there's a specific error to return from BeginFunc itself (e.g., pool exhausted),
    // it would come from args.Error(0).
    // If the expectation is that `f` runs and its error is propagated:
    if errFromF := f(nil); errFromF != nil { // Passing nil as pgx.Tx - requires repo mocks to handle nil Querier
        return errFromF
    }
    return args.Error(0) // Return error defined in m.On(...) for BeginFunc
}

func (m *MockPgxPool) Close() { /* no-op for mock */ }


// --- Test Setup ---
type testAppServiceComponents struct {
	service        *SMSSendingAppService
	mockOutboxRepo *MockOutboxRepository
	mockBilling    *MockBillingServiceClient
	mockProviders  map[string]provider.SMSSenderProvider
	mockRouter     *MockRouter
	mockBlacklist  *MockBlacklistRepository
	mockFilterWord *MockFilterWordRepository // Added
	mockNats       *messagebroker.NatsClient
	mockDbPool     *MockPgxPool
	logger         *slog.Logger
}

type MockRouter struct {
    mock.Mock
}
func (m *MockRouter) SelectProvider(ctx context.Context, recipient string, userIDStr string) (provider.SMSSenderProvider, error) {
    args := m.Called(ctx, recipient, userIDStr)
    if args.Get(0) == nil && args.Error(1) == nil { // Provider can be nil if no route matched
        return nil, nil
    }
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(provider.SMSSenderProvider), args.Error(1)
}


func setupAppServiceTest(t *testing.T) testAppServiceComponents {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockOutboxRepo := new(MockOutboxRepository)
	mockBilling := new(MockBillingServiceClient)
	mockDefaultProvider := &MockSMSSenderProvider{ProviderName: "mock"}
	providers := map[string]provider.SMSSenderProvider{
		"mock": mockDefaultProvider,
	}
	mockRouter := new(MockRouter)
	mockBlacklistRepo := new(MockBlacklistRepository)
	mockFilterWordRepo := new(MockFilterWordRepository) // Added
	mockDbPool := NewMockPgxPool(t)

	service := NewSMSSendingAppService(
		mockOutboxRepo,
		providers,
		"mock",
		mockBilling,
		nil,
		mockDbPool,
		logger,
		mockRouter,
		mockBlacklistRepo,
		mockFilterWordRepo, // Added
	)

	return testAppServiceComponents{
		service:        service,
		mockOutboxRepo: mockOutboxRepo,
		mockBilling:    mockBilling,
		mockProviders:  providers,
		mockRouter:     mockRouter,
		mockBlacklist:  mockBlacklistRepo,
		mockFilterWord: mockFilterWordRepo, // Added
		mockDbPool:     mockDbPool,
		logger:         logger,
	}
}


func TestSMSSendingAppService_ProcessSMSJob_RecipientBlacklisted(t *testing.T) {
	comps := setupAppServiceTest(t)
	jobPayload := NATSJobPayload{OutboxMessageID: "test-id-blacklisted"}
	testUserID := uuid.New()
	mockMessage := &coreSmsDomain.OutboxMessage{
		ID:        jobPayload.OutboxMessageID,
		UserID:    testUserID.String(),
		Recipient: "blacklisted_recipient",
		Status:    coreSmsDomain.MessageStatusQueued,
	}

    // Expect BeginFunc to be called.
    // The function passed to BeginFunc should itself return nil because the blacklist check
    // results in the message being marked as rejected, and the transaction should commit this state.
    comps.mockDbPool.On("BeginFunc", mock.Anything, mock.AnythingOfType("func(pgx.Tx) error")).
        Run(func(args mock.Arguments) {
            fn := args.Get(1).(func(pgx.Tx) error)
            err := fn(nil) // Execute the transaction function with a nil Tx
            assert.NoError(t, err, "Transaction function should succeed for blacklisted message")
        }).Return(nil).Once() // BeginFunc itself returns nil (commit)


	comps.mockOutboxRepo.On("GetByID", mock.Anything, mock.Anything, jobPayload.OutboxMessageID).Return(mockMessage, nil).Once()

	expectedReason := "User request"
	comps.mockBlacklist.On("IsBlacklisted", mock.Anything, mockMessage.Recipient, uuid.NullUUID{UUID: testUserID, Valid: true}).
		Return(true, expectedReason, nil).Once()

	expectedErrorMessage := "Recipient blacklisted: " + expectedReason
	comps.mockOutboxRepo.On("UpdatePostSendInfo", mock.Anything, mock.Anything, mockMessage.ID, coreSmsDomain.MessageStatusRejected,
		(*string)(nil), (*string)(nil), mock.AnythingOfType("time.Time"), &expectedErrorMessage).
		Return(nil).Once()


	err := comps.service.processSMSJob(context.Background(), jobPayload)
	assert.NoError(t, err, "processSMSJob should return nil as transaction is expected to succeed")

	comps.mockOutboxRepo.AssertExpectations(t)
	comps.mockBlacklist.AssertExpectations(t)
	comps.mockBilling.AssertNotCalled(t, "DeductCredit") // Billing should not be called
    defaultProviderMock, _ := comps.mockProviders["mock"].(*MockSMSSenderProvider)
    defaultProviderMock.AssertNotCalled(t, "Send") // Provider send should not be called
    comps.mockDbPool.AssertExpectations(t)
}

func TestSMSSendingAppService_ProcessSMSJob_BlacklistCheckError(t *testing.T) {
	comps := setupAppServiceTest(t)
    jobPayload := NATSJobPayload{OutboxMessageID: "test-id-bl-error"}
	testUserID := uuid.New()
	mockMessage := &coreSmsDomain.OutboxMessage{
		ID:        jobPayload.OutboxMessageID,
		UserID:    testUserID.String(),
		Recipient: "recipient_bl_error",
		Status:    coreSmsDomain.MessageStatusQueued,
	}
    dbError := errors.New("blacklist DB error")
    expectedOverallErrorMsg := "Blacklist check failed: " + dbError.Error()

    // Expect BeginFunc to be called.
    // The function passed to BeginFunc is expected to return an error originating from the blacklist check.
    comps.mockDbPool.On("BeginFunc", mock.Anything, mock.AnythingOfType("func(pgx.Tx) error")).
        Run(func(args mock.Arguments) {
            fn := args.Get(1).(func(pgx.Tx) error)
            err := fn(nil) // Execute the transaction function
            assert.Error(t, err)
            assert.Contains(t, err.Error(), expectedOverallErrorMsg)
        }).Return(errors.New(expectedOverallErrorMsg)).Once() // BeginFunc itself returns the error (rollback)


    comps.mockOutboxRepo.On("GetByID", mock.Anything, mock.Anything, jobPayload.OutboxMessageID).Return(mockMessage, nil).Once()
    comps.mockBlacklist.On("IsBlacklisted", mock.Anything, mockMessage.Recipient, uuid.NullUUID{UUID: testUserID, Valid: true}).
        Return(false, "", dbError).Once()

    comps.mockOutboxRepo.On("UpdatePostSendInfo", mock.Anything, mock.Anything, mockMessage.ID, coreSmsDomain.MessageStatusFailed,
        (*string)(nil), (*string)(nil), mock.AnythingOfType("time.Time"), &expectedOverallErrorMsg).
        Return(nil).Once()


    err := comps.service.processSMSJob(context.Background(), jobPayload)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), expectedOverallErrorMsg)

    comps.mockOutboxRepo.AssertExpectations(t)
	comps.mockBlacklist.AssertExpectations(t)
	comps.mockBilling.AssertNotCalled(t, "DeductCredit")
    comps.mockDbPool.AssertExpectations(t)
}


func TestSMSSendingAppService_ProcessSMSJob_NotBlacklisted_NormalFlow(t *testing.T) {
	comps := setupAppServiceTest(t)
    jobPayload := NATSJobPayload{OutboxMessageID: "test-id-normal"}
	testUserID := uuid.New()
	mockMessage := &coreSmsDomain.OutboxMessage{
		ID:        jobPayload.OutboxMessageID,
		UserID:    testUserID.String(),
		Recipient: "normal_recipient",
		Status:    coreSmsDomain.MessageStatusQueued,
        SenderID:  "TestSender",
        Content:   "Hello",
	}

    comps.mockDbPool.On("BeginFunc", mock.Anything, mock.AnythingOfType("func(pgx.Tx) error")).
        Run(func(args mock.Arguments) {
            fn := args.Get(1).(func(pgx.Tx) error)
            assert.NoError(t, fn(nil))
        }).Return(nil).Once()


    comps.mockOutboxRepo.On("GetByID", mock.Anything, mock.Anything, jobPayload.OutboxMessageID).Return(mockMessage, nil).Once()
    comps.mockBlacklist.On("IsBlacklisted", mock.Anything, mockMessage.Recipient, uuid.NullUUID{UUID: testUserID, Valid: true}).
        Return(false, "", nil).Once() // Not blacklisted

	comps.mockFilterWord.On("GetActiveFilterWords", mock.Anything).Return([]string{}, nil).Once() // No filter words matched

    comps.mockOutboxRepo.On("UpdateStatus", mock.Anything, mock.Anything, mockMessage.ID, coreSmsDomain.MessageStatusProcessing, mock.AnythingOfType("*time.Time"), (*string)(nil), (*string)(nil)).Return(nil).Once()

    comps.mockBilling.On("DeductCredit", mock.Anything, mock.AnythingOfType("*billingservice.DeductCreditRequest")).
        Return(&billingservice.DeductCreditResponse{}, nil).Once()

    defaultProviderMock, _ := comps.mockProviders["mock"].(*MockSMSSenderProvider)
    comps.mockRouter.On("SelectProvider", mock.Anything, mockMessage.Recipient, mockMessage.UserID).
        Return(nil, nil).Once()

    providerResponse := &provider.SendResponseDetails{ProviderMessageID: "mock-msg-id", IsSuccess: true, ProviderStatus: "SENT"}
    defaultProviderMock.On("Send", mock.Anything, mock.AnythingOfType("provider.SendRequestDetails")).
        Return(providerResponse, nil).Once()

    comps.mockOutboxRepo.On("UpdatePostSendInfo", mock.Anything, mock.Anything, mockMessage.ID, coreSmsDomain.MessageStatusSentToProvider,
        &providerResponse.ProviderMessageID, &providerResponse.ProviderStatus, mock.AnythingOfType("time.Time"), (*string)(nil)).
        Return(nil).Once()


    err := comps.service.processSMSJob(context.Background(), jobPayload)
    assert.NoError(t, err)

    comps.mockOutboxRepo.AssertExpectations(t)
	comps.mockBlacklist.AssertExpectations(t)
	comps.mockFilterWord.AssertExpectations(t) // Added assertion for filterWord mock
    comps.mockBilling.AssertExpectations(t)
    comps.mockRouter.AssertExpectations(t)
    defaultProviderMock.AssertExpectations(t)
    comps.mockDbPool.AssertExpectations(t)
}


func TestSMSSendingAppService_ProcessSMSJob_ContentFiltered(t *testing.T) {
	comps := setupAppServiceTest(t)
	jobPayload := NATSJobPayload{OutboxMessageID: "test-id-filtered"}
	testUserID := uuid.New()
	filterWord := "forbidden"
	mockMessage := &coreSmsDomain.OutboxMessage{
		ID:        jobPayload.OutboxMessageID,
		UserID:    testUserID.String(),
		Recipient: "recipient-filtered",
		Content:   "This message contains a " + filterWord + " word.",
		Status:    coreSmsDomain.MessageStatusQueued,
	}

	comps.mockDbPool.On("BeginFunc", mock.Anything, mock.AnythingOfType("func(pgx.Tx) error")).
		Run(func(args mock.Arguments) {
			fn := args.Get(1).(func(pgx.Tx) error)
			assert.NoError(t, fn(nil))
		}).Return(nil).Once()

	comps.mockOutboxRepo.On("GetByID", mock.Anything, mock.Anything, jobPayload.OutboxMessageID).Return(mockMessage, nil).Once()
	comps.mockBlacklist.On("IsBlacklisted", mock.Anything, mockMessage.Recipient, uuid.NullUUID{UUID: testUserID, Valid: true}).
		Return(false, "", nil).Once() // Not blacklisted

	comps.mockFilterWord.On("GetActiveFilterWords", mock.Anything).Return([]string{filterWord, "another"}, nil).Once()

	expectedErrorMessage := "Message content rejected due to filter word: " + filterWord
	comps.mockOutboxRepo.On("UpdatePostSendInfo", mock.Anything, mock.Anything, mockMessage.ID, coreSmsDomain.MessageStatusRejected,
		(*string)(nil), (*string)(nil), mock.AnythingOfType("time.Time"), &expectedErrorMessage).
		Return(nil).Once()

	err := comps.service.processSMSJob(context.Background(), jobPayload)
	assert.NoError(t, err)

	comps.mockOutboxRepo.AssertExpectations(t)
	comps.mockBlacklist.AssertExpectations(t)
	comps.mockFilterWord.AssertExpectations(t)
	comps.mockBilling.AssertNotCalled(t, "DeductCredit")
	defaultProviderMock, _ := comps.mockProviders["mock"].(*MockSMSSenderProvider)
    defaultProviderMock.AssertNotCalled(t, "Send")
	comps.mockDbPool.AssertExpectations(t)
}

func TestSMSSendingAppService_ProcessSMSJob_FilterWordRepoError(t *testing.T) {
	comps := setupAppServiceTest(t)
	jobPayload := NATSJobPayload{OutboxMessageID: "test-id-filter-repo-err"}
	testUserID := uuid.New()
	mockMessage := &coreSmsDomain.OutboxMessage{
		ID:        jobPayload.OutboxMessageID,
		UserID:    testUserID.String(),
		Recipient: "recipient-filter-repo-err",
		Content:   "Some content",
		Status:    coreSmsDomain.MessageStatusQueued,
	}
	repoError := errors.New("filter repo DB error")
	expectedOverallErrorMsg := "Filter word check failed: " + repoError.Error()


	comps.mockDbPool.On("BeginFunc", mock.Anything, mock.AnythingOfType("func(pgx.Tx) error")).
		Run(func(args mock.Arguments) {
			fn := args.Get(1).(func(pgx.Tx) error)
			err := fn(nil)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), expectedOverallErrorMsg)
		}).Return(errors.New(expectedOverallErrorMsg)).Once()


	comps.mockOutboxRepo.On("GetByID", mock.Anything, mock.Anything, jobPayload.OutboxMessageID).Return(mockMessage, nil).Once()
	comps.mockBlacklist.On("IsBlacklisted", mock.Anything, mockMessage.Recipient, uuid.NullUUID{UUID: testUserID, Valid: true}).
		Return(false, "", nil).Once() // Not blacklisted

	comps.mockFilterWord.On("GetActiveFilterWords", mock.Anything).Return(nil, repoError).Once()

	comps.mockOutboxRepo.On("UpdatePostSendInfo", mock.Anything, mock.Anything, mockMessage.ID, coreSmsDomain.MessageStatusFailed,
        (*string)(nil), (*string)(nil), mock.AnythingOfType("time.Time"), &expectedOverallErrorMsg).
        Return(nil).Once()

	err := comps.service.processSMSJob(context.Background(), jobPayload)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), expectedOverallErrorMsg)

	comps.mockOutboxRepo.AssertExpectations(t)
	comps.mockBlacklist.AssertExpectations(t)
	comps.mockFilterWord.AssertExpectations(t)
	comps.mockBilling.AssertNotCalled(t, "DeductCredit")
	comps.mockDbPool.AssertExpectations(t)
}


// TODO: Add tests for:
// - UserID parsing failure in processSMSJob during blacklist check (how it affects msgUserID and IsBlacklisted call)
// - Routing selection leading to a specific provider (not default)
// - Error conditions from billing client (DeductCredit returns error)
// - Error conditions from provider.Send method
// - Error conditions from various outboxRepo.Update methods within the transaction
// - Idempotency: message already processed/not in "queued" state (should return nil, no action)
// - Content not matching any filter words (should proceed to billing etc.) - This is covered by NotBlacklisted_NormalFlow now
// - Empty list of active filter words (should proceed to billing etc.) - Also covered by NotBlacklisted_NormalFlow
```
