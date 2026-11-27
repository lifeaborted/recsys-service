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
	"sync"
	"syscall"
	"time"

	"recsys/internal/auth"
	"recsys/internal/config"
	"recsys/internal/contract"
	"recsys/internal/domain"
	"recsys/internal/logger"
	"recsys/internal/store/postgres"
	transporthttp "recsys/internal/transport/http"
	"recsys/internal/transport/recommendgrpc"

	"google.golang.org/grpc"
)

type pgTransportAdapter struct {
	store *postgres.Store
}

func (a pgTransportAdapter) CheckHealth(ctx context.Context) (string, error) {
	return a.store.CheckHealth(ctx)
}

func (a pgTransportAdapter) CatalogMeta(ctx context.Context) (transporthttp.CatalogMetaDTO, error) {
	meta, err := a.store.CatalogMeta(ctx)
	if err != nil {
		return transporthttp.CatalogMetaDTO{}, err
	}
	var out transporthttp.CatalogMetaDTO
	if meta.CatalogVersion != "" {
		ver := meta.CatalogVersion
		out.CatalogVersion = &ver
	}
	out.FeedGeneratedAt = meta.FeedGeneratedAt
	out.CatalogImportedAt = meta.ImportedAt
	return out, nil
}

func (a pgTransportAdapter) ListProducts(ctx context.Context, limit int) ([]transporthttp.ProductDTO, error) {
	rows, err := a.store.ListProducts(ctx, limit)
	if err != nil {
		return nil, err
	}
	return toProductDTOs(rows), nil
}

func (a pgTransportAdapter) ListProductsPage(ctx context.Context, limit, offset int, query string, withHistory bool) ([]transporthttp.ProductDTO, int64, error) {
	rows, total, err := a.store.ListProductsPage(ctx, limit, offset, query, withHistory)
	if err != nil {
		return nil, 0, err
	}
	return toProductDTOs(rows), total, nil
}

func toProductDTOs(rows []postgres.CatalogProduct) []transporthttp.ProductDTO {
	out := make([]transporthttp.ProductDTO, len(rows))
	for i, r := range rows {
		out[i] = transporthttp.ProductDTO{
			ItemID: r.ItemID, OfferID: r.OfferID, SKU: r.SKU, Title: r.Title,
			Category: r.Category, Vendor: r.Vendor, Price: r.Price,
			CurrencyCode: r.CurrencyCode, Available: r.Available, HasHistory: r.HasHistory, URL: r.URL,
		}
	}
	return out
}

func (a pgTransportAdapter) ProductDetail(ctx context.Context, itemID string) (contract.ProductDetail, error) {
	return a.store.ProductDetail(ctx, itemID)
}

func (a pgTransportAdapter) ScoreHistory(ctx context.Context, itemID string) (contract.ScoreHistory, error) {
	return a.store.ScoreHistory(ctx, itemID)
}

