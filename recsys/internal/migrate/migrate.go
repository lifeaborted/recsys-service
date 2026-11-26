package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lib/pq"
)

// Migration — одна SQL-миграция на диске.
type Migration struct {
	Name     string
	Path     string
	Checksum string
	Applied  bool
}

// Migrator применяет SQL-файлы по порядку и ведёт учёт в public.<table>.
type Migrator struct {
	db     *sql.DB
	dir    string
	table  string
	schema string
}

func New(db *sql.DB, dir, table, schema string) *Migrator {
	return &Migrator{db: db, dir: dir, table: table, schema: schema}
}

func (m *Migrator) metaTable() string {
	return "public." + pq.QuoteIdentifier(m.table)
}

// Init создаёт схему и таблицу учёта миграций.
func (m *Migrator) Init(ctx context.Context) error {
	_, err := m.db.ExecContext(ctx, fmt.Sprintf(`
		CREATE SCHEMA IF NOT EXISTS %s;
		CREATE TABLE IF NOT EXISTS %s (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
	`, pq.QuoteIdentifier(m.schema), m.metaTable()))
	if err != nil {
		return fmt.Errorf("создать таблицу миграций: %w", err)
	}
	return nil
}

// Files возвращает миграции в порядке имени файла.
func (m *Migrator) Files() ([]Migration, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, fmt.Errorf("прочитать каталог миграций %s: %w", m.dir, err)
	}

	files := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".sql") || strings.HasSuffix(name, ".down.sql") {
			continue
		}
		files = append(files, Migration{Name: name, Path: filepath.Join(m.dir, name)})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}

func checksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (m *Migrator) applied(ctx context.Context) (map[string]string, error) {
	rows, err := m.db.QueryContext(ctx, fmt.Sprintf("SELECT name, checksum FROM %s", m.metaTable()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var name, sum string
		if err := rows.Scan(&name, &sum); err != nil {
			return nil, err
		}
		result[name] = sum
	}
	return result, rows.Err()
}

// Status печатает, какие миграции уже применены.
func (m *Migrator) Status(ctx context.Context, out io.Writer) error {
	applied, err := m.applied(ctx)
	if err != nil {
		return err
	}
	files, err := m.Files()
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "%-28s %-10s %s\n", "МИГРАЦИЯ", "СТАТУС", "CHECKSUM")
	for _, file := range files {
		body, err := os.ReadFile(file.Path)
		if err != nil {
			return err
		}
		sum := checksum(body)
		state := "expected"
		if stored, ok := applied[storedName(file.Name)]; ok {
			state = "applied"
			if stored != sum {
				state = "CHANGED!"
			}
		}
		fmt.Fprintf(out, "%-28s %-10s %s\n", file.Name, state, sum[:12])
	}
	return nil
}

// storedName хранится без .sql — так проще переименовывать файлы.
func storedName(name string) string {
	return strings.TrimSuffix(name, ".sql")
}

// Up применяет все не применённые миграции.
func (m *Migrator) Up(ctx context.Context, out io.Writer) error {
	applied, err := m.applied(ctx)
	if err != nil {
		return err
	}
	files, err := m.Files()
	if err != nil {
		return err
	}

	for _, file := range files {
		body, err := os.ReadFile(file.Path)
		if err != nil {
			return err
		}
		sum := checksum(body)
		if stored, ok := applied[storedName(file.Name)]; ok {
			if stored != sum {
				fmt.Fprintf(out, "! %s изменена после применения ( хранится %s, сейчас %s )\n", file.Name, stored[:12], sum[:12])
			} else {
				fmt.Fprintf(out, "= %s уже применена\n", file.Name)
			}
			continue
		}

		tx, err := m.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("%s: начать транзакцию: %w", file.Name, err)
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("%s: выполнить SQL: %w", file.Name, err)
		}
		if _, err := tx.ExecContext(
			ctx,
			fmt.Sprintf("INSERT INTO %s (name, checksum) VALUES ($1, $2)", m.metaTable()),
			storedName(file.Name),
			sum,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("%s: записать в журнал: %w", file.Name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("%s: зафиксировать: %w", file.Name, err)
		}
		fmt.Fprintf(out, "+ %s применена (%s)\n", file.Name, sum[:12])
	}
	return nil
}

// Down откатывает миграции в обратном порядке, если есть .down.sql.
func (m *Migrator) Down(ctx context.Context, out io.Writer) error {
	applied, err := m.applied(ctx)
	if err != nil {
		return err
	}
	files, err := m.Files()
	if err != nil {
		return err
	}

	for i := len(files) - 1; i >= 0; i-- {
		file := files[i]
		if _, ok := applied[storedName(file.Name)]; !ok {
			continue
		}
		downPath := strings.TrimSuffix(file.Path, ".sql") + ".down.sql"
		body, err := os.ReadFile(downPath)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintf(out, "- %s: нет %s, пропуск\n", file.Name, filepath.Base(downPath))
				continue
			}
			return err
		}

		tx, err := m.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("%s: откат: %w", file.Name, err)
		}
		if _, err := tx.ExecContext(
			ctx,
			fmt.Sprintf("DELETE FROM %s WHERE name = $1", m.metaTable()),
			storedName(file.Name),
		); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		fmt.Fprintf(out, "- %s откачена\n", file.Name)
	}
	return nil
}
