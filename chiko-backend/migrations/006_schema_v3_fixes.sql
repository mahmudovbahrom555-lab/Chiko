-- ШАГ v3: Дополнения по аудиту Sprints v2 → v3
-- Три изменения: timezone, return_requests, return_correction constraint.

-- ============================================================
-- 1. producers.timezone
-- Нужен для корректного сброса дневного лимита заказов в 00:00
-- по локальному времени поставщика. Не хардкодить Asia/Tashkent.
-- ============================================================
ALTER TABLE producers
    ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT 'Asia/Tashkent';

-- ============================================================
-- 2. return_requests
-- Структурированный запрос возврата от клиента (шаг 1 flow раздела 7 ТЗ).
-- Отдельно от debt_transactions: хранит жалобу клиента + фото,
-- по created_at считается SLA-таймер 48 часов.
-- Когда producer создаёт return_correction в debt_transactions →
-- return_request.status = 'resolved'.
-- ============================================================
CREATE TABLE IF NOT EXISTS return_requests (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id      UUID        NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    order_id     UUID        NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    -- Позиции с проблемой: [{product_id, qty, reason}]
    items_jsonb  JSONB       NOT NULL DEFAULT '[]'::jsonb,
    photo_urls   TEXT[]      NOT NULL DEFAULT '{}',
    status       TEXT        NOT NULL DEFAULT 'pending',
    escalated    BOOLEAN     NOT NULL DEFAULT FALSE,
    created_by   UUID        NOT NULL,  -- auth.users (клиент)
    resolved_at  TIMESTAMPTZ,           -- когда producer создал correction
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE return_requests
    ADD CONSTRAINT return_requests_status_check
    CHECK (status IN ('pending', 'attention', 'resolved', 'disputed'));

CREATE INDEX IF NOT EXISTS return_requests_chat_idx
    ON return_requests(chat_id);

CREATE INDEX IF NOT EXISTS return_requests_status_idx
    ON return_requests(status);

-- Индекс для SLA-job: быстро найти просроченные необработанные запросы
CREATE INDEX IF NOT EXISTS return_requests_sla_idx
    ON return_requests(created_at)
    WHERE status IN ('pending', 'attention');

-- Эскалированные — закрепляются вверху списка у producer
CREATE INDEX IF NOT EXISTS return_requests_escalated_idx
    ON return_requests(chat_id, created_at DESC)
    WHERE escalated = TRUE;

-- ============================================================
-- 3. return_correction всегда confirmed
-- ТЗ раздел 7: "долг уменьшается на сумму возврата СРАЗУ, статус Confirmed
-- (это действие самого производителя, повторного подтверждения НЕ ТРЕБУЕТ)".
-- Гарантируем на уровне БД: нельзя создать return_correction с pending.
-- ============================================================
ALTER TABLE debt_transactions
    ADD CONSTRAINT debt_return_correction_confirmed
    CHECK (
        type != 'return_correction'
        OR status = 'confirmed'
    );

-- ============================================================
-- 4. RLS для return_requests
-- ============================================================
ALTER TABLE return_requests ENABLE ROW LEVEL SECURITY;

-- Обе стороны чата читают
CREATE POLICY return_requests_select ON return_requests
    FOR SELECT USING (is_chat_participant(chat_id));

-- Создаёт только клиент чата
CREATE POLICY return_requests_insert ON return_requests
    FOR INSERT WITH CHECK (
        is_chat_participant(chat_id)
        AND created_by = auth.uid()
        -- Только client может инициировать (producer создаёт correction напрямую)
        AND EXISTS (
            SELECT 1 FROM chats
            WHERE chats.id = return_requests.chat_id
              AND chats.client_id = auth.uid()
        )
    );

-- UPDATE только через систему (статус escalated/attention/resolved меняет Go-сервис)
-- Клиент меняет на 'disputed' через отдельный endpoint (Go использует service_role)
CREATE POLICY return_requests_update ON return_requests
    FOR UPDATE USING (false);  -- прямой UPDATE запрещён, только через service_role
