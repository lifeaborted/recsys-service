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

// EmbeddedOffers returns the stored input digest for each offer of one model.
func (s *Store) EmbeddedOffers(ctx context.Context, model string) (map[string]string, int, error) {
	rows, err := s.pool.Query(ctx, `
SELECT offer_id, vector_dims(embedding), text_sha256
FROM recommendations.catalog_offer_embeddings
WHERE model_version = $1`, model)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	seen := map[string]string{}
	dimensions := 0
	for rows.Next() {
		var id, digest string
		var dims int
		if err := rows.Scan(&id, &dims, &digest); err != nil {
			return nil, 0, err
		}
		if dimensions == 0 {
			dimensions = dims
		} else if dims != dimensions {
			return nil, 0, fmt.Errorf("stored embeddings have mixed dimensions")
		}
		seen[id] = digest
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
	dimensions := len(vector)
	distance := "embedding OPERATOR(public.<=>) $1::public.vector"
	if dimensions > 0 && dimensions <= 4000 {
		distance = fmt.Sprintf("embedding::public.halfvec(%[1]d) OPERATOR(public.<=>) $1::public.halfvec(%[1]d)", dimensions)
	}
	query := fmt.Sprintf(`
SELECT offer_id, 1 - (embedding OPERATOR(public.<=>) $1::public.vector)
FROM recommendations.catalog_offer_embeddings
WHERE model_version = $2 AND public.vector_dims(embedding) = %d
ORDER BY %s
LIMIT $3 `, dimensions, distance)
	rows, err := s.pool.Query(ctx, query, vectorLiteral(vector), model, limit)
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

// IndexOfferEmbeddings maintains one HNSW expression index for the model's dimensions.
func (s *Store) IndexOfferEmbeddings(ctx context.Context, model string) error {
	dims, err := s.embeddingDimension(ctx, model)
	if err != nil {
		return err
	}
	if dims < 1 || dims > 4000 {
		return fmt.Errorf("cannot build a halfvec HNSW index for %d dimensions", dims)
	}
	_, err = s.pool.Exec(ctx, fmt.Sprintf(`
CREATE INDEX IF NOT EXISTS catalog_offer_embeddings_cosine_half_%[1]d
    ON recommendations.catalog_offer_embeddings
    USING hnsw ((embedding::public.halfvec(%[1]d)) public.halfvec_cosine_ops)
    WHERE public.vector_dims(embedding) = %[1]d `, dims))
	return err
}

func (s *Store) embeddingDimension(ctx context.Context, model string) (int, error) {
	var dims int
	err := s.pool.QueryRow(ctx, `
SELECT COALESCE((SELECT public.vector_dims(embedding)
	FROM recommendations.catalog_offer_embeddings WHERE model_version = $1 LIMIT 1), 0)
`, model).Scan(&dims)
	return dims, err
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
