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
		comps.mockPbRepo.ExpectedCalls = nil // Reset for sub-test
		expectedPbs := []*domain.Phonebook{{ID: uuid.New(), UserID: userID, Name: "PB1"}}
		comps.mockPbRepo.On("ListByUserID", ctx, userID, 0, 10).Return(expectedPbs, nil).Once()

		pbs, count, err := comps.app.ListPhonebooks(ctx, userID, 0, 10)

		require.NoError(t, err)
		assert.Equal(t, expectedPbs, pbs)
		assert.Equal(t, int64(len(expectedPbs)), count)
		comps.mockPbRepo.AssertExpectations(t)
	})

	t.Run("SuccessEmptyList", func(t *testing.T) {
		comps.mockPbRepo.ExpectedCalls = nil // Reset for sub-test
		comps.mockPbRepo.On("ListByUserID", ctx, userID, 0, 10).Return([]*domain.Phonebook{}, nil).Once()

		pbs, count, err := comps.app.ListPhonebooks(ctx, userID, 0, 10)

		require.NoError(t, err)
		assert.Empty(t, pbs)
		assert.Equal(t, int64(0), count)
		comps.mockPbRepo.AssertExpectations(t)
	})

	t.Run("RepoError", func(t *testing.T) {
		comps.mockPbRepo.ExpectedCalls = nil // Reset for sub-test
		expectedErr := errors.New("repo list error")
		comps.mockPbRepo.On("ListByUserID", ctx, userID, 0, 10).Return(nil, expectedErr).Once()

		pbs, count, err := comps.app.ListPhonebooks(ctx, userID, 0, 10)

		require.Error(t, err)
		assert.Nil(t, pbs)
		assert.Equal(t, int64(0), count)
		assert.Equal(t, expectedErr, err)
		comps.mockPbRepo.AssertExpectations(t)
	})
}

func TestApplication_UpdatePhonebook(t *testing.T) {
	comps := setupPhonebookAppTest(t)
	ctx := context.Background()
	pbID := uuid.New()
	userID := uuid.New()
	originalName := "Original Name"
	updatedName := "Updated Name"
	description := "Description"

	existingPb := &domain.Phonebook{
		ID:          pbID,
		UserID:      userID,
		Name:        originalName,
		Description: "Old Description",
		CreatedAt:   time.Now().Add(-time.Hour),
		UpdatedAt:   time.Now().Add(-time.Hour),
	}

	t.Run("Success", func(t *testing.T) {
		comps.mockPbRepo.ExpectedCalls = nil
		comps.mockCtRepo.ExpectedCalls = nil
		comps.mockPbRepo.On("GetByID", ctx, pbID, userID).Return(existingPb, nil).Once()
		comps.mockPbRepo.On("Update", ctx, mock.MatchedBy(func(pb *domain.Phonebook) bool {
			return pb.ID == pbID && pb.Name == updatedName && pb.Description == description
		})).Return(nil).Once()

		updatedPb, err := comps.app.UpdatePhonebook(ctx, pbID, userID, updatedName, description)

		require.NoError(t, err)
		require.NotNil(t, updatedPb)
		assert.Equal(t, updatedName, updatedPb.Name)
		assert.Equal(t, description, updatedPb.Description)
		comps.mockPbRepo.AssertExpectations(t)
	})

	t.Run("NotFound", func(t *testing.T) {
		comps.mockPbRepo.ExpectedCalls = nil
		comps.mockCtRepo.ExpectedCalls = nil
		comps.mockPbRepo.On("GetByID", ctx, pbID, userID).Return(nil, domain.ErrPhonebookNotFound).Once()

		_, err := comps.app.UpdatePhonebook(ctx, pbID, userID, updatedName, description)

		require.ErrorIs(t, err, domain.ErrPhonebookNotFound)
		comps.mockPbRepo.AssertExpectations(t)
		comps.mockPbRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	})

	t.Run("RepoErrorOnGet", func(t *testing.T) {
		comps.mockPbRepo.ExpectedCalls = nil
		comps.mockCtRepo.ExpectedCalls = nil
		expectedErr := errors.New("repo get error")
		comps.mockPbRepo.On("GetByID", ctx, pbID, userID).Return(nil, expectedErr).Once()

		_, err := comps.app.UpdatePhonebook(ctx, pbID, userID, updatedName, description)

		require.Error(t, err)
		assert.Equal(t, expectedErr, err)
		comps.mockPbRepo.AssertExpectations(t)
	})

	t.Run("RepoErrorOnUpdate", func(t *testing.T) {
		comps.mockPbRepo.ExpectedCalls = nil
		comps.mockCtRepo.ExpectedCalls = nil
		expectedErr := errors.New("repo update error")
		comps.mockPbRepo.On("GetByID", ctx, pbID, userID).Return(existingPb, nil).Once()
		comps.mockPbRepo.On("Update", ctx, mock.AnythingOfType("*domain.Phonebook")).Return(expectedErr).Once()

		_, err := comps.app.UpdatePhonebook(ctx, pbID, userID, updatedName, description)

		require.Error(t, err)
		assert.Equal(t, expectedErr, err)
		comps.mockPbRepo.AssertExpectations(t)
	})
}

