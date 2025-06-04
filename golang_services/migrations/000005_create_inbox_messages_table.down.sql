-- Migration: Create inbox_messages table
-- Direction: Down

DROP TABLE IF EXISTS inbox_messages;

-- Note: Indexes are typically dropped automatically when the table is dropped.
-- If you needed to drop them manually before dropping the table, you would use:
-- DROP INDEX IF EXISTS idx_inbox_provider_name_message_id;
-- DROP INDEX IF EXISTS idx_inbox_recipient_address;
-- DROP INDEX IF EXISTS idx_inbox_user_id;
-- DROP INDEX IF EXISTS idx_inbox_received_by_gateway_at;
-- DROP INDEX IF EXISTS idx_inbox_processed_at;
