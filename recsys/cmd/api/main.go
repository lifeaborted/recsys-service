package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"recsys/internal/transport/http/routes/health"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	address := os.Getenv("RECSYS_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", health.Get)
	server := &http.Server{
		Addr:              address,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Info("starting API scaffold", "addr", address)
	if err := server.ListenAndServe(); err != nil {
		logger.Error("HTTP server stopped", "error", err)
		os.Exit(1)
	}
}
