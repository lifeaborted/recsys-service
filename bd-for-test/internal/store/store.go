package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"recsys-migration/internal/config"
)

// Item — формат карточки, который ждёт фронтенд (см. src/api/types.ts).
type Item struct {
	ItemID   string   `json:"item_id"`
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle"`
	Category string   `json:"category"`
	Score    float64  `json:"score"`
	Reason   string   `json:"reason"`
	Price    float64  `json:"price"`
	Currency string   `json:"currency"`
	Tags     []string `json:"tags"`
	ImageURL string   `json:"image_url"`
	URL      string   `json:"url"`
}

// Result — выдача рекомендаций вместе с метаданными запроса.
type Result struct {
	UserID       string
	ClientID     string
	ModelVersion string
	RequestID    string
	GeneratedAt  time.Time
	Items        []Item
}

// UserRef — соответствие пользователя фронтенда пользователю и клиенту БД.
type UserRef struct {
	ExternalID string
	DBUserID   int64
	ClientID   string
}

// Store хранит соединение и настройки отображения пользователей.
type Store struct {
	db  *sql.DB
	cfg *config.Config
}

func New(db *sql.DB, cfg *config.Config) *Store {
	return &Store{db: db, cfg: cfg}
}

// Health проверяет соединение.
func (s *Store) Health(ctx context.Context) (string, error) {
	if err := s.db.QueryRowContext(ctx, "SELECT 1").Scan(new(int)); err != nil {
		return "", err
	}
	return s.cfg.API.ModelVersion, nil
}

// ResolveUser определяет client_id для запроса рекомендаций.
func (s *Store) ResolveUser(ctx context.Context, externalID string) (UserRef, error) {
	ref := UserRef{ExternalID: externalID, ClientID: s.cfg.API.FallbackClientID}
	if mapping, ok := s.cfg.API.Users[externalID]; ok {
		ref.DBUserID = mapping.DBUserID
		if mapping.ClientID != "" {
			ref.ClientID = mapping.ClientID
		}
	} else if id, err := strconv.ParseInt(externalID, 10, 64); err == nil {
		ref.DBUserID = id
	}
	// Если client_id из конфига отсутствует в БД — берём любой существующий.
	var exists bool
	err := s.db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM recommendations.clients WHERE client_id = $1)",
		ref.ClientID,
	).Scan(&exists)
	if err != nil {
		return ref, err
	}
	if !exists {
		// Если fallback-клиента нет, берём любого существующего.
		if err := s.db.QueryRowContext(ctx,
			"SELECT client_id FROM recommendations.clients ORDER BY client_id LIMIT 1",
		).Scan(&ref.ClientID); err != nil {
			return ref, fmt.Errorf("не найден ни один client_id (запустите cmd/migration seed): %w", err)
		}
	}
	return ref, nil
}

type dbRun struct {
	ID           int64
	ModelVersion string
	RequestedAt  time.Time
}

func (s *Store) latestRun(ctx context.Context, clientID string) (dbRun, error) {
	var run dbRun
	err := s.db.QueryRowContext(ctx, `
		SELECT run_id, model_version, requested_at
		FROM recommendations.recommendation_runs
		WHERE client_id = $1
		ORDER BY requested_at DESC, run_id DESC
		LIMIT 1
	`, clientID).Scan(&run.ID, &run.ModelVersion, &run.RequestedAt)
	return run, err
}

