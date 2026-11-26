package recommend

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"recsys/internal/contract"
	"recsys/internal/domain"
)

type memory struct {
	client          domain.Client
	history         []domain.Sale
	cutoff, through time.Time
	products        map[domain.ItemID]Product
	sets            map[contract.CandidateKey][]domain.Candidate
	err             error
}

func (m *memory) Profile(context.Context, domain.ClientID) (domain.Client, error) {
	return m.client, nil
}
func (m *memory) History(_ context.Context, _ domain.ClientID, through time.Time) ([]domain.Sale, error) {
	m.through = through
	var rows []domain.Sale
	for _, s := range m.history {
		if through.IsZero() || !s.OrderWeek.After(through) {
			rows = append(rows, s)
		}
	}
	return rows, nil
}
func (m *memory) Products(_ context.Context, _ string, ids []domain.ItemID) (map[domain.ItemID]Product, error) {
	p := map[domain.ItemID]Product{}
	for _, id := range ids {
		if v, ok := m.products[id]; ok {
			p[id] = v
		}
	}
	return p, nil
}
func (m *memory) SnapshotInfo(context.Context, string) (SnapshotInfo, error) {
	return SnapshotInfo{"s", m.cutoff}, nil
}
func (m *memory) Candidates(_ context.Context, s string, k contract.CandidateKey, n int) ([]domain.Candidate, error) {
	if s != "s" {
		return nil, errors.New("wrong snapshot")
	}
	v := m.sets[k]
	return v[:min(n, len(v))], m.err
}
func (m *memory) add(k contract.CandidateKey, id domain.ItemID) {
	from := "cohort:demo"
	if k.SourceItemID != "" {
		from = "item:" + string(k.SourceItemID)
	}
	e := domain.Evidence{ID: fmt.Sprint(k, "/", id), From: from, To: "item:" + string(id), Type: k.Signal, Source: "demo", SnapshotID: "s", Reason: "Проверено на демокаталоге", Weight: 1}
	m.sets[k] = append(m.sets[k], domain.Candidate{ItemID: id, Score: 1, Evidence: []domain.Evidence{e}})
}

func TestRecommendModule(t *testing.T) {
	const base, short, long = "DEMO-BASE", "DEMO-RENEW-12", "DEMO-RENEW-24"
	industry := "Другое"
	m := &memory{client: domain.Client{ID: "u", Group: domain.B2B, Industry: &industry}, cutoff: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), history: []domain.Sale{{ItemID: base}}, sets: map[contract.CandidateKey][]domain.Candidate{}, products: map[domain.ItemID]Product{base: {Platform: "linux"}}}
	query := contract.RecommendationQuery{Mode: contract.Online, ClientID: "u"}
	engine := New(m)
	run := func() contract.RecommendationResult {
		t.Helper()
		r, err := engine.Recommend(context.Background(), query)
		if err != nil {
			t.Fatal(err)
		}
		checkGraph(t, r)
		return r
	}
	for id, months := range map[domain.ItemID]int{short: 12, long: 24} {
		offer := string(id)
		m.products[id] = Product{Title: offer, Platform: "linux", Available: true, InSales: true, Audience: domain.B2B, OfferID: &offer, FamilyID: "suite", FamilySource: "catalog-rule", Differences: []domain.Difference{{Field: "term", Value: months, Unit: "month", Source: "catalog"}}}
		m.add(contract.CandidateKey{Signal: "renewal", SourceItemID: base}, id)
	}
	r := run()
	if len(r.Items) != 1 || r.PartialReason == nil {
		t.Fatalf("partial result: %+v", r)
	}
	item := r.Items[0]
	if item.Kind != domain.Renewal || item.ItemID != short || len(item.Variants) != 2 || item.Variants[0].Differences[0].Value != 12 || item.Variants[1].Differences[0].Value != 24 || item.Explanation == "" {
		t.Fatalf("renewal: %+v", item)
	}
	if !reflect.DeepEqual(r, run()) {
		t.Fatal("unstable response")
	}
	m.history = append(m.history, domain.Sale{ItemID: short, OrderWeek: m.cutoff.AddDate(0, 0, 7)})
	r = run()
	if len(r.Items) != 1 || r.Items[0].ItemID != long || len(r.Items[0].Variants) != 1 {
		t.Fatalf("purchased variant returned: %+v", r)
	}
	query.Mode = contract.Backtest
	query.SnapshotID = "s"
	m.products["no-offer"] = Product{InSales: true}
	m.add(contract.CandidateKey{Signal: "sales"}, "no-offer")
	r = run()
	if !m.through.Equal(m.cutoff) || len(r.Items) != 3 || r.Items[0].ItemID != short || r.Items[1].ItemID != long || r.Items[2].OfferID != nil || r.PartialReason != nil {
		t.Fatalf("historical result: %+v", r)
	}
	query.Mode = contract.Online
	m.client.Group = domain.B2C
	m.history = m.history[:1]
	offer := "demo"
	m.products["extra"] = Product{OfferID: &offer, Available: true}
	m.add(contract.CandidateKey{Signal: "complement", SourceItemID: base}, "extra")
	r = run()
	if len(r.Items) != 1 || r.Items[0].Kind != domain.Complement {
		t.Fatal(r)
	}
	m.history = nil
	m.add(contract.CandidateKey{Signal: "popularity", ClientGroup: domain.B2C}, "extra")
	r = run()
	if len(r.Items) != 1 || r.Items[0].Kind != domain.Popular {
		t.Fatal(r)
	}
	m.client.Group = domain.B2B
	m.add(contract.CandidateKey{Signal: "popularity", ClientGroup: domain.B2B}, long)
	m.add(contract.CandidateKey{Signal: "popularity", ClientGroup: domain.B2B, Industry: industry}, short)
	r = run()
	if len(r.Items) != 1 || r.Items[0].ItemID != long {
		t.Fatal(r)
	}
	m.err = errors.New("read failed")
	if _, err := engine.Recommend(context.Background(), query); !errors.Is(err, m.err) {
		t.Fatalf("reader error: %v", err)
	}
}

