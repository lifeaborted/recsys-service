package transporthttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"recsys/internal/auth"
	"recsys/internal/contract"
	"recsys/internal/domain"
	"recsys/internal/logger"
	transporthttp "recsys/internal/transport/http"
)

type memoryPlatformBackend struct {
	mu         sync.Mutex
	healthy    bool
	schemaVer  string
	products   []transporthttp.ProductDTO
	canonical  map[string]domain.ItemID
	purchases  map[domain.ClientID][]domain.ItemID
	recQueries []contract.RecommendationQuery
}

func newMemoryPlatformBackend() *memoryPlatformBackend {
	offerBase := "demo-offer-base"
	offerRenew12 := "demo-offer-renew-12"
	skuBase := "DEMO-BASE"
	skuRenew := "DEMO-RENEW-12"
	priceBase := 10000.0
	priceRenew := 8000.0
	currency := "RUB"
	urlBase := "https://store.softline.ru/demo/base"
	urlRenew := "https://store.softline.ru/demo/renew-12"

	return &memoryPlatformBackend{
		healthy:   true,
		schemaVer: "0004_snapshots_features_evidence",
		products: []transporthttp.ProductDTO{
			{
				ItemID:       "DEMO-BASE",
				OfferID:      &offerBase,
				SKU:          &skuBase,
				Title:        "Базовая корпоративная лицензия",
				Category:     "Безопасность",
				Vendor:       "DemoVendor",
				Price:        &priceBase,
				CurrencyCode: &currency,
				Available:    true,
				URL:          &urlBase,
			},
			{
				ItemID:       "DEMO-RENEW-12",
				OfferID:      &offerRenew12,
				SKU:          &skuRenew,
				Title:        "Продление на 12 месяцев",
				Category:     "Безопасность",
				Vendor:       "DemoVendor",
				Price:        &priceRenew,
				CurrencyCode: &currency,
				Available:    true,
				URL:          &urlRenew,
			},
		},
		canonical: map[string]domain.ItemID{
			"DEMO-BASE":           "DEMO-BASE",
			"demo-offer-base":     "DEMO-BASE",
			"DEMO-RENEW-12":       "DEMO-RENEW-12",
			"demo-offer-renew-12": "DEMO-RENEW-12",
		},
		purchases: make(map[domain.ClientID][]domain.ItemID),
	}
}

func (m *memoryPlatformBackend) CheckHealth(_ context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.healthy {
		return "", errors.New("schema mismatch or db down")
	}
	return m.schemaVer, nil
}

func (m *memoryPlatformBackend) ListProducts(_ context.Context, limit int) ([]transporthttp.ProductDTO, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit > len(m.products) {
		limit = len(m.products)
	}
	out := make([]transporthttp.ProductDTO, limit)
	copy(out, m.products[:limit])
	return out, nil
}

func (m *memoryPlatformBackend) CreatePurchase(_ context.Context, clientID domain.ClientID, rawItemID string, _ time.Time) (transporthttp.PurchaseResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	canon, ok := m.canonical[rawItemID]
	if !ok {
		return transporthttp.PurchaseResult{}, transporthttp.ErrUnknownItem
	}
	m.purchases[clientID] = append(m.purchases[clientID], canon)
	return transporthttp.PurchaseResult{
		OrderID: "ORD-" + string(clientID) + "-" + string(canon),
		ItemID:  canon,
	}, nil
}

