-- Migration for creating tariffs and user_tariffs tables

CREATE TABLE IF NOT EXISTS tariffs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) UNIQUE NOT NULL,
    -- For simplicity, price_per_sms is a flat rate.
    -- For complex pricing (per country, operator, volume tiers),
    -- this could be a JSONB field or linked to other tables.
    price_per_sms BIGINT NOT NULL CHECK (price_per_sms >= 0), -- Price in smallest currency unit
    currency VARCHAR(10) NOT NULL,
    is_active BOOLEAN DEFAULT TRUE NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tariffs_is_active ON tariffs(is_active);
CREATE INDEX IF NOT EXISTS idx_tariffs_name ON tariffs(name);

CREATE TABLE IF NOT EXISTS user_tariffs (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    tariff_id UUID NOT NULL REFERENCES tariffs(id) ON DELETE RESTRICT, -- Prevent deleting a tariff if in use
    assigned_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
    -- If a history of tariff assignments is needed, this table would need a different PK
    -- and potentially effective_from/effective_to dates.
    -- For now, it represents the current active tariff for a user.
);

CREATE INDEX IF NOT EXISTS idx_user_tariffs_tariff_id ON user_tariffs(tariff_id);
