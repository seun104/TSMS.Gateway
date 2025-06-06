CREATE TABLE IF NOT EXISTS payment_intents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount BIGINT NOT NULL, -- Amount in smallest currency unit (e.g., cents)
    currency VARCHAR(10) NOT NULL,
    status VARCHAR(50) NOT NULL, -- e.g., 'pending', 'requires_action', 'succeeded', 'failed', 'cancelled'
    gateway_payment_intent_id VARCHAR(255) UNIQUE, -- ID from the payment gateway
    gateway_client_secret TEXT, -- For client-side actions (e.g., Stripe client secret)
    error_message TEXT,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_payment_intents_user_id ON payment_intents(user_id);
CREATE INDEX IF NOT EXISTS idx_payment_intents_gateway_id ON payment_intents(gateway_payment_intent_id);
CREATE INDEX IF NOT EXISTS idx_payment_intents_status ON payment_intents(status);
