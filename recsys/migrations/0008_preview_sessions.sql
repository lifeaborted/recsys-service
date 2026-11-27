ALTER TABLE recommendations.stand_sessions
    ADD COLUMN IF NOT EXISTS client_id text REFERENCES recommendations.clients(client_id);

DELETE FROM recommendations.stand_sessions s
USING recommendations.stand_accounts a
WHERE a.account_id = s.account_id
  AND a.client_id IN ('DEMO-NOTVER', 'DEMO-VERB2B', 'DEMO-VERB2C')
  AND EXISTS (SELECT 1 FROM recommendations.model_snapshots WHERE snapshot_id = 'demo-v1')
  AND NOT EXISTS (SELECT 1 FROM recommendations.model_snapshots WHERE snapshot_id <> 'demo-v1')
  AND NOT EXISTS (SELECT 1 FROM recommendations.sales);

-- Reuse the existing security category for the stand antivirus.
UPDATE recommendations.catalog_offers o
SET category_id = base.category_id
FROM recommendations.catalog_offers base
WHERE o.offer_id = 'demo-offer-b2c-1'
  AND base.offer_id = 'demo-offer-base'
  AND base.category_id IS NOT NULL
  AND EXISTS (SELECT 1 FROM recommendations.model_snapshots WHERE snapshot_id = 'demo-v1');
