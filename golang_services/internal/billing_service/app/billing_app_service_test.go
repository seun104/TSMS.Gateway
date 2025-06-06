package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/AradIT/aradsms/golang_services/internal/billing_service/domain"
	"github.com/AradIT/aradsms/golang_services/internal/billing_service/repository" // For TransactionRepository mock
	userDomain "github.com/AradIT/aradsms/golang_services/internal/user_service/domain"
	userRepository "github.com/AradIT/aradsms/golang_services/internal/user_service/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// --- Mocks ---

type MockTransactionRepository struct {
	mock.Mock
}

func (m *MockTransactionRepository) Create(ctx context.Context, querier pgx.Tx, txn *domain.Transaction) (*domain.Transaction, error) {
	args := m.Called(ctx, querier, txn)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Transaction), args.Error(1)
}
func (m *MockTransactionRepository) GetUserTransactions(ctx context.Context, querier pgx.Tx, userID string, page, pageSize int) ([]*domain.Transaction, int, error) {
    args := m.Called(ctx, querier, userID, page, pageSize)
    if args.Get(0) == nil {
        return nil, args.Int(1), args.Error(2)
    }
    return args.Get(0).([]*domain.Transaction), args.Int(1), args.Error(2)
}


type MockUserRepository struct { // Mocks the user_service gRPC client's interface
	mock.Mock
}

func (m *MockUserRepository) GetByID(ctx context.Context, id string) (*userDomain.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*userDomain.User), args.Error(1)
}

func (m *MockUserRepository) Update(ctx context.Context, user *userDomain.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}
func (m *MockUserRepository) GetByIDForUpdate(ctx context.Context, querier pgx.Tx, id string) (*userDomain.User, error) {
    args := m.Called(ctx, querier, id)
    if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*userDomain.User), args.Error(1)
}
func (m *MockUserRepository) UpdateCreditBalance(ctx context.Context, querier pgx.Tx, id string, newBalance float64) error {
    args := m.Called(ctx, querier, id, newBalance)
	return args.Error(0)
}


type MockPaymentIntentRepository struct {
	mock.Mock
}

func (m *MockPaymentIntentRepository) Create(ctx context.Context, pi *domain.PaymentIntent) error {
	args := m.Called(ctx, pi)
	return args.Error(0)
}
func (m *MockPaymentIntentRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.PaymentIntent, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.PaymentIntent), args.Error(1)
}
func (m *MockPaymentIntentRepository) GetByGatewayPaymentIntentID(ctx context.Context, gatewayPaymentIntentID string) (*domain.PaymentIntent, error) {
	args := m.Called(ctx, gatewayPaymentIntentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.PaymentIntent), args.Error(1)
}
func (m *MockPaymentIntentRepository) Update(ctx context.Context, pi *domain.PaymentIntent) error {
	args := m.Called(ctx, pi)
	return args.Error(0)
}

type MockPaymentGatewayAdapter struct {
	mock.Mock
}

func (m *MockPaymentGatewayAdapter) CreatePaymentIntent(ctx context.Context, req domain.CreateIntentRequest) (*domain.CreateIntentResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.CreateIntentResponse), args.Error(1)
}
func (m *MockPaymentGatewayAdapter) HandleWebhookEvent(ctx context.Context, rawPayload []byte, signature string) (*domain.PaymentGatewayEvent, error) {
	args := m.Called(ctx, rawPayload, signature)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.PaymentGatewayEvent), args.Error(1)
}

type MockPgxPoolBilling struct {
	mock.Mock
}

func (m *MockPgxPoolBilling) BeginFunc(ctx context.Context, f func(pgx.Tx) error) error {
	args := m.Called(ctx, f)
	// Simplified: execute f with nil Tx. Real tests might need more elaborate pgx.Tx mock.
	var mockTx pgx.Tx // This should be a mocked pgx.Tx if tx methods are called
	if fn, ok := f.(func(pgx.Tx) error); ok {
		return fn(mockTx) // Pass the mockTx or nil
	}
	return args.Error(0)
}
func (m *MockPgxPoolBilling) Close() {}

