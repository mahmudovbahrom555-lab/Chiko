-- Migration 015: demand_items.status + demand_preferences (история сопоставлений).
--
-- Заменяем is_included (bool) на status (open→proposed→ordered).
-- demand_preferences запоминает: в этом чате для позиции с таким именем
-- производитель уже выбирал этот товар. Suggestions будут его поднимать первым.

-- 1. Замена is_included → status
ALTER TABLE demand_items
    DROP COLUMN IF EXISTS is_included;

ALTER TABLE demand_items
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'open'
    CHECK (status IN ('open', 'proposed', 'ordered'));

-- Индекс: быстро найти позиции по статусу (GetSuggestions фильтрует open).
CREATE INDEX IF NOT EXISTS demand_items_status_idx
    ON demand_items(chat_id, status);

-- 2. demand_preferences — память о выборе производителя.
--    PRIMARY KEY (chat_id, name_normalized) — один preferred на пару чат+название.
CREATE TABLE IF NOT EXISTS demand_preferences (
    chat_id         UUID        NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    name_normalized TEXT        NOT NULL,   -- LOWER(TRIM(demand_item.name))
    product_id      UUID        NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chat_id, name_normalized)
);
