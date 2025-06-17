package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/AradIT/aradsms/golang_services/internal/phonebook_service/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type MockPhonebookRepository struct {
	mock.Mock
}

func (m *MockPhonebookRepository) Create(ctx context.Context, pb *domain.Phonebook) error {
	args := m.Called(ctx, pb)
	return args.Error(0)
}

func (m *MockPhonebookRepository) GetByID(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*domain.Phonebook, error) {
	args := m.Called(ctx, id, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Phonebook), args.Error(1)
}

func (m *MockPhonebookRepository) ListByUserID(ctx context.Context, userID uuid.UUID, offset, limit int) ([]*domain.Phonebook, error) {
	args := m.Called(ctx, userID, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Phonebook), args.Error(1)
}

func (m *MockPhonebookRepository) Update(ctx context.Context, pb *domain.Phonebook) error {
	args := m.Called(ctx, pb)
	return args.Error(0)
}

func (m *MockPhonebookRepository) Delete(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	args := m.Called(ctx, id, userID)
	return args.Error(0)
}

type MockContactRepository struct {
	mock.Mock
}

func (m *MockContactRepository) Create(ctx context.Context, c *domain.Contact) error {
	args := m.Called(ctx, c)
	return args.Error(0)
}

func (m *MockContactRepository) GetByID(ctx context.Context, id uuid.UUID, phonebookID uuid.UUID) (*domain.Contact, error) {
	args := m.Called(ctx, id, phonebookID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Contact), args.Error(1)
}

func (m *MockContactRepository) ListByPhonebookID(ctx context.Context, phonebookID uuid.UUID, offset, limit int) ([]*domain.Contact, error) {
	args := m.Called(ctx, phonebookID, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Contact), args.Error(1)
}

func (m *MockContactRepository) Update(ctx context.Context, c *domain.Contact) error {
	args := m.Called(ctx, c)
	return args.Error(0)
}

func (m *MockContactRepository) Delete(ctx context.Context, id uuid.UUID, phonebookID uuid.UUID) error {
	args := m.Called(ctx, id, phonebookID)
	return args.Error(0)
}

func (m *MockContactRepository) FindByNumberInPhonebook(ctx context.Context, number string, phonebookID uuid.UUID) (*domain.Contact, error) {
	args := m.Called(ctx, number, phonebookID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Contact), args.Error(1)
}

// --- Test Setup ---
type phonebookAppTestComponents struct {
	app        *Application
	mockPbRepo *MockPhonebookRepository
	mockCtRepo *MockContactRepository
	logger     *slog.Logger
}

func setupPhonebookAppTest(t *testing.T) phonebookAppTestComponents {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockPbRepo := new(MockPhonebookRepository)
	mockCtRepo := new(MockContactRepository)

	app := NewApplication(
		mockPbRepo,
		mockCtRepo,
		logger,
	)
	return phonebookAppTestComponents{
		app:        app,
		mockPbRepo: mockPbRepo,
		mockCtRepo: mockCtRepo,
		logger:     logger,
	}
}

// --- Phonebook Method Tests ---

func TestApplication_CreatePhonebook(t *testing.T) {
	comps := setupPhonebookAppTest(t)
	ctx := context.Background()
	userID := uuid.New()
	name := "Test Phonebook"
	description := "A test phonebook"

	t.Run("Success", func(t *testing.T) {
		comps.mockPbRepo.On("Create", ctx, mock.MatchedBy(func(pb *domain.Phonebook) bool {
			return pb.UserID == userID && pb.Name == name && pb.Description == description
		})).Return(nil).Once()

		pb, err := comps.app.CreatePhonebook(ctx, userID, name, description)

		require.NoError(t, err)
		require.NotNil(t, pb)
		assert.Equal(t, name, pb.Name)
		assert.Equal(t, userID, pb.UserID)
		comps.mockPbRepo.AssertExpectations(t)
	})

	t.Run("RepoError", func(t *testing.T) {
		expectedErr := errors.New("repo create error")
		comps.mockPbRepo.On("Create", ctx, mock.AnythingOfType("*domain.Phonebook")).Return(expectedErr).Once()

		pb, err := comps.app.CreatePhonebook(ctx, userID, name, description)

		require.Error(t, err)
		assert.Nil(t, pb)
		assert.Equal(t, expectedErr, err)
		comps.mockPbRepo.AssertExpectations(t)
	})
}