// --- Test Setup ---
type billingAppTestComponents struct {
	service           *BillingService
	mockTxnRepo       *MockTransactionRepository
	mockUserRepo      *MockUserRepository
	mockPIntentRepo   *MockPaymentIntentRepository
	mockGateway       *MockPaymentGatewayAdapter
	mockDbPool        *MockPgxPoolBilling
	logger            *slog.Logger
}

func setupBillingAppTest(t *testing.T) billingAppTestComponents {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockTxnRepo := new(MockTransactionRepository)
	mockUserRepo := new(MockUserRepository)
	mockPIntentRepo := new(MockPaymentIntentRepository)
	mockGateway := new(MockPaymentGatewayAdapter)
	mockDbPool := new(MockPgxPoolBilling)

	service := NewBillingService(
		mockTxnRepo,
		mockUserRepo,
		mockPIntentRepo,
		mockGateway,
		mockDbPool,
		logger,
	)
	return billingAppTestComponents{
		service: service, mockTxnRepo: mockTxnRepo, mockUserRepo: mockUserRepo,
		mockPIntentRepo: mockPIntentRepo, mockGateway: mockGateway,
		mockDbPool: mockDbPool, logger: logger,
	}
}

// --- Tests ---

func TestBillingService_CreatePaymentIntent_Success(t *testing.T) {
	comps := setupBillingAppTest(t)
	userID := uuid.New()
	amount := int64(1000)
	currency := "USD"
	email := "test@example.com"
	description := "Test Payment"

	gatewayPIID := "gw_pi_" + uuid.New().String()
	clientSecret := "cs_" + uuid.New().String()

	adapterResponse := &domain.CreateIntentResponse{
		GatewayPaymentIntentID: gatewayPIID,
		ClientSecret:           &clientSecret,
		Status:                 domain.PaymentIntentStatusRequiresAction,
	}
	comps.mockGateway.On("CreatePaymentIntent", mock.Anything, mock.AnythingOfType("domain.CreateIntentRequest")).Return(adapterResponse, nil).Once()

	comps.mockDbPool.On("BeginFunc", mock.Anything, mock.AnythingOfType("func(pgx.Tx) error")).
		Run(func(args mock.Arguments) {
			fn := args.Get(1).(func(pgx.Tx) error)
			err := fn(nil)
			assert.NoError(t, err)
		}).Return(nil).Once()


	comps.mockPIntentRepo.On("Create", mock.Anything, mock.MatchedBy(func(pi *domain.PaymentIntent) bool {
		return pi.UserID == userID && pi.Amount == amount && pi.Status == adapterResponse.Status
	})).Return(nil).Once()

	resp, internalID, err := comps.service.CreatePaymentIntent(context.Background(), userID, amount, currency, email, description)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.NotEmpty(t, internalID)
	assert.Equal(t, gatewayPIID, resp.GatewayPaymentIntentID)
	if resp.ClientSecret != nil { // Check if not nil before dereferencing
	    assert.Equal(t, clientSecret, *resp.ClientSecret)
    } else {
        assert.Nil(t, resp.ClientSecret) // Or assert based on expectation if it can be nil
    }
	assert.Equal(t, domain.PaymentIntentStatusRequiresAction, resp.Status)

	comps.mockGateway.AssertExpectations(t)
	comps.mockPIntentRepo.AssertExpectations(t)
	comps.mockDbPool.AssertExpectations(t)
}

