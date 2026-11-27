// Package transporthttp implements the net/http server, middleware, and single-route handlers for /api/v1.
package transporthttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"recsys/internal/auth"
	"recsys/internal/contract"
	"recsys/internal/domain"
	"recsys/internal/logger"
	"recsys/internal/transport/http/respond"
)

var ErrUnknownItem = errors.New("unknown catalog or historical item")

// HealthChecker verifies database connectivity and schema migration readiness.
type HealthChecker interface {
	CheckHealth(ctx context.Context) (schemaVersion string, err error)
}

// ProductDTO represents a catalog product returned by GET /api/v1/products.
type ProductDTO struct {
	ItemID       domain.ItemID `json:"item_id"`
	OfferID      *string       `json:"offer_id,omitempty"`
	SKU          *string       `json:"sku"`
	Title        string        `json:"title"`
	Category     string        `json:"category"`
	Vendor       string        `json:"vendor"`
	Price        *float64      `json:"price,omitempty"`
	CurrencyCode *string       `json:"currency_code,omitempty"`
	Available    bool          `json:"available"`
	URL          *string       `json:"url,omitempty"`
}

// CatalogMetaDTO carries optional catalog feed version and update timestamps.
type CatalogMetaDTO struct {
	CatalogVersion    *string
	FeedGeneratedAt   *time.Time
	CatalogImportedAt *time.Time
}

// CatalogMetaReader optionally supplies catalog feed version and update date for /health and /products.
type CatalogMetaReader interface {
	CatalogMeta(ctx context.Context) (CatalogMetaDTO, error)
}

// CatalogStore reads storefront products for GET /api/v1/products.
type CatalogStore interface {
	ListProducts(ctx context.Context, limit int) ([]ProductDTO, error)
}

// PurchaseResult describes a persisted profile purchase.
type PurchaseResult struct {
	OrderID string        `json:"order_id"`
	ItemID  domain.ItemID `json:"item_id"`
	OfferID *string       `json:"offer_id,omitempty"`
}

// PurchaseStore records online purchases for the authenticated profile.
type PurchaseStore interface {
	CreatePurchase(ctx context.Context, clientID domain.ClientID, rawItemID string, now time.Time) (PurchaseResult, error)
}

// Config wires dependencies into the HTTP server via constructor injection.
type Config struct {
	Auth           *auth.Service
	Health         HealthChecker
	Catalog        CatalogStore
	CatalogMeta    CatalogMetaReader
	Purchases      PurchaseStore
	Recommender    contract.Recommender
	Logger         *slog.Logger
	OpenAPISpec    []byte
	RequestTimeout time.Duration
}

// Server dispatches OpenAPI routes under /api/v1 and Swagger UI under /swagger/.
type Server struct {
	auth        *auth.Service
	health      HealthChecker
	catalog     CatalogStore
	catalogMeta CatalogMetaReader
	purchases   PurchaseStore
	recommender contract.Recommender
	log         *slog.Logger
	openAPISpec []byte
	timeout     time.Duration
	handler     http.Handler
}

// NewServer constructs an http.Handler implementing all 7 OpenAPI operations and /swagger/.
func NewServer(cfg Config) *Server {
	log := cfg.Logger
	if log == nil {
		log = logger.New(io.Discard, slog.LevelInfo)
	}
	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	s := &Server{
		auth:        cfg.Auth,
		health:      cfg.Health,
		catalog:     cfg.Catalog,
		catalogMeta: cfg.CatalogMeta,
		purchases:   cfg.Purchases,
		recommender: cfg.Recommender,
		log:         log,
		openAPISpec: append([]byte(nil), cfg.OpenAPISpec...),
		timeout:     timeout,
	}

	mux := http.NewServeMux()

	// Seven OpenAPI operations under /api/v1 (one route per file).
	mux.HandleFunc("GET /api/v1/health", s.handleGetHealth)
	mux.HandleFunc("POST /api/v1/auth/register", s.handlePostAuthRegister)
	mux.HandleFunc("POST /api/v1/auth/login", s.handlePostAuthLogin)
	mux.HandleFunc("GET /api/v1/users/me", s.requireSession(s.rejectProfileOverride(s.handleGetUsersMe)))
	mux.HandleFunc("GET /api/v1/products", s.requireSession(s.rejectProfileOverride(s.handleGetProducts)))
	mux.HandleFunc("POST /api/v1/purchases", s.requireSession(s.rejectProfileOverride(s.handlePostPurchases)))
	mux.HandleFunc("GET /api/v1/recommendations/me", s.requireSession(s.rejectProfileOverride(s.handleGetRecommendationsMe)))

	// Swagger UI & OpenAPI specification served from the same OpenAPI document.
	mux.HandleFunc("GET /swagger", s.handleGetSwagger)
	mux.HandleFunc("GET /swagger/", s.handleGetSwagger)
	mux.HandleFunc("GET /swagger/openapi.yaml", s.handleGetSwaggerSpec)

	// Fallback handler so unknown routes also return JSON Error with X-Request-ID.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		respond.Error(w, r, http.StatusNotFound, "not_found", "Route not found")
	})

	var h http.Handler = mux
	h = s.withTimeout(h)
	h = s.withAccessLog(h)
	h = s.withCORS(h)
	h = s.withRequestID(h)
	s.handler = h
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// decodeStrictJSON decodes a bounded JSON request body with DisallowUnknownFields and rejects trailing data.
func decodeStrictJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return errors.New("request body is required")
	}
	limited := io.LimitReader(r.Body, 1<<20)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return errors.New("request body is empty")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("unexpected trailing JSON content")
	}
	return nil
}
