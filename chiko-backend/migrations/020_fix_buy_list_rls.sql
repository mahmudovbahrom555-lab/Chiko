-- Migration 020: исправление критических RLS багов в buy_list (migration 019).
--
-- Баг 1: orders_anon_select USING(guest_token IS NOT NULL AND ... IS NOT NULL)
--   из-за приоритета операторов IS NOT NULL применялся к BOOLEAN результату,
--   делая условие эффективно USING(guest_token IS NOT NULL) — видны ВСЕ заказы.
-- Баг 2: order_items_anon_select не сравнивал guest_token с запросом клиента.
--
-- Также добавляем RLS на demand_preferences и buy_list_mappings (missing в 015/018).

-- ── Исправляем orders_anon_select ────────────────────────────────────────────
DROP POLICY IF EXISTS orders_anon_select ON orders;

-- Анон видит заказ ТОЛЬКО если знает конкретный guest_token.
-- UUID сравниваем через TEXT чтобы избежать implicit cast ошибок.
-- Суpabase передаёт заголовки через current_setting('request.headers').
-- Если заголовок не передан — функция вернёт '' и сравнение провалится.
CREATE POLICY orders_anon_select ON orders
    FOR SELECT TO anon
    USING (
        guest_token IS NOT NULL
        AND guest_token::text = COALESCE(
            (current_setting('request.headers', true)::jsonb->>'x-guest-token'),
            ''
        )
    );

-- ── Исправляем order_items_anon_select ───────────────────────────────────────
DROP POLICY IF EXISTS order_items_anon_select ON order_items;

CREATE POLICY order_items_anon_select ON order_items
    FOR SELECT TO anon
    USING (
        EXISTS (
            SELECT 1 FROM orders o
            WHERE o.id = order_items.order_id
              AND o.status = 'guest_buy_list'
              AND o.guest_token IS NOT NULL
              AND o.guest_token::text = COALESCE(
                  (current_setting('request.headers', true)::jsonb->>'x-guest-token'),
                  ''
              )
        )
    );

-- ── RLS для demand_preferences ───────────────────────────────────────────────
ALTER TABLE demand_preferences ENABLE ROW LEVEL SECURITY;

DO $$ BEGIN
    CREATE POLICY demand_preferences_participant ON demand_preferences
        FOR ALL USING (is_chat_participant(chat_id));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- ── RLS для buy_list_mappings ─────────────────────────────────────────────────
ALTER TABLE buy_list_mappings ENABLE ROW LEVEL SECURITY;

DO $$ BEGIN
    CREATE POLICY buy_list_mappings_participant ON buy_list_mappings
        FOR ALL USING (is_chat_participant(chat_id));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- ── Добавляем demand event types в events.type CHECK ─────────────────────────
-- Текущий CHECK (из 002) не включает demand_item_* события.
-- Все logEvent() в demand/service.go молча падали.
DO $$ BEGIN
    ALTER TABLE events DROP CONSTRAINT IF EXISTS events_type_check;
    ALTER TABLE events ADD CONSTRAINT events_type_check CHECK (type IN (
        -- каталог
        'catalog_open', 'product_view',
        -- заказы
        'order_create', 'order_confirm', 'order_cancel', 'draft_save',
        -- demand (добавлено)
        'demand_item_added', 'demand_item_proposed',
        'demand_item_ordered', 'demand_item_cancelled',
        -- push
        'push_disabled'
    ));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- ── Замещающий индекс для demand_items (status='open' вместо is_included) ───
-- Migration 014 создал индекс WHERE is_included = FALSE,
-- migration 015 удалила is_included колонку (индекс удалён автоматически).
-- Создаём правильный эквивалент:
CREATE INDEX IF NOT EXISTS demand_items_open_urgency_idx
    ON demand_items(chat_id, urgency)
    WHERE status = 'open';
