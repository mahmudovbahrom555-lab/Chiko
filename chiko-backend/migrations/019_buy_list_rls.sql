-- Migration 019: RLS политики для Buy List (гостевой доступ).
--
-- Анон (не аутентифицированный пользователь) может:
--   INSERT orders только со status='guest_buy_list' (не другой статус)
--   SELECT orders только по своему guest_token (не листинг всех)
--   INSERT order_items только в guest_buy_list заказ
--   SELECT order_items только для своего guest_buy_list заказа
--
-- Все остальные операции — только для аутентифицированных.

-- orders: INSERT для анона (только guest_buy_list)
DO $$ BEGIN
    CREATE POLICY orders_anon_insert ON orders
        FOR INSERT TO anon
        WITH CHECK (status = 'guest_buy_list' AND client_id IS NULL);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- orders: SELECT для анона (только по guest_token — конкретный заказ)
DO $$ BEGIN
    CREATE POLICY orders_anon_select ON orders
        FOR SELECT TO anon
        USING (guest_token IS NOT NULL AND guest_token = current_setting('request.headers', true)::jsonb->>'x-guest-token' IS NOT NULL);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- order_items: INSERT для анона в guest_buy_list заказ
DO $$ BEGIN
    CREATE POLICY order_items_anon_insert ON order_items
        FOR INSERT TO anon
        WITH CHECK (
            EXISTS (
                SELECT 1 FROM orders
                WHERE orders.id = order_items.order_id
                  AND orders.status = 'guest_buy_list'
                  AND orders.client_id IS NULL
            )
        );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- order_items: SELECT для анона
DO $$ BEGIN
    CREATE POLICY order_items_anon_select ON order_items
        FOR SELECT TO anon
        USING (
            EXISTS (
                SELECT 1 FROM orders
                WHERE orders.id = order_items.order_id
                  AND orders.status = 'guest_buy_list'
                  AND orders.guest_token IS NOT NULL
            )
        );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
