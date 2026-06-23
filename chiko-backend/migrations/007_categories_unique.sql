-- Баг 3: уникальность (producer_id, name) на categories.
-- Без этого ON CONFLICT DO NOTHING в EnsureDefaultCategories
-- работает только по UUID PK — дубли не предотвращались.

ALTER TABLE categories
    ADD CONSTRAINT categories_producer_name_unique
    UNIQUE (producer_id, name);
