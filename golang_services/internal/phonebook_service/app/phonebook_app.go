package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/AradIT/aradsms/golang_services/internal/phonebook_service/domain" // Corrected path
)

// Application provides an interface for phonebook and contact management operations.
type Application struct {
	phonebookRepo domain.PhonebookRepository
	contactRepo   domain.ContactRepository
	logger        *slog.Logger
}

// NewApplication creates a new Application instance.
func NewApplication(
	pbRepo domain.PhonebookRepository,
	cRepo domain.ContactRepository,
	logger *slog.Logger,
) *Application {
	return &Application{
		phonebookRepo: pbRepo,
		contactRepo:   cRepo,
		logger:        logger,
	}
}

// --- Phonebook Methods ---

// CreatePhonebook creates a new phonebook for a given user.
func (a *Application) CreatePhonebook(ctx context.Context, userID uuid.UUID, name string, description string) (*domain.Phonebook, error) {
	id := uuid.New()
	pb := domain.NewPhonebook(id, userID, name, description)

	err := a.phonebookRepo.Create(ctx, pb)
	if err != nil {
		a.logger.ErrorContext(ctx, "Failed to create phonebook in app layer", "error", err, "user_id", userID, "name", name)
		return nil, err
	}
	a.logger.InfoContext(ctx, "Phonebook created in app layer", "phonebook_id", pb.ID, "user_id", userID)
	return pb, nil
}

// GetPhonebook retrieves a specific phonebook by its ID, ensuring it belongs to the userID.
func (a *Application) GetPhonebook(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*domain.Phonebook, error) {
	pb, err := a.phonebookRepo.GetByID(ctx, id, userID)
	if err != nil {
		// Specific error logging is done in the repository, here we just pass it up or handle domain errors.
		return nil, err // domain.ErrNotFound or other db error
	}
	return pb, nil
}

// ListPhonebooks retrieves a list of phonebooks for a given user with pagination.
func (a *Application) ListPhonebooks(ctx context.Context, userID uuid.UUID, offset, limit int) ([]*domain.Phonebook, int64, error) {
	// For total_count, a separate repository method or an additional query might be needed.
	// For simplicity, let's assume ListByUserID could be extended or we do a count separately.
	// Here, we'll just return the count of items retrieved in this page for now, or 0 if not implemented.
	phonebooks, err := a.phonebookRepo.ListByUserID(ctx, userID, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	// Placeholder for total count - in a real app, you'd query this.
	// For example: totalCount, err := a.phonebookRepo.CountByUserID(ctx, userID)
	// For now, just returning the length of the current batch as a placeholder for total_count for the response.
	// This is not a true total count for pagination but fits the ListPhonebooksResponse structure.
	// A proper implementation would require another DB call for the total count.
	return phonebooks, int64(len(phonebooks)), nil // Returning len(phonebooks) as a mock total_count
}

// UpdatePhonebook updates an existing phonebook.
func (a *Application) UpdatePhonebook(ctx context.Context, id uuid.UUID, userID uuid.UUID, name string, description string) (*domain.Phonebook, error) {
	// First, verify ownership and get the existing phonebook
	pb, err := a.phonebookRepo.GetByID(ctx, id, userID)
	if err != nil {
		return nil, err // Handles domain.ErrNotFound or other errors
	}

	// Update fields
	pb.Name = name
	pb.Description = description
	pb.UpdatedAt = time.Now().UTC()

	err = a.phonebookRepo.Update(ctx, pb)
	if err != nil {
		return nil, err
	}
	return pb, nil
}

// DeletePhonebook deletes a phonebook.
func (a *Application) DeletePhonebook(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	return a.phonebookRepo.Delete(ctx, id, userID)
}

// --- Contact Methods ---

// CreateContact adds a new contact to a specified phonebook.
// Assumes phonebook ownership is verified by the caller (e.g., gRPC handler checking UserID against PhonebookID).
func (a *Application) CreateContact(ctx context.Context, phonebookID uuid.UUID, number, firstName, lastName, email string, customFields map[string]string, subscribed bool) (*domain.Contact, error) {
	contactID := uuid.New()
	contact := domain.NewContact(contactID, phonebookID, number, firstName, lastName, email, customFields, subscribed)

	err := a.contactRepo.Create(ctx, contact)
	if err != nil {
		return nil, err
	}
	return contact, nil
}

// GetContact retrieves a specific contact by its ID and phonebookID.
func (a *Application) GetContact(ctx context.Context, id uuid.UUID, phonebookID uuid.UUID) (*domain.Contact, error) {
	return a.contactRepo.GetByID(ctx, id, phonebookID)
}

// ListContacts retrieves contacts for a given phonebook with pagination.
func (a *Application) ListContacts(ctx context.Context, phonebookID uuid.UUID, offset, limit int) ([]*domain.Contact, int64, error) {
	contacts, err := a.contactRepo.ListByPhonebookID(ctx, phonebookID, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	// Placeholder for total count for contacts as well.
	return contacts, int64(len(contacts)), nil // Returning len(contacts) as a mock total_count
}

// UpdateContact updates an existing contact.
func (a *Application) UpdateContact(ctx context.Context, id uuid.UUID, phonebookID uuid.UUID, number, firstName, lastName, email string, customFields map[string]string, subscribed bool) (*domain.Contact, error) {
	// Get existing contact to ensure it's part of the specified phonebook
	contact, err := a.contactRepo.GetByID(ctx, id, phonebookID)
	if err != nil {
		return nil, err // Handles domain.ErrNotFound
	}

	contact.Number = number
	contact.FirstName = firstName
	contact.LastName = lastName
	contact.Email = email
	if customFields != nil { // Allow clearing custom fields if an empty map is passed, or no-op if nil
		contact.CustomFields = customFields
	} else {
		contact.CustomFields = make(map[string]string) // Ensure it's not nil
	}
	contact.Subscribed = subscribed
	contact.UpdatedAt = time.Now().UTC()

	err = a.contactRepo.Update(ctx, contact)
	if err != nil {
		return nil, err
	}
	return contact, nil
}

// DeleteContact deletes a contact.
func (a *Application) DeleteContact(ctx context.Context, id uuid.UUID, phonebookID uuid.UUID) error {
	return a.contactRepo.Delete(ctx, id, phonebookID)
}

// FindContactByNumberInPhonebook finds a contact by number within a specific phonebook.
func (a *Application) FindContactByNumberInPhonebook(ctx context.Context, number string, phonebookID uuid.UUID) (*domain.Contact, error) {
	return a.contactRepo.FindByNumberInPhonebook(ctx, number, phonebookID)
}
