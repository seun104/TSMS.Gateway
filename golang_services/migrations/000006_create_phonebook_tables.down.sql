-- Migration: Create phonebook_tables (phonebooks, contacts)
-- Direction: Down

-- Drop in reverse order of creation due to foreign key constraints

DROP TABLE IF EXISTS contacts;
DROP TABLE IF EXISTS phonebooks;

-- Indexes associated with these tables will be dropped automatically.
