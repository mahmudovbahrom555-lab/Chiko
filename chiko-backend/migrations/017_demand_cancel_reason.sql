-- Migration 017: cancel_reason для аналитики пилота.
--
-- Зачем: не просто "сколько отменили", а "почему".
-- Это главный инсайт для развития продукта после пилота.
--
-- Предустановленные причины (UI показывает кнопки):
--   no_stock        → товара нет у поставщика
--   price_mismatch  → цена не устроила
--   bought_elsewhere→ купили у другого поставщика
--   need_disappeared→ потребность исчезла (распродали остаток)
--   other           → другое (с комментарием)
--
-- Без CHECK constraint — хотим гибкость если причины расширятся.

ALTER TABLE demand_items
    ADD COLUMN IF NOT EXISTS cancel_reason TEXT,
    ADD COLUMN IF NOT EXISTS cancel_note   TEXT;
