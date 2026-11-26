package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

// Database — параметры подключения к PostgreSQL.
type Database struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	DBName   string `json:"dbname"`
	SSLMode  string `json:"sslmode"`
	// MaintenanceDB — база, через которую создаётся целевая БД.
	MaintenanceDB string `json:"maintenance_db"`
}

// UserMapping связывает External ID фронтенда с пользователем и клиентом БД.
type UserMapping struct {
	DBUserID int64  `json:"db_user_id"`
	ClientID string `json:"client_id"`
}

// APIConfig — настройки REST-сервиса рекомендаций.
type APIConfig struct {
	Listen           string                 `json:"listen"`
	ModelVersion     string                 `json:"model_version"`
	FallbackClientID string                 `json:"fallback_client_id"`
	Users            map[string]UserMapping `json:"users"`
}

// Config — конфигурация мигратора и API (файл config.json).
type Config struct {
	Database        Database  `json:"database"`
	Schema          string    `json:"schema"`
	MigrationsDir   string    `json:"migrations_dir"`
	MigrationsTable string    `json:"migrations_table"`
	API             APIConfig `json:"api"`
	basedir         string
}

// Load читает JSON и раскрывает ${ENV} в значениях (например, ${PGPASSWORD}).
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: прочитать %s: %w", path, err)
	}
	expanded := os.ExpandEnv(string(raw))

	cfg := &Config{}
	if err := json.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("config: разобрать %s: %w", path, err)
	}
	cfg.basedir = filepath.Dir(path)

	if cfg.Database.Host == "" {
		cfg.Database.Host = "localhost"
	}
	if cfg.Database.Port == 0 {
		cfg.Database.Port = 5432
	}
	if cfg.Database.SSLMode == "" {
		cfg.Database.SSLMode = "disable"
	}
	if cfg.Database.MaintenanceDB == "" {
		cfg.Database.MaintenanceDB = "postgres"
	}
	if cfg.Schema == "" {
		cfg.Schema = "recommendations"
	}
	if cfg.MigrationsDir == "" {
		cfg.MigrationsDir = "migrations"
	}
	if cfg.MigrationsTable == "" {
		cfg.MigrationsTable = "schema_migrations"
	}
	if cfg.API.Listen == "" {
		cfg.API.Listen = ":8000"
	}
	if cfg.API.ModelVersion == "" {
		cfg.API.ModelVersion = "go-baseline-1.0"
	}
	if cfg.API.FallbackClientID == "" {
		cfg.API.FallbackClientID = "C02417"
	}
	if cfg.API.Users == nil {
		cfg.API.Users = make(map[string]UserMapping)
	}
	if !filepath.IsAbs(cfg.MigrationsDir) {
		cfg.MigrationsDir = filepath.Join(cfg.basedir, cfg.MigrationsDir)
	}
	return cfg, nil
}

// SeedFile возвращает путь к демо-данным рядом с конфигом.
func (c *Config) SeedFile() string {
	return filepath.Join(c.basedir, "seeds", "seed_demo.sql")
}

// DSN строит URL для lib/pq с целевой базой.
func (c *Config) DSN() string {
	return c.DSNFor(c.Database.DBName)
}

// DSNFor строит URL для другой базы на том же сервере (например, postgres).
func (c *Config) DSNFor(dbname string) string {
	q := url.Values{}
	q.Set("sslmode", c.Database.SSLMode)
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?%s",
		url.QueryEscape(c.Database.User),
		url.QueryEscape(c.Database.Password),
		c.Database.Host,
		c.Database.Port,
		url.QueryEscape(dbname),
		q.Encode(),
	)
}