func checkGraph(t *testing.T, r contract.RecommendationResult) {
	t.Helper()
	nodes, edges := map[string]bool{}, map[string]bool{}
	for _, n := range r.Graph.Nodes {
		nodes[n.ID] = true
	}
	for _, e := range r.Graph.Edges {
		if !nodes[e.From] || !nodes[e.To] {
			t.Fatal("missing graph endpoint")
		}
		edges[e.ID] = true
	}
	for _, i := range r.Items {
		if len(i.EvidenceIDs) == 0 {
			t.Fatal("missing evidence")
		}
		for _, id := range i.EvidenceIDs {
			if !edges[id] {
				t.Fatal("missing graph edge")
			}
		}
	}
}

func TestRefillAfterFilters(t *testing.T) {
	m := &memory{client: domain.Client{ID: "u", Group: domain.B2C}, history: []domain.Sale{{ItemID: "base"}}, sets: map[contract.CandidateKey][]domain.Candidate{}, products: map[domain.ItemID]Product{"base": {Platform: "linux"}}}
	for i := 0; i < 67; i++ {
		offer := fmt.Sprintf("p%02d", i)
		id := domain.ItemID(offer)
		p := Product{OfferID: &offer, Available: true}
		if i < 64 {
			switch i % 3 {
			case 0:
				p.Audience = domain.B2B
			case 1:
				p.Platform = "osx"
			case 2:
				p.Available = false
			}
		}
		m.products[id] = p
		m.add(contract.CandidateKey{Signal: "catalog"}, id)
	}
	r, err := New(m).Recommend(context.Background(), contract.RecommendationQuery{ClientID: "u", Mode: contract.Online})
	if err != nil || len(r.Items) != 3 || r.Items[0].ItemID != "p64" || r.Items[2].ItemID != "p66" {
		t.Fatalf("refill: %+v %v", r.Items, err)
	}
}

func TestOrganizationRestrictionsApplyOnlyOnline(t *testing.T) {
	offer := "offer"
	product := Product{OfferID: &offer, InSales: true, Available: true, Audience: domain.B2B, OrganizationType: "академическая"}
	generic := domain.Client{Group: domain.B2B, Type: "ЮЛ"}
	academic := domain.Client{Group: domain.B2B, Type: "Академическая организация"}
	if eligible(product, generic, nil, contract.Online) || !eligible(product, academic, nil, contract.Online) {
		t.Fatal("online recommendation must match the confirmed organization type")
	}
	product.Available = false
	product.OfferID = nil
	if eligible(product, academic, nil, contract.Online) || !eligible(product, generic, nil, contract.Backtest) {
		t.Fatal("historical pool must not depend on current availability, offer, or organization restrictions")
	}
}
