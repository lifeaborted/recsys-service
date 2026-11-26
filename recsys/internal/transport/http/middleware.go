package transporthttp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"recsys/internal/auth"
	"recsys/internal/logger"
	"recsys/internal/transport/http/respond"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

func (s *Server) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := sanitizeRequestID(r.Header.Get(respond.RequestIDHeader))
		if reqID == "" {
			reqID = generateRequestID()
		}
		w.Header().Set(respond.RequestIDHeader, reqID)
		ctx := logger.WithRequestID(r.Context(), reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withTimeout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		outcome := "ok"
		switch {
		case rec.status >= 500:
			outcome = "server_error"
		case rec.status >= 400:
			outcome = "client_error"
		}

		// Auth request bodies, Authorization headers, and session tokens are intentionally never logged.
		s.log.InfoContext(r.Context(), "http request completed",
			slog.String("request_id", respond.RequestID(rec, r)),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.String("outcome", outcome),
			slog.Int64("duration_ms", time.Since(started).Milliseconds()),
		)
	})
}

func (s *Server) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			respond.Error(w, r, http.StatusUnauthorized, "unauthorized", "Missing or invalid session")
			return
		}
		rawAuth := r.Header.Get("Authorization")
		if !strings.HasPrefix(rawAuth, "Bearer ") {
			respond.Error(w, r, http.StatusUnauthorized, "unauthorized", "Missing or invalid session")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(rawAuth, "Bearer "))
		if token == "" {
			respond.Error(w, r, http.StatusUnauthorized, "unauthorized", "Missing or invalid session")
			return
		}
		profile, err := s.auth.Authenticate(r.Context(), token)
		if err != nil {
			if errors.Is(err, auth.ErrUnauthorized) {
				respond.Error(w, r, http.StatusUnauthorized, "unauthorized", "Missing or invalid session")
				return
			}
			respond.Error(w, r, http.StatusInternalServerError, "auth_error", "Failed to verify session")
			return
		}
		next(w, r.WithContext(auth.WithProfile(r.Context(), profile)))
	}
}

// rejectProfileOverride rejects attempts to override or spoof the session profile via query parameters.
func (s *Server) rejectProfileOverride(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for _, forbidden := range []string{"profile_id", "client_id", "user_id"} {
			if q.Has(forbidden) {
				respond.Error(w, r, http.StatusBadRequest, "profile_override_forbidden", "Profile is determined strictly from the session")
				return
			}
		}
		next(w, r)
	}
}

func sanitizeRequestID(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || len(trimmed) > 128 {
		return ""
	}
	for _, r := range trimmed {
		if r < 0x21 || r > 0x7e {
			return ""
		}
	}
	return trimmed
}

func generateRequestID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "req-fallback"
	}
	return "req-" + hex.EncodeToString(buf)
}
