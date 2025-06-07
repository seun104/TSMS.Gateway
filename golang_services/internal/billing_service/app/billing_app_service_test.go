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

type MockTariffRepository struct {
	mock.Mock
}

func (m *MockTariffRepository) GetTariffByID(ctx context.Context, id uuid.UUID) (*domain.Tariff, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Tariff), args.Error(1)
}

func (m *MockTariffRepository) GetActiveUserTariff(ctx context.Context, userID uuid.UUID) (*domain.Tariff, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Tariff), args.Error(1)
}

func (m *MockTariffRepository) GetDefaultActiveTariff(ctx context.Context) (*domain.Tariff, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Tariff), args.Error(1)
}


type MockPgxPoolBilling struct {
	mock.Mock
}

func (m *MockPgxPoolBilling) BeginFunc(ctx context.Context, f func(pgx.Tx) error) error {
	args := m.Called(ctx, f)
	var mockTx pgx.Tx
	if fn, ok := f.(func(pgx.Tx) error); ok {
		return fn(mockTx)
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
	mockTariffRepo    *MockTariffRepository // Added
	mockDbPool        *MockPgxPoolBilling
	logger            *slog.Logger
}

func setupBillingAppTest(t *testing.T) billingAppTestComponents {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockTxnRepo := new(MockTransactionRepository)
	mockUserRepo := new(MockUserRepository)
	mockPIntentRepo := new(MockPaymentIntentRepository)
	mockGateway := new(MockPaymentGatewayAdapter)
	mockTariffRepo := new(MockTariffRepository) // Added
	mockDbPool := new(MockPgxPoolBilling)

	service := NewBillingService(
		mockTxnRepo,
		mockUserRepo,
		mockPIntentRepo,
		mockGateway,
		mockTariffRepo, // Added
		mockDbPool,
		logger,
	)
	return billingAppTestComponents{
		service: service, mockTxnRepo: mockTxnRepo, mockUserRepo: mockUserRepo,
		mockPIntentRepo: mockPIntentRepo, mockGateway: mockGateway,
		mockTariffRepo: mockTariffRepo, // Added
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
			txn.PaymentIntentID != nil && *txn.PaymentIntentID == paymentIntentFromDB.ID &&
			txn.Status == domain.TransactionStatusCompleted && // Verify Status
			((eventFromAdapter.GatewayTransactionID == nil && txn.PaymentGatewayTxnID == nil) || (txn.PaymentGatewayTxnID != nil && eventFromAdapter.GatewayTransactionID != nil && *txn.PaymentGatewayTxnID == *eventFromAdapter.GatewayTransactionID)) // Verify GatewayTransactionID
	})).Return(&domain.Transaction{ID: uuid.NewString()}, nil).Once() // Ensure returned Tx has an ID for logging if service uses it

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

func TestBillingService_CalculateSMSCost(t *testing.T) {
	comps := setupBillingAppTest(t)
	userID := uuid.New()
	numMessages := 3

	activeUserTariff := &domain.Tariff{ID: uuid.New(), Name: "UserSpecial", PricePerSMS: 50, Currency: "USD", IsActive: true}
	defaultActiveTariff := &domain.Tariff{ID: uuid.New(), Name: "Default", PricePerSMS: 70, Currency: "USD", IsActive: true}

	t.Run("UserHasActiveTariff", func(t *testing.T) {
		comps.mockTariffRepo.On("GetActiveUserTariff", mock.Anything, userID).Return(activeUserTariff, nil).Once()

		cost, currency, err := comps.service.CalculateSMSCost(context.Background(), userID, numMessages)
		assert.NoError(t, err)
		assert.Equal(t, int64(150), cost) // 50 * 3
		assert.Equal(t, "USD", currency)
		comps.mockTariffRepo.AssertExpectations(t)
		// Reset mock for next sub-test if using the same comps.mockTariffRepo instance and On().Once()
		// However, setupBillingAppTest creates new mocks each time it's called if we were to call it per sub-test.
		// For this structure, ensure mock is reset or expectations are specific if comps is shared.
		// Re-init mock for safety in subtests if not using t.Run with fresh setups.
		// For now, assuming independent mock calls due to .Once() or fresh mock instances per sub-test.
	})

	t.Run("UserHasNoActiveTariff_UsesDefault", func(t *testing.T) {
		// Need fresh mock instance for tariffRepo if On().Once() was used and comps is not re-created
		freshComps := setupBillingAppTest(t) // Creates fresh mocks

		freshComps.mockTariffRepo.On("GetActiveUserTariff", mock.Anything, userID).Return(nil, nil).Once()
		freshComps.mockTariffRepo.On("GetDefaultActiveTariff", mock.Anything).Return(defaultActiveTariff, nil).Once()

		cost, currency, err := freshComps.service.CalculateSMSCost(context.Background(), userID, numMessages)
		assert.NoError(t, err)
		assert.Equal(t, int64(210), cost) // 70 * 3
		assert.Equal(t, "USD", currency)
		freshComps.mockTariffRepo.AssertExpectations(t)
	})

	t.Run("NoUserTariff_NoDefaultTariff", func(t *testing.T) {
		freshComps := setupBillingAppTest(t)
		freshComps.mockTariffRepo.On("GetActiveUserTariff", mock.Anything, userID).Return(nil, nil).Once()
		freshComps.mockTariffRepo.On("GetDefaultActiveTariff", mock.Anything).Return(nil, nil).Once() // No default found

		_, _, err := freshComps.service.CalculateSMSCost(context.Background(), userID, numMessages)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no applicable tariff found")
		freshComps.mockTariffRepo.AssertExpectations(t)
	})

    t.Run("ErrorFetchingUserTariff", func(t *testing.T) {
        freshComps := setupBillingAppTest(t)
        dbErr := errors.New("db error fetching user tariff")
        freshComps.mockTariffRepo.On("GetActiveUserTariff", mock.Anything, userID).Return(nil, dbErr).Once()

        _, _, err := freshComps.service.CalculateSMSCost(context.Background(), userID, numMessages)
        assert.Error(t, err)
        assert.Contains(t, err.Error(), dbErr.Error())
        freshComps.mockTariffRepo.AssertExpectations(t)
    })

    t.Run("ErrorFetchingDefaultTariff", func(t *testing.T) {
        freshComps := setupBillingAppTest(t)
        dbErr := errors.New("db error fetching default tariff")
        freshComps.mockTariffRepo.On("GetActiveUserTariff", mock.Anything, userID).Return(nil, nil).Once()
        freshComps.mockTariffRepo.On("GetDefaultActiveTariff", mock.Anything).Return(nil, dbErr).Once()

        _, _, err := freshComps.service.CalculateSMSCost(context.Background(), userID, numMessages)
        assert.Error(t, err)
        assert.Contains(t, err.Error(), dbErr.Error())
        freshComps.mockTariffRepo.AssertExpectations(t)
    })

	t.Run("InactiveTariffSelected", func(t *testing.T) {
        // This scenario depends on repository methods returning only active tariffs.
        // If GetActiveUserTariff or GetDefaultActiveTariff could return an inactive one (which they shouldn't based on their SQL),
        // then CalculateSMSCost's explicit IsActive check would be hit.
        // For now, repository methods are expected to only return active ones.
        // If a test needs to check the service layer's IsActive check, the mock repo would return an Inactive tariff.
		freshComps := setupBillingAppTest(t)
        inactiveTariff := &domain.Tariff{ID: uuid.New(), Name: "InactiveSpecial", PricePerSMS: 10, Currency: "USD", IsActive: false}
        freshComps.mockTariffRepo.On("GetActiveUserTariff", mock.Anything, userID).Return(inactiveTariff, nil).Once()

        _, _, err := freshComps.service.CalculateSMSCost(context.Background(), userID, numMessages)
        assert.Error(t, err)
        assert.Contains(t, err.Error(), "selected tariff 'InactiveSpecial' is not active")
        freshComps.mockTariffRepo.AssertExpectations(t)
    })
}

