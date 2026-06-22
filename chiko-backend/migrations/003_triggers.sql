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
-- Срабатывает: AFTER INSERT ON debt_transactions
-- Пересчитывает баланс по формуле из ТЗ раздел 6.2:
--   SUM(amount * sign) WHERE status IN ('pending', 'confirmed')
-- ============================================================
CREATE OR REPLACE FUNCTION fn_update_client_metrics_on_debt()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_balance NUMERIC(15,2);
BEGIN
    -- Текущий баланс по чату (Pending + Confirmed, без Disputed)
    SELECT COALESCE(SUM(amount * sign), 0)
    INTO   v_balance
    FROM   debt_transactions
    WHERE  chat_id = NEW.chat_id
      AND  status IN ('pending', 'confirmed');

    -- Upsert в client_metrics
    INSERT INTO client_metrics (chat_id, total_receivables, updated_at)
    VALUES (NEW.chat_id, v_balance, NOW())
    ON CONFLICT (chat_id) DO UPDATE
    SET total_receivables = v_balance,
        updated_at        = NOW();

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_update_metrics_on_debt ON debt_transactions;
CREATE TRIGGER trg_update_metrics_on_debt
    AFTER INSERT ON debt_transactions
    FOR EACH ROW
    EXECUTE FUNCTION fn_update_client_metrics_on_debt();


-- ============================================================
-- ТРИГГЕР 3: Запрет UPDATE/DELETE на debt_transactions (append-only)
-- Принцип 1 из master-context: журнал неизменяем.
-- ============================================================
CREATE OR REPLACE FUNCTION fn_debt_readonly()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'debt_transactions is append-only. Use INSERT with type=''correction'' to fix mistakes.';
END;
$$;

DROP TRIGGER IF EXISTS trg_debt_no_update ON debt_transactions;
CREATE TRIGGER trg_debt_no_update
    BEFORE UPDATE ON debt_transactions
    FOR EACH ROW
    EXECUTE FUNCTION fn_debt_readonly();

DROP TRIGGER IF EXISTS trg_debt_no_delete ON debt_transactions;
CREATE TRIGGER trg_debt_no_delete
    BEFORE DELETE ON debt_transactions
    FOR EACH ROW
    EXECUTE FUNCTION fn_debt_readonly();


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