func (a pgTransportAdapter) TableStats(ctx context.Context) (map[string]int64, error) {
	return a.store.TableStats(ctx)
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

func (a pgTransportAdapter) LatestPreviewSession(ctx context.Context, template domain.ClientID) (contract.PreviewSession, error) {
	return a.store.LatestPreviewSession(ctx, template)
}

func (a pgTransportAdapter) ListPurchases(ctx context.Context, clientID domain.ClientID) ([]contract.PurchaseHistoryItem, error) {
	return a.store.ListPurchases(ctx, clientID)
}

func (m mockBackend) ListPurchases(_ context.Context, _ domain.ClientID) ([]contract.PurchaseHistoryItem, error) {
	return []contract.PurchaseHistoryItem{}, nil
}

type mockRunsStore struct {
	mu     sync.Mutex
	nextID int64
	runs   []contract.RunRecord
}

func (m *mockRunsStore) CreateRun(_ context.Context, clientID domain.ClientID, modelVersion string, items []contract.RunItemInput) (contract.RunRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	run := contract.RunRecord{
		RunID: m.nextID, ClientID: string(clientID), ModelVersion: modelVersion,
		RequestedAt: time.Now(), Items: []contract.RunItemRecord{},
	}
	for i, item := range items {
		if i >= 3 {
			break
		}
		run.Items = append(run.Items, contract.RunItemRecord{
			ItemID: item.ItemID, OfferID: item.OfferID, Explanation: item.Explanation, Score: item.Score,
		})
	}
	m.runs = append([]contract.RunRecord{run}, m.runs...)
	return run, nil
}

func (m *mockRunsStore) ListRuns(_ context.Context, clientID domain.ClientID, limit int) ([]contract.RunRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := []contract.RunRecord{}
	for _, run := range m.runs {
		if run.ClientID == string(clientID) && len(result) < limit {
			result = append(result, run)
		}
	}
	return result, nil
}

func (m *mockRunsStore) CreateEvent(_ context.Context, clientID domain.ClientID, runID int64, offerID string, eventType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.runs {
		if m.runs[i].RunID != runID || m.runs[i].ClientID != string(clientID) {
			continue
		}
		for _, item := range m.runs[i].Items {
			if item.OfferID == offerID {
				if eventType == "click" {
					m.runs[i].Clicks++
				} else {
					m.runs[i].Purchases++
				}
				return nil
			}
		}
	}
	return contract.ErrRunNotFound
}

type mockBackend struct {
	examplesDir string
}

func (m mockBackend) CheckHealth(_ context.Context) (string, error) { return "mock", nil }

func (m mockBackend) CatalogMeta(_ context.Context) (transporthttp.CatalogMetaDTO, error) {
	ver := "demo-v1"
	genAt := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	return transporthttp.CatalogMetaDTO{
		CatalogVersion:  &ver,
		FeedGeneratedAt: &genAt,
	}, nil
}

func (m mockBackend) ListProducts(_ context.Context, _ int) ([]transporthttp.ProductDTO, error) {
	return []transporthttp.ProductDTO{}, nil
}

func (m mockBackend) ListProductsPage(ctx context.Context, limit, offset int, query string, _ bool) ([]transporthttp.ProductDTO, int64, error) {
	items, err := m.ListProducts(ctx, limit)
	return items, int64(len(items)), err
}

func (m mockBackend) ProductDetail(_ context.Context, itemID string) (contract.ProductDetail, error) {
	return contract.ProductDetail{
		ItemID: itemID, OfferID: "mock-" + itemID, Title: itemID,
		Category: "Синтетический пример", Vendor: "Синтетика",
		Available: true, CurrencyCode: "RUB", Parameters: []contract.Parameter{},
	}, nil
}

func (m mockBackend) ScoreHistory(_ context.Context, itemID string) (contract.ScoreHistory, error) {
	return contract.ScoreHistory{ItemID: itemID, Average: 0.75, Points: []contract.ScorePoint{}}, nil
}

func (m mockBackend) TableStats(_ context.Context) (map[string]int64, error) {
	return map[string]int64{
		"clients": 2, "catalog_offers": 8, "sales": 4,
		"online_purchases": 0, "recommendation_runs": 0, "recommendation_events": 0,
	}, nil
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

func buildMockServer(ctx context.Context, cfg config.Config, log *slog.Logger) (*transporthttp.Server, error) {
	openAPISpec, _ := os.ReadFile(cfg.OpenAPIPath)
	memRepo := auth.NewMemoryRepository()
	authSvc := auth.NewService(memRepo, auth.WithSessionTTL(cfg.SessionTTL), auth.WithBcryptCost(cfg.BcryptCost))
	_, _, _ = authSvc.Register(ctx, auth.RegisterInput{Login: "demo-b2b", Password: "demo-b2b-pass", ClientGroup: domain.B2B})
	_, _, _ = authSvc.Register(ctx, auth.RegisterInput{Login: "demo-b2c", Password: "demo-b2c-pass", ClientGroup: domain.B2C})
	mb := mockBackend{examplesDir: cfg.ExamplesDir}
	return transporthttp.NewServer(transporthttp.Config{
		Auth:           authSvc,
		Health:         mb,
		Catalog:        mb,
		CatalogMeta:    mb,
		ProductDetails: mb,
		ScoreHistory:   mb,
		Stats:          mb,
		Runs:           &mockRunsStore{},
		Purchases:      mb,
		Recommender:    mb,
		Logger:         log,
		OpenAPISpec:    openAPISpec,
		RequestTimeout: cfg.RequestTimeout,
	}), nil
}

func main() {
	log := logger.New(os.Stdout, slog.LevelInfo)
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var handler http.Handler
	var store *postgres.Store
	if cfg.Mock {
		srv, err := buildMockServer(ctx, cfg, log)
		if err != nil {
			log.Error("failed to initialize mock server", "error", err)
			os.Exit(1)
		}
		handler = srv
	} else {
		openAPISpec, err := os.ReadFile(cfg.OpenAPIPath)
		if err != nil {
			log.Warn("could not read OpenAPI file; /swagger/openapi.yaml will return 404", "path", cfg.OpenAPIPath, "error", err)
		}

		openCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		store, err = postgres.OpenContext(openCtx, cfg.DatabaseURL, postgres.WithOnlineSources(os.Getenv("RECSYS_ONLINE_SNAPSHOT"), ""), postgres.WithClosedPreview(os.Getenv("RECSYS_CLOSED_PREVIEW") == "1"))
		cancel()
		if err != nil {
			log.Error("failed to open PostgreSQL pool", "error", err)
			os.Exit(1)
		}
		defer store.Close()

		adapter := pgTransportAdapter{store: store}
		creds, err := recommendgrpc.ClientCredentials(cfg.GRPC)
		if err != nil {
			log.Error("invalid recommendation gRPC credentials", "error", err)
			os.Exit(1)
		}
		conn, err := grpc.NewClient(cfg.GRPC.Target, grpc.WithTransportCredentials(creds))
		if err != nil {
			log.Error("failed to configure recommendation gRPC client", "error", err)
			os.Exit(1)
		}
		defer conn.Close()
		handler = transporthttp.NewServer(transporthttp.Config{
			Auth:           auth.NewService(store, auth.WithSessionTTL(cfg.SessionTTL), auth.WithBcryptCost(cfg.BcryptCost)),
			Health:         adapter,
			Catalog:        adapter,
			CatalogMeta:    adapter,
			ProductDetails: adapter,
			ScoreHistory:   adapter,
			Stats:          adapter,
			Runs:           store,
			Purchases:      adapter,
			Recommender:    recommendgrpc.NewClient(conn),
			Logger:         log,
			OpenAPISpec:    openAPISpec,
			RequestTimeout: cfg.RequestTimeout,
		})
	}

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
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
