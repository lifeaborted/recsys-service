-- Additive migration 0004: pg_trgm & pgvector enrichment extensions, multi-version catalog feeds,
-- versioned item features, verified evidence records, and atomic recommendation snapshots.

CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;

DO $ext$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'vector') THEN
        CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;
    END IF;
END
$ext$;

-- Stores feed metadata per catalog_version while preserving recommendations.catalog_offers(offer_id) PK.
CREATE TABLE IF NOT EXISTS recommendations.catalog_feed_versions (
    catalog_version text PRIMARY KEY CHECK (btrim(catalog_version) <> ''),
    snapshot_seq_id bigint REFERENCES recommendations.catalog_snapshots(id),
    source_url text NOT NULL,
    feed_generated_at timestamptz NOT NULL,
    imported_at timestamptz NOT NULL DEFAULT now()
);

-- Stores per-version offer rows keyed by (catalog_version, offer_id) so multiple feed versions coexist
-- without violating catalog_offers_pkey (offer_id) or breaking existing storefront foreign keys.
CREATE TABLE IF NOT EXISTS recommendations.catalog_version_offers (
    catalog_version text NOT NULL REFERENCES recommendations.catalog_feed_versions(catalog_version) ON DELETE CASCADE,
    offer_id text NOT NULL,
    category_id text NOT NULL,
    vendor_name text,
    group_id text,
    name text NOT NULL,
    vendor_code text,
    url text NOT NULL,
    description text,
    picture_url text,
    available boolean NOT NULL,
    price numeric CHECK (price IS NULL OR price >= 0),
    old_price numeric CHECK (old_price IS NULL OR old_price >= 0),
    currency_code text,
    parameters jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(parameters) = 'array'),
    vendor_code_key text,
    url_code_key text,
    name_key text,
    PRIMARY KEY (catalog_version, offer_id)
);

CREATE INDEX IF NOT EXISTS catalog_version_offers_name_trgm_idx
    ON recommendations.catalog_version_offers USING gin (name public.gin_trgm_ops);

CREATE INDEX IF NOT EXISTS catalog_version_offers_vendor_code_trgm_idx
    ON recommendations.catalog_version_offers USING gin (vendor_code public.gin_trgm_ops);

CREATE INDEX IF NOT EXISTS catalog_version_offers_keys_idx
    ON recommendations.catalog_version_offers (catalog_version, vendor_code_key, url_code_key, name_key);

DO $vec$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector') THEN
        ALTER TABLE recommendations.catalog_version_offers
            ADD COLUMN IF NOT EXISTS embedding public.vector(1536);

        CREATE TABLE IF NOT EXISTS recommendations.embedding_cache (
            provider_id text NOT NULL,
            model_version text NOT NULL,
            dimensions integer NOT NULL CHECK (dimensions > 0),
            text_sha256 text NOT NULL,
            source_meta jsonb NOT NULL DEFAULT '{}'::jsonb,
            embedding public.vector NOT NULL,
            created_at timestamptz NOT NULL DEFAULT now(),
            PRIMARY KEY (provider_id, model_version, dimensions, text_sha256)
        );
    END IF;
END
$vec$;

CREATE TABLE IF NOT EXISTS recommendations.catalog_version_item_matches (
    catalog_version text NOT NULL REFERENCES recommendations.catalog_feed_versions(catalog_version) ON DELETE CASCADE,
    item_id text NOT NULL REFERENCES recommendations.historical_items(item_id),
    offer_id text,
    match_method text,
    status text NOT NULL,
    basis text NOT NULL DEFAULT '',
    candidates jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (catalog_version, item_id)
);

-- Versioned product features and variant differences keyed by (feature_version, item_id).
CREATE TABLE IF NOT EXISTS recommendations.item_feature_versions (
    feature_version text NOT NULL CHECK (btrim(feature_version) <> ''),
    item_id text NOT NULL REFERENCES recommendations.historical_items(item_id),
    offer_id text,
    title text NOT NULL,
    sku text,
    url text,
    platform text NOT NULL DEFAULT '',
    audience text NOT NULL DEFAULT '',
    available boolean NOT NULL DEFAULT false,
    in_sales boolean NOT NULL DEFAULT false,
    family_id text,
    family_source text,
    differences jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(differences) = 'array'),
    attributes jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(attributes) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (feature_version, item_id)
);

-- Recommendation snapshots bound to dataset_release (version + workbook_sha256).
CREATE TABLE IF NOT EXISTS recommendations.model_snapshots (
    snapshot_id text PRIMARY KEY CHECK (btrim(snapshot_id) <> ''),
    dataset_version text NOT NULL REFERENCES recommendations.dataset_release(version),
    workbook_sha256 text NOT NULL CHECK (btrim(workbook_sha256) <> ''),
    catalog_version text NOT NULL DEFAULT '',
    feature_version text NOT NULL DEFAULT '',
    train_through date NOT NULL,
    is_active boolean NOT NULL DEFAULT false,
    published_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS model_snapshots_single_active_idx
    ON recommendations.model_snapshots ((is_active))
    WHERE is_active;

-- Versioned evidence edges scoped to snapshot_id.
CREATE TABLE IF NOT EXISTS recommendations.evidence_records (
    snapshot_id text NOT NULL REFERENCES recommendations.model_snapshots(snapshot_id) ON DELETE CASCADE,
    evidence_id text NOT NULL,
    from_node text NOT NULL,
    to_node text NOT NULL,
    signal_type text NOT NULL,
    weight double precision NOT NULL,
    reason text NOT NULL,
    source text NOT NULL,
    source_url text,
    source_quote text,
    source_version text,
    observed_at timestamptz,
    constraints jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (snapshot_id, evidence_id)
);

-- Candidate pools preserving item_id even when no catalog offer exists for a historical item.
CREATE TABLE IF NOT EXISTS recommendations.snapshot_candidates (
    snapshot_id text NOT NULL REFERENCES recommendations.model_snapshots(snapshot_id) ON DELETE CASCADE,
    signal text NOT NULL,
    source_item_id text NOT NULL DEFAULT '',
    client_group text NOT NULL DEFAULT '',
    client_type text NOT NULL DEFAULT '',
    industry text NOT NULL DEFAULT '',
    segment text NOT NULL DEFAULT '',
    rank integer NOT NULL CHECK (rank >= 1),
    item_id text NOT NULL,
    recommendation_kind text NOT NULL,
    score double precision NOT NULL,
    evidence_ids jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(evidence_ids) = 'array'),
    PRIMARY KEY (snapshot_id, signal, source_item_id, client_group, client_type, industry, segment, item_id)
);

CREATE INDEX IF NOT EXISTS snapshot_candidates_lookup_idx
    ON recommendations.snapshot_candidates (
        snapshot_id, signal, source_item_id, client_group, client_type, industry, segment,
        score DESC, item_id ASC
    );
