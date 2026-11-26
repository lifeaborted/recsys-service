package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"recsys/internal/enrichment"
	"recsys/internal/explain"
)

func TestSemanticSearch(t *testing.T) {
	const fixture = "../../internal/enrichment/testdata/"
	args := []string{
		"-feed", fixture + "mini-feed.xml", "-items", fixture + "mini-items.json",
		"-sources", fixture + "mini-sources.json", "-feed-url", "https://example.com/demo-feed.xml",
		"-catalog-version", "mini-v1", "-snapshot", "demo-v1",
	}
	const itemID = "DEMO-WRONG-SKU"
	var approval explain.SearchApproval
	if err := readJSON(fixture+"mini-search-approval.json", &approval); err != nil {
		t.Fatal(err)
	}
	query := approval.Queries[itemID].Text
	requests := make(chan []string, 16)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
			r.Method != http.MethodPost || r.URL.Path != "/v1/embeddings" ||
			r.Header.Get("Authorization") != "Bearer test-key" || req.Model != "test-embed" {
			t.Error("unexpected embedding request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- req.Input
		vectors := map[string][]float64{
			"Demo Suite Base":       {2, 0},
			"Demo Suite Renewal 12": {0, 1},
			"Demo Suite Renewal 24": {-1, 0},
			"Demo Ambiguous A":      {1, 1},
			"Demo Ambiguous B":      {1, 1},
			query:                   {1, 0},
		}
		data := make([]map[string]any, 0, len(req.Input))
		for i, input := range req.Input {
			vector, ok := vectors[input]
			if !ok {
				t.Errorf("unexpected embedding input %q", input)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			data = append(data, map[string]any{"index": i, "embedding": vector})
		}
		json.NewEncoder(w).Encode(map[string]any{"model": req.Model, "data": data})
	}))
	defer provider.Close()
	t.Setenv("EMBED_BASE_URL", provider.URL+"/v1")
	t.Setenv("EMBED_MODEL", "test-embed")
	t.Setenv("EMBED_API_KEY", "test-key")
	t.Setenv("EMBED_BATCH_SIZE", "2")

	match := func(t *testing.T, args []string) map[string]enrichment.Match {
		t.Helper()
		var output bytes.Buffer
		if err := run(t.Context(), args, &output, io.Discard); err != nil {
			t.Fatal(err)
		}
		var result enrichment.Result
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		matches := make(map[string]enrichment.Match)
		for _, m := range result.Matches {
			matches[string(m.ItemID)] = m
		}
		if len(result.Relations) != 1 {
			t.Fatal("reviewed relation changed")
		}
		return matches
	}

	t.Run("offline by default", func(t *testing.T) {
		t.Setenv("EMBED_BATCH_SIZE", "invalid")
		if found := match(t, args)[itemID]; found.Status != "unmatched" || len(requests) != 0 {
			t.Fatalf("unexpected offline match: %+v, requests: %d", found, len(requests))
		}
	})
	t.Run("ranked candidates for review", func(t *testing.T) {
		matches := match(t, append(slices.Clone(args), "-semantic-search", fixture+"mini-search-approval.json"))
		want := []string{"501", "504", "505", "502", "503"}
		found := matches[itemID]
		if found.Status != "review" || found.OfferID != "" || found.Method != "search" || !slices.Equal(found.Candidates, want) {
			t.Fatalf("unexpected semantic match: %+v", found)
		}
		var ids []string
		for _, hit := range found.SearchHits {
			ids = append(ids, hit.OfferID)
			if !strings.Contains(hit.Basis, "cosine similarity") || !strings.Contains(hit.Basis, "test-embed") {
				t.Fatalf("missing match basis: %+v", hit)
			}
		}
		if !slices.Equal(ids, want) {
			t.Fatalf("unexpected hit order: %v", ids)
		}
		for id, method := range map[string]string{
			"DEMO-BASE": "vendor_code", "DEMO-RENEW-24": "url_code", "DEMO-NAME-ONLY": "name", "DEMO-AMBIGUOUS": "vendor_code",
		} {
			if m := matches[id]; m.Method != method || len(m.SearchHits) != 0 {
				t.Fatalf("exact match changed: %+v", m)
			}
		}
		wantBatches := [][]string{
			{"Demo Suite Base", "Demo Suite Renewal 12"},
			{"Demo Suite Renewal 24", "Demo Ambiguous A"},
			{"Demo Ambiguous B"}, {query},
		}
		if len(requests) != len(wantBatches) {
			t.Fatalf("unexpected number of requests: %d", len(requests))
		}
		for _, want := range wantBatches {
			if got := <-requests; !slices.Equal(got, want) {
				t.Fatalf("embedding batch %q, want %q", got, want)
			}
		}
	})
	t.Run("caps results and skips unapproved positions", func(t *testing.T) {
		provider, err := explain.EmbeddingsFromEnv()
		if err != nil {
			t.Fatal(err)
		}
		search, err := explain.NewCatalogSearch(provider, enrichment.FeedVersion{ID: approval.CatalogVersion, URL: approval.FeedURL},
			[]enrichment.Offer{
				{ID: "1", Name: "Demo Suite Renewal 24"},
				{ID: "2", Name: "Demo Suite Renewal 12"},
				{ID: "3", Name: "Demo Suite Base"},
			}, approval)
		if err != nil {
			t.Fatal(err)
		}
		if hits, err := search.Search(t.Context(), approval.CatalogVersion, enrichment.SearchQuery{ItemID: "another-item"}, 1); err != nil ||
			len(hits) != 0 || len(requests) != 0 {
			t.Fatalf("unapproved position reached the provider: %+v, %v", hits, err)
		}
		for range 2 {
			hits, err := search.Search(t.Context(), approval.CatalogVersion, enrichment.SearchQuery{ItemID: itemID}, 1)
			if err != nil || len(hits) != 1 || hits[0].OfferID != "3" {
				t.Fatalf("unexpected best candidate: %+v, %v", hits, err)
			}
		}
		if len(requests) != 3 {
			t.Fatalf("repeat search rebuilt the index or re-sent the query, requests: %d", len(requests))
		}
		for range 3 {
			<-requests
		}
	})
	t.Run("rejects unmatched feed approval and unapproved query", func(t *testing.T) {
		for _, change := range []func(*explain.SearchApproval){
			func(a *explain.SearchApproval) { a.CatalogVersion = "other" },
			func(a *explain.SearchApproval) { a.Approved = false },
			func(a *explain.SearchApproval) {
				text := a.Queries[itemID]
				text.Approved = false
				a.Queries[itemID] = text
			},
		} {
			var local explain.SearchApproval
			if err := readJSON(fixture+"mini-search-approval.json", &local); err != nil {
				t.Fatal(err)
			}
			change(&local)
			path := filepath.Join(t.TempDir(), "approval.json")
			data, err := json.Marshal(local)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if err := run(t.Context(), append(slices.Clone(args), "-semantic-search", path), &output, io.Discard); err == nil ||
				output.Len() != 0 || len(requests) != 0 {
				t.Fatalf("approval failure must precede network and output: %v", err)
			}
		}
	})
	t.Run("provider failure leaves output empty", func(t *testing.T) {
		failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer failing.Close()
		t.Setenv("EMBED_BASE_URL", failing.URL)
		var output bytes.Buffer
		err := run(t.Context(), append(slices.Clone(args), "-semantic-search", fixture+"mini-search-approval.json"), &output, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "HTTP 503") || output.Len() != 0 {
			t.Fatalf("unexpected provider failure handling: %v, %s", err, &output)
		}
	})
}
