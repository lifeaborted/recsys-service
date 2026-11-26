CREATE TABLE IF NOT EXISTS recommendations.dataset_release (
    version text PRIMARY KEY,
    workbook_sha256 text NOT NULL,
    imported_at timestamptz NOT NULL DEFAULT now()
);
