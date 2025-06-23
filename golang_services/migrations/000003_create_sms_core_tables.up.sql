-- Migration for core SMS tables: outbox_messages, inbox_messages, sms_providers, private_numbers, routes

-- Enum for message status (if not created globally, though often better as application enum)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'message_status_enum') THEN
        CREATE TYPE message_status_enum AS ENUM (
            'queued',
            'processing',
            'sent',
            'failed_to_send',
            'delivered',
            'undelivered',
            'expired',
            'rejected'
        );
    END IF;
END$$;

CREATE TABLE IF NOT EXISTS sms_providers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) UNIQUE NOT NULL,
    api_config JSONB, -- Store API keys, endpoint URLs, rate limits, etc.
    is_active BOOLEAN DEFAULT TRUE NOT NULL,
    supports_delivery_report_polling BOOLEAN DEFAULT FALSE NOT NULL,
    supports_delivery_report_callback BOOLEAN DEFAULT FALSE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS private_numbers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL, -- Can be NULL if it's a system/shared number
    number_value VARCHAR(50) UNIQUE NOT NULL, -- The actual sender ID or phone number
    sms_provider_id UUID REFERENCES sms_providers(id) ON DELETE SET NULL, -- Default provider for this number
    is_shared BOOLEAN DEFAULT FALSE NOT NULL,
    capabilities JSONB, -- e.g., {"can_receive_sms": true, "is_alphanumeric": false, "type": "long_code|short_code|alphanumeric"}
    is_active BOOLEAN DEFAULT TRUE NOT NULL,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_private_numbers_user_id ON private_numbers(user_id);
CREATE INDEX IF NOT EXISTS idx_private_numbers_is_active ON private_numbers(is_active);

CREATE TABLE IF NOT EXISTS routes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) UNIQUE NOT NULL,
    priority INT DEFAULT 0 NOT NULL, -- Lower value means higher priority
    criteria JSONB, -- e.g., {"country_code": "98", "operator_prefix": "912", "user_id": "uuid"}
    sms_provider_id UUID NOT NULL REFERENCES sms_providers(id) ON DELETE CASCADE,
    is_active BOOLEAN DEFAULT TRUE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_routes_priority ON routes(priority);
CREATE INDEX IF NOT EXISTS idx_routes_is_active ON routes(is_active);


CREATE TABLE IF NOT EXISTS outbox_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    batch_id UUID, -- Optional, for grouping bulk messages, can be indexed if queried often
    sender_id_text VARCHAR(50) NOT NULL, -- The actual sender ID string used
    private_number_id UUID REFERENCES private_numbers(id) ON DELETE SET NULL, -- If sent via a configured private number
    recipient VARCHAR(50) NOT NULL,
    content TEXT NOT NULL,
    status message_status_enum DEFAULT 'queued' NOT NULL,
    segments INT DEFAULT 1 NOT NULL,
    provider_message_id VARCHAR(255),
    provider_status_code VARCHAR(100),
    error_message TEXT,
    user_data TEXT,
    scheduled_for TIMESTAMPTZ,
    processed_at TIMESTAMPTZ,
    sent_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    last_status_update_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    sms_provider_id UUID REFERENCES sms_providers(id) ON DELETE SET NULL,
    route_id UUID REFERENCES routes(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_outbox_user_id_status ON outbox_messages(user_id, status);
CREATE INDEX IF NOT EXISTS idx_outbox_recipient ON outbox_messages(recipient);
CREATE INDEX IF NOT EXISTS idx_outbox_provider_message_id ON outbox_messages(provider_message_id);
CREATE INDEX IF NOT EXISTS idx_outbox_status ON outbox_messages(status);
CREATE INDEX IF NOT EXISTS idx_outbox_created_at ON outbox_messages(created_at);
CREATE INDEX IF NOT EXISTS idx_outbox_scheduled_for ON outbox_messages(scheduled_for) WHERE status = 'queued';


CREATE TABLE IF NOT EXISTS inbox_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL, -- User associated if private_number has a user_id
    private_number_id UUID NOT NULL REFERENCES private_numbers(id) ON DELETE CASCADE,
    provider_message_id VARCHAR(255), -- Can be NULL if provider doesn't supply one or if internally generated
    sender_number VARCHAR(50) NOT NULL,
    recipient_number VARCHAR(50) NOT NULL, -- The private number it was sent to
    content TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL, -- Timestamp from provider or when ingested by system
    is_parsed BOOLEAN DEFAULT FALSE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_inbox_recipient_number ON inbox_messages(recipient_number);
CREATE INDEX IF NOT EXISTS idx_inbox_sender_number ON inbox_messages(sender_number);
CREATE INDEX IF NOT EXISTS idx_inbox_received_at ON inbox_messages(received_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_inbox_provider_message_id_private_number_id ON inbox_messages(provider_message_id, private_number_id) WHERE provider_message_id IS NOT NULL;