func TestApplication_DeletePhonebook(t *testing.T) {
	comps := setupPhonebookAppTest(t)
	ctx := context.Background()
	pbID := uuid.New()
	userID := uuid.New()

	// Note: The current app.DeletePhonebook only calls phonebookRepo.Delete.
	// It does not call GetByID first, nor does it delete associated contacts.
	// Tests will reflect this current implementation. If behavior changes, tests need updates.

	t.Run("Success", func(t *testing.T) {
		comps.mockPbRepo.ExpectedCalls = nil
		comps.mockCtRepo.ExpectedCalls = nil
		comps.mockPbRepo.On("Delete", ctx, pbID, userID).Return(nil).Once()
		// If app.DeletePhonebook were to also delete contacts:
		// comps.mockCtRepo.On("DeleteByPhonebookID", ctx, pbID).Return(nil).Once()


		err := comps.app.DeletePhonebook(ctx, pbID, userID)
		require.NoError(t, err)
		comps.mockPbRepo.AssertExpectations(t)
		// comps.mockCtRepo.AssertExpectations(t) // If contacts were deleted
	})

	t.Run("NotFoundOrAuthError", func(t *testing.T) { // Repo Delete might return ErrPhonebookNotFound if GORM hook or similar used for auth
		comps.mockPbRepo.ExpectedCalls = nil
		comps.mockCtRepo.ExpectedCalls = nil
		comps.mockPbRepo.On("Delete", ctx, pbID, userID).Return(domain.ErrPhonebookNotFound).Once()

		err := comps.app.DeletePhonebook(ctx, pbID, userID)
		require.ErrorIs(t, err, domain.ErrPhonebookNotFound)
		comps.mockPbRepo.AssertExpectations(t)
	})

	t.Run("RepoErrorOnDelete", func(t *testing.T) {
		comps.mockPbRepo.ExpectedCalls = nil
		comps.mockCtRepo.ExpectedCalls = nil
		expectedErr := errors.New("repo delete error")
		comps.mockPbRepo.On("Delete", ctx, pbID, userID).Return(expectedErr).Once()

		err := comps.app.DeletePhonebook(ctx, pbID, userID)
		require.Error(t, err)
		assert.Equal(t, expectedErr, err)
		comps.mockPbRepo.AssertExpectations(t)
	})
}


// --- Contact Method Tests ---

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


