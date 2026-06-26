-- Migration 016: добавляем статус 'cancelled' в demand_items.
--
-- Без cancelled открытые позиции накапливаются вечно.
-- Кто может отменить: любой участник чата.
-- Отменённые позиции остаются видимыми (с badge "Отменено") —
-- не удаляются, чтобы сохранить историю.

ALTER TABLE demand_items
    DROP CONSTRAINT IF EXISTS demand_items_status_check;

ALTER TABLE demand_items
    ADD CONSTRAINT demand_items_status_check
    CHECK (status IN ('open', 'proposed', 'ordered', 'cancelled'));
