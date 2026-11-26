// Package postgres — адаптер к схеме recommendations в PostgreSQL.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// Item — карточка витрины, которую отдает API.
type Item struct {
	ItemID   string  `json:"item_id"`
	Title    string  `json:"title"`
	Subtitle string  `json:"subtitle"`
	Category string  `json:"category"`
	Score    float64 `json:"score"`
	Reason   string  `json:"reason"`
	Price    float64 `json:"price"`
	Currency string  `json:"currency"`
	URL      string  `json:"url"`
}

// Result — выдача вместе с метаданными run'а.
type Result struct {
	ClientID     string
	RequestID    string
	ModelVersion string
	GeneratedAt  time.Time
	Items        []Item
}

// RunInfo — сохранённая выдача для отладки.
type RunInfo struct {
	RunID        int64     `json:"run_id"`
	ModelVersion string    `json:"model_version"`
	RequestedAt  time.Time `json:"requested_at"`
	Items        []Item    `json:"items"`
	Clicks       int       `json:"clicks"`
	Purchases    int       `json:"purchases"`
}

type Store struct {
	db           *sql.DB
	modelVersion string
}

func Open(dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	return &Store{db: db, modelVersion: "demo-popularity-v4"}, nil
}

// SetModelVersion задаёт версию модели из конфига стенда.
func (s *Store) SetModelVersion(version string) {
	if version != "" {
		s.modelVersion = version
	}
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Health(ctx context.Context) error {
	var one int
	return s.db.QueryRowContext(ctx, "SELECT 1 FROM recommendations.clients LIMIT 1").Scan(&one)
}

type candidate struct {
	offerID string
	buys    int64
}

func (s *Store) latestRun(ctx context.Context, clientID string) (int64, string, time.Time, error) {
	var id int64
	var version string
	var at time.Time
	err := s.db.QueryRowContext(ctx, `
		SELECT run_id, model_version, requested_at
		FROM recommendations.recommendation_runs
		WHERE client_id = $1
		ORDER BY requested_at DESC, run_id DESC
		LIMIT 1
	`, clientID).Scan(&id, &version, &at)
	return id, version, at, err
}

func (s *Store) seedBaseline(ctx context.Context, clientID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	var runID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO recommendations.recommendation_runs (client_id, model_version)
		VALUES ($1, $2)
		RETURNING run_id
	`, clientID, s.modelVersion).Scan(&runID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT h.offer_id, COUNT(DISTINCT h.client_id)::bigint AS buyers
		FROM recommendations.purchase_history h
		JOIN recommendations.catalog_offers o ON o.offer_id = h.offer_id
		WHERE o.available AND NOT EXISTS (
			SELECT 1 FROM recommendations.purchase_history seen
			WHERE seen.client_id = $1 AND seen.offer_id = h.offer_id
		)
		GROUP BY h.offer_id
		ORDER BY buyers DESC, h.offer_id
		LIMIT 3
	`, clientID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.offerID, &c.buys); err != nil {
			rows.Close()
			_ = tx.Rollback()
			return err
		}
		candidates = append(candidates, c)
	}
	rows.Close()

	if len(candidates) == 0 {
		fallback, err := tx.QueryContext(ctx, `
			SELECT offer_id FROM recommendations.catalog_offers
			WHERE available AND NOT EXISTS (
				SELECT 1 FROM recommendations.purchase_history seen
				WHERE seen.client_id = $1 AND seen.offer_id = catalog_offers.offer_id
			)
			ORDER BY offer_id LIMIT 3
		`, clientID)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		for fallback.Next() {
			var id string
			if err := fallback.Scan(&id); err != nil {
				fallback.Close()
				_ = tx.Rollback()
				return err
			}
			candidates = append(candidates, candidate{offerID: id})
		}
		fallback.Close()
	}

	for i, c := range candidates {
		if i >= 3 {
			break
		}
		reason := fmt.Sprintf("Товар купили %d разных клиентов", c.buys)
		if c.buys == 0 {
			reason = "Доступно в каталоге"
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO recommendations.recommendation_items
				(run_id, rank, offer_id, source_item_id, explanation, score)
			VALUES ($1, $2, $3, NULL, $4, $5)
		`, runID, i+1, c.offerID, reason, 0.9-float64(i)*0.05)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) runItems(ctx context.Context, runID int64, limit int) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT i.offer_id, COALESCE(c.name, i.offer_id),
		       COALESCE(c.vendor, ''), COALESCE(c.category, ''),
		       COALESCE(i.score, 0), i.explanation,
		       COALESCE(c.price, 0), COALESCE(c.currency_code, 'RUB'),
		       COALESCE(c.url, '')
		FROM recommendations.recommendation_items i
		LEFT JOIN recommendations.storefront_catalog c USING (offer_id)
		WHERE i.run_id = $1
		ORDER BY i.rank
		LIMIT $2
	`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Item, 0, limit)
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ItemID, &it.Title, &it.Subtitle, &it.Category,
			&it.Score, &it.Reason, &it.Price, &it.Currency, &it.URL); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (s *Store) Recommend(ctx context.Context, clientID string, limit int) (Result, error) {
	result := Result{ClientID: clientID, ModelVersion: s.modelVersion, GeneratedAt: time.Now().UTC()}
	runID, version, at, err := s.latestRun(ctx, clientID)
	if err == sql.ErrNoRows {
		if err := s.seedBaseline(ctx, clientID); err != nil {
			return result, fmt.Errorf("baseline: %w", err)
		}
		runID, version, at, err = s.latestRun(ctx, clientID)
	}
	if err != nil && err != sql.ErrNoRows {
		return result, err
	}
	if err == nil {
		result.RequestID = fmt.Sprintf("run-%d", runID)
		result.ModelVersion = version
		result.GeneratedAt = at
		result.Items, err = s.runItems(ctx, runID, limit)
		if err != nil {
			return result, err
		}
	} else {
		result.RequestID = "live-catalog"
	}
	return result, nil
}

