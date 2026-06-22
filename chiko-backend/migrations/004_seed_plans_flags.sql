-- ШАГ 1.1: Начальные данные — тарифы и feature flags
-- ЗАВИСИТ ОТ: 001_core_schema.sql

-- ============================================================
-- ТАРИФНЫЕ ПЛАНЫ
-- Принцип 6 ТЗ: тарифы в конфигурации, не в хардкоде.
-- order_limit_per_day = NULL → безлимит
-- ============================================================
INSERT INTO plans (name, price, order_limit_per_day, features_jsonb, active)
VALUES
    (
        'Trial',
        0.00,
        NULL,
        '{"unlimited_orders": true, "analytics": true, "excel_import": true, "guest_catalog": true, "trial_days": 90}'::jsonb,
        true
    ),
    (
        'Free',
        0.00,
        5,
        '{"unlimited_orders": false, "analytics": false, "excel_import": true, "guest_catalog": true}'::jsonb,
        true
    ),
    (
        'Pro',
        4.99,
        NULL,
        '{"unlimited_orders": true, "analytics": true, "excel_import": true, "guest_catalog": true}'::jsonb,
        true
    )
ON CONFLICT (name) DO NOTHING;

-- ============================================================
-- FEATURE FLAGS
-- discover — выключен в MVP (раздел 14 ТЗ)
-- ============================================================
INSERT INTO feature_flags (key, enabled, scope)
VALUES
    ('discover',              false, 'global'),
    ('sla_return_hours',      true,  'global'),   -- порог SLA для возвратов (значение в конфиге Go)
    ('push_full_screen',      true,  'global'),
    ('guest_catalog',         true,  'global'),
    ('analytics_dashboard',   true,  'global'),
    ('excel_import',          true,  'global'),
    ('voice_messages',        true,  'global')
ON CONFLICT (key) DO NOTHING;
