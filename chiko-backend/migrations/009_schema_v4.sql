-- ШАГ v4: поправки схемы по второму аудиту (Sprints v4 / TZ v3.11)

-- ============================================================
-- 1. chats.created_via — фиксирует путь создания чата (ТЗ 12.7, шаг 1.5)
-- ============================================================
ALTER TABLE chats
    ADD COLUMN IF NOT EXISTS created_via TEXT NOT NULL DEFAULT 'producer_added';

ALTER TABLE chats
    ADD CONSTRAINT chats_created_via_check
    CHECK (created_via IN ('producer_added', 'guest_link'));

-- 2. chats.client_phone_pending — телефон клиента до первого входа
--    Заполняется при создании пути (а); очищается когда client_id получен.
ALTER TABLE chats
    ADD COLUMN IF NOT EXISTS client_phone_pending TEXT;

-- ============================================================
-- 3. return_requests.resulting_transaction_id
--    Ссылка на debt_transaction, созданную корректировкой возврата (ТЗ 7).
-- ============================================================
ALTER TABLE return_requests
    ADD COLUMN IF NOT EXISTS resulting_transaction_id UUID
    REFERENCES debt_transactions(id) ON DELETE SET NULL;

-- ============================================================
-- 4. feature_flags.value_numeric — числовые параметры флагов
--    Поле scope описывает область ('global'/'producer'), а не хранит число.
--    (ТЗ 12.1: "value_numeric — отдельное поле для числовых параметров")
-- ============================================================
ALTER TABLE feature_flags
    ADD COLUMN IF NOT EXISTS value_numeric NUMERIC;

-- Переносим захардкоженный SLA-порог из scope в value_numeric
UPDATE feature_flags
SET    value_numeric = 48
WHERE  key = 'sla_return_hours'
  AND  value_numeric IS NULL;

-- ============================================================
-- 5. producers.producer_token → guest_token
--    TZ v3.11 / Sprints v4 называют это поле guest_token.
--    URL-параметр остаётся :producer_token (это имя параметра пути).
-- ============================================================
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name='producers' AND column_name='producer_token'
    ) THEN
        ALTER TABLE producers RENAME COLUMN producer_token TO guest_token;
    END IF;
END $$;

-- Обновляем индекс
DROP INDEX IF EXISTS producers_token_idx;
CREATE INDEX IF NOT EXISTS producers_guest_token_idx
    ON producers(guest_token)
    WHERE guest_token IS NOT NULL;

-- ============================================================
-- 6. RLS: return_requests — разрешить UPDATE resulting_transaction_id
--    (устанавливается сервисом при создании return_correction, через service_role)
-- ============================================================
-- Текущая политика update = false уже верная для прямых клиентских запросов.
-- service_role обходит RLS — дополнительных изменений не требуется.
