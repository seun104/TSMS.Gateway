-- Migration for billing related tables (transactions)

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'transaction_type_enum') THEN
        CREATE TYPE transaction_type_enum AS ENUM (
            'credit_purchase',
            'sms_charge',
            'refund',
            'service_activation_fee',
            'manual_adjustment'
        );
    END IF;
END$$;

CREATE TABLE IF NOT EXISTS transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE, -- Assuming users table exists from auth migration
    type transaction_type_enum NOT NULL,
    amount NUMERIC(19, 4) NOT NULL, -- Can be positive (credit) or negative (debit)
    currency_code VARCHAR(3) DEFAULT 'USD' NOT NULL,
    description TEXT,
    related_message_id UUID REFERENCES outbox_messages(id) ON DELETE SET NULL, -- Assuming outbox_messages table exists
    payment_gateway_txn_id VARCHAR(255),
    balance_after NUMERIC(19, 4), -- Optional: User's balance after this transaction for auditing
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_transactions_user_id ON transactions(user_id);
CREATE INDEX IF NOT EXISTS idx_transactions_type ON transactions(type);
CREATE INDEX IF NOT EXISTS idx_transactions_related_message_id ON transactions(related_message_id);
CREATE INDEX IF NOT EXISTS idx_transactions_created_at ON transactions(created_at);

-- Placeholder for payment_gateways table if needed in this phase,
-- but might be deferred or part of a fuller billing service implementation.
-- CREATE TABLE IF NOT EXISTS payment_gateways (
--     id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
--     name VARCHAR(100) UNIQUE NOT NULL,
--     config JSONB,
--     is_active BOOLEAN DEFAULT TRUE NOT NULL,
--     created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
--     updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
-- );
