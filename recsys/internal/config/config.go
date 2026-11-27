package config

import (
	"bufio"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Config holds runtime settings for the Go API and CLI tools.
type Config struct {
	GRPC            GRPCConfig
	Addr            string
	DatabaseURL     string
	RequestTimeout  time.Duration
	SessionTTL      time.Duration
	BcryptCost      int
	MigrationsDir   string
	MigrationsTable string
	MaintenanceDB   string
	Schema          string
	OpenAPIPath     string
	ExamplesDir     string
	Mock            bool
}

// GRPCConfig configures the recommendation service and its API client.
type GRPCConfig struct {
	ListenAddr string
	Target     string
	CertFile   string
	KeyFile    string
	CAFile     string
}

// LookupFunc resolves an environment variable by name.
type LookupFunc func(key string) (string, bool)

// Load reads environment variables (and optional RECSYS_ENV_FILE if specified).
func Load() Config {
	if envFile := strings.TrimSpace(os.Getenv("RECSYS_ENV_FILE")); envFile != "" {
		_ = LoadDotEnvFile(envFile)
	}
	return LoadFromLookup(os.LookupEnv)
}

// LoadFromLookup constructs Config deterministically from the supplied lookup function.
func LoadFromLookup(lookup LookupFunc) Config {
	get := func(key, fallback string) string {
		if v, ok := lookup(key); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return fallback
	}

	dsn := get("DATABASE_URL", "")
	if dsn == "" {
		dsn = get("RECSYS_DATABASE_URL", "")
	}
	if dsn == "" {
		dsn = buildDSNFromPlainVars(lookup)
	}

	reqTimeout := parseDuration(get("RECSYS_REQUEST_TIMEOUT", ""), 10*time.Second)
	sessionTTL := parseDuration(get("RECSYS_SESSION_TTL", ""), 24*time.Hour)

	cost := bcrypt.DefaultCost
	if rawCost := get("RECSYS_BCRYPT_COST", ""); rawCost != "" {
		if parsed, err := strconv.Atoi(rawCost); err == nil && parsed >= bcrypt.MinCost && parsed <= bcrypt.MaxCost {
			cost = parsed
		}
	}

	mockRaw := strings.ToLower(get("RECSYS_MOCK", ""))
	mock := mockRaw == "1" || mockRaw == "true" || mockRaw == "yes"

	return Config{
		GRPC: GRPCConfig{
			ListenAddr: get("RECSYS_GRPC_ADDR", "127.0.0.1:50051"),
			Target:     get("RECSYS_GRPC_TARGET", "127.0.0.1:50051"),
			CertFile:   get("RECSYS_GRPC_CERT_FILE", ""),
			KeyFile:    get("RECSYS_GRPC_KEY_FILE", ""),
			CAFile:     get("RECSYS_GRPC_CA_FILE", ""),
		},
		Addr:            get("RECSYS_ADDR", "127.0.0.1:8080"),
		DatabaseURL:     dsn,
		RequestTimeout:  reqTimeout,
		SessionTTL:      sessionTTL,
		BcryptCost:      cost,
		MigrationsDir:   get("RECSYS_MIGRATIONS_DIR", "migrations"),
		MigrationsTable: get("RECSYS_MIGRATIONS_TABLE", "schema_migrations"),
		MaintenanceDB:   get("RECSYS_MAINTENANCE_DB", "postgres"),
		Schema:          get("RECSYS_SCHEMA", "recommendations"),
		OpenAPIPath:     get("RECSYS_OPENAPI_PATH", "api/openapi.yaml"),
		ExamplesDir:     get("RECSYS_EXAMPLES_DIR", "api/examples"),
		Mock:            mock,
	}
}

func buildDSNFromPlainVars(lookup LookupFunc) string {
	hasAny := false
	for _, key := range []string{"HOST", "PORT", "USER", "PASSWORD", "DATABASE", "DATADASE", "SSLMODE"} {
		if v, ok := lookup(key); ok && strings.TrimSpace(v) != "" {
			hasAny = true
			break
		}
	}
	if !hasAny {
		return "postgres://postgres:postgres@localhost:5432/recsys?sslmode=disable"
	}

	get := func(key, fallback string) string {
		if v, ok := lookup(key); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return fallback
	}

	host := get("HOST", "localhost")
	port := get("PORT", "5432")
	if _, err := strconv.Atoi(port); err != nil {
		port = "5432"
	}
	user := get("USER", "postgres")
	password, _ := lookup("PASSWORD")
	password = strings.TrimSpace(password)

	dbName := get("DATABASE", "")
	if dbName == "" {
		dbName = get("DATADASE", "recsys")
	}
	sslMode := get("SSLMODE", "disable")

	u := &url.URL{
		Scheme: "postgres",
		Host:   host + ":" + port,
		Path:   "/" + dbName,
	}
	if password != "" {
		u.User = url.UserPassword(user, password)
	} else {
		u.User = url.User(user)
	}
	q := url.Values{}
	q.Set("sslmode", sslMode)
	u.RawQuery = q.Encode()
	return u.String()
}

func parseDuration(raw string, fallback time.Duration) time.Duration {
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

// LoadDotEnvFile loads KEY=VALUE pairs into process env without overriding already-set variables.
func LoadDotEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if k == "" {
			continue
		}
		if _, exists := os.LookupEnv(k); !exists {
			_ = os.Setenv(k, v)
		}
	}
	return scanner.Err()
}
