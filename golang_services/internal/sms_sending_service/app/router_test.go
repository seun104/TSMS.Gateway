package app

import (
	"context"
	"errors"
	"testing"
	"log/slog"
	"io"

	"github.com/aradsms/golang_services/internal/sms_sending_service/domain"
	"github.com/aradsms/golang_services/internal/sms_sending_service/provider"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockRouteRepository is a mock implementation of domain.RouteRepository
type MockRouteRepository struct {
	mock.Mock
}

func (m *MockRouteRepository) GetActiveRoutesOrderedByPriority(ctx context.Context) ([]*domain.Route, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Route), args.Error(1)
}

// MockSMSSenderProvider is a mock implementation of provider.SMSSenderProvider
type MockSMSSenderProvider struct {
	mock.Mock
	Name string
}

func (m *MockSMSSenderProvider) Send(ctx context.Context, details provider.SendRequestDetails) (*provider.SendResponseDetails, error) {
	args := m.Called(ctx, details)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*provider.SendResponseDetails), args.Error(1)
}

func (m *MockSMSSenderProvider) GetName() string {
	return m.Name
}

func TestRouter_SelectProvider_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil)) // Discard logs for cleaner test output
	mockRepo := new(MockRouteRepository)

	providerA := &MockSMSSenderProvider{Name: "ProviderA"}
	providerB := &MockSMSSenderProvider{Name: "ProviderB"}
	providersMap := map[string]provider.SMSSenderProvider{
		"ProviderA": providerA,
		"ProviderB": providerB,
	}

	router := NewRouter(mockRepo, providersMap, logger)

	userID := uuid.New()
	countryCodeIran := "98"
	operatorPrefixMCI := "912"

	routes := []*domain.Route{
		{ // Route 1: Specific to user, country, operator (highest priority)
			ID: uuid.New(), Name: "UserSpecificMCI", Priority: 1,
			Criteria: domain.RouteCriteria{UserID: strPtr(userID.String()), CountryCode: &countryCodeIran, OperatorPrefix: &operatorPrefixMCI},
			SmsProviderName: "ProviderA", IsActive: true,
		},
		{ // Route 2: Specific to country and operator
			ID: uuid.New(), Name: "IranMCI", Priority: 10,
			Criteria: domain.RouteCriteria{CountryCode: &countryCodeIran, OperatorPrefix: &operatorPrefixMCI},
			SmsProviderName: "ProviderB", IsActive: true,
		},
		{ // Route 3: Specific to country only
			ID: uuid.New(), Name: "IranGeneric", Priority: 20,
			Criteria: domain.RouteCriteria{CountryCode: &countryCodeIran},
			SmsProviderName: "ProviderA", IsActive: true,
		},
	}

	mockRepo.On("GetActiveRoutesOrderedByPriority", mock.Anything).Return(routes, nil)

	// Test case 1: Matches Route 1 (most specific)
	selected, err := router.SelectProvider(context.Background(), "+989121234567", userID.String())
	assert.NoError(t, err)
	assert.NotNil(t, selected)
	assert.Equal(t, "ProviderA", selected.GetName())

	// Test case 2: Matches Route 2 (user doesn't match, but country/operator do)
	otherUserID := uuid.New().String()
	selected, err = router.SelectProvider(context.Background(), "+989127654321", otherUserID)
	assert.NoError(t, err)
	assert.NotNil(t, selected)
	assert.Equal(t, "ProviderB", selected.GetName())

	// Test case 3: Matches Route 3 (only country matches, different operator)
	selected, err = router.SelectProvider(context.Background(), "+989351111111", otherUserID)
	assert.NoError(t, err)
	assert.NotNil(t, selected)
	assert.Equal(t, "ProviderA", selected.GetName())

	// Test case 4: No specific route matches (e.g., different country)
	selected, err = router.SelectProvider(context.Background(), "+15551234567", userID.String())
	assert.NoError(t, err)
	assert.Nil(t, selected) // Expect nil as no route matches US number

	mockRepo.AssertExpectations(t)
}

func TestRouter_SelectProvider_NoRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockRepo := new(MockRouteRepository)
	router := NewRouter(mockRepo, make(map[string]provider.SMSSenderProvider), logger)

	mockRepo.On("GetActiveRoutesOrderedByPriority", mock.Anything).Return([]*domain.Route{}, nil)

	selected, err := router.SelectProvider(context.Background(), "+989121234567", uuid.New().String())
	assert.NoError(t, err)
	assert.Nil(t, selected)
	mockRepo.AssertExpectations(t)
}

func TestRouter_SelectProvider_RepoError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockRepo := new(MockRouteRepository)
	router := NewRouter(mockRepo, make(map[string]provider.SMSSenderProvider), logger)

	mockRepo.On("GetActiveRoutesOrderedByPriority", mock.Anything).Return(nil, errors.New("database error"))

	selected, err := router.SelectProvider(context.Background(), "+989121234567", uuid.New().String())
	assert.Error(t, err)
	assert.Nil(t, selected)
	assert.Contains(t, err.Error(), "database error")
	mockRepo.AssertExpectations(t)
}

