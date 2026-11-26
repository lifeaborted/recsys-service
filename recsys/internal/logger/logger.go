package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

const redactedValue = "[REDACTED]"

type requestIDKey struct{}

// WithRequestID attaches a request correlation ID to ctx.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// RequestIDFromContext returns the request correlation ID stored in ctx, if any.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// New creates a structured JSON slog.Logger that scrubs secrets, tokens, passwords, and DSNs.
func New(w io.Writer, level slog.Level) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: redactAttr,
	})
	return slog.New(handler)
}

func redactAttr(_ []string, a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindGroup {
		attrs := a.Value.Group()
		cleaned := make([]slog.Attr, len(attrs))
		for i, child := range attrs {
			cleaned[i] = redactAttr(nil, child)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(cleaned...)}
	}
	if isSensitiveKey(a.Key) {
		return slog.String(a.Key, redactedValue)
	}
	return a
}

func isSensitiveKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	switch k {
	case "password", "password_hash", "passwd", "pass",
		"token", "access_token", "refresh_token", "session_token",
		"authorization", "cookie", "set-cookie",
		"secret", "client_secret", "api_key", "apikey",
		"llm_api_key", "embed_api_key",
		"dsn", "database_url", "recsys_database_url":
		return true
	}
	return strings.Contains(k, "password") ||
		strings.Contains(k, "secret") ||
		strings.Contains(k, "token") ||
		strings.Contains(k, "api_key")
}
