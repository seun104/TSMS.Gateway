package http

import "time"

// --- Phonebook DTOs ---

// CreatePhonebookRequestDTO is used for creating a new phonebook.
type CreatePhonebookRequestDTO struct {
	Name        string `json:"name" validate:"required,min=1,max=255"`
	Description string `json:"description,omitempty" validate:"max=1000"`
}

// UpdatePhonebookRequestDTO is used for updating an existing phonebook.
// All fields are optional; only provided fields will be updated.
type UpdatePhonebookRequestDTO struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=1000"` // omitempty for string will not distinguish empty vs not provided, pointer is better
}

// PhonebookResponseDTO represents a phonebook in HTTP responses.
type PhonebookResponseDTO struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ListPhonebooksResponseDTO is the response for listing phonebooks.
type ListPhonebooksResponseDTO struct {
	Phonebooks []PhonebookResponseDTO `json:"phonebooks"`
	TotalCount int64                  `json:"total_count"`
	Offset     int                    `json:"offset"`
	Limit      int                    `json:"limit"`
}

// --- Contact DTOs ---

// CreateContactRequestDTO is used for creating a new contact.
type CreateContactRequestDTO struct {
	Number       string            `json:"number" validate:"required,e164"` // Example: E.164 format validation
	FirstName    string            `json:"first_name,omitempty" validate:"max=255"`
	LastName     string            `json:"last_name,omitempty" validate:"max=255"`
	Email        string            `json:"email,omitempty" validate:"omitempty,email,max=255"`
	CustomFields map[string]string `json:"custom_fields,omitempty"`
	Subscribed   *bool             `json:"subscribed,omitempty"` // Pointer to distinguish between not set, true, false. Default can be handled by app logic.
}

// UpdateContactRequestDTO is used for updating an existing contact.
// All fields are optional.
type UpdateContactRequestDTO struct {
	Number       *string           `json:"number,omitempty" validate:"omitempty,e164"`
	FirstName    *string           `json:"first_name,omitempty" validate:"omitempty,max=255"`
	LastName     *string           `json:"last_name,omitempty" validate:"omitempty,max=255"`
	Email        *string           `json:"email,omitempty" validate:"omitempty,email,max=255"`
	CustomFields map[string]string `json:"custom_fields,omitempty"` // To clear, send empty map {}. To leave unchanged, omit.
	Subscribed   *bool             `json:"subscribed,omitempty"`
}

// ContactResponseDTO represents a contact in HTTP responses.
type ContactResponseDTO struct {
	ID           string            `json:"id"`
	PhonebookID  string            `json:"phonebook_id"`
	Number       string            `json:"number"`
	FirstName    string            `json:"first_name,omitempty"`
	LastName     string            `json:"last_name,omitempty"`
	Email        string            `json:"email,omitempty"`
	CustomFields map[string]string `json:"custom_fields,omitempty"`
	Subscribed   bool              `json:"subscribed"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// ListContactsResponseDTO is the response for listing contacts.
type ListContactsResponseDTO struct {
	Contacts   []ContactResponseDTO `json:"contacts"`
	TotalCount int64                `json:"total_count"`
	Offset     int                  `json:"offset"`
	Limit      int                  `json:"limit"`
}