func TestBillingService_DeductCreditForSMS_Success(t *testing.T) {
    comps := setupBillingAppTest(t)
    userID := uuid.New()
    numMessages := 2
    messageRefID := "msg_" + uuid.New().String()

    userTariff := &domain.Tariff{ID: uuid.New(), Name: "Standard", PricePerSMS: 75, Currency: "XYZ", IsActive: true} // 75 units per SMS
    expectedCost := int64(150) // 75 * 2
	expectedCostFloat := float64(expectedCost)

    mockUser := &userDomain.User{ID: userID.String(), Username: "costlyUser", CreditBalance: 500.0, CurrencyCode: "XYZ"}

	// Mock DB transaction
	comps.mockDbPool.On("BeginFunc", mock.Anything, mock.AnythingOfType("func(pgx.Tx) error")).
		Run(func(args mock.Arguments) {
			fn := args.Get(1).(func(pgx.Tx) error)
			assert.NoError(t, fn(nil)) // Expect inner function to succeed
		}).Return(nil).Once() // Overall transaction succeeds


    // Mock GetActiveUserTariff (used by CalculateSMSCost)
    comps.mockTariffRepo.On("GetActiveUserTariff", mock.Anything, userID).Return(userTariff, nil).Once()
    // Mock GetUserByID (for BalanceBefore)
    comps.mockUserRepo.On("GetByID", mock.Anything, userID.String()).Return(mockUser, nil).Once()
    // Mock UserRepo.Update (gRPC call to user-service)
    comps.mockUserRepo.On("Update", mock.Anything, mock.MatchedBy(func(u *userDomain.User) bool {
        return u.ID == userID.String() && u.CreditBalance == (mockUser.CreditBalance - expectedCostFloat)
    })).Return(nil).Once()
    // Mock TransactionRepo.Create
    comps.mockTxnRepo.On("Create", mock.Anything, mock.Anything, mock.MatchedBy(func(txn *domain.Transaction) bool {
        return txn.UserID == userID.String() &&
               txn.Type == domain.TransactionTypeDebit &&
               txn.Amount == expectedCostFloat &&
               txn.CurrencyCode == userTariff.Currency &&
               txn.BalanceBefore == mockUser.CreditBalance &&
               txn.BalanceAfter == (mockUser.CreditBalance - expectedCostFloat) &&
			   txn.Status == domain.TransactionStatusCompleted && // Verify Status
               txn.RelatedMessageID != nil && *txn.RelatedMessageID == messageRefID
    })).Return(&domain.Transaction{ID: uuid.NewString()}, nil).Once()


    transactionDetails := domain.TransactionDetails{
        Description: "Charge for SMS",
        ReferenceID: messageRefID,
    }
    createdTx, err := comps.service.DeductCreditForSMS(context.Background(), userID, numMessages, transactionDetails)

    assert.NoError(t, err)
    assert.NotNil(t, createdTx)
    comps.mockTariffRepo.AssertExpectations(t)
    comps.mockUserRepo.AssertExpectations(t)
    comps.mockTxnRepo.AssertExpectations(t)
    comps.mockDbPool.AssertExpectations(t)
}

// TODO: Add more tests for DeductCreditForSMS:
// - Cost calculation fails (no tariff found, etc.)
// - User not found by userRepo.GetByID
// - Insufficient credit
// - userRepo.Update (gRPC to user-service) fails
// - transactionRepo.Create fails
// - DB transaction commit fails (mockDbPool.BeginFunc returns error on its own)
```
