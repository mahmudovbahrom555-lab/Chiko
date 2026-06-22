-- ШАГ 1.2: Row Level Security
-- Принцип 3: RLS через chat_id, не через прямые проверки в Go-коде.
-- ЗАВИСИТ ОТ: 001_core_schema.sql

-- ============================================================
-- СПРАВОЧНИКИ — публичные для чтения, без RLS
-- (currencies, regions, plans, feature_flags)
-- ============================================================
-- Эти таблицы не содержат пользовательских данных — RLS не нужен.
-- Доступны всем аутентифицированным пользователям на SELECT.

GRANT SELECT ON currencies     TO authenticated;
GRANT SELECT ON regions        TO authenticated;
GRANT SELECT ON plans          TO authenticated;
GRANT SELECT ON feature_flags  TO authenticated;

-- Анонимный доступ к currencies и plans (нужен для гостевого каталога шаг 4.2)
GRANT SELECT ON currencies    TO anon;
GRANT SELECT ON plans         TO anon;

-- ============================================================
-- ВКЛЮЧАЕМ RLS НА ВСЕХ ПОЛЬЗОВАТЕЛЬСКИХ ТАБЛИЦАХ
-- ============================================================
ALTER TABLE producers          ENABLE ROW LEVEL SECURITY;
ALTER TABLE chats              ENABLE ROW LEVEL SECURITY;
ALTER TABLE categories         ENABLE ROW LEVEL SECURITY;
ALTER TABLE products           ENABLE ROW LEVEL SECURITY;
ALTER TABLE orders             ENABLE ROW LEVEL SECURITY;
ALTER TABLE order_items        ENABLE ROW LEVEL SECURITY;
ALTER TABLE order_changes      ENABLE ROW LEVEL SECURITY;
ALTER TABLE messages           ENABLE ROW LEVEL SECURITY;
ALTER TABLE order_comments     ENABLE ROW LEVEL SECURITY;
ALTER TABLE debt_transactions  ENABLE ROW LEVEL SECURITY;
ALTER TABLE client_discounts   ENABLE ROW LEVEL SECURITY;
ALTER TABLE product_discounts  ENABLE ROW LEVEL SECURITY;
ALTER TABLE volume_tiers       ENABLE ROW LEVEL SECURITY;
ALTER TABLE client_metrics     ENABLE ROW LEVEL SECURITY;
ALTER TABLE events             ENABLE ROW LEVEL SECURITY;
ALTER TABLE subscriptions      ENABLE ROW LEVEL SECURITY;
ALTER TABLE guest_sessions     ENABLE ROW LEVEL SECURITY;

-- ============================================================
-- ВСПОМОГАТЕЛЬНАЯ ФУНКЦИЯ: является ли текущий пользователь стороной чата?
-- ============================================================
CREATE OR REPLACE FUNCTION is_chat_participant(p_chat_id UUID)
RETURNS BOOLEAN
LANGUAGE sql
STABLE
SECURITY DEFINER
AS $$
    SELECT EXISTS (
        SELECT 1 FROM chats
        WHERE id = p_chat_id
          AND (producer_id = auth.uid() OR client_id = auth.uid())
    );
$$;

-- ============================================================
-- PRODUCERS
-- ============================================================
CREATE POLICY producers_select ON producers
    FOR SELECT USING (id = auth.uid());

CREATE POLICY producers_insert ON producers
    FOR INSERT WITH CHECK (id = auth.uid());

CREATE POLICY producers_update ON producers
    FOR UPDATE USING (id = auth.uid());

-- ============================================================
-- CHATS
-- ============================================================
CREATE POLICY chats_select ON chats
    FOR SELECT USING (producer_id = auth.uid() OR client_id = auth.uid());

CREATE POLICY chats_insert ON chats
    FOR INSERT WITH CHECK (producer_id = auth.uid());

-- ============================================================
-- CATEGORIES (читают обе стороны чата, меняет только producer)
-- ============================================================
CREATE POLICY categories_select ON categories
    FOR SELECT USING (
        producer_id = auth.uid()
        OR EXISTS (
            SELECT 1 FROM chats
            WHERE chats.producer_id = categories.producer_id
              AND chats.client_id   = auth.uid()
        )
    );

CREATE POLICY categories_insert ON categories
    FOR INSERT WITH CHECK (producer_id = auth.uid());

