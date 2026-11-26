package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"recsys/internal/auth"
	"recsys/internal/config"
	"recsys/internal/contract"
	"recsys/internal/domain"
	"recsys/internal/explain"
	"recsys/internal/logger"
	"recsys/internal/recommend"
	"recsys/internal/store/postgres"
	transporthttp "recsys/internal/transport/http"
)

type pgTransportAdapter struct {
	store *postgres.Store
}

func (a pgTransportAdapter) CheckHealth(ctx context.Context) (string, error) {
	return a.store.CheckHealth(ctx)
}

func (a pgTransportAdapter) ListProducts(ctx context.Context, limit int) ([]transporthttp.ProductDTO, error) {
	rows, err := a.store.ListProducts(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]transporthttp.ProductDTO, len(rows))
	for i, r := range rows {
		out[i] = transporthttp.ProductDTO{
			ItemID:       r.ItemID,
			OfferID:      r.OfferID,
			SKU:          r.SKU,
			Title:        r.Title,
			Category:     r.Category,
			Vendor:       r.Vendor,
			Price:        r.Price,
			CurrencyCode: r.CurrencyCode,
			Available:    r.Available,
			URL:          r.URL,
		}
	}
	return out, nil
}

func (a pgTransportAdapter) CreatePurchase(ctx context.Context, clientID domain.ClientID, rawItemID string, now time.Time) (transporthttp.PurchaseResult, error) {
	rec, err := a.store.CreatePurchase(ctx, clientID, rawItemID, now)
	if err != nil {
		if errors.Is(err, postgres.ErrItemNotFound) {
			return transporthttp.PurchaseResult{}, transporthttp.ErrUnknownItem
		}
		return transporthttp.PurchaseResult{}, err
	}
	return transporthttp.PurchaseResult{
		OrderID: rec.OrderID,
		ItemID:  rec.ItemID,
		OfferID: rec.OfferID,
	}, nil
}

type mockBackend struct {
	examplesDir string
}

func (m mockBackend) CheckHealth(_ context.Context) (string, error) { return "mock", nil }

func (m mockBackend) ListProducts(_ context.Context, _ int) ([]transporthttp.ProductDTO, error) {
	return []transporthttp.ProductDTO{}, nil
}

func (m mockBackend) CreatePurchase(_ context.Context, _ domain.ClientID, rawItemID string, _ time.Time) (transporthttp.PurchaseResult, error) {
	return transporthttp.PurchaseResult{OrderID: "MOCK-ORD-1", ItemID: domain.ItemID(rawItemID)}, nil
}

func (m mockBackend) Recommend(_ context.Context, q contract.RecommendationQuery) (contract.RecommendationResult, error) {
	file := "recommendations.full.json"
	if q.ClientID == "DEMO-NEW" || q.ClientID == "DEMO-B2C" {
		file = "recommendations.partial.json"
	}
	raw, err := os.ReadFile(filepath.Join(m.examplesDir, file))
	if err != nil {
		return contract.RecommendationResult{}, err
	}
	var resp contract.RecommendationResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return contract.RecommendationResult{}, err
	}
	return resp.RecommendationResult, nil
}

func main() {
	log := logger.New(os.Stdout, slog.LevelInfo)
	cfg := config.Load()

	openAPISpec, err := os.ReadFile(cfg.OpenAPIPath)
	if err != nil {
		log.Warn("could not read OpenAPI file; /swagger/openapi.yaml will return 404", "path", cfg.OpenAPIPath, "error", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var httpCfg transporthttp.Config
	httpCfg.Logger = log
	httpCfg.OpenAPISpec = openAPISpec
	httpCfg.RequestTimeout = cfg.RequestTimeout

	var store *postgres.Store
	if cfg.Mock {
		memRepo := auth.NewMemoryRepository()
		authSvc := auth.NewService(memRepo, auth.WithSessionTTL(cfg.SessionTTL), auth.WithBcryptCost(cfg.BcryptCost))
		_, _, _ = authSvc.Register(ctx, auth.RegisterInput{Login: "demo-b2b", Password: "demo-b2b-pass", ClientGroup: domain.B2B})
		_, _, _ = authSvc.Register(ctx, auth.RegisterInput{Login: "demo-b2c", Password: "demo-b2c-pass", ClientGroup: domain.B2C})
		mb := mockBackend{examplesDir: cfg.ExamplesDir}
		httpCfg.Auth = authSvc
		httpCfg.Health = mb
		httpCfg.Catalog = mb
		httpCfg.Purchases = mb
		httpCfg.Recommender = mb
	} else {
		openCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		store, err = postgres.OpenContext(openCtx, cfg.DatabaseURL)
		cancel()
		if err != nil {
			log.Error("failed to open PostgreSQL pool", "error", err)
			os.Exit(1)
		}
		defer store.Close()

		adapter := pgTransportAdapter{store: store}
		httpCfg.Auth = auth.NewService(store, auth.WithSessionTTL(cfg.SessionTTL), auth.WithBcryptCost(cfg.BcryptCost))
		httpCfg.Health = adapter
		httpCfg.Catalog = adapter
		httpCfg.Purchases = adapter
		// Constructor-injected contract.Recommender (single wiring point for final assembly).
		engine := recommend.New(store)
		if os.Getenv("LLM_BASE_URL") != "" && os.Getenv("LLM_MODEL") != "" {
			engine.UseReranker(explain.ChatFromEnv())
		}
		httpCfg.Recommender = engine
	}

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           transporthttp.NewServer(httpCfg),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      cfg.RequestTimeout + 5*time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("starting recsys HTTP server", "addr", cfg.Addr, "mock", cfg.Mock)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutCtx)
	case err := <-errCh:
		if err != nil {
			log.Error("HTTP server stopped with error", "error", err)
			os.Exit(1)
		}
	}
}
