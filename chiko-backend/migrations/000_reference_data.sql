-- ШАГ 0: Справочники (currencies, regions)
-- Создаётся ПЕРВЫМ — всё остальное зависит от FK на эти таблицы.
-- Нет RLS — публичные справочники, доступны всем авторизованным.

CREATE TABLE IF NOT EXISTS currencies (
    code    CHAR(3)      PRIMARY KEY,   -- ISO 4217 код (UZS, USD, EUR ...)
    name_ru TEXT         NOT NULL,
    name_en TEXT         NOT NULL
);

CREATE TABLE IF NOT EXISTS regions (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    country_code     CHAR(2)     NOT NULL,   -- ISO 3166-1 alpha-2 (UZ, RU, KZ ...)
    region_code      TEXT        NOT NULL,   -- ISO 3166-2 (UZ-TK, UZ-AN ...) или == country_code для стран
    name_ru          TEXT        NOT NULL,
    name_en          TEXT        NOT NULL,
    name_local       TEXT,                   -- Название на языке страны
    parent_region_id UUID        REFERENCES regions(id) ON DELETE SET NULL,
    UNIQUE (country_code, region_code)       -- нужен для ON CONFLICT в seed-файлах
);

-- Индексы для Discover (выключен в MVP, но справочник готов)
CREATE INDEX IF NOT EXISTS regions_country_code_idx ON regions(country_code);
CREATE INDEX IF NOT EXISTS regions_parent_idx ON regions(parent_region_id) WHERE parent_region_id IS NOT NULL;
