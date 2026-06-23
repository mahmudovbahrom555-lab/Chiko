-- ШАГ 1.1: Ограничения, индексы
-- ЗАВИСИТ ОТ: 001_core_schema.sql

-- ============================================================
-- CHECK CONSTRAINTS — статусы через CHECK (не ENUM, как требует ТЗ)
-- ============================================================
ALTER TABLE orders
    ADD CONSTRAINT orders_status_check
    CHECK (status IN ('draft', 'confirmed', 'cancelled'));

ALTER TABLE debt_transactions
    ADD CONSTRAINT debt_status_check
    CHECK (status IN ('pending', 'confirmed', 'disputed'));

ALTER TABLE debt_transactions
    ADD CONSTRAINT debt_type_check
    CHECK (type IN ('delivery', 'payment', 'return_correction', 'correction'));

ALTER TABLE subscriptions
    ADD CONSTRAINT subscriptions_status_check
    CHECK (status IN ('trial', 'active', 'expired'));

ALTER TABLE messages
    ADD CONSTRAINT messages_type_check
    CHECK (type IN ('text', 'voice', 'system'));

ALTER TABLE volume_tiers
    ADD CONSTRAINT volume_tiers_type_check
    CHECK (type IN ('quantity', 'amount'));

ALTER TABLE volume_tiers
    ADD CONSTRAINT volume_tiers_applies_check
    CHECK (applies_to IN ('order', 'product'));

ALTER TABLE events
    ADD CONSTRAINT events_type_check
    CHECK (type IN ('catalog_open', 'product_view', 'order_create', 'order_confirm', 'order_cancel', 'draft_save'));

-- ============================================================
-- ИНДЕКСЫ НА ОПЕРАЦИОННЫХ ТАБЛИЦАХ
-- ============================================================

-- chats — быстрый поиск чатов поставщика и клиента
CREATE INDEX IF NOT EXISTS chats_producer_idx ON chats(producer_id);
CREATE INDEX IF NOT EXISTS chats_client_idx   ON chats(client_id);

-- orders — по чату и статусу (для лимита дня в Free-плане)
CREATE INDEX IF NOT EXISTS orders_chat_idx         ON orders(chat_id);
CREATE INDEX IF NOT EXISTS orders_status_idx       ON orders(status);
CREATE INDEX IF NOT EXISTS orders_chat_status_idx  ON orders(chat_id, status);
CREATE INDEX IF NOT EXISTS orders_created_at_idx   ON orders(created_at);

-- order_items — по заказу
CREATE INDEX IF NOT EXISTS order_items_order_idx   ON order_items(order_id);
CREATE INDEX IF NOT EXISTS order_items_product_idx ON order_items(product_id);

-- order_changes — для истории (сортировка по ts)
CREATE INDEX IF NOT EXISTS order_changes_order_idx ON order_changes(order_id);
CREATE INDEX IF NOT EXISTS order_changes_ts_idx    ON order_changes(ts);

-- messages — по чату и времени
CREATE INDEX IF NOT EXISTS messages_chat_idx ON messages(chat_id);
CREATE INDEX IF NOT EXISTS messages_ts_idx   ON messages(ts);

-- order_comments — по заказу
CREATE INDEX IF NOT EXISTS order_comments_order_idx ON order_comments(order_id);

-- debt_transactions — по чату, статусу и времени (ключевой для расчёта долга)
CREATE INDEX IF NOT EXISTS debt_chat_idx          ON debt_transactions(chat_id);
CREATE INDEX IF NOT EXISTS debt_status_idx        ON debt_transactions(status);
CREATE INDEX IF NOT EXISTS debt_chat_status_idx   ON debt_transactions(chat_id, status);
CREATE INDEX IF NOT EXISTS debt_created_at_idx    ON debt_transactions(created_at);

-- products — по поставщику и категории
CREATE INDEX IF NOT EXISTS products_producer_idx  ON products(producer_id);
CREATE INDEX IF NOT EXISTS products_category_idx  ON products(category_id);
CREATE INDEX IF NOT EXISTS products_active_idx    ON products(producer_id, is_active);

-- GIN-индекс для fuzzy search по названию товара (pg_trgm, нужен для шага 2.2)
CREATE INDEX IF NOT EXISTS products_name_trgm_idx ON products USING gin(name gin_trgm_ops);

-- categories
CREATE INDEX IF NOT EXISTS categories_producer_idx ON categories(producer_id);

-- events — по пользователю и времени
CREATE INDEX IF NOT EXISTS events_user_idx ON events(user_id);
CREATE INDEX IF NOT EXISTS events_ts_idx   ON events(ts);
CREATE INDEX IF NOT EXISTS events_type_idx ON events(type);

-- subscriptions — для быстрого получения плана поставщика
CREATE INDEX IF NOT EXISTS subscriptions_plan_idx ON subscriptions(plan_id);

-- guest_sessions — для очистки истёкших
CREATE INDEX IF NOT EXISTS guest_sessions_expires_idx ON guest_sessions(expires_at);
CREATE INDEX IF NOT EXISTS guest_sessions_producer_idx ON guest_sessions(producer_id);

-- volume_tiers — по поставщику
CREATE INDEX IF NOT EXISTS volume_tiers_producer_idx ON volume_tiers(producer_id);

-- product_discounts — по товару и сроку
CREATE INDEX IF NOT EXISTS product_discounts_product_idx ON product_discounts(product_id);

-- producers — по producer_token (гостевой каталог, шаг 4.2)
CREATE INDEX IF NOT EXISTS producers_token_idx ON producers(producer_token)
    WHERE producer_token IS NOT NULL;
