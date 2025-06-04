-- Migration: Create inbox_messages table
-- Direction: Up

CREATE TABLE IF NOT EXISTS inbox_messages (
    id UUID PRIMARY KEY,
    sender_address VARCHAR(255) NOT NULL,      -- Corresponds to domain.InboxMessage.From
    recipient_address VARCHAR(255) NOT NULL,   -- Corresponds to domain.InboxMessage.To
    text_content TEXT NOT NULL,                -- Corresponds to domain.InboxMessage.Text
    provider_message_id VARCHAR(255) NOT NULL, -- Corresponds to domain.InboxMessage.ProviderMessageID
    provider_name VARCHAR(100) NOT NULL,       -- Corresponds to domain.InboxMessage.ProviderName
    user_id UUID NULL,                         -- Corresponds to domain.InboxMessage.UserID
    private_number_id UUID NULL,               -- Corresponds to domain.InboxMessage.PrivateNumberID
    received_by_provider_at TIMESTAMPTZ NULL,  -- Corresponds to domain.InboxMessage.ReceivedByProviderAt
    received_by_gateway_at TIMESTAMPTZ NOT NULL, -- Corresponds to domain.InboxMessage.ReceivedByGatewayAt
    processed_at TIMESTAMPTZ NOT NULL,         -- Corresponds to domain.InboxMessage.ProcessedAt
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Add a unique constraint on provider_name and provider_message_id to prevent duplicates from same provider
CREATE UNIQUE INDEX IF NOT EXISTS idx_inbox_provider_name_message_id ON inbox_messages (provider_name, provider_message_id);

-- Add indexes for frequently queried fields
CREATE INDEX IF NOT EXISTS idx_inbox_recipient_address ON inbox_messages (recipient_address);
CREATE INDEX IF NOT EXISTS idx_inbox_user_id ON inbox_messages (user_id);
CREATE INDEX IF NOT EXISTS idx_inbox_received_by_gateway_at ON inbox_messages (received_by_gateway_at);
CREATE INDEX IF NOT EXISTS idx_inbox_processed_at ON inbox_messages (processed_at);

COMMENT ON COLUMN inbox_messages.sender_address IS 'The phone number or identifier of the message sender.';
COMMENT ON COLUMN inbox_messages.recipient_address IS 'The phone number or identifier on our platform that received the message.';
COMMENT ON COLUMN inbox_messages.text_content IS 'The textual content of the SMS message.';
COMMENT ON COLUMN inbox_messages.provider_message_id IS 'Unique message ID assigned by the external SMS provider.';
COMMENT ON COLUMN inbox_messages.provider_name IS 'Name of the SMS provider that delivered this message (e.g., twilio, infobip).';
COMMENT ON COLUMN inbox_messages.user_id IS 'Nullable foreign key to the users table, if the message can be associated with a user.';
COMMENT ON COLUMN inbox_messages.private_number_id IS 'Nullable foreign key to a private_numbers table, if applicable.';
COMMENT ON COLUMN inbox_messages.received_by_provider_at IS 'Timestamp of when the external SMS provider originally received the message.';
COMMENT ON COLUMN inbox_messages.received_by_gateway_at IS 'Timestamp of when the Arad SMS Gateway (public-api-service) received the callback from the provider.';
COMMENT ON COLUMN inbox_messages.processed_at IS 'Timestamp of when the inbound-processor-service processed this message.';
COMMENT ON TABLE inbox_messages IS 'Stores incoming SMS messages received from external providers.';
