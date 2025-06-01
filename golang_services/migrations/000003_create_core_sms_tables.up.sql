-- Migration for core SMS functionality tables

-- First, create the ENUM type for message_status if it doesn't exist
-- This block ensures idempotency for the ENUM creation.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'message_status') THEN
        CREATE TYPE message_status AS ENUM (
            'queued',
            'processing',
            'sent_to_provider',
            'failed_provider_submission',
            'delivered',
            'undelivered',
            'expired',
            'rejected',
            'accepted', -- Accepted by provider, awaiting DLR
            'unknown'   -- Default or error state for DLR
        );
    END IF;
END$$;

CREATE TABLE IF NOT EXISTS sms_providers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) UNIQUE NOT NULL,
    api_config_json JSONB, -- Store API keys, endpoint URLs etc.
    is_active BOOLEAN DEFAULT TRUE NOT NULL,
    supports_delivery_report_polling BOOLEAN DEFAULT FALSE NOT NULL,
    supports_delivery_report_callback BOOLEAN DEFAULT FALSE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS private_numbers ( -- Or Sender IDs
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE, -- User who owns/registered this number, can be null for system/shared numbers
    number_value VARCHAR(50) UNIQUE NOT NULL, -- The actual sender ID/number, ensure uniqueness
    sms_provider_id UUID REFERENCES sms_providers(id) ON DELETE SET NULL, -- Provider through which this number operates (if specific)
    is_shared BOOLEAN DEFAULT FALSE NOT NULL,
    capabilities_json JSONB, -- e.g., {"can_receive_sms": true, "is_alphanumeric": false, "purpose": "transactional"}
    is_active BOOLEAN DEFAULT TRUE NOT NULL,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_private_numbers_user_id ON private_numbers(user_id);
CREATE INDEX IF NOT EXISTS idx_private_numbers_number_value ON private_numbers(number_value); -- For quick lookup

CREATE TABLE IF NOT EXISTS routes ( -- For SMS routing
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) UNIQUE NOT NULL,
    priority INT DEFAULT 0 NOT NULL, -- Lower is higher priority
    criteria_json JSONB, -- e.g., {"country_code": "98", "operator_prefix": "912", "user_id": "uuid"}
    sms_provider_id UUID NOT NULL REFERENCES sms_providers(id) ON DELETE CASCADE,
    is_active BOOLEAN DEFAULT TRUE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_routes_priority ON routes(priority);


CREATE TABLE IF NOT EXISTS outbox_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    batch_id UUID, -- Optional, for grouping bulk messages
    sender_id VARCHAR(50) NOT NULL, -- This is the 'from' number/alphanumeric. Could reference private_numbers.id if strictly enforced.
    recipient VARCHAR(50) NOT NULL,
    content TEXT NOT NULL,
    status message_status DEFAULT 'queued' NOT NULL,
    segments INT DEFAULT 1 NOT NULL,
    provider_message_id VARCHAR(255),
    provider_status_code VARCHAR(100),
    error_message TEXT,
    user_data TEXT,
    scheduled_for TIMESTAMPTZ,
    processed_at TIMESTAMPTZ,
    sent_to_provider_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    last_status_update_at TIMESTAMPTZ,
    sms_provider_id UUID REFERENCES sms_providers(id) ON DELETE SET NULL,
    route_id UUID REFERENCES routes(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_outbox_user_status ON outbox_messages(user_id, status);
CREATE INDEX IF NOT EXISTS idx_outbox_recipient ON outbox_messages(recipient);
CREATE INDEX IF NOT EXISTS idx_outbox_provider_message_id ON outbox_messages(provider_message_id);
CREATE INDEX IF NOT EXISTS idx_outbox_status_created_at ON outbox_messages(status, created_at); -- For workers polling by status
CREATE INDEX IF NOT EXISTS idx_outbox_scheduled_for ON outbox_messages(scheduled_for) WHERE status = 'queued';


CREATE TABLE IF NOT EXISTS inbox_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- user_id UUID REFERENCES users(id) ON DELETE SET NULL, -- User associated with the private number, can be derived via private_number_id
    private_number_id UUID NOT NULL REFERENCES private_numbers(id) ON DELETE CASCADE,
    provider_message_id VARCHAR(255), -- ID from the SMS provider, might not be unique across all providers if not prefixed
    sender_number VARCHAR(50) NOT NULL,
    recipient_number VARCHAR(50) NOT NULL, -- The private number it was sent to (denormalized from private_numbers.number_value for quick query)
    content TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL, -- Timestamp from provider or when ingested
    is_parsed BOOLEAN DEFAULT FALSE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_inbox_recipient_number ON inbox_messages(recipient_number);
CREATE INDEX IF NOT EXISTS idx_inbox_sender_number ON inbox_messages(sender_number);
CREATE INDEX IF NOT EXISTS idx_inbox_received_at ON inbox_messages(received_at);
-- Add a unique constraint on provider_message_id and private_number_id if provider_message_id is unique per number for a provider
-- ALTER TABLE inbox_messages ADD CONSTRAINT unique_provider_msg_per_number UNIQUE (provider_message_id, private_number_id);
