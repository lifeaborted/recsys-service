// REST API рекомендаций для фронтенда (Vite проксирует /api на :8000).
//
//	go run ./cmd/api
//	curl http://localhost:8000/health
//	curl "http://localhost:8000/recommendations?user_id=u1&limit=9"
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"

	"recsys-migration/internal/api"
	"recsys-migration/internal/config"
	"recsys-migration/internal/store"
)

func main() {
	configPath := flag.String("config", "config.json", "путь к config.json")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "api:", err)
		os.Exit(1)
	}

	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		fmt.Fprintln(os.Stderr, "api: открыть БД:", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "api: подключиться к %s:%d/%s: %v\n",
			cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName, err)
		os.Exit(1)
	}

	// Если база пуста (например, её пересоздали), подгружаем демо-данные —
	// иначе рекомендации упадут по FK на clients / catalog_offers.
	var offers int
	if err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM recommendations.catalog_offers").Scan(&offers); err != nil {
		fmt.Fprintf(os.Stderr, "api: проверить каталог (запустите cmd/migration up): %v\n", err)
		os.Exit(1)
	}
	if offers == 0 {
		body, err := os.ReadFile(cfg.SeedFile())
		if err != nil {
			fmt.Fprintf(os.Stderr, "api: прочитать %s: %v\n", cfg.SeedFile(), err)
			os.Exit(1)
		}
		if _, err := db.ExecContext(ctx, string(body)); err != nil {
			fmt.Fprintf(os.Stderr, "api: загрузить демо-данные: %v\n", err)
			os.Exit(1)
		}
		log.Printf("+ демо-данные загружены автоматически (каталог пуст)")
	}

	handler := api.New(store.New(db, cfg), cfg).Routes()
	server := &http.Server{
		Addr:              cfg.API.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("Recsys API: http://localhost%s (модель %s)", cfg.API.Listen, cfg.API.ModelVersion)
	log.Printf("Прокси Vite: /api -> http://localhost%s", cfg.API.Listen)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, "api:", err)
		os.Exit(1)
	}
}
