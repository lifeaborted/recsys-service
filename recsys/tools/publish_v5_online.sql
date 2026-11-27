\set ON_ERROR_STOP on
-- Publish a pinned online-only snapshot for the closed v5 database.
-- This script uses the imported 26 November storefront and leaves the historical snapshot active.
BEGIN;
SELECT pg_advisory_xact_lock(5042026);

DO $guard$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM recommendations.model_snapshots
    WHERE snapshot_id = 'v5-train-20260831' AND dataset_version = 'v5'
      AND train_through = DATE '2026-08-31'
  ) THEN
    RAISE EXCEPTION 'The historical v5 snapshot is required';
  END IF;
  IF EXISTS (SELECT 1 FROM recommendations.model_snapshots WHERE snapshot_id = 'v5-online-20261126') THEN
    RAISE EXCEPTION 'The immutable online snapshot already exists';
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM recommendations.catalog_snapshots
    WHERE id = 1 AND feed_generated_at::date = DATE '2026-11-26'
      AND source_url = 'https://store.softline.ru/feeds/Mindbox.yml.gz'
  ) THEN
    RAISE EXCEPTION 'The imported catalog of 26 November is required';
  END IF;
END
$guard$;

CREATE TEMP TABLE v5_online_items ON COMMIT DROP AS
WITH sales AS (
  SELECT item_id, max(nullif(item_name,'')) AS title, max(nullif(sku,'')) AS sku,
         max(nullif(product_group,'')) AS family_id,
         count(DISTINCT nullif(product_group,'')) AS family_count,
         count(DISTINCT client_id) FILTER (WHERE order_week <= DATE '2026-08-31') AS train_buyers
  FROM recommendations.sales GROUP BY item_id
), matched AS (
  SELECT s.*, o.offer_id, o.name AS offer_name, o.vendor_code, o.url,
         o.available IS TRUE AND e.offer_id IS NOT NULL AS purchasable,
         coalesce(params.audience,'') AS user_type, coalesce(params.organization_type,'') AS organization_type,
         row_number() OVER (
           PARTITION BY o.offer_id
           ORDER BY CASE h.match_method WHEN 'vendor_code' THEN 0 ELSE 1 END, s.train_buyers DESC, s.item_id COLLATE "C"
         ) AS offer_rank
  FROM sales s
  LEFT JOIN recommendations.historical_item_offers h ON h.item_id = s.item_id AND h.is_selected
  LEFT JOIN recommendations.catalog_offers o ON o.offer_id = h.offer_id AND o.snapshot_id = 1
  LEFT JOIN recommendations.catalog_offer_embeddings e
    ON e.offer_id = o.offer_id AND e.model_version = 'qwen3-vl-embedding-2b'
  LEFT JOIN LATERAL (
    SELECT max(lower(btrim(p->>'value'))) FILTER (WHERE lower(p->>'name') = 'пользователь') AS audience,
           max(lower(btrim(p->>'value'))) FILTER (WHERE lower(p->>'name') = 'тип организации') AS organization_type
    FROM jsonb_array_elements(
      CASE WHEN jsonb_typeof(o.parameters) = 'array' THEN o.parameters ELSE '[]'::jsonb END
    ) p
  ) params ON true
)
SELECT *, purchasable AND offer_rank = 1 AND user_type IN ('юрлицо','физлицо','нко','','не определено') AS online_available
FROM matched;

DO $guard$
BEGIN
  IF (SELECT count(*) FROM v5_online_items) <> 817
     OR (SELECT count(DISTINCT item_id) FROM v5_online_items) <> 817
     OR EXISTS (SELECT 1 FROM v5_online_items WHERE family_count <> 1) THEN
    RAISE EXCEPTION 'Unexpected sales universe or product-group mapping';
  END IF;
  IF (SELECT count(*) FROM v5_online_items WHERE purchasable) <> 364
     OR (SELECT count(*) FROM v5_online_items WHERE online_available) <> 359 THEN
    RAISE EXCEPTION 'Unexpected offer coverage; inspect the catalog before publication';
  END IF;
  IF EXISTS (
    SELECT offer_id FROM v5_online_items WHERE purchasable
    GROUP BY offer_id HAVING count(DISTINCT family_id) > 1
  ) THEN
    RAISE EXCEPTION 'An offer maps to multiple product groups';
  END IF;
END
$guard$;

INSERT INTO recommendations.catalog_feed_versions
  (catalog_version, snapshot_seq_id, source_url, feed_generated_at, imported_at)
SELECT 'v5-online-feed-20261126', id, source_url, feed_generated_at AT TIME ZONE 'Asia/Yekaterinburg', imported_at
FROM recommendations.catalog_snapshots WHERE id = 1;

INSERT INTO recommendations.model_snapshots
  (snapshot_id, dataset_version, workbook_sha256, catalog_version, feature_version, train_through, is_active)
SELECT 'v5-online-20261126', dataset_version, workbook_sha256,
       'v5-online-feed-20261126', 'v5-online-20261126', train_through, false
