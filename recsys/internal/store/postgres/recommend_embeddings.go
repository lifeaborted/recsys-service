package postgres

import (
	"context"

	"recsys/internal/domain"
	"recsys/internal/recommend"
)

var _ recommend.SimilarItems = (*Store)(nil)

// SimilarItems scores products from the online snapshot against vectors of owned products.
func (s *Store) SimilarItems(ctx context.Context, snapshot string, owned []domain.ItemID, limit int) ([]domain.Candidate, error) {
	if s.embeddingModel == "" || len(owned) == 0 || limit < 1 {
		return nil, nil
	}

	rows, err := s.pool.Query(ctx, `
WITH products AS (
    SELECT item.item_id, item.available, e.embedding
    FROM recommendations.model_snapshots m
    JOIN recommendations.item_feature_versions item USING (feature_version)
    JOIN recommendations.catalog_offer_embeddings e ON e.offer_id = item.offer_id
    WHERE m.snapshot_id = $1 AND e.model_version = $2
), matches AS (
    SELECT DISTINCT ON (candidate.item_id)
        candidate.item_id, owned.item_id AS owned_id,
        1 - (candidate.embedding OPERATOR(public.<=>) owned.embedding) AS score
    FROM products candidate
    CROSS JOIN products owned
    WHERE owned.item_id = ANY($3::text[]) AND NOT (candidate.item_id = ANY($3::text[]))
      AND candidate.available
      AND NOT EXISTS (
          SELECT 1 FROM recommendations.snapshot_candidates sc
          WHERE sc.snapshot_id = $1 AND sc.item_id = candidate.item_id
            AND sc.recommendation_kind = 'renewal' AND sc.source_item_id = ANY($3::text[])
      )
    ORDER BY candidate.item_id, score DESC, owned.item_id
)
SELECT item_id, owned_id, score FROM matches
WHERE score > 0 AND score <= 1
ORDER BY score DESC, item_id
LIMIT $4`, snapshot, s.embeddingModel, owned, limit)
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
