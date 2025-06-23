DROP TABLE IF EXISTS inbox_messages;
DROP TABLE IF EXISTS outbox_messages;
DROP TABLE IF EXISTS routes;
DROP TABLE IF EXISTS private_numbers;
DROP TABLE IF EXISTS sms_providers;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_type WHERE typname = 'message_status_enum') THEN
        DROP TYPE message_status_enum;
    END IF;
END$$;
