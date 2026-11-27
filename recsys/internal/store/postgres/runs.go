package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"recsys/internal/contract"
	"recsys/internal/domain"
)

// CreateRun persists a stand run together with its ranked items.
func (s *Store) CreateRun(
	ctx context.Context,
	clientID domain.ClientID,
	modelVersion string,
	items []contract.RunItemInput,
) (contract.RunRecord, error) {
	var out contract.RunRecord
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)

	if modelVersion == "" {
		modelVersion = "stand/manual"
	}
	if err = tx.QueryRow(ctx, `
		INSERT INTO recommendations.recommendation_runs (client_id, model_version)
		VALUES ($1, $2)
		RETURNING run_id, requested_at
	`, string(clientID), modelVersion).Scan(&out.RunID, &out.RequestedAt); err != nil {
		return out, fmt.Errorf("insert run: %w", err)
	}
	out.ModelVersion = modelVersion
	out.Items = []contract.RunItemRecord{}

	for i, item := range items {
		if i >= 3 || item.ItemID == "" {
			continue
		}
		offerID := item.OfferID
		if offerID == "" {
			if err = tx.QueryRow(ctx, `
				SELECT f.offer_id
				FROM recommendations.item_feature_versions f
				JOIN recommendations.model_snapshots m
				  ON m.feature_version = f.feature_version
				  AND (($2 = '' AND m.is_active) OR m.snapshot_id = NULLIF($2, ''))
				WHERE f.item_id = $1
				LIMIT 1
			`, item.ItemID, s.onlineSnapshot).Scan(&offerID); err != nil {
				if err == pgx.ErrNoRows {
					continue
				}
				return out, fmt.Errorf("resolve offer for %s: %w", item.ItemID, err)
			}
		}
		if offerID == "" {
			continue
		}
		explanation := item.Explanation
		if explanation == "" {
			explanation = "Создано на стенде"
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO recommendations.recommendation_items
				(run_id, rank, offer_id, source_item_id, explanation, score)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, out.RunID, i+1, offerID, item.ItemID, explanation, item.Score); err != nil {
			return out, fmt.Errorf("insert run item %s: %w", item.ItemID, err)
		}
		out.Items = append(out.Items, contract.RunItemRecord{
			ItemID: item.ItemID, OfferID: offerID, Explanation: explanation, Score: item.Score,
		})
	}

	if err = tx.Commit(ctx); err != nil {
		return out, err
	}
	return out, nil
}

// ListRuns returns recent runs with click/purchase counters for the profile.
func (s *Store) ListRuns(ctx context.Context, clientID domain.ClientID, limit int) ([]contract.RunRecord, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx, `
		SELECT r.run_id, r.model_version, r.requested_at,
		       count(e.event_id) FILTER (WHERE e.event_type = 'click') AS clicks,
		       count(e.event_id) FILTER (WHERE e.event_type = 'purchase') AS purchases
		FROM recommendations.recommendation_runs r
		LEFT JOIN recommendations.recommendation_events e ON e.run_id = r.run_id
		WHERE r.client_id = $1
		GROUP BY r.run_id
		ORDER BY r.requested_at DESC, r.run_id DESC
		LIMIT $2
	`, string(clientID), limit)
	if err != nil {
		return nil, err
	}

	runs := []contract.RunRecord{}
	for rows.Next() {
		var run contract.RunRecord
		var clicks, purchases int64
		if err = rows.Scan(&run.RunID, &run.ModelVersion, &run.RequestedAt, &clicks, &purchases); err != nil {
			rows.Close()
			return nil, err
		}
		run.Clicks, run.Purchases = int(clicks), int(purchases)
		run.Items = []contract.RunItemRecord{}
		runs = append(runs, run)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	for i := range runs {
		itemRows, err := s.pool.Query(ctx, `
			SELECT source_item_id, offer_id, explanation, COALESCE(score, 0)
			FROM recommendations.recommendation_items
			WHERE run_id = $1
			ORDER BY rank
		`, runs[i].RunID)
		if err != nil {
			return nil, err
		}
		for itemRows.Next() {
			var item contract.RunItemRecord
			if err = itemRows.Scan(&item.ItemID, &item.OfferID, &item.Explanation, &item.Score); err != nil {
				itemRows.Close()
				return nil, err
			}
			runs[i].Items = append(runs[i].Items, item)
		}
		err = itemRows.Err()
		itemRows.Close()
		if err != nil {
			return nil, err
		}
	}
	return runs, nil
}

// CreateEvent records a click or purchase for a stand run.
func (s *Store) CreateEvent(ctx context.Context, clientID domain.ClientID, runID int64, offerID, eventType string) error {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO recommendations.recommendation_events (run_id, offer_id, event_type)
		SELECT run.run_id, $2, $3
		FROM recommendations.recommendation_runs run
		WHERE run.run_id = $1 AND run.client_id = $4
		  AND EXISTS (SELECT 1 FROM recommendations.recommendation_items item
		              WHERE item.run_id = run.run_id AND item.offer_id = $2)
	`, runID, offerID, eventType, string(clientID))
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return contract.ErrRunNotFound
	}
	return nil
}