func TestApplication_GetContact(t *testing.T) {
	comps := setupPhonebookAppTest(t)
	ctx := context.Background()
	contactID := uuid.New()
	pbID := uuid.New()

	t.Run("SuccessFound", func(t *testing.T) {
		comps.mockPbRepo.ExpectedCalls = nil; comps.mockCtRepo.ExpectedCalls = nil
		expectedContact := &domain.Contact{ID: contactID, PhonebookID: pbID, Number: "111"}
		comps.mockCtRepo.On("GetByID", ctx, contactID, pbID).Return(expectedContact, nil).Once()

		contact, err := comps.app.GetContact(ctx, contactID, pbID)
		require.NoError(t, err)
		assert.Equal(t, expectedContact, contact)
		comps.mockCtRepo.AssertExpectations(t)
	})

	t.Run("NotFound", func(t *testing.T) {
		comps.mockPbRepo.ExpectedCalls = nil; comps.mockCtRepo.ExpectedCalls = nil
		comps.mockCtRepo.On("GetByID", ctx, contactID, pbID).Return(nil, domain.ErrContactNotFound).Once()
		contact, err := comps.app.GetContact(ctx, contactID, pbID)
		require.ErrorIs(t, err, domain.ErrContactNotFound)
		assert.Nil(t, contact)
		comps.mockCtRepo.AssertExpectations(t)
	})

	t.Run("RepoError", func(t *testing.T) {
		comps.mockPbRepo.ExpectedCalls = nil; comps.mockCtRepo.ExpectedCalls = nil
		expectedErr := errors.New("repo get error")
		comps.mockCtRepo.On("GetByID", ctx, contactID, pbID).Return(nil, expectedErr).Once()
		contact, err := comps.app.GetContact(ctx, contactID, pbID)
		require.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.Nil(t, contact)
		comps.mockCtRepo.AssertExpectations(t)
	})
}

func TestApplication_ListContacts(t *testing.T) {
	comps := setupPhonebookAppTest(t)
	ctx := context.Background()
	pbID := uuid.New()
	// Note: The current app.ListContacts does not verify phonebook ownership by userID.
	// It directly calls contactRepo.ListByPhonebookID.
	// If ownership check is added to app.ListContacts, these tests would need to mock pbRepo.GetByID as well.

	t.Run("SuccessWithItems", func(t *testing.T) {
		comps.mockPbRepo.ExpectedCalls = nil; comps.mockCtRepo.ExpectedCalls = nil
		expectedContacts := []*domain.Contact{{ID: uuid.New(), PhonebookID: pbID, Number: "222"}}
		comps.mockCtRepo.On("ListByPhonebookID", ctx, pbID, 0, 10).Return(expectedContacts, nil).Once()

		contacts, count, err := comps.app.ListContacts(ctx, pbID, 0, 10)

		require.NoError(t, err)
		assert.Equal(t, expectedContacts, contacts)
		assert.Equal(t, int64(len(expectedContacts)), count)
		comps.mockCtRepo.AssertExpectations(t)
	})

	t.Run("SuccessEmptyList", func(t *testing.T) {
		comps.mockPbRepo.ExpectedCalls = nil; comps.mockCtRepo.ExpectedCalls = nil
		comps.mockCtRepo.On("ListByPhonebookID", ctx, pbID, 0, 10).Return([]*domain.Contact{}, nil).Once()
		contacts, count, err := comps.app.ListContacts(ctx, pbID, 0, 10)
		require.NoError(t, err)
		assert.Empty(t, contacts)
		assert.Equal(t, int64(0), count)
		comps.mockCtRepo.AssertExpectations(t)
	})

	t.Run("RepoErrorContactList", func(t *testing.T) {
		comps.mockPbRepo.ExpectedCalls = nil; comps.mockCtRepo.ExpectedCalls = nil
		expectedErr := errors.New("repo list contacts error")
		comps.mockCtRepo.On("ListByPhonebookID", ctx, pbID, 0, 10).Return(nil, expectedErr).Once()
		contacts, count, err := comps.app.ListContacts(ctx, pbID, 0, 10)
		require.Error(t, err)
		assert.Nil(t, contacts)
		assert.Equal(t, int64(0), count)
		assert.Equal(t, expectedErr, err)
		comps.mockCtRepo.AssertExpectations(t)
	})
}

