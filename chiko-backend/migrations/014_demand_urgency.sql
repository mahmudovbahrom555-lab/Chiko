-- Migration 014: добавляем urgency и переименовываем is_filled → is_included.
--
-- urgency: приоритет потребности, задаётся розницей.
-- is_included: производитель добавил позицию в черновик заказа.
--   Статус самого заказа отслеживается в orders — дублировать нет смысла.

ALTER TABLE demand_items
    ADD COLUMN IF NOT EXISTS urgency TEXT NOT NULL DEFAULT 'planned'
    CHECK (urgency IN ('urgent', 'soon', 'planned'));

-- Переименовываем is_filled → is_included (clearer intent).
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name='demand_items' AND column_name='is_filled'
    ) THEN
        ALTER TABLE demand_items RENAME COLUMN is_filled TO is_included;
    END IF;
END $$;

-- Индекс для фильтрации по срочности (производитель видит urgent первыми).
CREATE INDEX IF NOT EXISTS demand_items_urgency_idx
    ON demand_items(chat_id, urgency)
    WHERE is_included = FALSE;
