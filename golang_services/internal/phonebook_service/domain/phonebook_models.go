package domain

import (
	"database/sql" // For sql.NullString if we choose that for Description
	"time"

	"github.com/google/uuid"
)

// Phonebook represents a collection of contacts owned by a user.
type Phonebook struct {
	ID          uuid.UUID    `json:"id"`
	UserID      uuid.UUID    `json:"user_id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"` // Using string; can be empty if optional
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// NewPhonebook creates a new Phonebook instance.
// ID is typically generated before calling this.
func NewPhonebook(id uuid.UUID, userID uuid.UUID, name string, description string) *Phonebook {
	now := time.Now().UTC()
	return &Phonebook{
		ID:          id,
		UserID:      userID,
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// Contact represents an individual contact within a phonebook.
type Contact struct {
	ID           uuid.UUID          `json:"id"`
	PhonebookID  uuid.UUID          `json:"phonebook_id"`
	Number       string             `json:"number"`
	FirstName    string             `json:"first_name,omitempty"`
	LastName     string             `json:"last_name,omitempty"`
	Email        string             `json:"email,omitempty"`
	CustomFields map[string]string  `json:"custom_fields,omitempty"` // Stored as JSONB in PostgreSQL
	Subscribed   bool               `json:"subscribed"`              // Default subscription status
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

// NewContact creates a new Contact instance.
// ID is typically generated before calling this.
// CustomFields can be nil if not provided.
func NewContact(
	id uuid.UUID,
	phonebookID uuid.UUID,
	number string,
	firstName string,
	lastName string,
	email string,
	customFields map[string]string,
	subscribed bool,
) *Contact {
	now := time.Now().UTC()
	cf := customFields
	if cf == nil {
		cf = make(map[string]string) // Ensure it's not nil for JSON marshaling if empty
	}
	return &Contact{
		ID:           id,
		PhonebookID:  phonebookID,
		Number:       number,
		FirstName:    firstName,
		LastName:     lastName,
		Email:        email,
		CustomFields: cf,
		Subscribed:   subscribed,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// Using string for optional fields like Description, FirstName, LastName, Email.
// If a clear distinction between an empty string "" and SQL NULL is required,
// then sql.NullString would be more appropriate for those fields.
// For CustomFields, map[string]string is fine; it will be marshaled to JSON for JSONB storage.
// If a field in CustomFields needs to be explicitly NULL vs an empty string, that's harder with map[string]string.
// In such cases, map[string]*string or map[string]interface{} with careful handling might be options,
// but map[string]string is often sufficient.
