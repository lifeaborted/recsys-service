package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// OfferName is a public catalog title that can be embedded.
type OfferName struct {
	ID   string
	Name string
}

// EnsureOfferEmbeddings creates the pgvector store used by the catalog job.
func (s *Store) EnsureOfferEmbeddings(ctx context.Context) error {
	var available bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'vector')`).Scan(&available); err != nil {
		return err
	}
	if !available {
		return fmt.Errorf("postgresql was built without pgvector")
	}
	_, err := s.pool.Exec(ctx, `
CREATE SCHEMA IF NOT EXISTS recommendations;
CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;
CREATE TABLE IF NOT EXISTS recommendations.catalog_offer_embeddings (
    model_version text NOT NULL,
    offer_id text NOT NULL,
    text_sha256 text NOT NULL,
    embedding public.vector NOT NULL,
    PRIMARY KEY (model_version, offer_id)
)`)
	return err
}

// AvailableOfferNames lists priced offers that are currently for sale.
func (s *Store) AvailableOfferNames(ctx context.Context) ([]OfferName, error) {
	rows, err := s.pool.Query(ctx, `
SELECT offer_id, name
FROM recommendations.catalog_offers
WHERE available AND price > 0 AND btrim(name) <> ''
ORDER BY offer_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var offers []OfferName
	for rows.Next() {
		var offer OfferName
		if err := rows.Scan(&offer.ID, &offer.Name); err != nil {
			return nil, err
		}
		offers = append(offers, offer)
	}
	return offers, rows.Err()
}

// EmbeddedOffers returns offer ids already stored for one model.
func (s *Store) EmbeddedOffers(ctx context.Context, model string) (map[string]struct{}, int, error) {
	rows, err := s.pool.Query(ctx, `
SELECT offer_id, vector_dims(embedding)
FROM recommendations.catalog_offer_embeddings
WHERE model_version = $1`, model)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	seen := map[string]struct{}{}
	dimensions := 0
	for rows.Next() {
		var id string
		var dims int
		if err := rows.Scan(&id, &dims); err != nil {
			return nil, 0, err
		}
		if dimensions == 0 {
			dimensions = dims
		} else if dims != dimensions {
			return nil, 0, fmt.Errorf("stored embeddings have mixed dimensions")
		}
		seen[id] = struct{}{}
	}
	return seen, dimensions, rows.Err()
}

// EmbeddingRow is one stored catalog vector.
type EmbeddingRow struct {
	OfferID    string
	TextSHA256 string
	Vector     []float64
}

// SaveOfferEmbeddings upserts a batch. Every vector must match the dimensions already stored for the model.
func (s *Store) SaveOfferEmbeddings(ctx context.Context, model string, rows []EmbeddingRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, row := range rows {
		batch.Queue(`
INSERT INTO recommendations.catalog_offer_embeddings (model_version, offer_id, text_sha256, embedding)
VALUES ($1, $2, $3, $4::public.vector)
ON CONFLICT (model_version, offer_id) DO UPDATE
SET text_sha256 = EXCLUDED.text_sha256, embedding = EXCLUDED.embedding`,
			model, row.OfferID, row.TextSHA256, vectorLiteral(row.Vector))
	}
	results := s.pool.SendBatch(ctx, batch)
	defer results.Close()
	for range rows {
		if _, err := results.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// NearestOffers returns catalog offer ids closest to the query vector.
func (s *Store) NearestOffers(ctx context.Context, model string, vector []float64, limit int) ([]struct {
	OfferID string
	Score   float64
}, error) {
	if limit < 1 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
SELECT offer_id, 1 - (embedding <=> $1::public.vector) AS score
FROM recommendations.catalog_offer_embeddings
WHERE model_version = $2
ORDER BY embedding <=> $1::public.vector
LIMIT $3`, vectorLiteral(vector), model, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []struct {
		OfferID string
		Score   float64
	}
	for rows.Next() {
		var hit struct {
			OfferID string
			Score   float64
		}
		if err := rows.Scan(&hit.OfferID, &hit.Score); err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}

// IndexOfferEmbeddings pins the column to the stored dimension and builds a cosine index.
func (s *Store) IndexOfferEmbeddings(ctx context.Context) error {
	var dims int
	if err := s.pool.QueryRow(ctx, `
SELECT vector_dims(embedding) FROM recommendations.catalog_offer_embeddings LIMIT 1`).Scan(&dims); err != nil {
		return err
	}
	if dims < 1 {
		return fmt.Errorf("stored embedding has no dimensions")
	}
	_, err := s.pool.Exec(ctx, fmt.Sprintf(`
ALTER TABLE recommendations.catalog_offer_embeddings
    ALTER COLUMN embedding TYPE public.vector(%d);
CREATE INDEX IF NOT EXISTS catalog_offer_embeddings_cosine
    ON recommendations.catalog_offer_embeddings
    USING hnsw (embedding vector_cosine_ops)`, dims))
	return err
}

func vectorLiteral(vector []float64) string {
	var b strings.Builder
	b.Grow(len(vector) * 8)
	b.WriteByte('[')
	for i, value := range vector {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(value, 'f', -1, 64))
	}
	b.WriteByte(']')
	return b.String()
}
