ALTER TABLE recommendations.sales
    ADD COLUMN IF NOT EXISTS product_group text;

ALTER TABLE recommendations.item_feature_versions
    ADD COLUMN IF NOT EXISTS organization_type text NOT NULL DEFAULT '';