func TestApplication_GetPhonebook(t *testing.T) {
	comps := setupPhonebookAppTest(t)
	ctx := context.Background()
	pbID := uuid.New()
	userID := uuid.New()

	t.Run("SuccessFound", func(t *testing.T) {
		expectedPb := &domain.Phonebook{ID: pbID, UserID: userID, Name: "Found PB"}
		comps.mockPbRepo.On("GetByID", ctx, pbID, userID).Return(expectedPb, nil).Once()

		pb, err := comps.app.GetPhonebook(ctx, pbID, userID)
		require.NoError(t, err)
		assert.Equal(t, expectedPb, pb)
		comps.mockPbRepo.AssertExpectations(t)
	})

	t.Run("NotFound", func(t *testing.T) {
		comps.mockPbRepo.On("GetByID", ctx, pbID, userID).Return(nil, domain.ErrPhonebookNotFound).Once()
		pb, err := comps.app.GetPhonebook(ctx, pbID, userID)
		require.ErrorIs(t, err, domain.ErrPhonebookNotFound)
		assert.Nil(t, pb)
		comps.mockPbRepo.AssertExpectations(t)
	})
}

func TestApplication_ListPhonebooks(t *testing.T) {
	comps := setupPhonebookAppTest(t)
	ctx := context.Background()
	userID := uuid.New()

	t.Run("SuccessWithItems", func(t *testing.T) {
		expectedPbs := []*domain.Phonebook{{ID: uuid.New(), UserID: userID, Name: "PB1"}}
		comps.mockPbRepo.On("ListByUserID", ctx, userID, 0, 10).Return(expectedPbs, nil).Once()

		pbs, count, err := comps.app.ListPhonebooks(ctx, userID, 0, 10)

		require.NoError(t, err)
		assert.Equal(t, expectedPbs, pbs)
		assert.Equal(t, int64(len(expectedPbs)), count) // App layer uses len() for count
		comps.mockPbRepo.AssertExpectations(t)
	})
}

// --- Contact Method Tests --- (Example for CreateContact)

func TestApplication_CreateContact(t *testing.T) {
	comps := setupPhonebookAppTest(t)
	ctx := context.Background()
	pbID := uuid.New()
	number := "1234567890"

	t.Run("Success", func(t *testing.T) {
		comps.mockCtRepo.On("Create", ctx, mock.MatchedBy(func(c *domain.Contact) bool {
			return c.PhonebookID == pbID && c.Number == number
		})).Return(nil).Once()

		contact, err := comps.app.CreateContact(ctx, pbID, number, "John", "Doe", "john@example.com", nil, true)

		require.NoError(t, err)
		require.NotNil(t, contact)
		assert.Equal(t, number, contact.Number)
		comps.mockCtRepo.AssertExpectations(t)
	})

	t.Run("RepoError", func(t *testing.T) {
		expectedErr := errors.New("contact create error")
		comps.mockCtRepo.On("Create", ctx, mock.AnythingOfType("*domain.Contact")).Return(expectedErr).Once()

		contact, err := comps.app.CreateContact(ctx, pbID, number, "John", "Doe", "john@example.com", nil, true)

		require.Error(t, err)
		assert.Nil(t, contact)
		assert.Equal(t, expectedErr, err)
		comps.mockCtRepo.AssertExpectations(t)
	})
}

// Add more tests for other Phonebook and Contact methods (Update, Delete, List, GetContact, FindContactByNumberInPhonebook)
// following similar patterns.
// For Update methods, remember to mock GetByID first, then Update.
// For Delete methods, ensure GetByID (for ownership) is mocked if applicable in app logic before Delete.
// For ListContacts, similar to ListPhonebooks.
// For FindContactByNumberInPhonebook, test found and not found cases.
// Remember to reset mock expectations for each sub-test if `comps` is reused across them without re-initialization.
// Manual reset: comps.mockPbRepo.ExpectedCalls = nil; comps.mockCtRepo.ExpectedCalls = nil
// Or ensure each t.Run uses its own `comps := setupPhonebookAppTest(t)` if state needs to be fully isolated.
// The current setup creates fresh mocks for each top-level Test function, but sub-tests share `comps`.
// So, manual reset or specific mock.On().Once() per sub-test is crucial.
// The .Once() helps manage this for simple cases.
```

Now, I also need to provide the diff to correct the import paths in `phonebook_app.go`.
