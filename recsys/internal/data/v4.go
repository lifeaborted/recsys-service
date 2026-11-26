package data

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// ResolveDatabaseURL reads the PostgreSQL DSN from RECSYS_DATABASE_URL or DATABASE_URL
// without falling back to hardcoded credentials.
func ResolveDatabaseURL() (string, error) {
	if dsn := strings.TrimSpace(os.Getenv("RECSYS_DATABASE_URL")); dsn != "" {
		return dsn, nil
	}
	if dsn := strings.TrimSpace(os.Getenv("DATABASE_URL")); dsn != "" {
		return dsn, nil
	}
	return "", fmt.Errorf("задайте RECSYS_DATABASE_URL (или DATABASE_URL) в окружении для подключения к PostgreSQL")
}

// MaskDSN replaces any password in a database URL before logging.
func MaskDSN(dsn string) string {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "<invalid-dsn>"
	}
	if parsed.User != nil {
		parsed.User = url.UserPassword(parsed.User.Username(), "***")
	}
	return parsed.String()
}
