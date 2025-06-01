DROP TABLE IF EXISTS transactions;
-- DROP TABLE IF EXISTS payment_gateways; -- If created

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_type WHERE typname = 'transaction_type_enum') THEN
        DROP TYPE transaction_type_enum;
    END IF;
END$$;
