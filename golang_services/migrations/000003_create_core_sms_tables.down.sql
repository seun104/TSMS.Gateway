DROP TABLE IF EXISTS inbox_messages;
DROP TABLE IF EXISTS outbox_messages;
DROP TABLE IF EXISTS routes;
DROP TABLE IF EXISTS private_numbers;
DROP TABLE IF EXISTS sms_providers;

-- Optionally drop the ENUM type if no other tables use it and it's safe to do so.
-- This might fail if other objects depend on it.
-- DO $$
-- BEGIN
--     IF EXISTS (SELECT 1 FROM pg_type WHERE typname = 'message_status') THEN
--         DROP TYPE message_status;
--     END IF;
-- END$$;
