-- Migration for creating blacklist and filter_words tables

CREATE TABLE IF NOT EXISTS blacklisted_numbers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone_number VARCHAR(50) NOT NULL,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL, -- Can be NULL for global blacklist
    reason TEXT,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,

    -- A number can appear once globally (user_id IS NULL) and once for each user.
    CONSTRAINT uq_blacklisted_numbers_phone_user UNIQUE (phone_number, user_id)
);

CREATE INDEX IF NOT EXISTS idx_blacklisted_numbers_phone_number ON blacklisted_numbers(phone_number);
CREATE INDEX IF NOT EXISTS idx_blacklisted_numbers_user_id ON blacklisted_numbers(user_id) WHERE user_id IS NOT NULL;


CREATE TABLE IF NOT EXISTS filter_words (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    word TEXT NOT NULL,
    is_active BOOLEAN DEFAULT TRUE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- For case-insensitive unique matching of filter words, use a unique index on lower(word)
CREATE UNIQUE INDEX IF NOT EXISTS uq_filter_words_lower_word ON filter_words (lower(word));
CREATE INDEX IF NOT EXISTS idx_filter_words_is_active ON filter_words(is_active);
