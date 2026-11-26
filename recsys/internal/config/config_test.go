package config_test

import (
	"testing"
	"time"

	"recsys/internal/config"
)

func TestLoadFromEnv_PrioritizesDatabaseURLOverPlainVars(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL": "postgres://app:secret@db.internal:5432/prod?sslmode=require",
		"HOST":         "localhost",
		"PORT":         "5432",
		"USER":         "postgres",
		"PASSWORD":     "ignored",
		"DATADASE":     "ogo",
	}
	cfg := config.LoadFromLookup(func(k string) (string, bool) {
		v, ok := env[k]
		return v, ok
	})

	if cfg.DatabaseURL != "postgres://app:secret@db.internal:5432/prod?sslmode=require" {
		t.Fatalf("got DatabaseURL=%q, want DATABASE_URL value", cfg.DatabaseURL)
	}
}

func TestLoadFromEnv_BuildsDSNFromPlainVarsIncludingDatadaseTypo(t *testing.T) {
	env := map[string]string{
		"HOST":                   "127.0.0.1",
		"PORT":                   "5433",
		"USER":                   "pguser",
		"PASSWORD":               "p@ss:word",
		"DATADASE":               "ogo",
		"RECSYS_ADDR":            "0.0.0.0:9090",
		"RECSYS_REQUEST_TIMEOUT": "15s",
		"RECSYS_SESSION_TTL":     "2h",
	}
	cfg := config.LoadFromLookup(func(k string) (string, bool) {
		v, ok := env[k]
		return v, ok
	})

	wantDSN := "postgres://pguser:p%40ss%3Aword@127.0.0.1:5433/ogo?sslmode=disable"
	if cfg.DatabaseURL != wantDSN {
		t.Fatalf("got DatabaseURL=%q, want %q", cfg.DatabaseURL, wantDSN)
	}
	if cfg.Addr != "0.0.0.0:9090" {
		t.Fatalf("got Addr=%q, want 0.0.0.0:9090", cfg.Addr)
	}
	if cfg.RequestTimeout != 15*time.Second {
		t.Fatalf("got RequestTimeout=%v, want 15s", cfg.RequestTimeout)
	}
	if cfg.SessionTTL != 2*time.Hour {
		t.Fatalf("got SessionTTL=%v, want 2h", cfg.SessionTTL)
	}
}