func (m *memoryPlatformBackend) Recommend(_ context.Context, q contract.RecommendationQuery) (contract.RecommendationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recQueries = append(m.recQueries, q)

	bought := make(map[domain.ItemID]bool)
	for _, id := range m.purchases[q.ClientID] {
		bought[id] = true
	}

	if bought["DEMO-BASE"] && !bought["DEMO-RENEW-12"] {
		offerID := "demo-offer-renew-12"
		sku := "DEMO-RENEW"
		family := "DEMO-SUITE"
		url12 := "https://store.softline.ru/demo/renew-12"
		url24 := "https://store.softline.ru/demo/renew-24"
		partial := "insufficient_eligible_products"
		return contract.RecommendationResult{
			SnapshotID:    "demo-v1",
			PartialReason: &partial,
			Items: []domain.Recommendation{{
				ItemID:      "DEMO-RENEW-12",
				OfferID:     &offerID,
				SKU:         &sku,
				Kind:        domain.Renewal,
				FamilyID:    &family,
				Title:       "Продление на 12 месяцев",
				URL:         &url12,
				Explanation: "Для купленной редакции доступно продление на 12 или 24 месяца.",
				Variants: []domain.Variant{
					{
						ItemID: "DEMO-RENEW-12",
						URL:    &url12,
						Differences: []domain.Difference{
							{Field: "term", Value: 12, Unit: "month", Source: "synthetic-offer-12"},
						},
					},
					{
						ItemID: "DEMO-RENEW-24",
						URL:    &url24,
						Differences: []domain.Difference{
							{Field: "term", Value: 24, Unit: "month", Source: "synthetic-offer-24"},
						},
					},
				},
				EvidenceIDs: []string{"demo-renewal"},
			}},
			Graph: domain.Graph{
				Nodes: []domain.Node{
					{ID: "item:DEMO-BASE", Type: "product", Label: "Базовая корпоративная лицензия"},
					{ID: "item:DEMO-RENEW-12", Type: "product", Label: "Продление на 12 месяцев"},
				},
				Edges: []domain.Evidence{{
					ID:         "demo-renewal",
					From:       "item:DEMO-BASE",
					To:         "item:DEMO-RENEW-12",
					Type:       "renewal",
					Weight:     1.0,
					Reason:     "Совместимая редакция в синтетическом наборе.",
					Source:     "synthetic-rule-1",
					SnapshotID: "demo-v1",
				}},
			},
		}, nil
	}

	// Empty slices deliberately passed as nil to verify JSON normalization to [] and null.
	return contract.RecommendationResult{
		SnapshotID:    "demo-v1",
		Items:         nil,
		Graph:         domain.Graph{},
		PartialReason: nil,
	}, nil
}