func TestRouter_SelectProvider_ProviderNotFoundInMap(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockRepo := new(MockRouteRepository)

	// providersMap is empty, but route refers to "ProviderA"
	providersMap := make(map[string]provider.SMSSenderProvider)
	router := NewRouter(mockRepo, providersMap, logger)

	countryCode := "98"
	routes := []*domain.Route{
		{
			ID: uuid.New(), Name: "ValidRouteMissingProvider", Priority: 1,
			Criteria: domain.RouteCriteria{CountryCode: &countryCode},
			SmsProviderName: "ProviderA", // This provider is not in providersMap
			IsActive: true,
		},
	}
	mockRepo.On("GetActiveRoutesOrderedByPriority", mock.Anything).Return(routes, nil)

	selected, err := router.SelectProvider(context.Background(), "+989121234567", uuid.New().String())
	assert.NoError(t, err)
	assert.Nil(t, selected) // Should be nil because provider instance "ProviderA" is missing
	mockRepo.AssertExpectations(t)
}


// Helper function to get a pointer to a string
func strPtr(s string) *string {
	return &s
}

func TestRouter_matches(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(nil, nil, logger) // Repo and providers not needed for testing matches func directly

	userID1 := uuid.New()
	userID1Str := userID1.String()

	ccIran := "98"
	opMCI := "912"
	opIrancell := "935"
	ccUS := "1"

	testCases := []struct {
		name      string
		recipient string
		userID    uuid.UUID
		criteria  domain.RouteCriteria
		expected  bool
	}{
		{"Exact match: CC, OP, User", "+989121234567", userID1, domain.RouteCriteria{CountryCode: &ccIran, OperatorPrefix: &opMCI, UserID: &userID1Str}, true},
		{"Match: CC, OP; User different", "+989121234567", uuid.New(), domain.RouteCriteria{CountryCode: &ccIran, OperatorPrefix: &opMCI, UserID: &userID1Str}, false},
		{"Match: CC, OP only", "+989121234567", uuid.Nil, domain.RouteCriteria{CountryCode: &ccIran, OperatorPrefix: &opMCI}, true},
		{"Match: CC only", "+989351234567", uuid.Nil, domain.RouteCriteria{CountryCode: &ccIran}, true},
		{"No Match: CC different", "+19121234567", uuid.Nil, domain.RouteCriteria{CountryCode: &ccIran}, false},
		{"Match: OP only (no CC in criteria)", "9121234567", uuid.Nil, domain.RouteCriteria{OperatorPrefix: &opMCI}, true},
		{"No Match: OP different", "+989351234567", uuid.Nil, domain.RouteCriteria{CountryCode: &ccIran, OperatorPrefix: &opMCI}, false},
		{"Match: UserID only", "+447123456789", userID1, domain.RouteCriteria{UserID: &userID1Str}, true},
		{"No Match: UserID different", "+447123456789", uuid.New(), domain.RouteCriteria{UserID: &userID1Str}, false},
		{"Empty criteria", "+123456789", uuid.New(), domain.RouteCriteria{}, true},
		{"Recipient without +, CC match", "989121234567", uuid.Nil, domain.RouteCriteria{CountryCode: &ccIran, OperatorPrefix: &opMCI}, true},
		{"Recipient without +, CC and OP match", "989121234567", uuid.Nil, domain.RouteCriteria{CountryCode: &ccIran, OperatorPrefix: &opMCI}, true},
		{"Recipient with +, CC and OP in criteria, OP doesn't match after stripping CC", "+989351234567", uuid.Nil, domain.RouteCriteria{CountryCode: &ccIran, OperatorPrefix: &opMCI}, false},
        {"Criteria with CC and OP, recipient only matches CC", "+989001234567", uuid.Nil, domain.RouteCriteria{CountryCode: &ccIran, OperatorPrefix: &opMCI}, false},
		{"Criteria with OP only, recipient has CC but matches OP after CC", "+989121234567", uuid.Nil, domain.RouteCriteria{OperatorPrefix: &opMCI}, true},
		{"Criteria with US CC", "+15551234567", uuid.Nil, domain.RouteCriteria{CountryCode: &ccUS}, true},
		{"Criteria with US CC and OP", "+15551234567", uuid.Nil, domain.RouteCriteria{CountryCode: &ccUS, OperatorPrefix: strPtr("555")}, true},
		{"No Match: UserID criteria with nil message userID", "+123", uuid.Nil, domain.RouteCriteria{UserID: &userID1Str}, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, router.matches(tc.recipient, tc.userID, tc.criteria))
		})
	}
}
