package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/your-repo/project/internal/phonebook_service/domain"
)

type PgContactRepository struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

func NewPgContactRepository(db *pgxpool.Pool, logger *slog.Logger) *PgContactRepository {
	return &PgContactRepository{db: db, logger: logger}
}

func (r *PgContactRepository) Create(ctx context.Context, ct *domain.Contact) error {
	query := `
		INSERT INTO contacts (id, phonebook_id, number, first_name, last_name, email, custom_fields, subscribed, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	customFieldsJSON, err := json.Marshal(ct.CustomFields)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error marshaling contact custom_fields", "error", err, "contact_id", ct.ID)
		return err
	}

	_, err = r.db.Exec(ctx, query,
		ct.ID, ct.PhonebookID, ct.Number, ct.FirstName, ct.LastName, ct.Email,
		customFieldsJSON, ct.Subscribed, ct.CreatedAt, ct.UpdatedAt,
	)
	if err != nil {
		// Check for unique constraint violation (phonebook_id, number)
		// This requires knowledge of the constraint name or using pgconn.PgError
		// For simplicity, a generic error is returned now. A real app might parse err.
		if strings.Contains(err.Error(), "idx_contacts_phonebook_id_number") || strings.Contains(err.Error(), "contacts_phonebook_id_number_key") {
			r.logger.WarnContext(ctx, "Duplicate contact number in phonebook", "error", err, "number", ct.Number, "phonebook_id", ct.PhonebookID)
			return domain.ErrDuplicateEntry
		}
		r.logger.ErrorContext(ctx, "Error creating contact", "error", err, "contact_id", ct.ID, "phonebook_id", ct.PhonebookID)
		return err
	}
	r.logger.InfoContext(ctx, "Contact created successfully", "contact_id", ct.ID, "phonebook_id", ct.PhonebookID)
	return nil
}

func (r *PgContactRepository) GetByID(ctx context.Context, id uuid.UUID, phonebookID uuid.UUID) (*domain.Contact, error) {
	query := `
		SELECT id, phonebook_id, number, first_name, last_name, email, custom_fields, subscribed, created_at, updated_at
		FROM contacts
		WHERE id = $1 AND phonebook_id = $2
	`
	// phonebookID is used here to ensure the contact belongs to the expected phonebook, adding a layer of authorization.
	// This assumes that the service layer calling this already verified user ownership of the phonebookID.
	ct := &domain.Contact{}
	var customFieldsJSON []byte
	err := r.db.QueryRow(ctx, query, id, phonebookID).Scan(
		&ct.ID, &ct.PhonebookID, &ct.Number, &ct.FirstName, &ct.LastName, &ct.Email,
		&customFieldsJSON, &ct.Subscribed, &ct.CreatedAt, &ct.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.WarnContext(ctx, "Contact not found or not in specified phonebook", "contact_id", id, "phonebook_id", phonebookID)
			return nil, domain.ErrNotFound
		}
		r.logger.ErrorContext(ctx, "Error getting contact by ID", "error", err, "contact_id", id)
		return nil, err
	}
	if err := json.Unmarshal(customFieldsJSON, &ct.CustomFields); err != nil {
		r.logger.ErrorContext(ctx, "Error unmarshaling contact custom_fields", "error", err, "contact_id", id)
		return nil, err
	}
	if ct.CustomFields == nil { // Ensure map is not nil if JSONB was 'null' or empty
		ct.CustomFields = make(map[string]string)
	}
	return ct, nil
}

func (r *PgContactRepository) ListByPhonebookID(ctx context.Context, phonebookID uuid.UUID, offset, limit int) ([]*domain.Contact, error) {
	query := `
		SELECT id, phonebook_id, number, first_name, last_name, email, custom_fields, subscribed, created_at, updated_at
		FROM contacts
		WHERE phonebook_id = $1
		ORDER BY last_name ASC, first_name ASC, number ASC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.Query(ctx, query, phonebookID, limit, offset)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error listing contacts by phonebook ID", "error", err, "phonebook_id", phonebookID)
		return nil, err
	}
	defer rows.Close()

	var contacts []*domain.Contact
	for rows.Next() {
		ct := &domain.Contact{}
		var customFieldsJSON []byte
		if err := rows.Scan(
			&ct.ID, &ct.PhonebookID, &ct.Number, &ct.FirstName, &ct.LastName, &ct.Email,
			&customFieldsJSON, &ct.Subscribed, &ct.CreatedAt, &ct.UpdatedAt,
		); err != nil {
			r.logger.ErrorContext(ctx, "Error scanning contact row", "error", err, "phonebook_id", phonebookID)
			return nil, err
		}
		if err := json.Unmarshal(customFieldsJSON, &ct.CustomFields); err != nil {
			r.logger.ErrorContext(ctx, "Error unmarshaling contact custom_fields on list", "error", err, "contact_id", ct.ID)
			return nil, err
		}
		if ct.CustomFields == nil {
			ct.CustomFields = make(map[string]string)
		}
		contacts = append(contacts, ct)
	}
	if err := rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "Error iterating contact rows", "error", err, "phonebook_id", phonebookID)
		return nil, err
	}
	return contacts, nil
}

