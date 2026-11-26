package config

import "os"

// Config — настройки стенда recommendation-агента.
type Config struct {
	Addr        string
	DatabaseURL string
	ExamplesDir string
	Mock        bool
}

// Load читает окружение. Все значения имеют разумные значения по умолчанию,
// чтобы демонстрация запускалась без дополнительной настройки.
func Load() Config {
	return Config{
		Addr:        env("RECSYS_ADDR", "127.0.0.1:8080"),
		DatabaseURL: env("RECSYS_DATABASE_URL", "postgres://postgres:postgres@localhost:5432/recsys?sslmode=disable"),
		ExamplesDir: env("RECSYS_EXAMPLES_DIR", "api/examples"),
		Mock:        os.Getenv("RECSYS_MOCK") == "1" || os.Getenv("RECSYS_MOCK") == "true",
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