CREATE POLICY categories_update ON categories
    FOR UPDATE USING (producer_id = auth.uid());

CREATE POLICY categories_delete ON categories
    FOR DELETE USING (producer_id = auth.uid());

-- ============================================================
-- PRODUCTS (читают обе стороны, меняет только producer)
-- ============================================================
CREATE POLICY products_select ON products
    FOR SELECT USING (
        producer_id = auth.uid()
        OR EXISTS (
            SELECT 1 FROM chats
            WHERE chats.producer_id = products.producer_id
              AND chats.client_id   = auth.uid()
        )
    );

-- Анонимный SELECT для гостевого каталога (ограничен по producer_token в Go-слое)
CREATE POLICY products_anon_select ON products
    FOR SELECT TO anon USING (is_active = true);

CREATE POLICY products_insert ON products
    FOR INSERT WITH CHECK (producer_id = auth.uid());

CREATE POLICY products_update ON products
    FOR UPDATE USING (producer_id = auth.uid());

CREATE POLICY products_delete ON products
    FOR DELETE USING (producer_id = auth.uid());

-- ============================================================
-- ORDERS, ORDER_ITEMS (обе стороны чата)
-- ============================================================
CREATE POLICY orders_select ON orders
    FOR SELECT USING (is_chat_participant(chat_id));

CREATE POLICY orders_insert ON orders
    FOR INSERT WITH CHECK (is_chat_participant(chat_id) AND created_by = auth.uid());

CREATE POLICY orders_update ON orders
    FOR UPDATE USING (is_chat_participant(chat_id));

-- order_items — через родительский заказ
CREATE POLICY order_items_select ON order_items
    FOR SELECT USING (
        EXISTS (SELECT 1 FROM orders WHERE orders.id = order_items.order_id AND is_chat_participant(orders.chat_id))
    );

CREATE POLICY order_items_insert ON order_items
    FOR INSERT WITH CHECK (
        EXISTS (SELECT 1 FROM orders WHERE orders.id = order_items.order_id AND is_chat_participant(orders.chat_id))
        AND added_by = auth.uid()
    );

CREATE POLICY order_items_update ON order_items
    FOR UPDATE USING (
        EXISTS (SELECT 1 FROM orders WHERE orders.id = order_items.order_id AND is_chat_participant(orders.chat_id))
    );

-- ============================================================
-- ORDER_CHANGES (обе стороны, только INSERT + SELECT)
-- ============================================================
CREATE POLICY order_changes_select ON order_changes
    FOR SELECT USING (
        EXISTS (SELECT 1 FROM orders WHERE orders.id = order_changes.order_id AND is_chat_participant(orders.chat_id))
    );

CREATE POLICY order_changes_insert ON order_changes
    FOR INSERT WITH CHECK (
        EXISTS (SELECT 1 FROM orders WHERE orders.id = order_changes.order_id AND is_chat_participant(orders.chat_id))
        AND changed_by = auth.uid()
    );

-- ============================================================
-- MESSAGES (обе стороны чата)
-- ============================================================
CREATE POLICY messages_select ON messages
    FOR SELECT USING (is_chat_participant(chat_id));

CREATE POLICY messages_insert ON messages
    FOR INSERT WITH CHECK (is_chat_participant(chat_id) AND author_id = auth.uid());

-- ============================================================
-- ORDER_COMMENTS (обе стороны)
-- ============================================================
CREATE POLICY order_comments_select ON order_comments
    FOR SELECT USING (
        EXISTS (SELECT 1 FROM orders WHERE orders.id = order_comments.order_id AND is_chat_participant(orders.chat_id))
    );

CREATE POLICY order_comments_insert ON order_comments
    FOR INSERT WITH CHECK (
        EXISTS (SELECT 1 FROM orders WHERE orders.id = order_comments.order_id AND is_chat_participant(orders.chat_id))
        AND author_id = auth.uid()
    );

-- ============================================================
-- DEBT_TRANSACTIONS
-- SELECT: обе стороны
-- INSERT: обе стороны (любая может инициировать операцию)
-- return_correction: ТОЛЬКО producer (ТЗ раздел 7)
-- UPDATE/DELETE: запрещён триггером (003_triggers.sql)
-- ============================================================
CREATE POLICY debt_select ON debt_transactions
    FOR SELECT USING (is_chat_participant(chat_id));

