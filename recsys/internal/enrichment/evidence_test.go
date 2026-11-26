package enrichment_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"recsys/internal/domain"
	"recsys/internal/enrichment"
	"recsys/internal/explain"
)

func loadJSON[T any](t *testing.T, path string) T {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestEvidenceModule(t *testing.T) {
	ctx := context.Background()
	raw, err := os.ReadFile("testdata/mini-feed.xml")
	if err != nil {
		t.Fatal(err)
	}
	var zipped bytes.Buffer
	w := gzip.NewWriter(&zipped)
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var offers []enrichment.Offer
	feed, err := enrichment.ReadFeed(&zipped, "mini-v1", "https://example.com/demo-feed.xml", time.UTC,
		func(_ enrichment.FeedVersion, offer enrichment.Offer) error {
			offers = append(offers, offer)
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != 5 {
		t.Fatalf("got %d offers", len(offers))
	}
	if unknown := offers[2].Facts["edition"]; unknown.Value != nil || unknown.Source != nil {
		t.Fatal("missing edition must stay unknown")
	}
	if got := offers[2].Facts["available"]; got.Value == nil || *got.Value != "false" || got.Source == nil {
		t.Fatal("availability must retain false and source")
	}
	items := loadJSON[[]enrichment.Item](t, "testdata/mini-items.json")
	review := loadJSON[enrichment.Review](t, "testdata/mini-sources.json")
	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	result, err := enrichment.Build(ctx, feed, "demo-v1", &cutoff, offers, items, review, nil)
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(offers)
	slices.Reverse(items)
	repeat, err := enrichment.Build(ctx, feed, "demo-v1", &cutoff, offers, items, review, nil)
	if err != nil || !reflect.DeepEqual(result, repeat) {
		t.Fatalf("reprocessing changed result: %v", err)
	}
	matches := make(map[domain.ItemID]enrichment.Match)
	for _, match := range result.Matches {
		matches[match.ItemID] = match
	}
	for _, want := range []struct {
		id                    domain.ItemID
		status, method, offer string
	}{
		{"DEMO-BASE", "selected", "vendor_code", "501"},
		{"DEMO-RENEW-12", "selected", "vendor_code", "502"},
		{"DEMO-RENEW-24", "selected", "url_code", "503"},
		{"DEMO-AMBIGUOUS", "ambiguous", "vendor_code", ""},
		{"DEMO-NAME-ONLY", "review", "name", ""},
		{"DEMO-WRONG-SKU", "unmatched", "", ""},
	} {
		got := matches[want.id]
		if got.Status != want.status || got.Method != want.method || got.OfferID != want.offer {
			t.Errorf("%s: got %+v", want.id, got)
		}
	}
	if !slices.Equal(matches["DEMO-AMBIGUOUS"].Candidates, []string{"504", "505"}) {
		t.Fatal("ambiguous candidates lost")
	}
	if len(result.Relations) != 1 || len(result.Rejected) != 1 || result.Rejected[0].ID != "demo-unverified" || !strings.Contains(result.Rejected[0].Reason, "quote") || !result.PostCutoff {
		t.Fatalf("unexpected verification result: %+v %+v", result.Relations, result.Rejected)
	}
	relation := result.Relations[0]
	if relation.Evidence.From != "item:DEMO-BASE" || relation.Evidence.To != "item:DEMO-RENEW-12" ||
		relation.Evidence.SnapshotID != "demo-v1" || relation.Source.Version != "demo-rules-v1" || len(relation.Constraints) != 3 {
		t.Fatalf("bad evidence: %+v", relation)
	}
	variants := result.Variants["DEMO-SUITE"]
	if len(variants) != 3 {
		t.Fatalf("wanted base and two renewals, got %+v", variants)
	}
	terms := make(map[domain.ItemID]domain.Difference)
	licenses := make(map[domain.ItemID]any)
	for _, variant := range variants {
		for _, delta := range variant.Differences {
			if delta.Field == "term" {
				terms[variant.ItemID] = delta
			}
			if delta.Field == "license_type" {
				licenses[variant.ItemID] = delta.Value
			}
		}
	}
	left, right := terms["DEMO-RENEW-12"], terms["DEMO-RENEW-24"]
	if licenses["DEMO-BASE"] != "base" || licenses["DEMO-RENEW-12"] != "renewal" || left.Value != 12 || right.Value != 24 ||
		left.Unit != "month" || right.Unit != "month" || left.Source == "" || right.Source == "" || left.Source == right.Source {
		t.Fatalf("bad variants: %+v", variants)
	}
	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer badServer.Close()
	chat := &explain.Chat{Config: explain.Config{BaseURL: badServer.URL, Model: "test"}}
	source := explain.PublicText{Text: relation.Source.Quote, SourceURL: relation.Source.URL, Version: relation.Source.Version, Approved: true}
	rendered := explain.Render(ctx, chat, &source, explain.EdgesFor("DEMO-RENEW-12", result.Relations), variants)
	if rendered.Provider != "template" || !strings.Contains(rendered.Text, "12 мес.") || !strings.Contains(rendered.Text, "24 мес.") {
		t.Fatalf("bad fallback: %+v", rendered)
	}
	review.Rules[0].From, review.Rules[0].To = review.Rules[0].To, review.Rules[0].From
	reverse, err := enrichment.Build(ctx, feed, "demo-v1", nil, offers, items, review, nil)
	if err != nil || len(reverse.Relations) != 0 || len(reverse.Rejected) != 2 || !strings.Contains(reverse.Rejected[0].Reason, "direction") {
		t.Fatalf("reversed relation accepted: %+v, %v", reverse.Rejected, err)
	}
}

type searchFunc func(context.Context, string, enrichment.SearchQuery, int) ([]enrichment.SearchHit, error)

func (f searchFunc) Search(ctx context.Context, version string, q enrichment.SearchQuery, n int) ([]enrichment.SearchHit, error) {
	return f(ctx, version, q, n)
}

func TestSearchOnlySuggests(t *testing.T) {
	matcher := enrichment.NewMatcher("mini-v1", []enrichment.Offer{{ID: "501", SKU: "BASE", Name: "Suite"}}, searchFunc(func(_ context.Context, version string, q enrichment.SearchQuery, _ int) ([]enrichment.SearchHit, error) {
		if version != "mini-v1" || q.SKU != "RENEWAL" {
			t.Fatal("wrong search scope")
		}
		return []enrichment.SearchHit{{OfferID: "501", Basis: "similar title"}}, nil
	}))
	match, err := matcher.Match(context.Background(), enrichment.Item{ID: "x", SKU: "RENEWAL", Name: "Suite"})
	if err != nil || match.Status != "review" || match.OfferID != "" || !slices.Equal(match.Candidates, []string{"501"}) {
		t.Fatalf("unexpected fuzzy match %+v, %v", match, err)
	}
}
