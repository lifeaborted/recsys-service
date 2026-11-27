package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"recsys/internal/contract"
	"recsys/internal/domain"
	"recsys/internal/enrichment"
	"recsys/internal/explain"
	"recsys/internal/recommend"
)

const publicPack = "../../internal/enrichment/testdata/public-pack/"
const packSnapshot = "public-pack-demo"

func publicPackArgs() []string {
	return []string{
		"-feed", publicPack + "feed.xml", "-items", publicPack + "items.json",
		"-sources", publicPack + "sources.json", "-feed-url", "https://example.com/public-pack/feed.xml",
		"-catalog-version", "public-pack-v1", "-snapshot", packSnapshot,
	}
}

type packOutput struct {
	enrichment.Result
	Proposals   []relationDraft  `json:"relation_proposals"`
	Explanation explain.Rendered `json:"explanation"`
}

func runPack(t *testing.T, args []string) (packOutput, string) {
	t.Helper()
	var output bytes.Buffer
	if err := run(t.Context(), args, &output, io.Discard); err != nil {
		t.Fatal(err)
	}
	var result packOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result, output.String()
}

func TestPublicPack(t *testing.T) {
	const bridgeQuote = "Norivault Bridge is being evaluated for use with Norivault Core."
	const weakQuote = "Norivault Agent includes a command-line installer."
	var requests atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Content string }
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" ||
			json.NewDecoder(r.Body).Decode(&req) != nil || len(req.Messages) != 2 {
			t.Error("unexpected chat request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var input struct{ From, To, Excerpt string }
		if err := json.Unmarshal([]byte(req.Messages[1].Content), &input); err != nil || input.From != "Norivault Core" {
			t.Errorf("unexpected extraction input: %+v, %v", input, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		kind, quote := "complement", ""
		switch input.To {
		case "Norivault Bridge":
			quote = bridgeQuote
		case "Norivault Agent":
			quote = weakQuote
		case "Veyro Archive":
			kind = "none"
			if !strings.Contains(input.Excerpt, "Veyro Archive is listed in the Backup category.") {
				t.Error("missing vendor's category listing")
			}
		default:
			t.Errorf("unexpected candidate %q", input.To)
		}
		if !strings.Contains(input.Excerpt, quote) {
			t.Errorf("missing source quote %q", quote)
		}
		requests.Add(1)
		content, _ := json.Marshal(map[string]string{"kind": kind, "quote": quote})
		json.NewEncoder(w).Encode(map[string]any{
			"model":   "pack-test",
			"choices": []any{map[string]any{"message": map[string]string{"content": string(content)}}},
		})
	}))
	defer provider.Close()
	t.Setenv("LLM_BASE_URL", provider.URL+"/v1")
	t.Setenv("LLM_MODEL", "pack-test")
	t.Setenv("LLM_API_KEY", "")

	args := append(publicPackArgs(), "-relation-requests", publicPack+"relation-requests.json", "-card", "PACK-BASE")
	result, first := runPack(t, args)
	_, second := runPack(t, args)
	if first != second || requests.Load() != 6 {
		t.Fatalf("repeated run changed output or provider calls: %d", requests.Load())
	}
	if len(result.Offers) != 10 || len(result.Relations) != 2 {
		t.Fatalf("unexpected pack: %d offers, %+v relations", len(result.Offers), result.Relations)
	}
	var review enrichment.Review
	if err := readJSON(publicPack+"sources.json", &review); err != nil {
		t.Fatal(err)
	}
	for i, expected := range []struct{ kind, target, name string }{
		{"complement", "PACK-AUDIT", "Norivault Audit"},
		{"upgrade", "PACK-PLUS", "Norivault Plus"},
	} {
		relation := result.Relations[i]
		if relation.Status != "confirmed" || relation.Evidence.Type != expected.kind ||
			relation.Evidence.From != "item:PACK-BASE" || relation.Evidence.To != "item:"+expected.target ||
			relation.Source.URL != review.Documents[0].Source.URL ||
			!strings.Contains(relation.Source.Quote, "Norivault Core") || !strings.Contains(relation.Source.Quote, expected.name) ||
			!strings.Contains(review.Documents[0].Text, relation.Source.Quote) {
			t.Fatalf("unexpected confirmed %s: %+v", expected.kind, relation)
		}
	}
	if len(result.Proposals) != 2 || result.Proposals[0].ID != "pack-bridge" ||
		result.Proposals[0].Status != "review" || result.Proposals[0].Quote != bridgeQuote ||
		result.Proposals[0].ReviewNote != "Совместимость требует проверки." ||
		result.Proposals[1].ID != "pack-rival" || result.Proposals[1].Status != "no_relation" {
		t.Fatalf("unexpected drafts: %+v", result.Proposals)
	}
	if len(result.Rejected) != 1 || result.Rejected[0].ID != "pack-weak" ||
		!strings.Contains(result.Rejected[0].Reason, "must name both products separately") {
		t.Fatalf("unexpected rejections: %+v", result.Rejected)
	}
	if os.Getenv("RECSYS_EVIDENCE_DEMO") == "1" {
		command := exec.CommandContext(t.Context(), "go", append([]string{"run", "."}, args...)...)
		output, err := command.Output()
		if err != nil {
			t.Fatalf("go run: %v, stderr: %s", err, command.Stderr)
		}
		if string(output) != first {
			t.Fatalf("CLI output differs from the tested result: %s", output)
		}
		for _, relation := range result.Relations {
			t.Logf("%s %s %s %q", relation.Status, relation.Evidence.Type, relation.Source.URL, relation.Source.Quote)
		}
		for _, draft := range result.Proposals {
			t.Logf("%s %s %s %s %q %s", draft.Status, draft.ID, draft.Kind, draft.SourceURL, draft.Quote, draft.ReviewNote)
		}
		for _, rejection := range result.Rejected {
			t.Logf("rejected %s: %s", rejection.ID, rejection.Reason)
		}
	}

	t.Run("model failure leaves template", func(t *testing.T) {
		var orderingCalled atomic.Bool
		failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Messages []struct{ Content string }
			}
			if json.NewDecoder(r.Body).Decode(&req) == nil && len(req.Messages) == 2 &&
				req.Messages[1].Content == review.Documents[0].Text {
				orderingCalled.Store(true)
			}
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer failing.Close()
		t.Setenv("LLM_BASE_URL", failing.URL+"/v1")
		failed, _ := runPack(t, append(slices.Clone(args), "-public-text", publicPack+"card-source.json"))
		if !orderingCalled.Load() || failed.Explanation != result.Explanation || failed.Explanation.Provider != "template" ||
			len(failed.Relations) != 2 || len(failed.Proposals) != 0 || len(failed.Rejected) != 3 {
			t.Fatalf("unexpected fallback: %+v", failed.Explanation)
		}
	})
}

