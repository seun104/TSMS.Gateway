package domain

import (
	"context"
	"github.com/google/uuid"
)

// PhonebookRepository defines the interface for managing Phonebook data.
type PhonebookRepository interface {
	Create(ctx context.Context, pb *Phonebook) error
	GetByID(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*Phonebook, error)
	ListByUserID(ctx context.Context, userID uuid.UUID, offset, limit int) ([]*Phonebook, error)
	Update(ctx context.Context, pb *Phonebook) error // pb should contain ID and UserID for ownership check
	Delete(ctx context.Context, id uuid.UUID, userID uuid.UUID) error
}

// ContactRepository defines the interface for managing Contact data.
type ContactRepository interface {
	Create(ctx context.Context, contact *Contact) error
	GetByID(ctx context.Context, id uuid.UUID, phonebookID uuid.UUID) (*Contact, error)
	ListByPhonebookID(ctx context.Context, phonebookID uuid.UUID, offset, limit int) ([]*Contact, error)
	Update(ctx context.Context, contact *Contact) error // contact should contain ID and PhonebookID
	Delete(ctx context.Context, id uuid.UUID, phonebookID uuid.UUID) error
	FindByNumberInPhonebook(ctx context.Context, number string, phonebookID uuid.UUID) (*Contact, error)
}
