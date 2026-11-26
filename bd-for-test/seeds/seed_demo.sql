-- Минимальные демо-данные, чтобы REST API отдавал карточки даже без импорта
-- полного дампа (260 тыс. строк). Идемпотентно: можно выполнять повторно.
-- Для реальной проверки алгоритма залейте recommendations.sql через import.ps1.

INSERT INTO recommendations.catalog_currencies (currency_code, rate)
VALUES ('RUB', 1)
ON CONFLICT (currency_code) DO NOTHING;

INSERT INTO recommendations.clients (excel_row, client_id, client_group, client_type, industry, segment, lk_access)
VALUES (900001, 'C02417', 'B2B', 'Юрлицо', 'IT', 'enterprise', 'да')
ON CONFLICT (excel_row) DO NOTHING;

INSERT INTO recommendations.catalog_snapshots (id, source_url, feed_generated_at)
OVERRIDING SYSTEM VALUE
VALUES (900001, 'demo://seed', now())
ON CONFLICT (id) DO NOTHING;

INSERT INTO recommendations.catalog_categories (category_id, parent_id, name) VALUES
  ('demo-security', NULL, 'Безопасность'),
  ('demo-office', NULL, 'Офисное ПО'),
  ('demo-analytics', NULL, 'Аналитика'),
  ('demo-os', NULL, 'Операционные системы')
ON CONFLICT (category_id) DO NOTHING;

INSERT INTO recommendations.catalog_vendors (vendor_id, name)
OVERRIDING SYSTEM VALUE
VALUES
  (900001, '«Лаборатория Касперского»'),
  (900002, 'PRO32'),
  (900003, 'Content AI'),
  (900004, 'Яндекс'),
  (900005, 'Астра')
ON CONFLICT (vendor_id) DO NOTHING;

INSERT INTO recommendations.catalog_offers (
  offer_id, snapshot_id, category_id, vendor_id, name, vendor_code, url,
  picture_url, available, price, old_price, currency_code, parameters,
  vendor_code_key, name_key
) VALUES
  ('p001', 900001, 'demo-security', 900001,
   'Kaspersky Security для почтовых серверов. Базовая', 'KL4313RARFS',
   'https://store.softline.ru/kaspersky/kl4313rarfs-3851/',
   '/assets/scale-dec-168x168.png', true, 1512, 0, 'RUB', '[]'::jsonb,
   'KL4313RARFS', 'kaspersky security для почтовых серверов базовая'),
  ('p002', 900001, 'demo-security', 900002,
   'PRO32 Office Security Base лицензия на 2 года и 3 устройства', 'PRO32-PUS-NS-EKEY-2-3',
   'https://store.softline.ru/pro32/pro32-pus-ns-ekey-2-3-518958/',
   '/assets/scale-dec-168x168(1).png', true, 3090, 0, 'RUB', '[]'::jsonb,
   'PRO32-PUS-NS-EKEY-2-3', 'pro32 office security base 2 года 3 устройства'),
  ('p003', 900001, 'demo-office', 900003,
   'ContentReader PDF 16, Business 1 Standalone (подписка на 1 год)', 'CR16-BDS1',
   'https://store.softline.ru/contentai/cr16-bds1-525823/',
   '/assets/scale-dec-168x168.jpg', true, 16800, 0, 'RUB', '[]'::jsonb,
   'CR16-BDS1', 'contentreader pdf 16 business standalone'),
  ('p004', 900001, 'demo-security', 900002,
   'PRO32 Office Security Base лицензия на 1 год и на 5 устройств', 'PRO32-OSB-EKEY-1Y-5',
   'https://store.softline.ru/pro32/pro32-osb-ekey-1y-5-ns-ru-aks-446656/',
   '/assets/scale-dec-168x168(1).png', true, 2490, 0, 'RUB', '[]'::jsonb,
   'PRO32-OSB-EKEY-1Y-5', 'pro32 office security base 1 год 5 устройств'),
  ('p005', 900001, 'demo-security', 900001,
   'Kaspersky Security для виртуальных и облачных сред. Desktop. Базовая', 'KL4155RARFS',
   'https://store.softline.ru/kaspersky/kl4155rarfs-160801/',
   '/assets/scale-dec-168x168(2).png', true, 2889.90, 0, 'RUB', '[]'::jsonb,
   'KL4155RARFS', 'kaspersky security desktop базовая'),
  ('p006', 900001, 'demo-security', 900002,
   'PRO32 Office Security Base лицензия на 1 год и на 10 устройств', 'PRO32-OSB-EKEY-1Y-10',
   'https://store.softline.ru/pro32/pro32-osb-ekey-1y-10-ns-ru-aks-446657/',
   '/assets/scale-dec-168x168(1).png', true, 4990, 0, 'RUB', '[]'::jsonb,
   'PRO32-OSB-EKEY-1Y-10', 'pro32 office security base 1 год 10 устройств'),
  ('p007', 900001, 'demo-security', 900001,
   'Kaspersky Security для виртуальных и облачных сред. Server. Базовая', 'KL4255RARFS',
   'https://store.softline.ru/kaspersky/kl4255rarfs-160810/',
   '/assets/scale-dec-168x168(2).png', true, 10212.30, 0, 'RUB', '[]'::jsonb,
   'KL4255RARFS', 'kaspersky security server базовая'),
  ('p008', 900001, 'demo-security', 900001,
   'Kaspersky Security для интернет-шлюзов. Базовая', 'KL4413RARFS',
   'https://store.softline.ru/kaspersky/kl4413rarfs-3939/',
   '/assets/scale-dec-168x168(3).png', true, 1034.37, 0, 'RUB', '[]'::jsonb,
   'KL4413RARFS', 'kaspersky security для интернет шлюзов базовая'),
  ('p009', 900001, 'demo-security', 900002,
   'PRO32 Office Security Base лицензия на 2 года и на 10 устройств', 'PRO32-OSB-EKEY-2Y-10',
   'https://store.softline.ru/pro32/pro32-osb-ekey-2y-10-ns-ru-aks-536429/',
   '/assets/scale-dec-168x168(1).png', true, 9980, 0, 'RUB', '[]'::jsonb,
   'PRO32-OSB-EKEY-2Y-10', 'pro32 office security base 2 года 10 устройств')
ON CONFLICT (offer_id) DO NOTHING;