FROM recommendations.model_snapshots WHERE snapshot_id = 'v5-train-20260831';

INSERT INTO recommendations.catalog_version_offers
  (catalog_version, offer_id, category_id, vendor_name, group_id, name, vendor_code, url, description,
   picture_url, available, price, old_price, currency_code, parameters, vendor_code_key, url_code_key, name_key)
SELECT 'v5-online-feed-20261126', o.offer_id, o.category_id, v.name, o.group_id, o.name, o.vendor_code,
       o.url, o.description, o.picture_url, o.available, o.price, o.old_price, o.currency_code,
       coalesce(o.parameters,'[]'::jsonb), o.vendor_code_key, o.url_code_key, o.name_key
FROM v5_online_items i
JOIN recommendations.catalog_offers o ON o.offer_id = i.offer_id
LEFT JOIN recommendations.catalog_vendors v ON v.vendor_id = o.vendor_id
WHERE i.online_available;

INSERT INTO recommendations.item_feature_versions
  (feature_version, item_id, offer_id, title, sku, url, audience, available, in_sales, family_id, family_source, organization_type)
SELECT 'v5-online-20261126', item_id, offer_id, coalesce(offer_name, title, item_id),
       coalesce(vendor_code, sku), url,
       CASE user_type WHEN 'юрлицо' THEN 'B2B' WHEN 'нко' THEN 'B2B' WHEN 'физлицо' THEN 'B2C' ELSE '' END,
       online_available, true, family_id, 'dataset:v5:product_group',
       CASE WHEN user_type = 'нко' AND organization_type IN ('', 'не определено')
            THEN 'некоммерческая' ELSE organization_type END
FROM v5_online_items;

INSERT INTO recommendations.snapshot_candidates
  (snapshot_id, signal, source_item_id, client_group, client_type, industry, segment, rank, item_id,
   recommendation_kind, score, evidence_ids)
SELECT 'v5-online-20261126', c.signal, c.source_item_id, c.client_group, c.client_type, c.industry,
       c.segment, c.rank, c.item_id, c.recommendation_kind, c.score, c.evidence_ids
FROM recommendations.snapshot_candidates c JOIN v5_online_items i USING (item_id)
WHERE c.snapshot_id = 'v5-train-20260831' AND i.online_available;

INSERT INTO recommendations.evidence_records
  (snapshot_id, evidence_id, from_node, to_node, signal_type, weight, reason, source, source_url,
   source_quote, source_version, observed_at, constraints)
SELECT 'v5-online-20261126', evidence_id, from_node, to_node, signal_type, weight, reason, source,
       source_url, source_quote, source_version, observed_at, constraints
FROM recommendations.evidence_records e
WHERE e.snapshot_id = 'v5-train-20260831'
  AND EXISTS (SELECT 1 FROM v5_online_items i WHERE i.online_available AND e.to_node = 'item:' || i.item_id);

INSERT INTO recommendations.evidence_records
  (snapshot_id, evidence_id, from_node, to_node, signal_type, weight, reason, source, source_url, source_version, observed_at)
SELECT 'v5-online-20261126', 'catalog/' || i.item_id, 'cohort:catalog', 'item:' || i.item_id,
       'catalog', 1, 'Товар доступен в снимке каталога Softline.', f.source_url || '@' || f.catalog_version,
       f.source_url, f.catalog_version, f.feed_generated_at
FROM v5_online_items i CROSS JOIN recommendations.catalog_feed_versions f
WHERE i.online_available AND f.catalog_version = 'v5-online-feed-20261126';

INSERT INTO recommendations.snapshot_candidates
  (snapshot_id, signal, source_item_id, client_group, client_type, industry, segment, rank, item_id,
   recommendation_kind, score, evidence_ids)
SELECT 'v5-online-20261126', 'catalog', '', '', '', '', '',
       row_number() OVER (ORDER BY train_buyers DESC, item_id COLLATE "C"), item_id,
       'popular', 1, jsonb_build_array('catalog/' || item_id)
FROM v5_online_items WHERE online_available;

DO $guard$
BEGIN
  IF EXISTS (
    SELECT 1 FROM recommendations.snapshot_candidates c
    CROSS JOIN LATERAL jsonb_array_elements_text(c.evidence_ids) AS ref(id)
    LEFT JOIN recommendations.evidence_records e ON e.snapshot_id = c.snapshot_id AND e.evidence_id = ref.id
    WHERE c.snapshot_id = 'v5-online-20261126' AND e.evidence_id IS NULL
  ) THEN
    RAISE EXCEPTION 'The new snapshot has a candidate without stored evidence';
  END IF;
END
$guard$;

SELECT count(*) AS sales_item_ids,
       count(*) FILTER (WHERE purchasable) AS available_matches,
       count(*) FILTER (WHERE online_available) AS distinct_active_offers
FROM v5_online_items;
COMMIT;
