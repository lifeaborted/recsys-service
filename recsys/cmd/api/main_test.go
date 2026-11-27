package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"recsys/internal/config"
)

func TestNewMockHTTPConfigServesHealthAndThreeCardExample(t *testing.T) {
	cfg := config.Config{
		Addr:           "127.0.0.1:0",
		RequestTimeout: 5 * time.Second,
		SessionTTL:     1 * time.Hour,
		BcryptCost:     4,
		ExamplesDir:    "../../api/examples",
		OpenAPIPath:    "../../api/openapi.yaml",
		Mock:           true,
	}

	srv, err := buildMockServer(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("buildMockServer: %v", err)
	}
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("unexpected health payload: %+v", body)
	}
}
