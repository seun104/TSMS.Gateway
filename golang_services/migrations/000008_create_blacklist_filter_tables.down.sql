-- Migration to drop blacklist and filter_words tables

DROP INDEX IF EXISTS uq_filter_words_lower_word;
DROP INDEX IF EXISTS idx_filter_words_is_active;
DROP TABLE IF EXISTS filter_words;

DROP INDEX IF EXISTS idx_blacklisted_numbers_phone_number;
DROP INDEX IF EXISTS idx_blacklisted_numbers_user_id;
DROP INDEX IF EXISTS uq_blacklisted_numbers_phone_user; -- Matches constraint name
DROP TABLE IF EXISTS blacklisted_numbers;
