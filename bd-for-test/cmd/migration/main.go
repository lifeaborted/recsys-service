// Команда применения SQL-миграций PostgreSQL.
//
//	go run ./cmd/migration create-db   # создать БД, если её нет
//	go run ./cmd/migration seed        # загрузить минимальные демо-данные
//	go run ./cmd/migration status
//	go run ./cmd/migration up
//	go run ./cmd/migration down
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lib/pq" // драйвер PostgreSQL + QuoteIdentifier

	"recsys-migration/internal/config"
	"recsys-migration/internal/migrate"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "migration:", err)
		os.Exit(1)
	}
}

func run() error {
	command := "up"
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command = args[0]
		args = args[1:]
	}

	flags := flag.NewFlagSet("migration", flag.ExitOnError)
	configPath := flags.String("config", "config.json", "путь к config.json")
	if err := flags.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if err := ensureDatabase(ctx, cfg); err != nil {
		return err
	}

	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		return fmt.Errorf("открыть БД: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("подключиться к %s:%d/%s: %w",
			cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName, err)
	}
	fmt.Fprintf(os.Stdout, "БД: %s:%d/%s (схема %s)\n",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName, cfg.Schema)

	if command == "create-db" {
		fmt.Fprintf(os.Stdout, "База %s готова.\n", cfg.Database.DBName)
		return nil
	}
	if command == "seed" {
		path := cfg.SeedFile()
		body, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("прочитать %s: %w", path, err)
		}
		if _, err := db.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("загрузить демо-данные из %s: %w", filepath.Base(path), err)
		}
		fmt.Fprintf(os.Stdout, "+ демо-данные загружены из %s\n", filepath.Base(path))
		return nil
	}

	m := migrate.New(db, cfg.MigrationsDir, cfg.MigrationsTable, cfg.Schema)
	if err := m.Init(ctx); err != nil {
		return err
	}

	switch command {
	case "up":
		return m.Up(ctx, os.Stdout)
	case "status":
		return m.Status(ctx, os.Stdout)
	case "down":
		return m.Down(ctx, os.Stdout)
	default:
		return fmt.Errorf("неизвестная команда %q (доступно: create-db, up, status, down, seed)", command)
	}
}

// ensureDatabase подключается к maintenance DB и создаёт целевую базу,
// если её ещё нет. Это убирает ошибку «database ... does not exist» на
// первом запуске хакатон-стенда.
func ensureDatabase(ctx context.Context, cfg *config.Config) error {
	admin, err := sql.Open("postgres", cfg.DSNFor(cfg.Database.MaintenanceDB))
	if err != nil {
		return fmt.Errorf("открыть служебную БД: %w", err)
	}
	defer admin.Close()

	if err := admin.PingContext(ctx); err != nil {
		return fmt.Errorf("подключиться к %s:%d/%s: %w",
			cfg.Database.Host, cfg.Database.Port, cfg.Database.MaintenanceDB, err)
	}

	var exists bool
	if err := admin.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)",
		cfg.Database.DBName,
	).Scan(&exists); err != nil {
		return fmt.Errorf("проверить наличие БД: %w", err)
	}
	if exists {
		return nil
	}

	name := pq.QuoteIdentifier(cfg.Database.DBName)
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		return fmt.Errorf("создать БД %s: %w", cfg.Database.DBName, err)
	}
	fmt.Fprintf(os.Stdout, "+ база %s создана\n", cfg.Database.DBName)
	return nil
}