func TestPlatformModule(t *testing.T) {
	now := time.Date(2026, 11, 26, 17, 0, 0, 0, time.UTC)
	authRepo := auth.NewMemoryRepository()
	authSvc := auth.NewService(authRepo,
		auth.WithClock(func() time.Time { return now }),
		auth.WithSessionTTL(1*time.Hour),
		auth.WithBcryptCost(bcrypt.MinCost),
	)
	backend := newMemoryPlatformBackend()

	var logBuf bytes.Buffer
	log := logger.New(&logBuf, slog.LevelInfo)

	openAPIPath := filepath.Join("..", "..", "..", "api", "openapi.yaml")
	openAPIBytes, err := os.ReadFile(openAPIPath)
	if err != nil {
		t.Fatalf("read openapi.yaml: %v", err)
	}

	srv := transporthttp.NewServer(transporthttp.Config{
		Auth:           authSvc,
		Health:         backend,
		Catalog:        backend,
		Purchases:      backend,
		Recommender:    backend,
		Logger:         log,
		OpenAPISpec:    openAPIBytes,
		RequestTimeout: 5 * time.Second,
	})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	doJSON := func(method, path string, token string, reqID string, body string) (*http.Response, []byte) {
		t.Helper()
		var reader *strings.Reader
		if body != "" {
			reader = strings.NewReader(body)
		} else {
			reader = strings.NewReader("")
		}
		req, err := http.NewRequest(method, ts.URL+path, reader)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if reqID != "" {
			req.Header.Set("X-Request-ID", reqID)
		}
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("do %s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		var out bytes.Buffer
		_, _ = out.ReadFrom(resp.Body)
		return resp, out.Bytes()
	}

	// 1. Health ready (200) and unready (503) + Swagger using exact OpenAPI spec.
	resp, raw := doJSON(http.MethodGet, "/api/v1/health", "", "req-health-ok", "")
	if resp.StatusCode != http.StatusOK || resp.Header.Get("X-Request-ID") != "req-health-ok" {
		t.Fatalf("health expected 200 & X-Request-ID=req-health-ok, got %d hdr=%q body=%s", resp.StatusCode, resp.Header.Get("X-Request-ID"), raw)
	}
	backend.mu.Lock()
	backend.healthy = false
	backend.mu.Unlock()
	resp, raw = doJSON(http.MethodGet, "/api/v1/health", "", "", "")
	if resp.StatusCode != http.StatusServiceUnavailable || resp.Header.Get("X-Request-ID") == "" {
		t.Fatalf("unhealthy expected 503 with X-Request-ID, got %d hdr=%q body=%s", resp.StatusCode, resp.Header.Get("X-Request-ID"), raw)
	}
	backend.mu.Lock()
	backend.healthy = true
	backend.mu.Unlock()

	resp, raw = doJSON(http.MethodGet, "/swagger/", "", "", "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), "/swagger/openapi.yaml") {
		t.Fatalf("GET /swagger/ expected 200 HTML referencing /swagger/openapi.yaml, got %d", resp.StatusCode)
	}
	resp, raw = doJSON(http.MethodGet, "/swagger/openapi.yaml", "", "", "")
	if resp.StatusCode != http.StatusOK || !bytes.Equal(raw, openAPIBytes) {
		t.Fatalf("GET /swagger/openapi.yaml did not serve exact api/openapi.yaml")
	}

	// 2. 401 without token across protected routes + X-Request-ID echoed on errors.
	for _, protected := range []struct {
		method, path, body string
	}{
		{http.MethodGet, "/api/v1/users/me", ""},
		{http.MethodGet, "/api/v1/products", ""},
		{http.MethodPost, "/api/v1/purchases", `{"item_id":"DEMO-BASE"}`},
		{http.MethodGet, "/api/v1/recommendations/me", ""},
	} {
		resp, raw = doJSON(protected.method, protected.path, "", "req-unauth-check", protected.body)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s %s without token: status=%d, want 401", protected.method, protected.path, resp.StatusCode)
		}
		if resp.Header.Get("X-Request-ID") != "req-unauth-check" {
			t.Fatalf("%s %s missing X-Request-ID on 401: %q", protected.method, protected.path, resp.Header.Get("X-Request-ID"))
		}
		var errBody struct {
			RequestID string `json:"request_id"`
			Error     struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(raw, &errBody); err != nil || errBody.RequestID != "req-unauth-check" || errBody.Error.Code == "" {
			t.Fatalf("invalid 401 error payload: %s", raw)
		}
	}

	// 3. Register two distinct accounts & login; check 409 conflict & secret exclusion from logs.
	secretPass1 := "S3cret-Alice-Pass-42!"
	secretPass2 := "S3cret-Bob-Pass-99!"
	resp, raw = doJSON(http.MethodPost, "/api/v1/auth/register", "", "", `{"login":"alice","password":"`+secretPass1+`","client_group":"B2B","industry":"IT"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register alice status=%d body=%s, want 201", resp.StatusCode, raw)
	}
	resp, _ = doJSON(http.MethodPost, "/api/v1/auth/register", "", "", `{"login":"alice","password":"other","client_group":"B2B"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate register status=%d, want 409", resp.StatusCode)
	}
	resp, _ = doJSON(http.MethodPost, "/api/v1/auth/register", "", "", `{"login":"badgroup","password":"pw","client_group":"INVALID"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid client_group status=%d, want 400", resp.StatusCode)
	}

	resp, raw = doJSON(http.MethodPost, "/api/v1/auth/register", "", "", `{"login":"bob","password":"`+secretPass2+`","client_group":"B2C"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register bob status=%d body=%s, want 201", resp.StatusCode, raw)
	}

	resp, raw = doJSON(http.MethodPost, "/api/v1/auth/login", "", "", `{"login":"alice","password":"wrong"}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password login status=%d, want 401", resp.StatusCode)
	}

	var login1, login2 struct {
		AccessToken string `json:"access_token"`
		ExpiresAt   string `json:"expires_at"`
	}
	resp, raw = doJSON(http.MethodPost, "/api/v1/auth/login", "", "", `{"login":"alice","password":"`+secretPass1+`"}`)
	if resp.StatusCode != http.StatusOK || json.Unmarshal(raw, &login1) != nil || login1.AccessToken == "" {
		t.Fatalf("login alice failed: status=%d body=%s", resp.StatusCode, raw)
	}
	resp, raw = doJSON(http.MethodPost, "/api/v1/auth/login", "", "", `{"login":"bob","password":"`+secretPass2+`"}`)
	if resp.StatusCode != http.StatusOK || json.Unmarshal(raw, &login2) != nil || login2.AccessToken == "" {
		t.Fatalf("login bob failed: status=%d body=%s", resp.StatusCode, raw)
	}

	// 4. Verify /users/me and /products validation.
	var me1, me2 struct {
		ProfileID   string `json:"profile_id"`
		ClientGroup string `json:"client_group"`
	}
	resp, raw = doJSON(http.MethodGet, "/api/v1/users/me", login1.AccessToken, "", "")
	if resp.StatusCode != http.StatusOK || json.Unmarshal(raw, &me1) != nil || me1.ClientGroup != "B2B" || me1.ProfileID == "" {
		t.Fatalf("users/me alice: status=%d body=%s", resp.StatusCode, raw)
	}
	resp, raw = doJSON(http.MethodGet, "/api/v1/users/me", login2.AccessToken, "", "")
	if resp.StatusCode != http.StatusOK || json.Unmarshal(raw, &me2) != nil || me2.ClientGroup != "B2C" || me2.ProfileID == me1.ProfileID {
		t.Fatalf("users/me bob: status=%d body=%s", resp.StatusCode, raw)
	}

	resp, _ = doJSON(http.MethodGet, "/api/v1/products?limit=0", login1.AccessToken, "", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("products limit=0 status=%d, want 400", resp.StatusCode)
	}
	resp, _ = doJSON(http.MethodGet, "/api/v1/products?limit=101", login1.AccessToken, "", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("products limit=101 status=%d, want 400", resp.StatusCode)
	}
	resp, raw = doJSON(http.MethodGet, "/api/v1/products?limit=1", login1.AccessToken, "", "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), "DEMO-BASE") {
		t.Fatalf("products limit=1 status=%d body=%s", resp.StatusCode, raw)
	}

	// 5. 400 on profile spoofing/override attempts & 404 on unknown item.
	for _, spoof := range []struct {
		path string
		body string
	}{
		{"/api/v1/purchases", `{"item_id":"DEMO-BASE","profile_id":"` + me2.ProfileID + `"}`},
		{"/api/v1/purchases", `{"item_id":"DEMO-BASE","client_id":"DEMO-B2C"}`},
		{"/api/v1/purchases?profile_id=" + me2.ProfileID, `{"item_id":"DEMO-BASE"}`},
		{"/api/v1/purchases?client_id=DEMO-B2C", `{"item_id":"DEMO-BASE"}`},
	} {
		resp, raw = doJSON(http.MethodPost, spoof.path, login1.AccessToken, "req-spoof", spoof.body)
		if resp.StatusCode != http.StatusBadRequest || resp.Header.Get("X-Request-ID") != "req-spoof" {
			t.Fatalf("spoof %s %s: got status=%d hdr=%q body=%s, want 400", spoof.path, spoof.body, resp.StatusCode, resp.Header.Get("X-Request-ID"), raw)
		}
	}
	resp, _ = doJSON(http.MethodGet, "/api/v1/recommendations/me?profile_id="+me2.ProfileID, login1.AccessToken, "", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("recommendations/me with profile_id override: got %d, want 400", resp.StatusCode)
	}
	resp, _ = doJSON(http.MethodPost, "/api/v1/purchases", login1.AccessToken, "", `{"item_id":"NON-EXISTENT-ITEM"}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown item purchase: got %d, want 404", resp.StatusCode)
	}

	// 6. Session isolation + canonical item_id purchase + contract.Recommender contract.
	// Alice purchases via offer alias "demo-offer-base" -> recorded as canonical "DEMO-BASE" for Alice only.
	resp, raw = doJSON(http.MethodPost, "/api/v1/purchases", login1.AccessToken, "req-buy-1", `{"item_id":"demo-offer-base"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("alice purchase failed: status=%d body=%s", resp.StatusCode, raw)
	}

	// Alice sees renewal recommendation with 2 variants and resolved graph.
	resp, raw = doJSON(http.MethodGet, "/api/v1/recommendations/me", login1.AccessToken, "req-rec-alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("alice recommendations failed: status=%d body=%s", resp.StatusCode, raw)
	}
	var aliceRec contract.RecommendationResponse
	if err := json.Unmarshal(raw, &aliceRec); err != nil {
		t.Fatalf("decode alice recommendations: %v", err)
	}
	if aliceRec.RequestID != "req-rec-alice" || len(aliceRec.Items) != 1 || aliceRec.Items[0].ItemID != "DEMO-RENEW-12" || len(aliceRec.Items[0].Variants) != 2 {
		t.Fatalf("unexpected alice recommendation payload: %+v", aliceRec)
	}

	// Bob's session remains completely isolated: 0 items, serialized as [] and partial_reason: null.
	resp, raw = doJSON(http.MethodGet, "/api/v1/recommendations/me", login2.AccessToken, "req-rec-bob", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bob recommendations failed: status=%d body=%s", resp.StatusCode, raw)
	}
	if !strings.Contains(string(raw), `"items":[]`) || !strings.Contains(string(raw), `"nodes":[]`) || !strings.Contains(string(raw), `"edges":[]`) || !strings.Contains(string(raw), `"partial_reason":null`) {
		t.Fatalf("expected empty lists as [] and partial_reason as null for bob, got: %s", raw)
	}

	// 7. Token expiration check -> 401 once TTL expires.
	now = now.Add(2 * time.Hour)
	resp, _ = doJSON(http.MethodGet, "/api/v1/users/me", login1.AccessToken, "", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired token status=%d, want 401", resp.StatusCode)
	}

	// 8. Verify slog logs recorded request_id & outcome and NEVER leaked passwords or tokens.
	logs := logBuf.String()
	if !strings.Contains(logs, `"request_id":"req-rec-alice"`) || !strings.Contains(logs, `"outcome":`) {
		t.Fatalf("expected structured slog lines with request_id and outcome, got: %s", logs)
	}
	for _, secret := range []string{secretPass1, secretPass2, login1.AccessToken, login2.AccessToken} {
		if strings.Contains(logs, secret) {
			t.Fatalf("HTTP logs leaked secret %q!", secret)
		}
	}
}

type catalogMetaStub struct {
	meta transporthttp.CatalogMetaDTO
	err  error
}

func (c catalogMetaStub) CatalogMeta(_ context.Context) (transporthttp.CatalogMetaDTO, error) {
	return c.meta, c.err
}

type fixedItemsRecommender struct {
	items []domain.Recommendation
}

func (f fixedItemsRecommender) Recommend(_ context.Context, _ contract.RecommendationQuery) (contract.RecommendationResult, error) {
	return contract.RecommendationResult{
		SnapshotID: "demo-v1",
		Items:      f.items,
		Graph:      domain.Graph{Nodes: []domain.Node{}, Edges: []domain.Evidence{}},
	}, nil
}

func TestPlatformCatalogMetaAndCarouselUpTo36(t *testing.T) {
	authRepo := auth.NewMemoryRepository()
	authSvc := auth.NewService(authRepo, auth.WithBcryptCost(bcrypt.MinCost))
	_, sess, err := authSvc.Register(context.Background(), auth.RegisterInput{
		Login:       "carousel-user",
		Password:    "carousel-pass",
		ClientGroup: domain.B2B,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	backend := newMemoryPlatformBackend()
	backend.schemaVer = "0006_offer_embeddings"

	genAt := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	impAt := time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)
	ver := "demo-feed-v1"

	items36 := make([]domain.Recommendation, 36)
	for i := range items36 {
		id := domain.ItemID("ITEM-" + string(rune('A'+(i%26))) + "-" + strings.Repeat("X", i/26+1))
		items36[i] = domain.Recommendation{
			ItemID:      id,
			Kind:        domain.Popular,
			Title:       "Product " + string(id),
			Explanation: "Popular in cohort",
			Variants:    []domain.Variant{},
			EvidenceIDs: []string{},
		}
	}

	openAPIBytes, err := os.ReadFile(filepath.Join("..", "..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read openapi.yaml: %v", err)
	}
	if !bytes.Contains(openAPIBytes, []byte("maxItems: 36")) {
		t.Fatalf("expected api/openapi.yaml to allow maxItems: 36")
	}

	srv := transporthttp.NewServer(transporthttp.Config{
		Auth:        authSvc,
		Health:      backend,
		Catalog:     backend,
		CatalogMeta: catalogMetaStub{meta: transporthttp.CatalogMetaDTO{CatalogVersion: &ver, FeedGeneratedAt: &genAt, CatalogImportedAt: &impAt}},
		Purchases:   backend,
		Recommender: fixedItemsRecommender{items: items36},
		OpenAPISpec: openAPIBytes,
	})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// 1. Health reports status=ok, schema_version=0006_offer_embeddings, and catalog feed timestamps.
	hResp, err := ts.Client().Get(ts.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer hResp.Body.Close()
	if hResp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d, want 200", hResp.StatusCode)
	}
	var hBody map[string]any
	if err := json.NewDecoder(hResp.Body).Decode(&hBody); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if hBody["status"] != "ok" || hBody["schema_version"] != "0006_offer_embeddings" || hBody["catalog_version"] != "demo-feed-v1" || hBody["feed_generated_at"] != "2026-08-31T12:00:00Z" {
		t.Fatalf("unexpected health body: %+v", hBody)
	}

	// 2. Old schema (< 0004) returns 503 Service Unavailable, never 500.
	backend.mu.Lock()
	backend.healthy = false
	backend.mu.Unlock()
	oldResp, err := ts.Client().Get(ts.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("GET /health unready: %v", err)
	}
	oldResp.Body.Close()
	if oldResp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("old schema health status = %d, want 503 (never 500)", oldResp.StatusCode)
	}

	// 3. Products endpoint includes catalog feed version & date without altering item card fields.
	reqProd, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/products?limit=2", nil)
	reqProd.Header.Set("Authorization", "Bearer "+sess.AccessToken)
	pResp, err := ts.Client().Do(reqProd)
	if err != nil {
		t.Fatalf("GET /products: %v", err)
	}
	defer pResp.Body.Close()
	var pBody struct {
		CatalogVersion    string                     `json:"catalog_version"`
		FeedGeneratedAt   string                     `json:"feed_generated_at"`
		CatalogImportedAt string                     `json:"catalog_imported_at"`
		Items             []transporthttp.ProductDTO `json:"items"`
	}
	if err := json.NewDecoder(pResp.Body).Decode(&pBody); err != nil {
		t.Fatalf("decode /products: %v", err)
	}
	if pBody.CatalogVersion != "demo-feed-v1" || pBody.FeedGeneratedAt != "2026-08-31T12:00:00Z" || len(pBody.Items) != 2 {
		t.Fatalf("unexpected /products response: %+v", pBody)
	}

	// 4. Recommendations endpoint serves 36 items cleanly while preserving card shape.
	reqRec, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/recommendations/me", nil)
	reqRec.Header.Set("Authorization", "Bearer "+sess.AccessToken)
	rResp, err := ts.Client().Do(reqRec)
	if err != nil {
		t.Fatalf("GET /recommendations/me: %v", err)
	}
	defer rResp.Body.Close()
	var recBody contract.RecommendationResponse
	if err := json.NewDecoder(rResp.Body).Decode(&recBody); err != nil {
		t.Fatalf("decode /recommendations/me: %v", err)
	}
	if len(recBody.Items) != 36 {
		t.Fatalf("got %d recommendation items, want 36", len(recBody.Items))
	}
}
