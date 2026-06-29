-- Migration 018: Buy List schema (ТЗ раздел 5, Шаг 3.3).
--
-- Buy List — вход в Chiko без регистрации: магазин пишет список как заметку,
-- поставщик сопоставляет строки с каталогом → черновик заказа.
--
-- Инвариант (ТЗ 5.7): Buy List — это НЕ отдельная таблица.
-- orders.status='guest_buy_list' → 'draft' → 'confirmed' без копирования данных.
-- Переход через UPDATE client_id + status на той же строке.

-- ── 1. orders: добавляем поля для гостевого заказа ───────────────────────────
ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS guest_token     UUID        UNIQUE,
    ADD COLUMN IF NOT EXISTS created_by_guest BOOLEAN    NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS guest_phone     TEXT,
    ADD COLUMN IF NOT EXISTS expires_at      TIMESTAMPTZ;

-- Добавляем 'guest_buy_list' в допустимые статусы.
-- Используем DO$$EXCEPTION$$ для идемпотентности (migration 012-паттерн).
DO $$ BEGIN
    ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_status_check;
    ALTER TABLE orders ADD CONSTRAINT orders_status_check
        CHECK (status IN ('guest_buy_list', 'draft', 'confirmed', 'cancelled'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- Индекс для поиска заказа по guest_token (публичный эндпоинт).
CREATE UNIQUE INDEX IF NOT EXISTS orders_guest_token_idx
    ON orders(guest_token)
    WHERE guest_token IS NOT NULL;

-- ── 2. order_items: raw_text + nullable product_id ────────────────────────────
-- raw_text: хранит исходную строку из Buy List до сопоставления с товаром.
-- product_id становится nullable: NULL = строка ещё не сопоставлена.
-- Несопоставленная строка НЕ входит в total/скидки/остатки (JOIN products пропускает NULL).

ALTER TABLE order_items
    ADD COLUMN IF NOT EXISTS raw_text TEXT;

-- Снимаем NOT NULL с product_id для поддержки несопоставленных строк.
DO $$
BEGIN
    ALTER TABLE order_items ALTER COLUMN product_id DROP NOT NULL;
EXCEPTION WHEN others THEN NULL; END $$;

-- ── 3. buy_list_mappings: память о сопоставлениях ────────────────────────────
-- Запоминает: в этом чате текст "масло" → product_id "Масло Premium 1Л".
-- Отличается от demand_preferences: там structured catalog name → product.
-- Здесь: free-text raw_text → product (нормализованный к LOWER(TRIM())).
CREATE TABLE IF NOT EXISTS buy_list_mappings (
    chat_id         UUID        NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    name_normalized TEXT        NOT NULL,   -- LOWER(TRIM(raw_text))
    product_id      UUID        NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chat_id, name_normalized)
);
