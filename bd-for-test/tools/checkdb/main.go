package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"

	"recsys-migration/internal/config"
)

func main() {
	dir := "."
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	cfg, err := config.Load(dir + "/config.json")
	if err != nil {
		fmt.Println(err)
		return
	}
	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		fmt.Println(err)
		return
	}
	defer db.Close()

	ctx := context.Background()
	var clients, offers, runs int
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM recommendations.clients").Scan(&clients)
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM recommendations.catalog_offers").Scan(&offers)
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM recommendations.recommendation_runs").Scan(&runs)
	fmt.Printf("clients=%d offers=%d runs=%d\n", clients, offers, runs)

	for _, id := range []string{"C02417", "C00001", "C12808"} {
		var ok bool
		_ = db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM recommendations.clients WHERE client_id=$1)", id).Scan(&ok)
		fmt.Printf("client %s exists=%v\n", id, ok)
	}
	rows, _ := db.QueryContext(ctx, "SELECT run_id, client_id FROM recommendations.recommendation_runs ORDER BY run_id DESC LIMIT 5")
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var id int64
			var client string
			_ = rows.Scan(&id, &client)
			fmt.Printf("run %d client=%s\n", id, client)
		}
	}
}
