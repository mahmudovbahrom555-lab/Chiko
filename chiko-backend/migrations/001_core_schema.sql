-- ШАГ 1.1: Полная схема БД
-- ЗАВИСИТ ОТ: 000_reference_data.sql (currencies, regions должны существовать)

-- Расширение для fuzzy search (создаём здесь, используем в 002)
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ============================================================
-- ПРОИЗВОДИТЕЛИ
-- ============================================================
CREATE TABLE IF NOT EXISTS producers (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name                  TEXT        NOT NULL,
    catalog_currency      CHAR(3)     NOT NULL REFERENCES currencies(code),
    onboarding_completed  BOOLEAN     NOT NULL DEFAULT FALSE,
    push_token            TEXT,
    push_enabled          BOOLEAN     NOT NULL DEFAULT TRUE,
    producer_token        TEXT        UNIQUE,   -- для гостевого каталога (шаг 4.2)
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- ЧАТЫ (пара producer ↔ client)
-- ============================================================
CREATE TABLE IF NOT EXISTS chats (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    producer_id UUID        NOT NULL REFERENCES producers(id) ON DELETE CASCADE,
    client_id   UUID        NOT NULL,   -- ссылается на auth.users (Supabase)
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (producer_id, client_id)
);

-- ============================================================
-- КАТЕГОРИИ ТОВАРОВ
-- ============================================================
CREATE TABLE IF NOT EXISTS categories (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    producer_id UUID        NOT NULL REFERENCES producers(id) ON DELETE CASCADE,
    name        TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- ТОВАРЫ
-- ============================================================
CREATE TABLE IF NOT EXISTS products (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    producer_id UUID        NOT NULL REFERENCES producers(id) ON DELETE CASCADE,
    category_id UUID        REFERENCES categories(id) ON DELETE SET NULL,
    name        TEXT        NOT NULL,
    unit        TEXT        NOT NULL DEFAULT 'шт',
    price       NUMERIC(15,2) NOT NULL CHECK (price >= 0),
    stock_qty   NUMERIC(15,3) NOT NULL DEFAULT 0,
    is_active   BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- ЗАКАЗЫ
-- ============================================================
CREATE TABLE IF NOT EXISTS orders (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id             UUID        NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    status              TEXT        NOT NULL DEFAULT 'draft',
    created_by          UUID        NOT NULL,   -- auth.users
    confirmed_by        UUID,                   -- auth.users, кто подтвердил
    confirmed_at        TIMESTAMPTZ,
    current_items_jsonb JSONB       NOT NULL DEFAULT '[]'::jsonb,  -- денормализованный снапшот
    total               NUMERIC(15,2) NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- ПОЗИЦИИ ЗАКАЗА
-- ============================================================
CREATE TABLE IF NOT EXISTS order_items (
    id           UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id     UUID          NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id   UUID          NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    qty          NUMERIC(15,3) NOT NULL CHECK (qty > 0),
    price        NUMERIC(15,2) NOT NULL CHECK (price >= 0),  -- зафиксированная цена
    added_by     UUID          NOT NULL,   -- auth.users
    locked_by    UUID,                     -- auth.users, мягкая блокировка строки
    locked_until TIMESTAMPTZ,              -- истекает через 3 сек
    created_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

-- ============================================================
-- ИСТОРИЯ ИЗМЕНЕНИЙ ЗАКАЗА (append-only)
-- ============================================================
CREATE TABLE IF NOT EXISTS order_changes (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id   UUID        NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    field      TEXT        NOT NULL,
    old_val    TEXT,
    new_val    TEXT,
    changed_by UUID        NOT NULL,   -- auth.users
    ts         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- СООБЩЕНИЯ ЧАТА
-- ============================================================
CREATE TABLE IF NOT EXISTS messages (
    id        UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id   UUID        NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    type      TEXT        NOT NULL DEFAULT 'text',   -- text | voice | system
    text      TEXT,
    voice_url TEXT,
    author_id UUID        NOT NULL,   -- auth.users
    ts        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- КОММЕНТАРИИ К ЗАКАЗУ
-- ============================================================
CREATE TABLE IF NOT EXISTS order_comments (
    id        UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id  UUID        NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    text      TEXT,
    voice_url TEXT,
    author_id UUID        NOT NULL,
    ts        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- ДОЛГОВЫЕ ТРАНЗАКЦИИ (append-only! никогда UPDATE/DELETE)
-- ============================================================
CREATE TABLE IF NOT EXISTS debt_transactions (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id         UUID          NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    type            TEXT          NOT NULL,   -- delivery | payment | return_correction | correction
    amount          NUMERIC(15,2) NOT NULL CHECK (amount > 0),
    sign            SMALLINT      NOT NULL CHECK (sign IN (1, -1)),  -- +1 долг растёт, -1 падает
    initiator_id    UUID          NOT NULL,   -- auth.users
    confirmed_by_id UUID,                     -- auth.users
    confirmed_at    TIMESTAMPTZ,
    status          TEXT          NOT NULL DEFAULT 'pending',
    comment         TEXT,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

-- ============================================================
-- СКИДКИ КЛИЕНТА
-- ============================================================
CREATE TABLE IF NOT EXISTS client_discounts (
    chat_id              UUID          NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    discount_pct         NUMERIC(5,2)  NOT NULL CHECK (discount_pct >= 0 AND discount_pct <= 100),
    created_by_producer  UUID          NOT NULL,
    created_at           TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chat_id)
);

-- ============================================================
-- СКИДКИ НА ТОВАРЫ
-- ============================================================
CREATE TABLE IF NOT EXISTS product_discounts (
    id           UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id   UUID          NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    discount_pct NUMERIC(5,2)  NOT NULL CHECK (discount_pct >= 0 AND discount_pct <= 100),
    valid_until  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

-- ============================================================
-- ОБЪЁМНЫЕ СКИДКИ (пороги)
-- ============================================================
CREATE TABLE IF NOT EXISTS volume_tiers (
    id           UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    producer_id  UUID          NOT NULL REFERENCES producers(id) ON DELETE CASCADE,
    type         TEXT          NOT NULL,   -- quantity | amount
    threshold    NUMERIC(15,2) NOT NULL CHECK (threshold > 0),
    discount_pct NUMERIC(5,2)  NOT NULL CHECK (discount_pct >= 0 AND discount_pct <= 100),
    applies_to   TEXT          NOT NULL DEFAULT 'order',  -- order | product
    created_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

-- ============================================================
-- МЕТРИКИ КЛИЕНТОВ (фундамент Sales Intelligence, не отображается в MVP)
-- ============================================================
CREATE TABLE IF NOT EXISTS client_metrics (
    chat_id                UUID          PRIMARY KEY REFERENCES chats(id) ON DELETE CASCADE,
    last_order_at          TIMESTAMPTZ,
    avg_order_cycle_days   NUMERIC(8,2),
    avg_order_value        NUMERIC(15,2),
    order_count            INTEGER       NOT NULL DEFAULT 0,
    growth_rate            NUMERIC(8,4),
    total_receivables      NUMERIC(15,2) NOT NULL DEFAULT 0,
    dispute_count          INTEGER       NOT NULL DEFAULT 0,
    payment_delay_avg_days NUMERIC(8,2),
    chat_message_count     INTEGER       NOT NULL DEFAULT 0,
    relationship_score     NUMERIC(5,2), -- пустое в MVP
    updated_at             TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

-- ============================================================
-- СОБЫТИЯ (фундамент аналитики)
-- ============================================================
CREATE TABLE IF NOT EXISTS events (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID        NOT NULL,
    type         TEXT        NOT NULL,   -- catalog_open | product_view | order_create | order_confirm | order_cancel
    entity_id    UUID,
    payload_jsonb JSONB,
    ts           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- FEATURE FLAGS
-- ============================================================
CREATE TABLE IF NOT EXISTS feature_flags (
    key       TEXT    PRIMARY KEY,
    enabled   BOOLEAN NOT NULL DEFAULT FALSE,
    scope     TEXT    NOT NULL DEFAULT 'global'
);

-- ============================================================
-- ТАРИФНЫЕ ПЛАНЫ
-- ============================================================
CREATE TABLE IF NOT EXISTS plans (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT          NOT NULL UNIQUE,
    price               NUMERIC(10,2) NOT NULL DEFAULT 0,
    order_limit_per_day INTEGER,       -- NULL = безлимит
    features_jsonb      JSONB         NOT NULL DEFAULT '{}'::jsonb,
    active              BOOLEAN       NOT NULL DEFAULT TRUE
);

-- ============================================================
-- ПОДПИСКИ ПОСТАВЩИКОВ
-- ============================================================
CREATE TABLE IF NOT EXISTS subscriptions (
    producer_id   UUID        PRIMARY KEY REFERENCES producers(id) ON DELETE CASCADE,
    plan_id       UUID        NOT NULL REFERENCES plans(id),
    trial_ends_at TIMESTAMPTZ,
    status        TEXT        NOT NULL DEFAULT 'trial',
    renews_at     TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- ГОСТЕВЫЕ СЕССИИ (шаг 4.2)
-- ============================================================
CREATE TABLE IF NOT EXISTS guest_sessions (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    producer_id  UUID        NOT NULL REFERENCES producers(id) ON DELETE CASCADE,
    cart_jsonb   JSONB       NOT NULL DEFAULT '[]'::jsonb,
    expires_at   TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '7 days'),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