CREATE POLICY debt_insert ON debt_transactions
    FOR INSERT WITH CHECK (
        is_chat_participant(chat_id)
        AND initiator_id = auth.uid()
        -- return_correction создаёт только producer чата
        AND (
            type != 'return_correction'
            OR EXISTS (
                SELECT 1 FROM chats
                WHERE chats.id = debt_transactions.chat_id
                  AND chats.producer_id = auth.uid()
            )
        )
    );

-- ============================================================
-- СКИДКИ (только producer создаёт и меняет)
-- ============================================================
CREATE POLICY client_discounts_select ON client_discounts
    FOR SELECT USING (is_chat_participant(chat_id));

CREATE POLICY client_discounts_insert ON client_discounts
    FOR INSERT WITH CHECK (
        EXISTS (SELECT 1 FROM chats WHERE chats.id = client_discounts.chat_id AND chats.producer_id = auth.uid())
    );

CREATE POLICY client_discounts_update ON client_discounts
    FOR UPDATE USING (
        EXISTS (SELECT 1 FROM chats WHERE chats.id = client_discounts.chat_id AND chats.producer_id = auth.uid())
    );

CREATE POLICY product_discounts_select ON product_discounts
    FOR SELECT USING (
        EXISTS (
            SELECT 1 FROM products p
            WHERE p.id = product_discounts.product_id
              AND (
                p.producer_id = auth.uid()
                OR EXISTS (
                    SELECT 1 FROM chats c WHERE c.producer_id = p.producer_id AND c.client_id = auth.uid()
                )
              )
        )
    );

CREATE POLICY product_discounts_insert ON product_discounts
    FOR INSERT WITH CHECK (
        EXISTS (SELECT 1 FROM products WHERE products.id = product_discounts.product_id AND products.producer_id = auth.uid())
    );

CREATE POLICY product_discounts_update ON product_discounts
    FOR UPDATE USING (
        EXISTS (SELECT 1 FROM products WHERE products.id = product_discounts.product_id AND products.producer_id = auth.uid())
    );

CREATE POLICY product_discounts_delete ON product_discounts
    FOR DELETE USING (
        EXISTS (SELECT 1 FROM products WHERE products.id = product_discounts.product_id AND products.producer_id = auth.uid())
    );

CREATE POLICY volume_tiers_select ON volume_tiers
    FOR SELECT USING (
        producer_id = auth.uid()
        OR EXISTS (SELECT 1 FROM chats WHERE chats.producer_id = volume_tiers.producer_id AND chats.client_id = auth.uid())
    );

CREATE POLICY volume_tiers_insert ON volume_tiers
    FOR INSERT WITH CHECK (producer_id = auth.uid());

CREATE POLICY volume_tiers_update ON volume_tiers
    FOR UPDATE USING (producer_id = auth.uid());

CREATE POLICY volume_tiers_delete ON volume_tiers
    FOR DELETE USING (producer_id = auth.uid());

-- ============================================================
-- CLIENT_METRICS (обе стороны чата читают, система пишет)
-- ============================================================
CREATE POLICY client_metrics_select ON client_metrics
    FOR SELECT USING (is_chat_participant(chat_id));

-- INSERT/UPDATE только через SECURITY DEFINER функции (триггеры)

-- ============================================================
-- EVENTS (только свои)
-- ============================================================
CREATE POLICY events_select ON events
    FOR SELECT USING (user_id = auth.uid());

CREATE POLICY events_insert ON events
    FOR INSERT WITH CHECK (user_id = auth.uid());

-- ============================================================
-- SUBSCRIPTIONS (только producer видит своё)
-- ============================================================
CREATE POLICY subscriptions_select ON subscriptions
    FOR SELECT USING (producer_id = auth.uid());

-- INSERT/UPDATE — только через service_role (из Go-бэкенда)

-- ============================================================
-- GUEST_SESSIONS (anon может создавать, producer видит своё)
-- ============================================================
CREATE POLICY guest_sessions_anon_insert ON guest_sessions
    FOR INSERT TO anon WITH CHECK (true);

CREATE POLICY guest_sessions_anon_select ON guest_sessions
    FOR SELECT TO anon USING (true);

CREATE POLICY guest_sessions_producer_select ON guest_sessions
    FOR SELECT USING (producer_id = auth.uid());