func TestApplication_UpdateContact(t *testing.T) {
	comps := setupPhonebookAppTest(t)
	ctx := context.Background()
	contactID := uuid.New()
	pbID := uuid.New()
	updatedNumber := "9876543210"

	existingContact := &domain.Contact{
		ID:          contactID,
		PhonebookID: pbID,
		Number:      "1234567890",
		FirstName:   "OldFirst",
	}

	t.Run("Success", func(t *testing.T) {
		comps.mockPbRepo.ExpectedCalls = nil; comps.mockCtRepo.ExpectedCalls = nil
		comps.mockCtRepo.On("GetByID", ctx, contactID, pbID).Return(existingContact, nil).Once()
		comps.mockCtRepo.On("Update", ctx, mock.MatchedBy(func(c *domain.Contact) bool {
			return c.ID == contactID && c.Number == updatedNumber
		})).Return(nil).Once()

		updatedContact, err := comps.app.UpdateContact(ctx, contactID, pbID, updatedNumber, "NewFirst", "NewLast", "new@example.com", map[string]string{"custom": "val"}, false)

		require.NoError(t, err)
		require.NotNil(t, updatedContact)
		assert.Equal(t, updatedNumber, updatedContact.Number)
		assert.Equal(t, "NewFirst", updatedContact.FirstName)
		comps.mockCtRepo.AssertExpectations(t)
	})

	t.Run("NotFound", func(t *testing.T) {
		comps.mockPbRepo.ExpectedCalls = nil; comps.mockCtRepo.ExpectedCalls = nil
		comps.mockCtRepo.On("GetByID", ctx, contactID, pbID).Return(nil, domain.ErrContactNotFound).Once()
		_, err := comps.app.UpdateContact(ctx, contactID, pbID, updatedNumber, "", "", "", nil, false)
		require.ErrorIs(t, err, domain.ErrContactNotFound)
		comps.mockCtRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
		comps.mockCtRepo.AssertExpectations(t)
	})
}

func TestApplication_DeleteContact(t *testing.T) {
	comps := setupPhonebookAppTest(t)
	ctx := context.Background()
	contactID := uuid.New()
	pbID := uuid.New()
	// Note: app.DeleteContact calls repo.Delete directly without a GetByID first.

	t.Run("Success", func(t *testing.T) {
		comps.mockPbRepo.ExpectedCalls = nil; comps.mockCtRepo.ExpectedCalls = nil
		comps.mockCtRepo.On("Delete", ctx, contactID, pbID).Return(nil).Once()
		err := comps.app.DeleteContact(ctx, contactID, pbID)
		require.NoError(t, err)
		comps.mockCtRepo.AssertExpectations(t)
	})

	t.Run("NotFound", func(t *testing.T) { // Assuming repo.Delete returns ErrContactNotFound
		comps.mockPbRepo.ExpectedCalls = nil; comps.mockCtRepo.ExpectedCalls = nil
		comps.mockCtRepo.On("Delete", ctx, contactID, pbID).Return(domain.ErrContactNotFound).Once()
		err := comps.app.DeleteContact(ctx, contactID, pbID)
		require.ErrorIs(t, err, domain.ErrContactNotFound)
		comps.mockCtRepo.AssertExpectations(t)
	})
}

func TestApplication_FindContactByNumberInPhonebook(t *testing.T) {
	comps := setupPhonebookAppTest(t)
	ctx := context.Background()
	number := "12345"
	pbID := uuid.New()

	t.Run("SuccessFound", func(t *testing.T) {
		comps.mockPbRepo.ExpectedCalls = nil; comps.mockCtRepo.ExpectedCalls = nil
		expectedContact := &domain.Contact{ID: uuid.New(), PhonebookID: pbID, Number: number}
		comps.mockCtRepo.On("FindByNumberInPhonebook", ctx, number, pbID).Return(expectedContact, nil).Once()
		contact, err := comps.app.FindContactByNumberInPhonebook(ctx, number, pbID)
		require.NoError(t, err)
		assert.Equal(t, expectedContact, contact)
		comps.mockCtRepo.AssertExpectations(t)
	})

	t.Run("NotFound", func(t *testing.T) {
		comps.mockPbRepo.ExpectedCalls = nil; comps.mockCtRepo.ExpectedCalls = nil
		comps.mockCtRepo.On("FindByNumberInPhonebook", ctx, number, pbID).Return(nil, domain.ErrContactNotFound).Once()
		contact, err := comps.app.FindContactByNumberInPhonebook(ctx, number, pbID)
		require.ErrorIs(t, err, domain.ErrContactNotFound)
		assert.Nil(t, contact)
		comps.mockCtRepo.AssertExpectations(t)
	})
}
```

Now, I also need to provide the diff to correct the import paths in `phonebook_app.go`.