// seedBaseline создаёт простейшую выдачу из истории покупок, чтобы API
// сразу отдавал данные из БД, даже если Recommendation-таблицы пусты.
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
	`, clientID, s.cfg.API.ModelVersion).Scan(&runID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT offer_id, COUNT(*)::bigint AS buys
		FROM recommendations.purchase_history
		WHERE offer_id IS NOT NULL
		GROUP BY offer_id
		ORDER BY buys DESC, offer_id
		LIMIT 3
	`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	type candidate struct {
		offerID string
		buys    int64
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
	if err := rows.Close(); err != nil {
		_ = tx.Rollback()
		return err
	}

	// Если истории покупок ещё нет (демо-база), берём первые офферы каталога.
	if len(candidates) == 0 {
		fallback, err := tx.QueryContext(ctx, `
			SELECT offer_id FROM recommendations.catalog_offers
			WHERE available ORDER BY offer_id LIMIT 3
		`)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		for fallback.Next() {
			var offerID string
			if err := fallback.Scan(&offerID); err != nil {
				fallback.Close()
				_ = tx.Rollback()
				return err
			}
			candidates = append(candidates, candidate{offerID: offerID, buys: 0})
		}
		if err := fallback.Close(); err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	for i, c := range candidates {
		if i+1 > 3 {
			break // constraint recommendation_items.rank: 1..3
		}
		score := 0.9 - float64(i)*0.05
		reason := fmt.Sprintf("Покупали вместе с вашими товарами (%d покупок)", c.buys)
		if c.buys == 0 {
			reason = "Популярно в каталоге Softline"
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO recommendations.recommendation_items
				(run_id, rank, offer_id, source_item_id, explanation, score)
			VALUES ($1, $2, $3, NULL, $4, $5)
		`, runID, i+1, c.offerID, reason, score)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

// Recommend возвращает выдачу для клиента: сначала сохранённый run, затем
// недостающие позиции из витрины каталога.
func (s *Store) Recommend(ctx context.Context, clientID string, limit int) (Result, error) {
	result := Result{
		ClientID:     clientID,
		ModelVersion: s.cfg.API.ModelVersion,
		GeneratedAt:  time.Now().UTC(),
		Items:        make([]Item, 0, limit),
	}

	run, err := s.latestRun(ctx, clientID)
	if err == sql.ErrNoRows {
		if err := s.seedBaseline(ctx, clientID); err != nil {
			return result, fmt.Errorf("создать базовую выдачу: %w", err)
		}
		run, err = s.latestRun(ctx, clientID)
	}
	if err != nil && err != sql.ErrNoRows {
		return result, err
	}

	if err == nil {
		result.ModelVersion = run.ModelVersion
		result.RequestID = fmt.Sprintf("run-%d", run.ID)
		result.GeneratedAt = run.RequestedAt
		result.Items, err = s.runItems(ctx, run.ID, limit)
		if err != nil {
			return result, err
		}
	} else {
		result.RequestID = "live-catalog"
	}

	if len(result.Items) < limit {
		seen := make(map[string]bool, len(result.Items))
		for _, item := range result.Items {
			seen[item.ItemID] = true
		}
		extra, err := s.catalogItems(ctx, limit-len(result.Items), seen)
		if err != nil {
			return result, err
		}
		result.Items = append(result.Items, extra...)
	}
	return result, nil
}

func (s *Store) runItems(ctx context.Context, runID int64, limit int) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT i.offer_id, COALESCE(c.name, i.offer_id),
		       COALESCE(c.vendor, ''), COALESCE(c.category, ''),
		       COALESCE(i.score, 0), i.explanation,
		       COALESCE(c.price, 0), COALESCE(c.currency_code, 'RUB'),
		       COALESCE(c.picture_url, ''), COALESCE(c.url, '')
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
		if err := rows.Scan(
			&it.ItemID, &it.Title, &it.Subtitle, &it.Category, &it.Score, &it.Reason,
			&it.Price, &it.Currency, &it.ImageURL, &it.URL,
		); err != nil {
			return nil, err
		}
		it.Tags = tags(it.Subtitle, it.Category)
		items = append(items, it)
	}
	return items, rows.Err()
}

func (s *Store) catalogItems(ctx context.Context, limit int, seen map[string]bool) ([]Item, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT offer_id, name, COALESCE(vendor, ''), COALESCE(category, ''),
		       COALESCE(price, 0), COALESCE(currency_code, 'RUB'),
		       COALESCE(picture_url, ''), COALESCE(url, '')
		FROM recommendations.storefront_catalog
		WHERE available
		ORDER BY offer_id
		LIMIT $1
	`, limit+len(seen)+10)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Item, 0, limit)
	for rows.Next() {
		var it Item
		if err := rows.Scan(
			&it.ItemID, &it.Title, &it.Subtitle, &it.Category,
			&it.Price, &it.Currency, &it.ImageURL, &it.URL,
		); err != nil {
			return nil, err
		}
		if seen[it.ItemID] {
			continue
		}
		seen[it.ItemID] = true
		it.Score = 0.5 - float64(len(items))*0.02
		if it.Score < 0.1 {
			it.Score = 0.1
		}
		it.Reason = "Популярно в каталоге Softline"
		it.Tags = tags(it.Subtitle, it.Category)
		items = append(items, it)
		if len(items) >= limit {
			break
		}
	}
	return items, rows.Err()
}

// RecordFeedback пишет событие в recommendation_events.
func (s *Store) RecordFeedback(ctx context.Context, clientID, itemID, action string) error {
	eventType := "click"
	if action == "purchase" {
		eventType = "purchase"
	}

	run, err := s.latestRun(ctx, clientID)
	if err == sql.ErrNoRows {
		if err := s.seedBaseline(ctx, clientID); err != nil {
			return err
		}
		run, err = s.latestRun(ctx, clientID)
	}
	if err != nil {
		return err
	}

	if err := s.ensureItem(ctx, run.ID, itemID); err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO recommendations.recommendation_events (run_id, offer_id, event_type)
		VALUES ($1, $2, $3)
	`, run.ID, itemID, eventType)
	return err
}

// ensureItem добавляет товар в выдачу, если по нему пришёл сигнал, а в
// recommendation_items его ещё нет (иначе FK на recommendation_events не даст
// записать событие). Занимаем свободный rank 1..3, при необходимости вытесняя
// последнюю позицию.
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
	occupied := make(map[int]bool)
	for rows.Next() {
		var rank int
		if err := rows.Scan(&rank); err != nil {
			rows.Close()
			return err
		}
		occupied[rank] = true
	}
	if err := rows.Close(); err != nil {
		return err
	}

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
		VALUES ($1, $2, $3, NULL, $4, $5)
	`, runID, rank, itemID, "Сигнал пользователя", 0.5)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("добавить товар в выдачу: %w", err)
	}
	return tx.Commit()
}