func (r *PgContactRepository) Update(ctx context.Context, ct *domain.Contact) error {
	query := `
		UPDATE contacts
		SET number = $1, first_name = $2, last_name = $3, email = $4, custom_fields = $5, subscribed = $6, updated_at = $7
		WHERE id = $8 AND phonebook_id = $9
	`
	ct.UpdatedAt = time.Now().UTC()
	customFieldsJSON, err := json.Marshal(ct.CustomFields)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error marshaling contact custom_fields for update", "error", err, "contact_id", ct.ID)
		return err
	}

	tag, err := r.db.Exec(ctx, query,
		ct.Number, ct.FirstName, ct.LastName, ct.Email, customFieldsJSON, ct.Subscribed, ct.UpdatedAt,
		ct.ID, ct.PhonebookID,
	)
	if err != nil {
		// Check for unique constraint violation (phonebook_id, number)
		if strings.Contains(err.Error(), "idx_contacts_phonebook_id_number") || strings.Contains(err.Error(), "contacts_phonebook_id_number_key") {
			r.logger.WarnContext(ctx, "Duplicate contact number in phonebook on update", "error", err, "number", ct.Number, "phonebook_id", ct.PhonebookID)
			return domain.ErrDuplicateEntry
		}
		r.logger.ErrorContext(ctx, "Error updating contact", "error", err, "contact_id", ct.ID)
		return err
	}
	if tag.RowsAffected() == 0 {
		r.logger.WarnContext(ctx, "Contact not found or not in specified phonebook for update", "contact_id", ct.ID, "phonebook_id", ct.PhonebookID)
		return domain.ErrNotFound
	}
	r.logger.InfoContext(ctx, "Contact updated successfully", "contact_id", ct.ID)
	return nil
}

func (r *PgContactRepository) Delete(ctx context.Context, id uuid.UUID, phonebookID uuid.UUID) error {
	query := `DELETE FROM contacts WHERE id = $1 AND phonebook_id = $2`
	tag, err := r.db.Exec(ctx, query, id, phonebookID)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error deleting contact", "error", err, "contact_id", id)
		return err
	}
	if tag.RowsAffected() == 0 {
		r.logger.WarnContext(ctx, "Contact not found or not in specified phonebook for delete", "contact_id", id, "phonebook_id", phonebookID)
		return domain.ErrNotFound
	}
	r.logger.InfoContext(ctx, "Contact deleted successfully", "contact_id", id)
	return nil
}

func (r *PgContactRepository) FindByNumberInPhonebook(ctx context.Context, number string, phonebookID uuid.UUID) (*domain.Contact, error) {
	query := `
		SELECT id, phonebook_id, number, first_name, last_name, email, custom_fields, subscribed, created_at, updated_at
		FROM contacts
		WHERE number = $1 AND phonebook_id = $2
	`
	ct := &domain.Contact{}
	var customFieldsJSON []byte
	err := r.db.QueryRow(ctx, query, number, phonebookID).Scan(
		&ct.ID, &ct.PhonebookID, &ct.Number, &ct.FirstName, &ct.LastName, &ct.Email,
		&customFieldsJSON, &ct.Subscribed, &ct.CreatedAt, &ct.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.InfoContext(ctx, "Contact not found by number in phonebook", "number", number, "phonebook_id", phonebookID)
			return nil, nil // Not found is nil, nil as per interface suggestion
		}
		r.logger.ErrorContext(ctx, "Error finding contact by number in phonebook", "error", err, "number", number, "phonebook_id", phonebookID)
		return nil, err
	}
	if err := json.Unmarshal(customFieldsJSON, &ct.CustomFields); err != nil {
		r.logger.ErrorContext(ctx, "Error unmarshaling contact custom_fields for FindByNumber", "error", err, "contact_id", ct.ID)
		return nil, err
	}
	if ct.CustomFields == nil {
		ct.CustomFields = make(map[string]string)
	}
	return ct, nil
}
