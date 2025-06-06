-- Migration to drop tariffs and user_tariffs tables

DROP INDEX IF EXISTS idx_user_tariffs_tariff_id;
DROP TABLE IF EXISTS user_tariffs;

DROP INDEX IF EXISTS idx_tariffs_is_active;
DROP INDEX IF EXISTS idx_tariffs_name;
DROP TABLE IF EXISTS tariffs;
