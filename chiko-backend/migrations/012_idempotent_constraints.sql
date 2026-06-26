-- Migration 012: make critical CHECK constraints idempotent.
--
-- PostgreSQL < 17 has no ADD CONSTRAINT IF NOT EXISTS.
-- Repeated migration runs (CI, partial rollbacks, local reset) fail with
-- "constraint already exists". Wrapping in DO $$ EXCEPTION $$ silently skips
-- already-present constraints.
--
-- Only covers constraints that are likely to be re-run; schema-defining
-- constraints in 001_core_schema are created fresh each time via DROP/CREATE.

DO $$ BEGIN
    ALTER TABLE orders ADD CONSTRAINT orders_status_check
        CHECK (status IN ('draft','confirmed','ready','delivered','cancelled'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    ALTER TABLE debt_transactions ADD CONSTRAINT debt_transactions_sign_check
        CHECK (sign IN (1, -1));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    ALTER TABLE debt_transactions ADD CONSTRAINT debt_transactions_status_check
        CHECK (status IN ('pending','confirmed','disputed'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    ALTER TABLE debt_transactions ADD CONSTRAINT debt_return_correction_confirmed
        CHECK (type != 'return_correction' OR status = 'confirmed');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    ALTER TABLE return_requests ADD CONSTRAINT return_requests_status_check
        CHECK (status IN ('pending','attention','resolved','disputed'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    ALTER TABLE chats ADD CONSTRAINT chats_created_via_check
        CHECK (created_via IN ('producer_added','guest_link'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    ALTER TABLE categories ADD CONSTRAINT categories_producer_name_unique
        UNIQUE (producer_id, name);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    ALTER TABLE order_items ADD CONSTRAINT order_items_order_product_unique
        UNIQUE (order_id, product_id);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- Missing index from 006: return_requests by order_id (FK without index).
CREATE INDEX IF NOT EXISTS return_requests_order_idx ON return_requests(order_id);