func (s *Store) ForceNewRun(ctx context.Context, clientID string) (int64, error) {
	if err := s.seedBaseline(ctx, clientID); err != nil {
		return 0, err
	}
	runID, _, _, err := s.latestRun(ctx, clientID)
	return runID, err
}

func (s *Store) ensureItem(ctx context.Context, runID int64, itemID string) error {
	var exists bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM recommendations.recommendation_items
		               WHERE run_id = $1 AND offer_id = $2)
	`, runID, itemID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT rank FROM recommendations.recommendation_items WHERE run_id = $1 ORDER BY rank", runID)
	if err != nil {
		return err
	}
	occupied := map[int]bool{}
	for rows.Next() {
		var rank int
		if err := rows.Scan(&rank); err != nil {
			rows.Close()
			return err
		}
		occupied[rank] = true
	}
	rows.Close()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	rank := 1
	for occupied[rank] && rank < 3 {
		rank++
	}
	if rank == 3 && occupied[3] {
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM recommendations.recommendation_items WHERE run_id = $1 AND rank = 3", runID); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO recommendations.recommendation_items
			(run_id, rank, offer_id, source_item_id, explanation, score)
		VALUES ($1, $2, $3, NULL, 'Сигнал пользователя', 0.5)
	`, runID, rank, itemID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) Feedback(ctx context.Context, clientID, itemID, action string) error {
	runID, _, _, err := s.latestRun(ctx, clientID)
	if err == sql.ErrNoRows {
		if err := s.seedBaseline(ctx, clientID); err != nil {
			return err
		}
		runID, _, _, err = s.latestRun(ctx, clientID)
	}
	if err != nil {
		return err
	}
	if err := s.ensureItem(ctx, runID, itemID); err != nil {
		return err
	}
	eventType := "click"
	if action == "purchase" {
		eventType = "purchase"
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO recommendations.recommendation_events (run_id, offer_id, event_type)
		VALUES ($1, $2, $3)
	`, runID, itemID, eventType)
	return err
}

func (s *Store) Stats(ctx context.Context) (map[string]int64, error) {
	tables := []string{
		"clients", "catalog_offers", "catalog_categories", "orders", "sales",
		"recommendation_runs", "recommendation_items", "recommendation_events",
	}
	result := make(map[string]int64, len(tables))
	for _, table := range tables {
		var count int64
		if err := s.db.QueryRowContext(ctx,
			fmt.Sprintf("SELECT count(*) FROM recommendations.%s", table)).Scan(&count); err != nil {
			return nil, err
		}
		result[table] = count
	}
	return result, nil
}

func (s *Store) Runs(ctx context.Context, clientID string, limit int) ([]RunInfo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT run_id, model_version, requested_at
		FROM recommendations.recommendation_runs
		WHERE client_id = $1 ORDER BY requested_at DESC, run_id DESC LIMIT $2
	`, clientID, limit)
	if err != nil {
		return nil, err
	}
	type run struct {
		id      int64
		version string
		at      time.Time
	}
	var found []run
	for rows.Next() {
		var r run
		if err := rows.Scan(&r.id, &r.version, &r.at); err != nil {
			rows.Close()
			return nil, err
		}
		found = append(found, r)
	}
	rows.Close()

	result := make([]RunInfo, 0, len(found))
	for _, r := range found {
		items, err := s.runItems(ctx, r.id, 10)
		if err != nil {
			return nil, err
		}
		info := RunInfo{RunID: r.id, ModelVersion: r.version, RequestedAt: r.at, Items: items}
		if err := s.db.QueryRowContext(ctx, `
			SELECT count(*) FILTER (WHERE event_type = 'click'),
			       count(*) FILTER (WHERE event_type = 'purchase')
			FROM recommendations.recommendation_events WHERE run_id = $1
		`, r.id).Scan(&info.Clicks, &info.Purchases); err != nil {
			return nil, err
		}
		result = append(result, info)
	}
	return result, nil
}
