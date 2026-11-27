package postgres

import (
	"context"

	"recsys/internal/contract"
)

// ScoreHistory groups stored run items by minute and averages the score
// across all users for the requested item.
func (s *Store) ScoreHistory(ctx context.Context, itemID string) (contract.ScoreHistory, error) {
	out := contract.ScoreHistory{ItemID: itemID, Points: []contract.ScorePoint{}}

	if err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(avg(i.score), 0)
		FROM recommendations.recommendation_items i
		JOIN recommendations.recommendation_runs r ON r.run_id = i.run_id
		WHERE i.source_item_id = $1 AND i.score IS NOT NULL
	`, itemID).Scan(&out.Average); err != nil {
		return out, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT date_trunc('minute', r.requested_at) AS bucket,
		       avg(i.score),
		       count(*)::int
		FROM recommendations.recommendation_items i
		JOIN recommendations.recommendation_runs r ON r.run_id = i.run_id
		WHERE i.source_item_id = $1 AND i.score IS NOT NULL
		GROUP BY bucket
		ORDER BY bucket
	`, itemID)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	for rows.Next() {
		var point contract.ScorePoint
		if err = rows.Scan(&point.At, &point.Score, &point.Runs); err != nil {
			return out, err
		}
		out.Points = append(out.Points, point)
	}
	return out, rows.Err()
}
