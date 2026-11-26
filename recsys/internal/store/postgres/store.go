// Package postgres provides pgx/v5 adapters for the recommendations schema in PostgreSQL 17.
package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"recsys/internal/auth"
	"recsys/internal/contract"
)

const ExpectedSchemaVersion = "0004_snapshots_features_evidence"

var (
	ErrNotFound     = errors.New("record not found")
	ErrItemNotFound = errors.New("unknown item")
	ErrNotReady     = errors.New("database or schema not ready")
)

// Compile-time interface compliance checks.
var (
	_ contract.DatasetWriter  = (*Store)(nil)
	_ contract.FactsReader    = (*Store)(nil)
	_ contract.SnapshotWriter = (*Store)(nil)
	_ auth.Repository         = (*Store)(nil)
)

// Store wraps a pgxpool.Pool for transactional access to schema recommendations.
type Store struct {
	pool       *pgxpool.Pool
	ownsPool   bool
	minVersion string
}

// Option customizes a Store instance.
type Option func(*Store)

// WithExpectedSchemaVersion overrides the minimum migration name required by CheckHealth.
func WithExpectedSchemaVersion(version string) Option {
	return func(s *Store) {
		if strings.TrimSpace(version) != "" {
			s.minVersion = strings.TrimSpace(version)
		}
	}
}

// NewPool creates and configures a pgxpool.Pool.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("empty PostgreSQL DSN")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse PostgreSQL DSN: %w", err)
	}
	if cfg.MaxConns == 0 {
		cfg.MaxConns = 16
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	return pool, nil
}

// New constructs a Store over a caller-managed pgxpool.Pool.
func New(pool *pgxpool.Pool, opts ...Option) *Store {
	s := &Store{
		pool:       pool,
		ownsPool:   false,
		minVersion: ExpectedSchemaVersion,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// OpenContext opens a new pgxpool.Pool and returns a Store that owns the pool.
func OpenContext(ctx context.Context, dsn string, opts ...Option) (*Store, error) {
	pool, err := NewPool(ctx, dsn)
	if err != nil {
		return nil, err
	}
	s := New(pool, opts...)
	s.ownsPool = true
	return s, nil
}

// Open preserves compatibility with existing CLI callers (such as cmd/backtest).
func Open(dsn string) (*Store, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return OpenContext(ctx, dsn)
}

// Pool exposes the underlying pgxpool.Pool.
func (s *Store) Pool() *pgxpool.Pool {
	if s == nil {
		return nil
	}
	return s.pool
}

// Close releases the pool if owned by this Store.
func (s *Store) Close() error {
	if s != nil && s.pool != nil && s.ownsPool {
		s.pool.Close()
	}
	return nil
}

// Health verifies connectivity and expected schema version.
func (s *Store) Health(ctx context.Context) error {
	_, err := s.CheckHealth(ctx)
	return err
}

// CheckHealth verifies PostgreSQL connectivity, schema existence, and applied migration version.
func (s *Store) CheckHealth(ctx context.Context) (string, error) {
	if s == nil || s.pool == nil {
		return "", fmt.Errorf("%w: nil database pool", ErrNotReady)
	}
	var schemaExists, metaExists bool
	err := s.pool.QueryRow(ctx, `
		SELECT
			to_regnamespace('recommendations') IS NOT NULL,
			to_regclass('public.schema_migrations') IS NOT NULL
	`).Scan(&schemaExists, &metaExists)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotReady, err)
	}
	if !schemaExists || !metaExists {
		return "", fmt.Errorf("%w: schema or migrations table missing", ErrNotReady)
	}

	var latest string
	err = s.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(name), '')
		FROM public.schema_migrations
	`).Scan(&latest)
	if err != nil {
		return "", fmt.Errorf("%w: query schema_migrations: %v", ErrNotReady, err)
	}
	if latest == "" || latest < s.minVersion {
		return "", fmt.Errorf("%w: schema version %q is behind required %q", ErrNotReady, latest, s.minVersion)
	}
	return latest, nil
}

var numberedMigrationRe = regexp.MustCompile(`^[0-9]+_.+\.sql$`)

// Migrate applies numbered SQL migrations from dir in lexical order and returns the latest version.
func (s *Store) Migrate(ctx context.Context, dir string) (string, error) {
	if s == nil || s.pool == nil {
		return "", errors.New("nil store pool")
	}
	if _, err := s.pool.Exec(ctx, `
		CREATE SCHEMA IF NOT EXISTS recommendations;
		CREATE TABLE IF NOT EXISTS public.schema_migrations (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
	`); err != nil {
		return "", fmt.Errorf("init schema_migrations: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read migrations dir %s: %w", dir, err)
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !numberedMigrationRe.MatchString(name) || strings.HasSuffix(name, ".down.sql") {
			continue
		}
		files = append(files, name)
	}
	sort.Strings(files)
	if len(files) == 0 {
		return "", fmt.Errorf("no migrations found in %s", dir)
	}

	rows, err := s.pool.Query(ctx, `SELECT name, checksum FROM public.schema_migrations`)
	if err != nil {
		return "", fmt.Errorf("read applied migrations: %w", err)
	}
	applied := make(map[string]string)
	for rows.Next() {
		var name, sum string
		if err := rows.Scan(&name, &sum); err != nil {
			rows.Close()
			return "", err
		}
		applied[name] = sum
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", err
	}

	latest := ""
	for _, filename := range files {
		name := strings.TrimSuffix(filename, ".sql")
		body, err := os.ReadFile(filepath.Join(dir, filename))
		if err != nil {
			return "", fmt.Errorf("read migration %s: %w", filename, err)
		}
		sumBytes := sha256.Sum256(body)
		sum := hex.EncodeToString(sumBytes[:])

		if _, ok := applied[name]; ok {
			latest = name
			continue
		}

		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return "", fmt.Errorf("begin tx for %s: %w", filename, err)
		}
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			_ = tx.Rollback(ctx)
			return "", fmt.Errorf("apply %s: %w", filename, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO public.schema_migrations (name, checksum)
			VALUES ($1, $2)
		`, name, sum); err != nil {
			_ = tx.Rollback(ctx)
			return "", fmt.Errorf("record %s: %w", filename, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return "", fmt.Errorf("commit %s: %w", filename, err)
		}
		latest = name
	}
	return latest, nil
}
