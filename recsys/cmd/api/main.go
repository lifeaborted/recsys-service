package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"recsys/internal/auth"
	"recsys/internal/config"
	"recsys/internal/store/postgres"
	authroutes "recsys/internal/transport/http/routes/auth"
	"recsys/internal/transport/http/routes/compat"
	"recsys/internal/transport/http/routes/health"
	"recsys/internal/transport/http/routes/recommendations"
)

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	var store *postgres.Store
	if !cfg.Mock {
		var err error
		store, err = postgres.Open(cfg.DatabaseURL)
		if err != nil {
			logger.Error("failed to open database", "error", err)
			os.Exit(1)
		}
		store.SetModelVersion(cfg.ModelVersion)
		defer store.Close()
	}

	clientIDs := make(map[string]string, len(cfg.Users))
	for id, mapping := range cfg.Users {
		clientIDs[id] = mapping.ClientID
	}
	auth.Configure(clientIDs, cfg.FallbackClientID)

	recs := &recommendations.Handler{Store: store, ExamplesDir: cfg.ExamplesDir, Mock: cfg.Mock}
	legacy := &compat.Handler{
		Store: store, ExamplesDir: cfg.ExamplesDir, Mock: cfg.Mock, ModelVersion: cfg.ModelVersion,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", health.Get(store, cfg.Mock))
	mux.HandleFunc("POST /api/v1/auth/login", authroutes.Login)
	mux.HandleFunc("POST /api/v1/auth/register", authroutes.Register)
	mux.HandleFunc("GET /api/v1/users/me", auth.Require(authroutes.Me))

	// Новый контракт демо-экрана (Bearer + /me).
	mux.HandleFunc("GET /api/v1/recommendations/me", auth.Require(recs.Get))
	mux.HandleFunc("POST /api/v1/recommendations/me/new", auth.Require(recs.NewRun))

	// Совместимость со старым клиентом ЛК (/api/recommendations?user_id=...).
	mux.HandleFunc("GET /api/v1/recommendations", legacy.Recommendations)
	mux.HandleFunc("POST /api/v1/recommendations/new", legacy.NewRun)
	mux.HandleFunc("POST /api/v1/feedback", legacy.Feedback)
	mux.HandleFunc("GET /api/v1/debug/stats", legacy.Stats)
	mux.HandleFunc("GET /api/v1/debug/runs", legacy.Runs)

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           withCORS(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Info("starting recsys API", "addr", cfg.Addr, "mock", cfg.Mock)
	if err := server.ListenAndServe(); err != nil {
		logger.Error("HTTP server stopped", "error", err)
		os.Exit(1)
	}
}
