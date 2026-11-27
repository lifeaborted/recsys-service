package recommend

import (
	"context"

	"recsys/internal/contract"
	"recsys/internal/domain"
)

// similarTier sits under a documented complement and above bare popularity.
const similarTier = 4

// SimilarItems is an optional online source. Stores that do not implement it are skipped.
type SimilarItems interface {
	SimilarItems(ctx context.Context, snapshotID string, owned []domain.ItemID, limit int) ([]domain.Candidate, error)
}

func collectSimilar(ctx context.Context, reader Reader, snapshot string, owned []domain.ItemID, bought map[domain.ItemID]bool, best map[domain.ItemID]ranked, pending map[domain.ItemID]bool) bool {
	source, ok := reader.(SimilarItems)
	if !ok || len(owned) == 0 {
		return true
	}
	rows, err := source.SimilarItems(ctx, snapshot, owned, cardLimit(contract.Online))
	if err != nil {
		return false
	}
	for _, candidate := range rows {
		if candidate.ItemID == "" || bought[candidate.ItemID] {
			continue
		}
		proof := similarProof(candidate, snapshot)
		if len(proof) == 0 {
			continue
		}
		row := ranked{candidate.ItemID, similarTier, candidate.Score, domain.Popular, proof}
		if old, exists := best[candidate.ItemID]; exists && compare(old, row) <= 0 {
			continue
		}
		best[candidate.ItemID] = row
		pending[candidate.ItemID] = true
	}
	return true
}

func similarProof(candidate domain.Candidate, snapshot string) []domain.Evidence {
	var valid []domain.Evidence
	for _, edge := range candidate.Evidence {
		if edge.ID == "" || edge.From == "" || edge.To != "item:"+string(candidate.ItemID) || edge.Source == "" || edge.Reason == "" || edge.SnapshotID != snapshot {
			continue
		}
		valid = append(valid, edge)
	}
	return valid
}
