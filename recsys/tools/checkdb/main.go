// Быстрая проверка целостности данных рекомендаций.
//
//	go run ./tools/checkdb
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"

	"recsys/internal/config"
)

func main() {
	cfg := config.Load()
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()
	ctx := context.Background()

	queries := []struct {
		label string
		sql   string
	}{
		{"clients", "SELECT count(*) FROM recommendations.clients"},
		{"catalog_offers", "SELECT count(*) FROM recommendations.catalog_offers"},
		{"sales", "SELECT count(*) FROM recommendations.sales"},
		{"runs", "SELECT count(*) FROM recommendations.recommendation_runs"},
		{"items", "SELECT count(*) FROM recommendations.recommendation_items"},
		{"events", "SELECT count(*) FROM recommendations.recommendation_events"},
		{"invalid ranks", "SELECT count(*) FROM recommendations.recommendation_items WHERE rank < 1 OR rank > 3"},
		{"orphan items", `SELECT count(*) FROM recommendations.recommendation_items i LEFT JOIN recommendations.catalog_offers o USING (offer_id) WHERE o.offer_id IS NULL`},
		{"invalid events", `SELECT count(*) FROM recommendations.recommendation_events WHERE event_type NOT IN ('click','purchase')`},
		{"dataset releases", "SELECT count(*) FROM recommendations.dataset_release"},
	}
	for _, q := range queries {
		var n int64
		if err := db.QueryRowContext(ctx, q.sql).Scan(&n); err != nil {
			fmt.Fprintf(os.Stderr, "%s: ERR %v\n", q.label, err)
			continue
		}
		fmt.Printf("%-20s %d\n", q.label, n)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT client_id, count(*) FROM recommendations.recommendation_runs
		GROUP BY client_id ORDER BY count(*) DESC LIMIT 5`)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	defer rows.Close()
	fmt.Println("runs by client:")
	for rows.Next() {
		var client string
		var n int64
		if err := rows.Scan(&client, &n); err == nil {
			fmt.Printf("  %-12s %d\n", client, n)
		}
	}
}