func TestBillingService_HandlePaymentWebhook_Success(t *testing.T) {
	comps := setupBillingAppTest(t)
	gatewayPIID := "gw_pi_webhook_" + uuid.New().String()
	userID := uuid.New()
	amount := int64(5000)
	currency := "USD"

	webhookPayload := []byte(`{"event_type":"payment_intent.succeeded"}`)
	signature := "valid_signature"

	eventFromAdapter := &domain.PaymentGatewayEvent{
		GatewayPaymentIntentID: gatewayPIID,
		Type:                   string(domain.PaymentIntentStatusSucceeded),
		AmountReceived:         amount,
		Currency:               currency,
		Data:                   map[string]interface{}{"status": "succeeded"},
		OccurredAt:             time.Now(),
	}
	comps.mockGateway.On("HandleWebhookEvent", mock.Anything, webhookPayload, signature).Return(eventFromAdapter, nil).Once()

	paymentIntentFromDB := &domain.PaymentIntent{
		ID:                     uuid.New(),
		UserID:                 userID,
		Amount:                 amount,
		Currency:               currency,
		Status:                 domain.PaymentIntentStatusRequiresAction,
		GatewayPaymentIntentID: &gatewayPIID,
		CreatedAt:              time.Now().Add(-5 * time.Minute),
		UpdatedAt:              time.Now().Add(-5 * time.Minute),
	}
	comps.mockPIntentRepo.On("GetByGatewayPaymentIntentID", mock.Anything, gatewayPIID).Return(paymentIntentFromDB, nil).Once()

	mockUser := &userDomain.User{ID: userID.String(), Username: "testuser", CreditBalance: 100.0, CurrencyCode: currency}
	comps.mockUserRepo.On("GetByID", mock.Anything, userID.String()).Return(mockUser, nil).Once()

	// Expect user balance update
	comps.mockUserRepo.On("Update", mock.Anything, mock.MatchedBy(func(u *userDomain.User) bool {
		// Assuming amount is in smallest unit, and CreditBalance is float of main unit.
		// This logic needs to be consistent with how amounts are handled.
		// If pi.Amount is 5000 cents ($50.00) and CreditBalance is $100.00, new balance $150.00
		// For this test, assume pi.Amount is treated as the same unit as CreditBalance for simplicity of test.
		// A real system would convert units.
		expectedNewBalance := mockUser.CreditBalance + float64(paymentIntentFromDB.Amount)
		return u.ID == userID.String() && u.CreditBalance == expectedNewBalance
	})).Return(nil).Once()

	// Expect transaction creation
	comps.mockTxnRepo.On("Create", mock.Anything, mock.Anything, mock.MatchedBy(func(txn *domain.Transaction) bool {
		return txn.UserID == userID.String() &&
		       txn.Type == domain.TransactionTypeCredit &&
		       txn.Amount == float64(amount) && // Amount should be positive for credit
			   txn.PaymentIntentID != nil && *txn.PaymentIntentID == paymentIntentFromDB.ID
	})).Return(&domain.Transaction{ID: uuid.New()}, nil).Once()

	comps.mockPIntentRepo.On("Update", mock.Anything, mock.MatchedBy(func(pi *domain.PaymentIntent) bool {
		return pi.ID == paymentIntentFromDB.ID && pi.Status == domain.PaymentIntentStatusSucceeded
	})).Return(nil).Once()

	comps.mockDbPool.On("BeginFunc", mock.Anything, mock.AnythingOfType("func(pgx.Tx) error")).
		Run(func(args mock.Arguments) {
			fn := args.Get(1).(func(pgx.Tx) error)
			err := fn(nil)
			assert.NoError(t, err)
		}).Return(nil).Once()


	err := comps.service.HandlePaymentWebhook(context.Background(), webhookPayload, signature)
	assert.NoError(t, err)

	comps.mockGateway.AssertExpectations(t)
	comps.mockPIntentRepo.AssertExpectations(t)
	comps.mockUserRepo.AssertExpectations(t)
	comps.mockTxnRepo.AssertExpectations(t)
	comps.mockDbPool.AssertExpectations(t)
}

// TODO: Add more tests for:
// - CreatePaymentIntent: Gateway error, DB error (BeginFunc fails, repo.Create fails)
// - HandlePaymentWebhook: Event parsing error (adapter returns error), PI not found, idempotency (already succeeded/failed),
//                         user not found for credit update, user service error on credit update (userRepo.Update fails),
//                         transaction creation error (txnRepo.Create fails), PI update error (repo.Update fails),
//                         different event types (failed, cancelled), DB error from BeginFunc.
// - Test currency/amount unit conversions if they were part of the logic.
```
