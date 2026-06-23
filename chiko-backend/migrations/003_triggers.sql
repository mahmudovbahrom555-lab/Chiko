-- ШАГ 1.1: Триггеры (минимальная бизнес-логика на уровне БД)
-- ЗАВИСИТ ОТ: 001_core_schema.sql, 002_constraints_indexes.sql

-- ============================================================
-- ТРИГГЕР 1: Авто-списание остатков при подтверждении заказа
-- Срабатывает: AFTER UPDATE ON orders WHERE NEW.status = 'confirmed'
-- Важно: если уходит в минус — не блокируем, просто пишем.
-- ============================================================
CREATE OR REPLACE FUNCTION fn_deduct_stock_on_confirm()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
BEGIN
    -- Только при переходе в confirmed (не при любом UPDATE)
    IF NEW.status = 'confirmed' AND OLD.status != 'confirmed' THEN
        UPDATE products p
        SET    stock_qty  = p.stock_qty - oi.qty,
               updated_at = NOW()
        FROM   order_items oi
        WHERE  oi.order_id   = NEW.id
          AND  oi.product_id = p.id;
    END IF;

    -- Восстановление остатков при отмене ранее подтверждённого заказа
    IF NEW.status = 'cancelled' AND OLD.status = 'confirmed' THEN
        UPDATE products p
        SET    stock_qty  = p.stock_qty + oi.qty,
               updated_at = NOW()
        FROM   order_items oi
        WHERE  oi.order_id   = NEW.id
          AND  oi.product_id = p.id;
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_deduct_stock_on_confirm ON orders;
CREATE TRIGGER trg_deduct_stock_on_confirm
    AFTER UPDATE OF status ON orders
    FOR EACH ROW
    EXECUTE FUNCTION fn_deduct_stock_on_confirm();


-- ============================================================
-- ТРИГГЕР 2: Обновление client_metrics.total_receivables
-- Срабатывает: AFTER INSERT OR UPDATE OF status ON debt_transactions
-- Пересчитывает баланс по формуле из ТЗ раздел 6.2:
--   SUM(amount * sign) WHERE status IN ('pending', 'confirmed')
-- Нужно на UPDATE тоже: pending→disputed выбрасывает запись из баланса.
-- ============================================================
CREATE OR REPLACE FUNCTION fn_update_client_metrics_on_debt()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_chat_id UUID;
    v_balance  NUMERIC(15,2);
BEGIN
    -- Определяем chat_id для пересчёта
    v_chat_id := COALESCE(NEW.chat_id, OLD.chat_id);

    SELECT COALESCE(SUM(amount * sign), 0)
    INTO   v_balance
    FROM   debt_transactions
    WHERE  chat_id = v_chat_id
      AND  status IN ('pending', 'confirmed');

    INSERT INTO client_metrics (chat_id, total_receivables, updated_at)
    VALUES (v_chat_id, v_balance, NOW())
    ON CONFLICT (chat_id) DO UPDATE
    SET total_receivables = v_balance,
        updated_at        = NOW();

    RETURN COALESCE(NEW, OLD);
END;
$$;

DROP TRIGGER IF EXISTS trg_update_metrics_on_debt ON debt_transactions;
CREATE TRIGGER trg_update_metrics_on_debt
    AFTER INSERT OR UPDATE OF status ON debt_transactions
    FOR EACH ROW
    EXECUTE FUNCTION fn_update_client_metrics_on_debt();


-- ============================================================
-- ТРИГГЕР 3: Частичный append-only на debt_transactions
-- Принцип 1 из master-context: финансовые данные неизменяемы.
-- НО: status, confirmed_by_id, confirmed_at — меняться ДОЛЖНЫ
--   (pending → confirmed/disputed — это рабочий процесс).
-- Блокируем изменение только финансово значимых полей.
-- ============================================================
CREATE OR REPLACE FUNCTION fn_debt_immutable_fields()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.amount       IS DISTINCT FROM NEW.amount       THEN
        RAISE EXCEPTION 'debt_transactions.amount is immutable';
    END IF;
    IF OLD.sign         IS DISTINCT FROM NEW.sign         THEN
        RAISE EXCEPTION 'debt_transactions.sign is immutable';
    END IF;
    IF OLD.type         IS DISTINCT FROM NEW.type         THEN
        RAISE EXCEPTION 'debt_transactions.type is immutable';
    END IF;
    IF OLD.chat_id      IS DISTINCT FROM NEW.chat_id      THEN
        RAISE EXCEPTION 'debt_transactions.chat_id is immutable';
    END IF;
    IF OLD.initiator_id IS DISTINCT FROM NEW.initiator_id THEN
        RAISE EXCEPTION 'debt_transactions.initiator_id is immutable';
    END IF;
    IF OLD.created_at   IS DISTINCT FROM NEW.created_at   THEN
        RAISE EXCEPTION 'debt_transactions.created_at is immutable';
    END IF;
    -- status, confirmed_by_id, confirmed_at, comment — разрешены к изменению
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_debt_no_update ON debt_transactions;
CREATE TRIGGER trg_debt_no_update
    BEFORE UPDATE ON debt_transactions
    FOR EACH ROW
    EXECUTE FUNCTION fn_debt_immutable_fields();

CREATE OR REPLACE FUNCTION fn_debt_no_delete()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'debt_transactions records cannot be deleted. Append a correction instead.';
END;
$$;

DROP TRIGGER IF EXISTS trg_debt_no_delete ON debt_transactions;
CREATE TRIGGER trg_debt_no_delete
    BEFORE DELETE ON debt_transactions
    FOR EACH ROW
    EXECUTE FUNCTION fn_debt_no_delete();


-- ============================================================
-- ТРИГГЕР 4: updated_at автообновление для ключевых таблиц
-- ============================================================
CREATE OR REPLACE FUNCTION fn_set_updated_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_producers_updated_at ON producers;
CREATE TRIGGER trg_producers_updated_at
    BEFORE UPDATE ON producers
    FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();

DROP TRIGGER IF EXISTS trg_products_updated_at ON products;
CREATE TRIGGER trg_products_updated_at
    BEFORE UPDATE ON products
    FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();

DROP TRIGGER IF EXISTS trg_orders_updated_at ON orders;
CREATE TRIGGER trg_orders_updated_at
    BEFORE UPDATE ON orders
    FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();

DROP TRIGGER IF EXISTS trg_subscriptions_updated_at ON subscriptions;
CREATE TRIGGER trg_subscriptions_updated_at
    BEFORE UPDATE ON subscriptions
    FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();
