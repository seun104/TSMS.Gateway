-- Migration: Create phonebook_tables (phonebooks, contacts)
-- Direction: Up

-- Create the phonebooks table
CREATE TABLE IF NOT EXISTS phonebooks (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Add an index on user_id for faster lookups of phonebooks by user
CREATE INDEX IF NOT EXISTS idx_phonebooks_user_id ON phonebooks (user_id);

COMMENT ON TABLE phonebooks IS 'Stores phonebooks, which are collections of contacts owned by users.';
COMMENT ON COLUMN phonebooks.user_id IS 'Foreign key to the users table, identifying the owner of the phonebook.';
COMMENT ON COLUMN phonebooks.name IS 'Name of the phonebook, provided by the user.';
COMMENT ON COLUMN phonebooks.description IS 'Optional description for the phonebook.';


-- Create the contacts table
CREATE TABLE IF NOT EXISTS contacts (
    id UUID PRIMARY KEY,
    phonebook_id UUID NOT NULL,
    number VARCHAR(50) NOT NULL, -- Assuming a reasonable max length for phone numbers
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    email VARCHAR(255),
    custom_fields JSONB, -- Using JSONB for flexible custom fields
    subscribed BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_phonebook
        FOREIGN KEY(phonebook_id)
        REFERENCES phonebooks(id)
        ON DELETE CASCADE -- If a phonebook is deleted, its contacts are also deleted
);

-- Add an index on phonebook_id for faster lookup of contacts within a phonebook
CREATE INDEX IF NOT EXISTS idx_contacts_phonebook_id ON contacts (phonebook_id);

-- Add an index on the number field for faster searching by number
CREATE INDEX IF NOT EXISTS idx_contacts_number ON contacts (number);

-- Add a unique constraint to prevent duplicate numbers within the same phonebook
CREATE UNIQUE INDEX IF NOT EXISTS idx_contacts_phonebook_id_number ON contacts (phonebook_id, number);

COMMENT ON TABLE contacts IS 'Stores individual contacts within a phonebook.';
COMMENT ON COLUMN contacts.phonebook_id IS 'Foreign key to the phonebooks table.';
COMMENT ON COLUMN contacts.number IS 'Phone number of the contact.';
COMMENT ON COLUMN contacts.custom_fields IS 'Flexible key-value pairs for additional contact information, stored as JSONB.';
COMMENT ON COLUMN contacts.subscribed IS 'Indicates if the contact is subscribed to communications (e.g., newsletters).';
