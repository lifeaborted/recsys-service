SELECT pg_advisory_xact_lock(5042026);

DO $guard$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM recommendations.model_snapshots
    WHERE snapshot_id='demo-v1' AND dataset_version='demo-v1'
      AND catalog_version='demo-feed-v1' AND is_active
  ) OR EXISTS (
    SELECT 1 FROM recommendations.dataset_release WHERE version <> 'demo-v1'
  ) OR EXISTS (SELECT 1 FROM recommendations.sales) THEN
    RAISE EXCEPTION 'Demo catalog expansion requires a dedicated synthetic database';
  END IF;
END
$guard$;

CREATE TEMP TABLE demo_extra_offers (
  item_id text PRIMARY KEY,
  offer_id text NOT NULL,
  audience text NOT NULL,
  title text NOT NULL,
  category_id text NOT NULL,
  category_name text NOT NULL,
  price numeric NOT NULL,
  owned_id text NOT NULL,
  score double precision NOT NULL,
  reason text NOT NULL
) ON COMMIT DROP;

INSERT INTO demo_extra_offers VALUES
  ('DEMO-TEAM-BACKUP', 'demo-offer-team-backup', 'B2B', 'Корпоративное резервное копирование',
   'demo-backup', 'Резервное копирование', 7900, 'DEMO-BASE', 0.62,
   'В демо-сценарии защищает рабочие файлы резервными копиями в дополнение к корпоративной лицензии.'),
  ('DEMO-HOME-BACKUP', 'demo-offer-home-backup', 'B2C', 'Резервное копирование для дома',
   'demo-backup', 'Резервное копирование', 1490, 'DEMO-B2C-1', 0.62,
   'Помогает восстановить личные файлы. Это дополнение к защите устройства антивирусом.'),
  ('DEMO-HOME-PASSWORDS', 'demo-offer-home-passwords', 'B2C', 'Менеджер паролей для дома',
   'demo-credentials', 'Защита аккаунтов', 1290, 'DEMO-B2C-1', 0.59,
   'Помогает создавать и хранить разные пароли, дополняя защиту устройства.');

INSERT INTO recommendations.catalog_categories (category_id, name)
SELECT DISTINCT category_id, category_name FROM demo_extra_offers
ON CONFLICT (category_id) DO NOTHING;

INSERT INTO recommendations.catalog_offers
  (offer_id, snapshot_id, category_id, vendor_id, name, vendor_code, url, description, available, price, currency_code, parameters)
SELECT d.offer_id, base.snapshot_id, d.category_id, base.vendor_id, d.title, d.item_id,
       '#/demo-product?item=' || d.item_id, d.reason, true, d.price, 'RUB', '[]'::jsonb
FROM demo_extra_offers d CROSS JOIN recommendations.catalog_offers base
WHERE base.offer_id='demo-offer-base'
ON CONFLICT (offer_id) DO NOTHING;

INSERT INTO recommendations.catalog_version_offers
  (catalog_version, offer_id, category_id, vendor_name, name, vendor_code, url, description, available, price, currency_code)
SELECT 'demo-feed-v1', d.offer_id, d.category_id, v.name, d.title, d.item_id, o.url, d.reason, true, d.price, 'RUB'
FROM demo_extra_offers d
JOIN recommendations.catalog_offers o USING (offer_id)
JOIN recommendations.catalog_vendors v USING (vendor_id)
ON CONFLICT (catalog_version, offer_id) DO NOTHING;

INSERT INTO recommendations.historical_items (item_id)
SELECT item_id FROM demo_extra_offers ON CONFLICT (item_id) DO NOTHING;

INSERT INTO recommendations.historical_item_offers (item_id, offer_id, match_method, is_selected)
SELECT item_id, offer_id, 'vendor_code', true FROM demo_extra_offers
ON CONFLICT (item_id, offer_id) DO NOTHING;

INSERT INTO recommendations.item_feature_versions
  (feature_version, item_id, offer_id, title, sku, url, audience, available, in_sales)
SELECT m.feature_version, d.item_id, d.offer_id, d.title, d.item_id, o.url, d.audience, true, false
FROM demo_extra_offers d
JOIN recommendations.catalog_offers o USING (offer_id)
CROSS JOIN recommendations.model_snapshots m
WHERE m.snapshot_id='demo-v1'
ON CONFLICT (feature_version, item_id) DO NOTHING;

INSERT INTO recommendations.evidence_records
  (snapshot_id, evidence_id, from_node, to_node, signal_type, weight, reason, source)
SELECT 'demo-v1', 'demo-complement/' || item_id, 'item:' || owned_id, 'item:' || item_id,
       'complement', score, reason, 'synthetic-catalog-expansion-v1'
FROM demo_extra_offers
ON CONFLICT (snapshot_id, evidence_id) DO NOTHING;

INSERT INTO recommendations.snapshot_candidates
  (snapshot_id, signal, source_item_id, client_group, client_type, industry, segment, rank, item_id,
   recommendation_kind, score, evidence_ids)
SELECT 'demo-v1', 'complement', d.owned_id, '', '', '', '',
       coalesce(existing.max_rank, 0) + row_number() OVER (PARTITION BY d.owned_id ORDER BY d.score DESC),
       d.item_id, 'complement', d.score, jsonb_build_array('demo-complement/' || d.item_id)
FROM demo_extra_offers d
LEFT JOIN LATERAL (
  SELECT max(c.rank) AS max_rank
  FROM recommendations.snapshot_candidates c
  WHERE c.snapshot_id='demo-v1' AND c.signal='complement' AND c.source_item_id=d.owned_id
    AND NOT EXISTS (SELECT 1 FROM demo_extra_offers extra WHERE extra.item_id=c.item_id)
) existing ON true
ON CONFLICT (snapshot_id, signal, source_item_id, client_group, client_type, industry, segment, item_id) DO NOTHING;
