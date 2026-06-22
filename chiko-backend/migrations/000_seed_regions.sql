-- ISO 3166-2 regions seed
-- Структура: страны как корневые записи (parent_region_id = NULL),
-- регионы/области внутри стран с parent_region_id на свою страну.
-- Детальные регионы: UZ, KZ, KG, TJ, TM, RU, AZ, GE, UA (приоритетные рынки).
-- Остальные страны — только корневая запись. Регионы добавляются отдельными миграциями.

-- ============================================================
-- СТРАНЫ (все, в алфавитном порядке по country_code)
-- ============================================================
INSERT INTO regions (country_code, region_code, name_ru, name_en, name_local, parent_region_id) VALUES
('AD', 'AD', 'Андорра',                         'Andorra',                           'Andorra',                     NULL),
('AE', 'AE', 'ОАЭ',                             'United Arab Emirates',              'الإمارات العربية المتحدة',    NULL),
('AF', 'AF', 'Афганистан',                      'Afghanistan',                       'افغانستان',                   NULL),
('AG', 'AG', 'Антигуа и Барбуда',               'Antigua and Barbuda',               'Antigua and Barbuda',         NULL),
('AL', 'AL', 'Албания',                         'Albania',                           'Shqipëria',                   NULL),
('AM', 'AM', 'Армения',                         'Armenia',                           'Հայաստան',                   NULL),
('AO', 'AO', 'Ангола',                          'Angola',                            'Angola',                      NULL),
('AR', 'AR', 'Аргентина',                       'Argentina',                         'Argentina',                   NULL),
('AT', 'AT', 'Австрия',                         'Austria',                           'Österreich',                  NULL),
('AU', 'AU', 'Австралия',                       'Australia',                         'Australia',                   NULL),
('AZ', 'AZ', 'Азербайджан',                     'Azerbaijan',                        'Azərbaycan',                  NULL),
('BA', 'BA', 'Босния и Герцеговина',            'Bosnia and Herzegovina',            'Bosna i Hercegovina',         NULL),
('BB', 'BB', 'Барбадос',                        'Barbados',                          'Barbados',                    NULL),
('BD', 'BD', 'Бангладеш',                       'Bangladesh',                        'বাংলাদেশ',                   NULL),
('BE', 'BE', 'Бельгия',                         'Belgium',                           'België',                      NULL),
('BF', 'BF', 'Буркина-Фасо',                    'Burkina Faso',                      'Burkina Faso',                NULL),
('BG', 'BG', 'Болгария',                        'Bulgaria',                          'България',                   NULL),
('BH', 'BH', 'Бахрейн',                         'Bahrain',                           'البحرين',                     NULL),
('BI', 'BI', 'Бурунди',                         'Burundi',                           'Burundi',                     NULL),
('BJ', 'BJ', 'Бенин',                           'Benin',                             'Bénin',                       NULL),
('BN', 'BN', 'Бруней',                          'Brunei',                            'Brunei',                      NULL),
('BO', 'BO', 'Боливия',                         'Bolivia',                           'Bolivia',                     NULL),
('BR', 'BR', 'Бразилия',                        'Brazil',                            'Brasil',                      NULL),
('BS', 'BS', 'Багамы',                          'Bahamas',                           'Bahamas',                     NULL),
('BT', 'BT', 'Бутан',                           'Bhutan',                            'འབྲུག',                      NULL),
('BW', 'BW', 'Ботсвана',                        'Botswana',                          'Botswana',                    NULL),
('BY', 'BY', 'Беларусь',                        'Belarus',                           'Беларусь',                    NULL),
('BZ', 'BZ', 'Белиз',                           'Belize',                            'Belize',                      NULL),
('CA', 'CA', 'Канада',                          'Canada',                            'Canada',                      NULL),
('CD', 'CD', 'ДР Конго',                        'DR Congo',                          'RD Congo',                    NULL),
('CF', 'CF', 'ЦАР',                             'Central African Republic',          'République centrafricaine',   NULL),
('CG', 'CG', 'Конго',                           'Republic of the Congo',             'République du Congo',         NULL),
('CH', 'CH', 'Швейцария',                       'Switzerland',                       'Schweiz',                     NULL),
('CI', 'CI', 'Кот-д'Ивуар',                    'Ivory Coast',                       'Côte d''Ivoire',              NULL),
('CL', 'CL', 'Чили',                            'Chile',                             'Chile',                       NULL),
('CM', 'CM', 'Камерун',                         'Cameroon',                          'Cameroun',                    NULL),
('CN', 'CN', 'Китай',                           'China',                             '中国',                        NULL),
('CO', 'CO', 'Колумбия',                        'Colombia',                          'Colombia',                    NULL),
('CR', 'CR', 'Коста-Рика',                      'Costa Rica',                        'Costa Rica',                  NULL),
('CU', 'CU', 'Куба',                            'Cuba',                              'Cuba',                        NULL),
('CV', 'CV', 'Кабо-Верде',                      'Cape Verde',                        'Cabo Verde',                  NULL),
('CY', 'CY', 'Кипр',                            'Cyprus',                            'Κύπρος',                     NULL),
('CZ', 'CZ', 'Чехия',                           'Czech Republic',                    'Česká republika',             NULL),
('DE', 'DE', 'Германия',                        'Germany',                           'Deutschland',                 NULL),
('DJ', 'DJ', 'Джибути',                         'Djibouti',                          'Djibouti',                    NULL),
('DK', 'DK', 'Дания',                           'Denmark',                           'Danmark',                     NULL),
('DM', 'DM', 'Доминика',                        'Dominica',                          'Dominica',                    NULL),
('DO', 'DO', 'Доминиканская Республика',        'Dominican Republic',                'República Dominicana',        NULL),
('DZ', 'DZ', 'Алжир',                           'Algeria',                           'الجزائر',                     NULL),
('EC', 'EC', 'Эквадор',                         'Ecuador',                           'Ecuador',                     NULL),
('EE', 'EE', 'Эстония',                         'Estonia',                           'Eesti',                       NULL),
('EG', 'EG', 'Египет',                          'Egypt',                             'مصر',                         NULL),
('ER', 'ER', 'Эритрея',                         'Eritrea',                           'ኤርትራ',                      NULL),
('ES', 'ES', 'Испания',                         'Spain',                             'España',                      NULL),
('ET', 'ET', 'Эфиопия',                         'Ethiopia',                          'ኢትዮጵያ',                     NULL),
('FI', 'FI', 'Финляндия',                       'Finland',                           'Suomi',                       NULL),
('FJ', 'FJ', 'Фиджи',                           'Fiji',                              'Fiji',                        NULL),
('FR', 'FR', 'Франция',                         'France',                            'France',                      NULL),
('GA', 'GA', 'Габон',                           'Gabon',                             'Gabon',                       NULL),
('GB', 'GB', 'Великобритания',                  'United Kingdom',                    'United Kingdom',              NULL),
('GD', 'GD', 'Гренада',                         'Grenada',                           'Grenada',                     NULL),
('GE', 'GE', 'Грузия',                          'Georgia',                           'საქართველო',                 NULL),
('GH', 'GH', 'Гана',                            'Ghana',                             'Ghana',                       NULL),
('GM', 'GM', 'Гамбия',                          'Gambia',                            'Gambia',                      NULL),
('GN', 'GN', 'Гвинея',                          'Guinea',                            'Guinée',                      NULL),
('GQ', 'GQ', 'Экваториальная Гвинея',           'Equatorial Guinea',                 'Guinea Ecuatorial',           NULL),
('GR', 'GR', 'Греция',                          'Greece',                            'Ελλάδα',                     NULL),
('GT', 'GT', 'Гватемала',                       'Guatemala',                         'Guatemala',                   NULL),
('GW', 'GW', 'Гвинея-Бисау',                   'Guinea-Bissau',                     'Guiné-Bissau',                NULL),
('GY', 'GY', 'Гайана',                          'Guyana',                            'Guyana',                      NULL),
('HN', 'HN', 'Гондурас',                        'Honduras',                          'Honduras',                    NULL),
('HR', 'HR', 'Хорватия',                        'Croatia',                           'Hrvatska',                    NULL),
('HT', 'HT', 'Гаити',                           'Haiti',                             'Haïti',                       NULL),
('HU', 'HU', 'Венгрия',                         'Hungary',                           'Magyarország',                NULL),
('ID', 'ID', 'Индонезия',                       'Indonesia',                         'Indonesia',                   NULL),
('IE', 'IE', 'Ирландия',                        'Ireland',                           'Éire',                        NULL),
('IL', 'IL', 'Израиль',                         'Israel',                            'ישראל',                      NULL),
('IN', 'IN', 'Индия',                           'India',                             'भारत',                       NULL),
('IQ', 'IQ', 'Ирак',                            'Iraq',                              'العراق',                      NULL),
('IR', 'IR', 'Иран',                            'Iran',                              'ایران',                       NULL),
('IS', 'IS', 'Исландия',                        'Iceland',                           'Ísland',                      NULL),
('IT', 'IT', 'Италия',                          'Italy',                             'Italia',                      NULL),
('JM', 'JM', 'Ямайка',                          'Jamaica',                           'Jamaica',                     NULL),
('JO', 'JO', 'Иордания',                        'Jordan',                            'الأردن',                      NULL),
('JP', 'JP', 'Япония',                          'Japan',                             '日本',                        NULL),
('KE', 'KE', 'Кения',                           'Kenya',                             'Kenya',                       NULL),
('KG', 'KG', 'Кыргызстан',                      'Kyrgyzstan',                        'Кыргызстан',                  NULL),
('KH', 'KH', 'Камбоджа',                        'Cambodia',                          'កម្ពុជា',                    NULL),
('KI', 'KI', 'Кирибати',                        'Kiribati',                          'Kiribati',                    NULL),
('KM', 'KM', 'Коморы',                          'Comoros',                           'Komori',                      NULL),
('KN', 'KN', 'Сент-Китс и Невис',               'Saint Kitts and Nevis',             'Saint Kitts and Nevis',       NULL),
('KP', 'KP', 'Северная Корея',                  'North Korea',                       '조선',                        NULL),
('KR', 'KR', 'Южная Корея',                     'South Korea',                       '대한민국',                   NULL),
('KW', 'KW', 'Кувейт',                          'Kuwait',                            'الكويت',                      NULL),
('KZ', 'KZ', 'Казахстан',                       'Kazakhstan',                        'Қазақстан',                   NULL),
('LA', 'LA', 'Лаос',                            'Laos',                              'ລາວ',                        NULL),
('LB', 'LB', 'Ливан',                           'Lebanon',                           'لبنان',                       NULL),
('LC', 'LC', 'Сент-Люсия',                      'Saint Lucia',                       'Saint Lucia',                 NULL),
('LI', 'LI', 'Лихтенштейн',                     'Liechtenstein',                     'Liechtenstein',               NULL),
('LK', 'LK', 'Шри-Ланка',                       'Sri Lanka',                         'ශ්‍රී ලංකාව',               NULL),
('LR', 'LR', 'Либерия',                         'Liberia',                           'Liberia',                     NULL),
('LS', 'LS', 'Лесото',                          'Lesotho',                           'Lesotho',                     NULL),
('LT', 'LT', 'Литва',                           'Lithuania',                         'Lietuva',                     NULL),
('LU', 'LU', 'Люксембург',                      'Luxembourg',                        'Luxembourg',                  NULL),
('LV', 'LV', 'Латвия',                          'Latvia',                            'Latvija',                     NULL),
('LY', 'LY', 'Ливия',                           'Libya',                             'ليبيا',                       NULL),
('MA', 'MA', 'Марокко',                         'Morocco',                           'المغرب',                      NULL),
('MC', 'MC', 'Монако',                          'Monaco',                            'Monaco',                      NULL),
('MD', 'MD', 'Молдова',                         'Moldova',                           'Moldova',                     NULL),
('ME', 'ME', 'Черногория',                      'Montenegro',                        'Crna Gora',                   NULL),
('MG', 'MG', 'Мадагаскар',                      'Madagascar',                        'Madagasikara',                NULL),
('MH', 'MH', 'Маршалловы острова',              'Marshall Islands',                  'Marshall Islands',            NULL),
('MK', 'MK', 'Северная Македония',              'North Macedonia',                   'Северна Македонија',          NULL),
('ML', 'ML', 'Мали',                            'Mali',                              'Mali',                        NULL),
('MM', 'MM', 'Мьянма',                          'Myanmar',                           'မြန်မာ',                     NULL),
('MN', 'MN', 'Монголия',                        'Mongolia',                          'Монгол Улс',                  NULL),
('MR', 'MR', 'Мавритания',                      'Mauritania',                        'موريتانيا',                   NULL),
('MT', 'MT', 'Мальта',                          'Malta',                             'Malta',                       NULL),
('MU', 'MU', 'Маврикий',                        'Mauritius',                         'Mauritius',                   NULL),
('MV', 'MV', 'Мальдивы',                        'Maldives',                          'ދިވެހިރާއްޖެ',              NULL),
('MW', 'MW', 'Малави',                          'Malawi',                            'Malawi',                      NULL),
('MX', 'MX', 'Мексика',                         'Mexico',                            'México',                      NULL),
('MY', 'MY', 'Малайзия',                        'Malaysia',                          'Malaysia',                    NULL),
('MZ', 'MZ', 'Мозамбик',                        'Mozambique',                        'Moçambique',                  NULL),
('NA', 'NA', 'Намибия',                         'Namibia',                           'Namibia',                     NULL),
('NE', 'NE', 'Нигер',                           'Niger',                             'Niger',                       NULL),
('NG', 'NG', 'Нигерия',                         'Nigeria',                           'Nigeria',                     NULL),
('NI', 'NI', 'Никарагуа',                       'Nicaragua',                         'Nicaragua',                   NULL),
('NL', 'NL', 'Нидерланды',                      'Netherlands',                       'Nederland',                   NULL),
('NO', 'NO', 'Норвегия',                        'Norway',                            'Norge',                       NULL),
('NP', 'NP', 'Непал',                           'Nepal',                             'नेपाल',                      NULL),
('NR', 'NR', 'Науру',                           'Nauru',                             'Nauru',                       NULL),
('NZ', 'NZ', 'Новая Зеландия',                  'New Zealand',                       'New Zealand',                 NULL),
('OM', 'OM', 'Оман',                            'Oman',                              'عُمَان',                      NULL),
('PA', 'PA', 'Панама',                          'Panama',                            'Panamá',                      NULL),
('PE', 'PE', 'Перу',                            'Peru',                              'Perú',                        NULL),
('PG', 'PG', 'Папуа — Новая Гвинея',            'Papua New Guinea',                  'Papua New Guinea',            NULL),
('PH', 'PH', 'Филиппины',                       'Philippines',                       'Pilipinas',                   NULL),
('PK', 'PK', 'Пакистан',                        'Pakistan',                          'پاکستان',                    NULL),
('PL', 'PL', 'Польша',                          'Poland',                            'Polska',                      NULL),
('PT', 'PT', 'Португалия',                      'Portugal',                          'Portugal',                    NULL),
('PW', 'PW', 'Палау',                           'Palau',                             'Palau',                       NULL),
('PY', 'PY', 'Парагвай',                        'Paraguay',                          'Paraguay',                    NULL),
('QA', 'QA', 'Катар',                           'Qatar',                             'قطر',                         NULL),
('RO', 'RO', 'Румыния',                         'Romania',                           'România',                     NULL),
('RS', 'RS', 'Сербия',                          'Serbia',                            'Srbija',                      NULL),
('RU', 'RU', 'Россия',                          'Russia',                            'Россия',                      NULL),
('RW', 'RW', 'Руанда',                          'Rwanda',                            'Rwanda',                      NULL),
('SA', 'SA', 'Саудовская Аравия',               'Saudi Arabia',                      'المملكة العربية السعودية',    NULL),
('SB', 'SB', 'Соломоновы острова',              'Solomon Islands',                   'Solomon Islands',             NULL),
('SC', 'SC', 'Сейшелы',                         'Seychelles',                        'Seychelles',                  NULL),
('SD', 'SD', 'Судан',                           'Sudan',                             'السودان',                     NULL),
('SE', 'SE', 'Швеция',                          'Sweden',                            'Sverige',                     NULL),
('SG', 'SG', 'Сингапур',                        'Singapore',                         'Singapore',                   NULL),
('SI', 'SI', 'Словения',                        'Slovenia',                          'Slovenija',                   NULL),
('SK', 'SK', 'Словакия',                        'Slovakia',                          'Slovensko',                   NULL),
('SL', 'SL', 'Сьерра-Леоне',                    'Sierra Leone',                      'Sierra Leone',                NULL),
('SM', 'SM', 'Сан-Марино',                      'San Marino',                        'San Marino',                  NULL),
('SN', 'SN', 'Сенегал',                         'Senegal',                           'Sénégal',                     NULL),
('SO', 'SO', 'Сомали',                          'Somalia',                           'Soomaaliya',                  NULL),
('SR', 'SR', 'Суринам',                         'Suriname',                          'Suriname',                    NULL),
('SS', 'SS', 'Южный Судан',                     'South Sudan',                       'South Sudan',                 NULL),
('ST', 'ST', 'Сан-Томе и Принсипи',             'São Tomé and Príncipe',             'São Tomé e Príncipe',         NULL),
('SV', 'SV', 'Сальвадор',                       'El Salvador',                       'El Salvador',                 NULL),
('SY', 'SY', 'Сирия',                           'Syria',                             'سوريا',                       NULL),
('SZ', 'SZ', 'Эсватини',                        'Eswatini',                          'Eswatini',                    NULL),
('TD', 'TD', 'Чад',                             'Chad',                              'Tchad',                       NULL),
('TG', 'TG', 'Того',                            'Togo',                              'Togo',                        NULL),
('TH', 'TH', 'Таиланд',                         'Thailand',                          'ประเทศไทย',                  NULL),
('TJ', 'TJ', 'Таджикистан',                     'Tajikistan',                        'Тоҷикистон',                  NULL),
('TL', 'TL', 'Тимор-Лесте',                     'Timor-Leste',                       'Timor-Leste',                 NULL),
('TM', 'TM', 'Туркменистан',                    'Turkmenistan',                      'Türkmenistan',                NULL),
('TN', 'TN', 'Тунис',                           'Tunisia',                           'تونس',                        NULL),
('TO', 'TO', 'Тонга',                           'Tonga',                             'Tonga',                       NULL),
('TR', 'TR', 'Турция',                          'Turkey',                            'Türkiye',                     NULL),
('TT', 'TT', 'Тринидад и Тобаго',              'Trinidad and Tobago',               'Trinidad and Tobago',         NULL),
('TV', 'TV', 'Тувалу',                          'Tuvalu',                            'Tuvalu',                      NULL),
('TZ', 'TZ', 'Танзания',                        'Tanzania',                          'Tanzania',                    NULL),
('UA', 'UA', 'Украина',                         'Ukraine',                           'Україна',                     NULL),
('UG', 'UG', 'Уганда',                          'Uganda',                            'Uganda',                      NULL),
('US', 'US', 'США',                             'United States',                     'United States',               NULL),
('UY', 'UY', 'Уругвай',                         'Uruguay',                           'Uruguay',                     NULL),
('UZ', 'UZ', 'Узбекистан',                      'Uzbekistan',                        'O''zbekiston',                NULL),
('VA', 'VA', 'Ватикан',                         'Vatican City',                      'Vaticano',                    NULL),
('VC', 'VC', 'Сент-Винсент и Гренадины',        'Saint Vincent and the Grenadines',  'Saint Vincent',               NULL),
('VE', 'VE', 'Венесуэла',                       'Venezuela',                         'Venezuela',                   NULL),
('VN', 'VN', 'Вьетнам',                         'Vietnam',                           'Việt Nam',                    NULL),
('VU', 'VU', 'Вануату',                         'Vanuatu',                           'Vanuatu',                     NULL),
('WS', 'WS', 'Самоа',                           'Samoa',                             'Sāmoa',                       NULL),
('YE', 'YE', 'Йемен',                           'Yemen',                             'اليمن',                       NULL),
('ZA', 'ZA', 'Южная Африка',                    'South Africa',                      'South Africa',                NULL),
('ZM', 'ZM', 'Замбия',                          'Zambia',                            'Zambia',                      NULL),
('ZW', 'ZW', 'Зимбабве',                        'Zimbabwe',                          'Zimbabwe',                    NULL)
ON CONFLICT DO NOTHING;

-- ============================================================
-- РЕГИОНЫ УЗБЕКИСТАНА (приоритетный рынок, детальный)
-- ============================================================
DO $$
DECLARE v_uz_id UUID;
BEGIN
    SELECT id INTO v_uz_id FROM regions WHERE country_code = 'UZ' AND region_code = 'UZ';

    INSERT INTO regions (country_code, region_code, name_ru, name_en, name_local, parent_region_id) VALUES
    ('UZ', 'UZ-TK', 'Ташкент (город)',      'Tashkent (city)',     'Toshkent (shahar)',   v_uz_id),
    ('UZ', 'UZ-TO', 'Ташкентская область',  'Tashkent Region',     'Toshkent viloyati',   v_uz_id),
    ('UZ', 'UZ-AN', 'Андижанская область',  'Andijan Region',      'Andijon viloyati',    v_uz_id),
    ('UZ', 'UZ-BU', 'Бухарская область',    'Bukhara Region',      'Buxoro viloyati',     v_uz_id),
    ('UZ', 'UZ-FA', 'Ферганская область',   'Fergana Region',      'Farg''ona viloyati',  v_uz_id),
    ('UZ', 'UZ-JI', 'Джизакская область',   'Jizzakh Region',      'Jizzax viloyati',     v_uz_id),
    ('UZ', 'UZ-KH', 'Хорезмская область',   'Khorezm Region',      'Xorazm viloyati',     v_uz_id),
    ('UZ', 'UZ-NG', 'Наманганская область', 'Namangan Region',     'Namangan viloyati',   v_uz_id),
    ('UZ', 'UZ-NW', 'Навоийская область',   'Navoiy Region',       'Navoiy viloyati',     v_uz_id),
    ('UZ', 'UZ-QA', 'Кашкадарьинская обл.','Kashkadarya Region',  'Qashqadaryo viloyati',v_uz_id),
    ('UZ', 'UZ-QR', 'Каракалпакстан',       'Karakalpakstan',      'Qoraqalpog''iston',   v_uz_id),
    ('UZ', 'UZ-SA', 'Самаркандская область','Samarkand Region',    'Samarqand viloyati',  v_uz_id),
    ('UZ', 'UZ-SI', 'Сырдарьинская область','Sirdarya Region',     'Sirdaryo viloyati',   v_uz_id),
    ('UZ', 'UZ-SU', 'Сурхандарьинская обл.','Surkhandarya Region','Surxondaryo viloyati', v_uz_id)
    ON CONFLICT DO NOTHING;
END $$;

-- ============================================================
-- РЕГИОНЫ КАЗАХСТАНА (фаза 2)
-- ============================================================
DO $$
DECLARE v_kz_id UUID;
BEGIN
    SELECT id INTO v_kz_id FROM regions WHERE country_code = 'KZ' AND region_code = 'KZ';

    INSERT INTO regions (country_code, region_code, name_ru, name_en, name_local, parent_region_id) VALUES
    ('KZ', 'KZ-AKM', 'Акмолинская область',   'Akmola Region',      'Ақмола облысы',    v_kz_id),
    ('KZ', 'KZ-AKT', 'Актюбинская область',   'Aktobe Region',      'Ақтөбе облысы',    v_kz_id),
    ('KZ', 'KZ-ALA', 'Алматы (город)',         'Almaty (city)',       'Алматы қаласы',    v_kz_id),
    ('KZ', 'KZ-ALM', 'Алматинская область',    'Almaty Region',      'Алматы облысы',    v_kz_id),
    ('KZ', 'KZ-AST', 'Астана (город)',         'Astana (city)',       'Астана қаласы',    v_kz_id),
    ('KZ', 'KZ-ATY', 'Атырауская область',     'Atyrau Region',      'Атырау облысы',    v_kz_id),
    ('KZ', 'KZ-KAR', 'Карагандинская область', 'Karaganda Region',   'Қарағанды облысы', v_kz_id),
    ('KZ', 'KZ-KUS', 'Костанайская область',   'Kostanay Region',    'Қостанай облысы',  v_kz_id),
    ('KZ', 'KZ-KZY', 'Кызылординская область', 'Kyzylorda Region',   'Қызылорда облысы', v_kz_id),
    ('KZ', 'KZ-MAN', 'Мангистауская область',  'Mangystau Region',   'Маңғыстау облысы', v_kz_id),
    ('KZ', 'KZ-PAV', 'Павлодарская область',   'Pavlodar Region',    'Павлодар облысы',  v_kz_id),
    ('KZ', 'KZ-SEV', 'Северо-Казахстанская',   'North Kazakhstan',   'СҚО',              v_kz_id),
    ('KZ', 'KZ-SHY', 'Шымкент (город)',         'Shymkent (city)',    'Шымкент қаласы',   v_kz_id),
    ('KZ', 'KZ-TUR', 'Туркестанская область',   'Turkestan Region',   'Түркістан облысы', v_kz_id),
    ('KZ', 'KZ-VOS', 'Восточно-Казахстанская', 'East Kazakhstan',    'ШҚО',              v_kz_id),
    ('KZ', 'KZ-ZAP', 'Западно-Казахстанская',  'West Kazakhstan',    'БҚО',              v_kz_id),
    ('KZ', 'KZ-ZHA', 'Жамбылская область',      'Jambyl Region',     'Жамбыл облысы',    v_kz_id)
    ON CONFLICT DO NOTHING;
END $$;

-- ============================================================
-- РЕГИОНЫ КЫРГЫЗСТАНА (фаза 2)
-- ============================================================
DO $$
DECLARE v_kg_id UUID;
BEGIN
    SELECT id INTO v_kg_id FROM regions WHERE country_code = 'KG' AND region_code = 'KG';

    INSERT INTO regions (country_code, region_code, name_ru, name_en, name_local, parent_region_id) VALUES
    ('KG', 'KG-B',  'Бишкек (город)',         'Bishkek (city)',      'Бишкек шаары',      v_kg_id),
    ('KG', 'KG-GB', 'Ош (город)',              'Osh (city)',          'Ош шаары',          v_kg_id),
    ('KG', 'KG-BA', 'Баткенская область',      'Batken Region',      'Баткен облусу',     v_kg_id),
    ('KG', 'KG-C',  'Чуйская область',         'Chuy Region',        'Чүй облусу',        v_kg_id),
    ('KG', 'KG-J',  'Джалал-Абадская область', 'Jalal-Abad Region',  'Жалал-Абад облусу', v_kg_id),
    ('KG', 'KG-N',  'Нарынская область',       'Naryn Region',       'Нарын облусу',      v_kg_id),
    ('KG', 'KG-O',  'Ошская область',          'Osh Region',         'Ош облусу',         v_kg_id),
    ('KG', 'KG-T',  'Таласская область',       'Talas Region',       'Талас облусу',      v_kg_id),
    ('KG', 'KG-Y',  'Иссык-Кульская область',  'Issyk-Kul Region',   'Ысык-Көл облусу',   v_kg_id)
    ON CONFLICT DO NOTHING;
END $$;

-- ============================================================
-- РЕГИОНЫ ТАДЖИКИСТАНА (фаза 2)
-- ============================================================
DO $$
DECLARE v_tj_id UUID;
BEGIN
    SELECT id INTO v_tj_id FROM regions WHERE country_code = 'TJ' AND region_code = 'TJ';

    INSERT INTO regions (country_code, region_code, name_ru, name_en, name_local, parent_region_id) VALUES
    ('TJ', 'TJ-DU', 'Душанбе (город)',           'Dushanbe (city)',         'Душанбе',               v_tj_id),
    ('TJ', 'TJ-GB', 'Горно-Бадахшанская АО',     'Gorno-Badakhshan',        'ВМКБ',                  v_tj_id),
    ('TJ', 'TJ-KT', 'Согдийская область',         'Sughd Region',            'Вилояти Суғд',          v_tj_id),
    ('TJ', 'TJ-KH', 'Хатлонская область',         'Khatlon Region',          'Вилояти Хатлон',        v_tj_id),
    ('TJ', 'TJ-RA', 'Районы республиканского подчинения', 'RRS',             'РНТ',                   v_tj_id)
    ON CONFLICT DO NOTHING;
END $$;

-- ============================================================
-- РЕГИОНЫ ТУРКМЕНИСТАНА (фаза 2)
-- ============================================================
DO $$
DECLARE v_tm_id UUID;
BEGIN
    SELECT id INTO v_tm_id FROM regions WHERE country_code = 'TM' AND region_code = 'TM';

    INSERT INTO regions (country_code, region_code, name_ru, name_en, name_local, parent_region_id) VALUES
    ('TM', 'TM-A', 'Ашхабад (город)',        'Ashgabat (city)',     'Aşgabat',       v_tm_id),
    ('TM', 'TM-AH', 'Ахалская область',      'Ahal Region',        'Ahal welaýaty', v_tm_id),
    ('TM', 'TM-B',  'Балканская область',     'Balkan Region',      'Balkan welaýaty',v_tm_id),
    ('TM', 'TM-D',  'Дашогузская область',    'Daşoguz Region',     'Daşoguz welaýaty',v_tm_id),
    ('TM', 'TM-L',  'Лебапская область',      'Lebap Region',       'Lebap welaýaty', v_tm_id),
    ('TM', 'TM-M',  'Марыйская область',      'Mary Region',        'Mary welaýaty',  v_tm_id)
    ON CONFLICT DO NOTHING;
END $$;
