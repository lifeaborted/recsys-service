// Импорт данных из pg_dump (COPY ... FROM stdin) без psql.
//
//	go run ./cmd/import -truncate
//	go run ./cmd/import -truncate -only=catalog_offers,clients -max-rows=5000
package main

import (
	"bufio"
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/lib/pq"

	"recsys-migration/internal/config"
)

var (
	copyHeader = regexp.MustCompile(`^COPY "recommendations"\."([^"]+)" \(([^)]+)\) FROM stdin;$`)
	columnName = regexp.MustCompile(`"([^"]+)"`)
)

func main() {
	configPath := flag.String("config", "config.json", "путь к config.json")
	dumpPath := flag.String("file", "recommendations.sql", "путь к pg_dump")
	truncate := flag.Bool("truncate", false, "очистить таблицы перед импортом")
	only := flag.String("only", "", "через запятую: какие таблицы импортировать")
	maxRows := flag.Int("max-rows", 0, "максимум строк на таблицу (0 — все)")
	flag.Parse()

	if err := run(*configPath, *dumpPath, *truncate, *only, *maxRows); err != nil {
		fmt.Fprintln(os.Stderr, "import:", err)
		os.Exit(1)
	}
}

func run(configPath, dumpPath string, truncate bool, only string, maxRows int) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	file, err := os.Open(dumpPath)
	if err != nil {
		return fmt.Errorf("открыть дамп %s: %w", dumpPath, err)
	}
	defer file.Close()

	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("подключиться к БД: %w", err)
	}

	selected := make(map[string]bool)
	if only != "" {
		for _, name := range strings.Split(only, ",") {
			selected[strings.TrimSpace(name)] = true
		}
	}

	if truncate {
		if err := truncateAll(ctx, db); err != nil {
			return err
		}
		fmt.Println("- таблицы очищены")
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 32*1024*1024)

	started := time.Now()
	total := int64(0)
	for scanner.Scan() {
		line := scanner.Text()
		match := copyHeader.FindStringSubmatch(line)
		if match == nil {
			continue // DDL, SET, комментарии — их применяет cmd/migration
		}

		table := match[1]
		if len(selected) > 0 && !selected[table] {
			if err := skipBlock(scanner); err != nil {
				return err
			}
			continue
		}

		columns := columnName.FindAllStringSubmatch(match[2], -1)
		names := make([]string, 0, len(columns))
		for _, col := range columns {
			names = append(names, col[1])
		}

		count, err := importBlock(ctx, db, table, names, scanner, maxRows)
		if err != nil {
			return err
		}
		total += count
		fmt.Printf("+ %-28s %d строк\n", table, count)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	fmt.Printf("Готово: %d строк за %s\n", total, time.Since(started).Round(time.Second))
	return nil
}

func truncateAll(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx,
		"SELECT tablename FROM pg_tables WHERE schemaname = 'recommendations'")
	if err != nil {
		return err
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		tables = append(tables, "recommendations."+pq.QuoteIdentifier(name))
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(tables) == 0 {
		return fmt.Errorf("схема recommendations пуста — сначала cmd/migration up")
	}
	_, err = db.ExecContext(ctx, "TRUNCATE TABLE "+strings.Join(tables, ", ")+" RESTART IDENTITY CASCADE")
	return err
}

func skipBlock(scanner *bufio.Scanner) error {
	for scanner.Scan() {
		if scanner.Text() == `\.` {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return fmt.Errorf("не найден конец COPY-блока")
}

func importBlock(
	ctx context.Context,
	db *sql.DB,
	table string,
	columns []string,
	scanner *bufio.Scanner,
	maxRows int,
) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}

	// В pg_dump данные идут не в топологическом порядке (например,
	// catalog_offers раньше catalog_snapshots), а FK уже созданы миграцией.
	// Временно отключаем проверку FK на этой сессии (нужны права суперпользователя).
	if _, err := tx.ExecContext(ctx, "SET LOCAL session_replication_role = replica"); err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("отключить FK: %w", err)
	}

	// CopyInSchema сам корректно экранирует схему и таблицу.
	stmt, err := tx.PrepareContext(ctx, pq.CopyInSchema("recommendations", table, columns...))
	if err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("%s: Prepare CopyIn: %w", table, err)
	}

	var count int64
	limitReached := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == `\.` {
			break
		}
		if limitReached || (maxRows > 0 && count >= int64(maxRows)) {
			limitReached = true
			continue
		}

		parts := strings.Split(line, "\t")
		values := make([]any, len(parts))
		for i, part := range parts {
			values[i] = decodeField(part)
		}
		if _, err := stmt.ExecContext(ctx, values...); err != nil {
			stmt.Close()
			_ = tx.Rollback()
			return count, fmt.Errorf("%s: строка %d: %w", table, count+1, err)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		stmt.Close()
		_ = tx.Rollback()
		return count, err
	}
	if _, err := stmt.ExecContext(ctx); err != nil {
		stmt.Close()
		_ = tx.Rollback()
		return count, fmt.Errorf("%s: закрыть COPY: %w", table, err)
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		return count, err
	}
	if err := tx.Commit(); err != nil {
		return count, fmt.Errorf("%s: commit: %w", table, err)
	}
	return count, nil
}

// decodeField разбирает текстовый формат COPY: \N = NULL, остальное — escape.
func decodeField(value string) any {
	if value == `\N` {
		return nil
	}
	if !strings.ContainsRune(value, '\\') {
		return value
	}

	var builder strings.Builder
	runes := []rune(value)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '\\' || i+1 >= len(runes) {
			builder.WriteRune(runes[i])
			continue
		}
		i++
		switch runes[i] {
		case 'n':
			builder.WriteRune('\n')
		case 'r':
			builder.WriteRune('\r')
		case 't':
			builder.WriteRune('\t')
		case 'b':
			builder.WriteRune('\b')
		case 'f':
			builder.WriteRune('\f')
		case 'v':
			builder.WriteRune('\v')
		default:
			builder.WriteRune(runes[i])
		}
	}
	return builder.String()
}