func TestEnrichRejectsHistoricalSnapshot(t *testing.T) {
	for _, snapshot := range []string{"v5-train-20260831", " V5-TRAIN-20260831 "} {
		args := publicPackArgs()
		args[len(args)-1] = snapshot
		args[1] = "missing-feed.xml"
		var output bytes.Buffer
		err := run(t.Context(), args, &output, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "reserved for historical evaluation") || output.Len() != 0 {
			t.Fatalf("expected refusal before reading the feed, got %v and %q", err, &output)
		}
	}
}

func TestPublicPackColdStart(t *testing.T) {
	var fixture struct {
		Client struct {
			ID    domain.ClientID `json:"client_id"`
			Group domain.ClientGroup
		}
		History    []domain.Sale
		Popularity []struct {
			Group  domain.ClientGroup
			ItemID domain.ItemID `json:"item_id"`
			Buyers int
		}
	}
	if err := readJSON(publicPack+"cold-start.json", &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.History) != 0 {
		t.Fatal("cold-start fixture must have empty history")
	}
	pack, _ := runPack(t, publicPackArgs())
	reader := coldStartReader{
		client:  domain.Client{ID: fixture.Client.ID, Group: fixture.Client.Group},
		history: fixture.History, products: map[domain.ItemID]recommend.Product{},
		popularity: map[domain.ClientGroup][]domain.Candidate{},
	}
	for _, row := range fixture.Popularity {
		cohort, reason := "all", "Популярно среди всех покупателей."
		if row.Group != "" {
			cohort = string(row.Group)
			reason = "Истории покупок пока нет. Этот товар часто выбирают покупатели вашей группы " + cohort + "."
		}
		reader.popularity[row.Group] = append(reader.popularity[row.Group], domain.Candidate{
			ItemID: row.ItemID, Kind: domain.Popular, Score: float64(row.Buyers),
			Evidence: []domain.Evidence{{
				ID: cohort + "/" + string(row.ItemID), From: "cohort:" + cohort, To: "item:" + string(row.ItemID),
				Type: "popularity", Reason: reason, Weight: float64(row.Buyers),
				Source: "https://example.com/public-pack/cold-start.json", SnapshotID: packSnapshot,
			}},
		})
	}
	offers := map[string]enrichment.Offer{}
	for _, offer := range pack.Offers {
		offers[offer.ID] = offer
	}
	for _, match := range pack.Matches {
		offer := offers[match.OfferID]
		if match.Status == "selected" && offer.Facts["available"].Value != nil {
			reader.products[match.ItemID] = recommend.Product{
				Title: offer.Name, OfferID: &offer.ID, Available: *offer.Facts["available"].Value == "true",
			}
		}
	}
	result, err := recommend.New(&reader).Recommend(t.Context(), contract.RecommendationQuery{
		ClientID: fixture.Client.ID, Mode: contract.Online, SnapshotID: packSnapshot,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) == 0 || result.Items[0].ItemID != "PACK-BASE" ||
		result.Items[0].Kind != domain.Popular || !strings.Contains(result.Items[0].Explanation, "вашей группы B2B") {
		t.Fatalf("expected group popularity to win: %+v", result.Items)
	}
	for _, edge := range result.Graph.Edges {
		if edge.Type != "popularity" || !strings.HasPrefix(edge.From, "cohort:") {
			t.Fatalf("unexpected purchase relation: %+v", edge)
		}
	}
	t.Log(result.Items[0].Explanation)
}

type coldStartReader struct {
	client     domain.Client
	history    []domain.Sale
	products   map[domain.ItemID]recommend.Product
	popularity map[domain.ClientGroup][]domain.Candidate
}

func (r *coldStartReader) Profile(context.Context, domain.ClientID) (domain.Client, error) {
	return r.client, nil
}

func (r *coldStartReader) History(context.Context, domain.ClientID, time.Time) ([]domain.Sale, error) {
	return r.history, nil
}

func (r *coldStartReader) SnapshotInfo(context.Context, string) (recommend.SnapshotInfo, error) {
	return recommend.SnapshotInfo{ID: packSnapshot}, nil
}

func (r *coldStartReader) Products(_ context.Context, _ string, ids []domain.ItemID) (map[domain.ItemID]recommend.Product, error) {
	products := make(map[domain.ItemID]recommend.Product, len(ids))
	for _, id := range ids {
		if product, ok := r.products[id]; ok {
			products[id] = product
		}
	}
	return products, nil
}

func (r *coldStartReader) Candidates(_ context.Context, _ string, key contract.CandidateKey, limit int) ([]domain.Candidate, error) {
	if key.Signal != "popularity" {
		return nil, nil
	}
	rows := r.popularity[key.ClientGroup]
	return rows[:min(limit, len(rows))], nil
}
