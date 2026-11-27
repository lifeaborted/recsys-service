package postgres

import (
	"context"
	"fmt"
)

// TableStats returns row counts used by the stand's "Тест рекомендаций" screen.
// It is read-only and does not touch the recommendation engine.
func (s *Store) TableStats(ctx context.Context) (map[string]int64, error) {
	queries := map[string]string{
		"clients":               `SELECT count(*) FROM recommendations.clients`,
		"catalog_offers":        `SELECT count(*) FROM recommendations.catalog_offers`,
		"sales":                 `SELECT count(*) FROM recommendations.sales`,
		"online_purchases":      `SELECT count(*) FROM recommendations.online_purchases`,
		"recommendation_runs":   `SELECT count(*) FROM recommendations.recommendation_runs`,
		"recommendation_events": `SELECT count(*) FROM recommendations.recommendation_events`,
	}
	out := make(map[string]int64, len(queries))
	for name, query := range queries {
		var count int64
		if err := s.pool.QueryRow(ctx, query).Scan(&count); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out[name] = count
	}
	return out, nil
}
