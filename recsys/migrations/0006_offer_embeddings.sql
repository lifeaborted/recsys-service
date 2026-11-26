-- Catalog vectors live here when the server has pgvector. A plain PostgreSQL
-- build skips the table; cmd/embed-catalog reports that instead of writing a wrong column.
DO $vec$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'vector') THEN
        CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector') THEN
        CREATE TABLE IF NOT EXISTS recommendations.catalog_offer_embeddings (
            model_version text NOT NULL,
            offer_id text NOT NULL,
            text_sha256 text NOT NULL,
            embedding public.vector NOT NULL,
            PRIMARY KEY (model_version, offer_id)
        );
    END IF;
END
$vec$;
