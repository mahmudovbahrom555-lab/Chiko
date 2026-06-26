-- Migration 013: demand_items — розница записывает что ей нужно (Вариант Б).
--
-- Список спроса приватен внутри чата: видят только участники (producer + client).
-- Производитель читает список и формирует черновик заказа из своего каталога.
-- Черновик → совместное редактирование → подтверждение (существующий flow).

CREATE TABLE IF NOT EXISTS demand_items (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id     UUID        NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    created_by  UUID        NOT NULL,           -- auth.users (обычно клиент)
    name        TEXT        NOT NULL,
    qty         NUMERIC(15,3) NOT NULL DEFAULT 1 CHECK (qty > 0),
    unit        TEXT        NOT NULL DEFAULT 'шт',
    note        TEXT,
    is_filled   BOOLEAN     NOT NULL DEFAULT FALSE, -- производитель включил в заказ
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS demand_items_chat_idx ON demand_items(chat_id);

-- Автообновление updated_at.
CREATE TRIGGER demand_items_updated_at
    BEFORE UPDATE ON demand_items
    FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();

-- RLS: только участники чата.
ALTER TABLE demand_items ENABLE ROW LEVEL SECURITY;

CREATE POLICY demand_items_select ON demand_items
    FOR SELECT USING (is_chat_participant(chat_id));

CREATE POLICY demand_items_insert ON demand_items
    FOR INSERT WITH CHECK (
        is_chat_participant(chat_id) AND created_by = auth.uid()
    );

CREATE POLICY demand_items_update ON demand_items
    FOR UPDATE USING (is_chat_participant(chat_id));

CREATE POLICY demand_items_delete ON demand_items
    FOR DELETE USING (created_by = auth.uid());
