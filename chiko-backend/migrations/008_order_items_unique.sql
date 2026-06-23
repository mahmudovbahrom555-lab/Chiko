-- Нужен для ON CONFLICT (order_id, product_id) в UpsertItem (шаг 3.1).
-- Один и тот же товар в одном заказе — одна строка с qty.

ALTER TABLE order_items
    ADD CONSTRAINT order_items_order_product_unique
    UNIQUE (order_id, product_id);
