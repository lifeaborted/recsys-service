package config

import (
	"bufio"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// UserMapping — соответствие профиля ЛК клиенту БД (было в config.json).
type UserMapping struct {
	ClientID string
}

// Config — настройки стенда recommendation-агента.
type Config struct {
	Addr             string
	DatabaseURL      string
	MaintenanceDB    string
	Schema           string
	MigrationsDir    string
	MigrationsTable  string
	ExamplesDir      string
	Mock             bool
	ModelVersion     string
	FallbackClientID string
	Users            map[string]UserMapping
}

// Load читает .env из рабочего каталога и окружение. Окружение имеет приоритет.
func Load() Config {
	loadDotEnv(".env")

	dsn := os.Getenv("RECSYS_DATABASE_URL")
	if dsn == "" {
		dsn = dsnFromPlainVars()
	}

	users := make(map[string]UserMapping)
	for _, id := range []string{"u1", "u2", "u3"} {
		key := "API_USER_" + strings.ToUpper(id) + "_CLIENT_ID"
		if clientID := os.Getenv(key); clientID != "" {
			users[id] = UserMapping{ClientID: clientID}
		}
	}
	if len(users) == 0 {
		users["u1"] = UserMapping{ClientID: "C02417"}
		users["u2"] = UserMapping{ClientID: "C02417"}
		users["u3"] = UserMapping{ClientID: "C02417"}
	}

	return Config{
		Addr:             env("RECSYS_ADDR", "127.0.0.1:8080"),
		DatabaseURL:      dsn,
		MaintenanceDB:    env("MAINTENANCE_DB", "postgres"),
		Schema:           env("SCHEMA", "recommendations"),
		MigrationsDir:    env("MIGRATIONS_DIR", "migrations"),
		MigrationsTable:  env("MIGRATIONS_TABLE", "schema_migrations"),
		ExamplesDir:      env("RECSYS_EXAMPLES_DIR", "api/examples"),
		Mock:             os.Getenv("RECSYS_MOCK") == "1" || os.Getenv("RECSYS_MOCK") == "true",
		ModelVersion:     env("API_MODEL_VERSION", "go-baseline-1.0"),
		FallbackClientID: env("API_FALLBACK_CLIENT_ID", "C02417"),
		Users:            users,
	}
}

// dsnFromPlainVars собирает DSN из HOST/PORT/USER/PASSWORD/DATADASE.
func dsnFromPlainVars() string {
	host := env("HOST", "localhost")
	port := env("PORT", "5432")
	user := env("USER", "postgres")
	password := os.Getenv("PASSWORD")
	dbname := os.Getenv("DATABASE")
	if dbname == "" {
		dbname = os.Getenv("DATADASE") // написание из присланного файла
	}
	if dbname == "" {
		dbname = "recsys"
	}
	if _, err := strconv.Atoi(port); err != nil {
		port = "5432"
	}

	q := url.Values{}
	q.Set("sslmode", env("SSLMODE", "disable"))
	return "postgres://" + url.QueryEscape(user) + ":" + url.QueryEscape(password) +
		"@" + host + ":" + port + "/" + url.QueryEscape(dbname) + "?" + q.Encode()
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// loadDotEnv — минимальный парсер KEY=VALUE (без внешних зависимостей).
func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if key != "" && os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
}
