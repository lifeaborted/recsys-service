package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"recsys/internal/logger"
)

func TestLoggerRedactsSecretsAndPreservesRequestID(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, slog.LevelInfo)

	ctx := logger.WithRequestID(context.Background(), "req-abc-123")
	log.InfoContext(ctx, "auth attempt completed",
		slog.String("request_id", logger.RequestIDFromContext(ctx)),
		slog.String("outcome", "ok"),
		slog.String("login", "alice"),
		slog.String("password", "super-secret-password"),
		slog.String("access_token", "tok-secret-999"),
		slog.String("authorization", "Bearer tok-secret-999"),
		slog.String("database_url", "postgres://postgres:secret@localhost:5432/db"),
		slog.Group("nested",
			slog.String("api_key", "sk-live-secret"),
			slog.Int("status", 200),
		),
	)

	raw := buf.String()
	for _, forbidden := range []string{
		"super-secret-password",
		"tok-secret-999",
		"postgres:secret",
		"sk-live-secret",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("log output leaked secret %q: %s", forbidden, raw)
		}
	}

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON log line: %v (%s)", err, raw)
	}
	if parsed["request_id"] != "req-abc-123" {
		t.Fatalf("got request_id=%v, want req-abc-123", parsed["request_id"])
	}
	if parsed["outcome"] != "ok" {
		t.Fatalf("got outcome=%v, want ok", parsed["outcome"])
	}
	if parsed["password"] != "[REDACTED]" {
		t.Fatalf("got password=%v, want [REDACTED]", parsed["password"])
	}
}