// RunInfo — сохранённая выдача для отладочной страницы.
type RunInfo struct {
	RunID        int64     `json:"run_id"`
	ModelVersion string    `json:"model_version"`
	RequestedAt  time.Time `json:"requested_at"`
	Items        []Item    `json:"items"`
	Clicks       int       `json:"clicks"`
	Purchases    int       `json:"purchases"`
}

// Runs возвращает последние выдачи клиента вместе с товарами и событиями.
func (s *Store) Runs(ctx context.Context, clientID string, limit int) ([]RunInfo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT run_id, model_version, requested_at
		FROM recommendations.recommendation_runs
		WHERE client_id = $1
		ORDER BY requested_at DESC, run_id DESC
		LIMIT $2
	`, clientID, limit)
	if err != nil {
		return nil, err
	}

	var runs []dbRun
	for rows.Next() {
		var run dbRun
		if err := rows.Scan(&run.ID, &run.ModelVersion, &run.RequestedAt); err != nil {
			rows.Close()
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	result := make([]RunInfo, 0, len(runs))
	for _, run := range runs {
		info := RunInfo{
			RunID:        run.ID,
			ModelVersion: run.ModelVersion,
			RequestedAt:  run.RequestedAt,
			Items:        []Item{},
		}
		info.Items, err = s.runItems(ctx, run.ID, 10)
		if err != nil {
			return nil, err
		}
		if err := s.db.QueryRowContext(ctx, `
			SELECT
				count(*) FILTER (WHERE event_type = 'click'),
				count(*) FILTER (WHERE event_type = 'purchase')
			FROM recommendations.recommendation_events
			WHERE run_id = $1
		`, run.ID).Scan(&info.Clicks, &info.Purchases); err != nil {
			return nil, err
		}
		result = append(result, info)
	}
	return result, nil
}

// ForceNewRun создаёт новую выдачу (полезно для проверки пересчёта).
func (s *Store) ForceNewRun(ctx context.Context, clientID string) (RunInfo, error) {
	if err := s.seedBaseline(ctx, clientID); err != nil {
		return RunInfo{}, err
	}
	run, err := s.latestRun(ctx, clientID)
	if err != nil {
		return RunInfo{}, err
	}
	items, err := s.runItems(ctx, run.ID, 10)
	if err != nil {
		return RunInfo{}, err
	}
	return RunInfo{
		RunID:        run.ID,
		ModelVersion: run.ModelVersion,
		RequestedAt:  run.RequestedAt,
		Items:        items,
	}, nil
}

// Stats возвращает количество строк по ключевым таблицам БД.
func (s *Store) Stats(ctx context.Context) (map[string]int64, error) {
	tables := []string{
		"clients", "catalog_offers", "catalog_categories", "orders", "sales",
		"recommendation_runs", "recommendation_items", "recommendation_events",
	}
	result := make(map[string]int64, len(tables))
	for _, table := range tables {
		query := fmt.Sprintf("SELECT count(*) FROM recommendations.%s", table)
		var count int64
		if err := s.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			return nil, fmt.Errorf("%s: %w", table, err)
		}
		result[table] = count
	}
	return result, nil
}

func tags(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}
