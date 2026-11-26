// Команда применения SQL-миграций PostgreSQL.
//
//	go run ./cmd/migration create-db
//	go run ./cmd/migration up
//	go run ./cmd/migration status
//	go run ./cmd/migration seed
//	go run ./cmd/migration down
package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lib/pq"

	"recsys/internal/config"
	"recsys/internal/migrate"
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
	if len(args) > 0 {
		return fmt.Errorf("неизвестные аргументы: %s", strings.Join(args, " "))
	}

	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if err := ensureDatabase(ctx, cfg.DatabaseURL); err != nil {
		return err
	}

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("открыть БД: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("подключиться к БД: %w", err)
	}
	fmt.Fprintln(os.Stdout, "БД:", maskDSN(cfg.DatabaseURL))

	switch command {
	case "create-db":
		fmt.Fprintln(os.Stdout, "База готова.")
		return nil
	case "seed":
		path := filepath.Join("seeds", "seed_demo.sql")
		body, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("прочитать %s: %w", path, err)
		}
		if _, err := db.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("загрузить демо-данные: %w", err)
		}
		fmt.Fprintf(os.Stdout, "+ демо-данные загружены из %s\n", filepath.Base(path))
		return nil
	}

	m := migrate.New(db, "migrations", "schema_migrations", "recommendations")
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
		return fmt.Errorf("неизвестная команда %q (create-db, up, status, down, seed)", command)
	}
}

func ensureDatabase(ctx context.Context, targetDSN string) error {
	parsed, err := url.Parse(targetDSN)
	if err != nil {
		return err
	}
	maintenance := *parsed
	maintenance.Path = "/postgres"

	admin, err := sql.Open("postgres", maintenance.String())
	if err != nil {
		return err
	}
	defer admin.Close()
	if err := admin.PingContext(ctx); err != nil {
		return fmt.Errorf("подключиться к служебной БД: %w", err)
	}

	name := strings.TrimPrefix(parsed.Path, "/")
	var exists bool
	if err := admin.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+pq.QuoteIdentifier(name)); err != nil {
		return fmt.Errorf("создать БД %s: %w", name, err)
	}
	fmt.Fprintf(os.Stdout, "+ база %s создана\n", name)
	return nil
}

func maskDSN(dsn string) string {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	if parsed.User != nil {
		parsed.User = url.UserPassword(parsed.User.Username(), "***")
	}
	return parsed.String()
}
