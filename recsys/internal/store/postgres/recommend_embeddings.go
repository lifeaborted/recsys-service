package postgres

import (
	"context"
	"fmt"

	"recsys/internal/domain"
	"recsys/internal/recommend"
)

var _ recommend.SimilarItems = (*Store)(nil)

// SimilarItems scores products from the online snapshot against vectors of owned products.
func (s *Store) SimilarItems(ctx context.Context, snapshot string, owned []domain.ItemID, limit int) ([]domain.Candidate, error) {
	if s.embeddingModel == "" || len(owned) == 0 || limit < 1 {
		return nil, nil
	}

	dims, err := s.embeddingDimension(ctx, s.embeddingModel)
	if err != nil || dims == 0 {
		return nil, err
	}
	distance := "candidate.embedding OPERATOR(public.<=>) owned.embedding"
	if dims <= 4000 {
		distance = fmt.Sprintf("candidate.embedding::public.halfvec(%[1]d) OPERATOR(public.<=>) owned.embedding::public.halfvec(%[1]d)", dims)
	}
	query := fmt.Sprintf(`
WITH feature AS MATERIALIZED (
    SELECT item.item_id, item.offer_id, item.available
    FROM recommendations.model_snapshots snapshot
    JOIN recommendations.item_feature_versions item USING (feature_version)
    WHERE snapshot.snapshot_id = $1 AND item.offer_id IS NOT NULL
), owned AS (
    SELECT feature.item_id, e.embedding
    FROM feature
    JOIN recommendations.catalog_offer_embeddings e USING (offer_id)
    WHERE e.model_version = $2 AND feature.item_id = ANY($3::text[])
), matches AS (
    SELECT nearby.item_id, owned.item_id AS owned_id,
           1 - (nearby.embedding OPERATOR(public.<=>) owned.embedding) AS score
    FROM owned
    CROSS JOIN LATERAL (
        SELECT f.item_id, candidate.embedding
        FROM recommendations.catalog_offer_embeddings candidate
        JOIN feature f ON f.offer_id = candidate.offer_id
        WHERE candidate.model_version = $2 AND public.vector_dims(candidate.embedding) = %d
          AND f.available AND NOT (f.item_id = ANY($3::text[]))
          AND NOT EXISTS (
              SELECT 1 FROM recommendations.snapshot_candidates sc
              WHERE sc.snapshot_id = $1 AND sc.item_id = f.item_id
                AND sc.recommendation_kind = 'renewal' AND sc.source_item_id = ANY($3::text[])
          )
        ORDER BY %s
        LIMIT $5
    ) nearby
), best AS (
    SELECT DISTINCT ON (item_id) item_id, owned_id, score
    FROM matches WHERE score > 0 AND score <= 1
    ORDER BY item_id, score DESC, owned_id
)
SELECT item_id, owned_id, score FROM best
ORDER BY score DESC, item_id
LIMIT $4 `, dims, distance)
	rows, err := s.pool.Query(ctx, query, snapshot, s.embeddingModel, owned, limit, 2*limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []domain.Candidate
	for rows.Next() {
		var candidate domain.Candidate
		var sourceID string
		if err := rows.Scan(&candidate.ItemID, &sourceID, &candidate.Score); err != nil {
			return nil, err
		}
		candidate.Kind = domain.Popular
		candidate.Evidence = []domain.Evidence{{
			ID:   "embedding:" + sourceID + "→" + string(candidate.ItemID),
			From: "item:" + sourceID, To: "item:" + string(candidate.ItemID),
			Type: "similarity", Weight: candidate.Score, Source: s.embeddingModel,
			Reason: "Описание товара семантически близко к ранее купленному товару.", SnapshotID: snapshot,
		}}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}
