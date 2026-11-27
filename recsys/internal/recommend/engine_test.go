package recommend

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	"strings"

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
	if len(r.Items) != 0 || r.PartialReason == nil {
		t.Fatalf("online renewal stays out of the recommendation block: %+v", r)
	}
	if !reflect.DeepEqual(r, run()) {
		t.Fatal("unstable response")
	}
	m.history = append(m.history, domain.Sale{ItemID: short, OrderWeek: m.cutoff.AddDate(0, 0, 7)})
	r = run()
	if len(r.Items) != 0 {
		t.Fatalf("purchased renewal returned online: %+v", r.Items)
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

func TestB2BSoloUsesCatalogComplement(t *testing.T) {
	m := &memory{
		client:   domain.Client{ID: "u", Group: domain.B2B},
		history:  []domain.Sale{{ItemID: "solo"}},
		products: map[domain.ItemID]Product{"solo": {Platform: "linux"}},
		sets:     map[contract.CandidateKey][]domain.Candidate{},
	}
	offer := "addon"
	m.products["addon"] = Product{OfferID: &offer, Available: true, Audience: domain.B2B, Title: "Модуль"}
	m.add(contract.CandidateKey{Signal: "complement", SourceItemID: "solo"}, "addon")
	m.add(contract.CandidateKey{Signal: "popularity", ClientGroup: domain.B2B}, "noise")
	noise := "noise"
	m.products["noise"] = Product{OfferID: &noise, Available: true, Audience: domain.B2B}
	r, err := New(m).Recommend(context.Background(), contract.RecommendationQuery{ClientID: "u", Mode: contract.Online})
	if err != nil || len(r.Items) == 0 || r.Items[0].Kind != domain.Complement || r.Items[0].ItemID != "addon" {
		t.Fatalf("catalog complement must outrank group popularity: %+v %v", r.Items, err)
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

func offerID(id string) *string { return &id }

func rivalFixture() *memory {
	owned, rival, other, renew := domain.ItemID("kas"), domain.ItemID("drweb"), domain.ItemID("office"), domain.ItemID("kas-renew")
	m := &memory{
		client:  domain.Client{ID: "u", Group: domain.B2B},
		cutoff:  time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		history: []domain.Sale{{ItemID: owned}},
		sets:    map[contract.CandidateKey][]domain.Candidate{},
		products: map[domain.ItemID]Product{
			owned: {Title: "Kaspersky Endpoint", Vendor: "Kaspersky", CategoryID: "antivirus"},
			rival: {Title: "Dr.Web", Vendor: "Dr.Web", CategoryID: "antivirus", Available: true, InSales: true, OfferID: offerID("rival")},
			other: {Title: "Office", Vendor: "Microsoft", CategoryID: "office", Available: true, InSales: true, OfferID: offerID("office")},
			renew: {Title: "Kaspersky renewal", Vendor: "Other", CategoryID: "antivirus", Available: true, InSales: true, OfferID: offerID("renew")},
		},
	}
	m.add(contract.CandidateKey{Signal: "catalog"}, rival)
	m.add(contract.CandidateKey{Signal: "catalog"}, other)
	m.add(contract.CandidateKey{Signal: "sales"}, rival)
	m.add(contract.CandidateKey{Signal: "sales"}, other)
	m.add(contract.CandidateKey{Signal: "renewal", SourceItemID: owned}, renew)
	return m
}

func itemIDs(result contract.RecommendationResult) []domain.ItemID {
	ids := make([]domain.ItemID, len(result.Items))
	for i, item := range result.Items {
		ids[i] = item.ItemID
	}
	return ids
}

func TestOnlineDropsSameCategoryRival(t *testing.T) {
	engine := New(rivalFixture())
	online, err := engine.Recommend(context.Background(), contract.RecommendationQuery{ClientID: "u", Mode: contract.Online})
	if err != nil {
		t.Fatal(err)
	}
	got := itemIDs(online)
	if !reflect.DeepEqual(got, []domain.ItemID{"office"}) {
		t.Fatalf("online cards: %v", got)
	}
	back, err := engine.Recommend(context.Background(), contract.RecommendationQuery{ClientID: "u", Mode: contract.Backtest, SnapshotID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	got = itemIDs(back)
	if !reflect.DeepEqual(got, []domain.ItemID{"kas-renew", "drweb", "office"}) {
		t.Fatalf("backtest cards: %v", got)
	}
}

type orderFunc func(context.Context, []string, []RerankItem) ([]domain.ItemID, error)

func (fn orderFunc) Order(ctx context.Context, owned []string, items []RerankItem) ([]domain.ItemID, error) {
	return fn(ctx, owned, items)
}

func TestOnlineRerankReordersOnlyTheShortlist(t *testing.T) {
	left, right := domain.ItemID("a"), domain.ItemID("b")
	m := &memory{
		client:  domain.Client{ID: "u", Group: domain.B2B},
		cutoff:  time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		history: []domain.Sale{{ItemID: "base"}},
		sets:    map[contract.CandidateKey][]domain.Candidate{},
		products: map[domain.ItemID]Product{
			"base": {Title: "Base"},
			left:   {Title: "A", Available: true, InSales: true, OfferID: offerID("a")},
			right:  {Title: "B", Available: true, InSales: true, OfferID: offerID("b")},
		},
	}
	m.add(contract.CandidateKey{Signal: "catalog"}, left)
	m.add(contract.CandidateKey{Signal: "catalog"}, right)
	m.add(contract.CandidateKey{Signal: "sales"}, left)
	m.add(contract.CandidateKey{Signal: "sales"}, right)
	engine := New(m)
	engine.UseReranker(orderFunc(func(_ context.Context, _ []string, items []RerankItem) ([]domain.ItemID, error) {
		return []domain.ItemID{items[1].ID, items[0].ID}, nil
	}))
	online, err := engine.Recommend(context.Background(), contract.RecommendationQuery{ClientID: "u", Mode: contract.Online})
	if err != nil {
		t.Fatal(err)
	}
	if got := itemIDs(online); !reflect.DeepEqual(got, []domain.ItemID{right, left}) {
		t.Fatalf("reranked cards: %v", got)
	}
	back, err := engine.Recommend(context.Background(), contract.RecommendationQuery{ClientID: "u", Mode: contract.Backtest, SnapshotID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if got := itemIDs(back); !reflect.DeepEqual(got, []domain.ItemID{left, right}) {
		t.Fatalf("backtest cards: %v", got)
	}
	engine.UseReranker(orderFunc(func(context.Context, []string, []RerankItem) ([]domain.ItemID, error) {
		return nil, errors.New("model down")
	}))
	online, err = engine.Recommend(context.Background(), contract.RecommendationQuery{ClientID: "u", Mode: contract.Online})
	if err != nil {
		t.Fatal(err)
	}
	if got := itemIDs(online); !reflect.DeepEqual(got, []domain.ItemID{left, right}) {
		t.Fatalf("fallback cards: %v", got)
	}
}

func TestOnlineRerankRunsOnceAfterPoolExpansion(t *testing.T) {
	m := &memory{
		client:   domain.Client{ID: "u", Group: domain.B2B},
		cutoff:   time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		sets:     map[contract.CandidateKey][]domain.Candidate{},
		products: map[domain.ItemID]Product{},
	}
	for i := 0; i < 65; i++ {
		id := domain.ItemID(fmt.Sprintf("item-%02d", i))
		m.products[id] = Product{Title: string(id), Available: i < 2 || i == 64, OfferID: offerID(string(id))}
		m.add(contract.CandidateKey{Signal: "catalog"}, id)
	}
	calls := 0
	engine := New(m)
	engine.UseReranker(orderFunc(func(_ context.Context, _ []string, items []RerankItem) ([]domain.ItemID, error) {
		calls++
		got := make([]domain.ItemID, len(items))
		for i, item := range items {
			got[i] = item.ID
		}
		return got, nil
	}))
	result, err := engine.Recommend(context.Background(), contract.RecommendationQuery{ClientID: "u", Mode: contract.Online, Limit: 3})
	if err != nil || len(result.Items) != 3 || calls != 1 {
		t.Fatalf("result has %d items, %d reranks: %v", len(result.Items), calls, err)
	}
}

func TestOnlineDoesNotRecommendOwnedOfferUnderAnotherItemID(t *testing.T) {
	reader := &memory{
		client:  domain.Client{ID: "u", Group: domain.B2B},
		cutoff:  time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		history: []domain.Sale{{ItemID: "owned"}},
		products: map[domain.ItemID]Product{
			"owned": {OfferID: offerID("same-offer")},
			"alias": {OfferID: offerID("same-offer"), InSales: true, Available: true},
			"other": {OfferID: offerID("other-offer"), InSales: true, Available: true},
		},
		sets: map[contract.CandidateKey][]domain.Candidate{},
	}
	for _, signal := range []string{"catalog", "sales"} {
		reader.add(contract.CandidateKey{Signal: signal}, "alias")
		reader.add(contract.CandidateKey{Signal: signal}, "other")
	}
	engine := New(reader)
	online, err := engine.Recommend(context.Background(), contract.RecommendationQuery{ClientID: "u", Mode: contract.Online})
	if err != nil || !reflect.DeepEqual(itemIDs(online), []domain.ItemID{"other"}) {
		t.Fatalf("online cards: %v, %v", itemIDs(online), err)
	}
	historical, err := engine.Recommend(context.Background(), contract.RecommendationQuery{ClientID: "u", Mode: contract.Backtest, SnapshotID: "s"})
	if err != nil || !reflect.DeepEqual(itemIDs(historical), []domain.ItemID{"alias", "other"}) {
		t.Fatalf("historical cards: %v, %v", itemIDs(historical), err)
	}
}

func TestOnlineHidesRenewalAndKeepsUpgrade(t *testing.T) {
	base := domain.ItemID("base")
	renew, upgrade := domain.ItemID("renew"), domain.ItemID("upgrade")
	m := &memory{
		client:  domain.Client{ID: "u", Group: domain.B2B},
		cutoff:  time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		history: []domain.Sale{{ItemID: base}},
		sets:    map[contract.CandidateKey][]domain.Candidate{},
		products: map[domain.ItemID]Product{
			base:    {Title: "Base"},
			renew:   {Title: "Renew", Available: true, InSales: true, OfferID: offerID("renew")},
			upgrade: {Title: "Upgrade", Available: true, InSales: true, OfferID: offerID("upgrade")},
		},
	}
	m.add(contract.CandidateKey{Signal: "renewal", SourceItemID: base}, renew)
	m.add(contract.CandidateKey{Signal: "upgrade", SourceItemID: base}, upgrade)
	online, err := New(m).Recommend(context.Background(), contract.RecommendationQuery{ClientID: "u", Mode: contract.Online})
	if err != nil {
		t.Fatal(err)
	}
	if got := itemIDs(online); !reflect.DeepEqual(got, []domain.ItemID{upgrade}) || online.Items[0].Kind != domain.Upgrade {
		t.Fatalf("online cards: %+v", online.Items)
	}
	for _, item := range online.Items {
		if item.Kind == domain.Renewal {
			t.Fatal(item)
		}
	}
	back, err := New(m).Recommend(context.Background(), contract.RecommendationQuery{ClientID: "u", Mode: contract.Backtest, SnapshotID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if got := itemIDs(back); len(got) == 0 || got[0] != renew {
		t.Fatalf("backtest cards: %v", got)
	}
}

func TestOnlineAndBacktestCardLimits(t *testing.T) {
	m := &memory{
		client:   domain.Client{ID: "u", Group: domain.B2C},
		cutoff:   time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		history:  []domain.Sale{{ItemID: "base"}},
		sets:     map[contract.CandidateKey][]domain.Candidate{},
		products: map[domain.ItemID]Product{"base": {}},
	}
	for i := range 40 {
		id := domain.ItemID("candidate-" + strconv.Itoa(i))
		m.products[id] = Product{Available: true, InSales: true, OfferID: offerID(string(id))}
		m.add(contract.CandidateKey{Signal: "catalog"}, id)
		m.add(contract.CandidateKey{Signal: "sales"}, id)
	}
	online, err := New(m).Recommend(context.Background(), contract.RecommendationQuery{ClientID: "u", Mode: contract.Online})
	if err != nil {
		t.Fatal(err)
	}
	if len(online.Items) != 36 || online.PartialReason != nil {
		t.Fatalf("online length: %d %v", len(online.Items), online.PartialReason)
	}
	back, err := New(m).Recommend(context.Background(), contract.RecommendationQuery{ClientID: "u", Mode: contract.Backtest, SnapshotID: "s", Limit: 36})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Items) != 3 || back.PartialReason != nil {
		t.Fatalf("backtest length: %d %v", len(back.Items), back.PartialReason)
	}
}

func TestColdStartUsesOnlyTheClientGroup(t *testing.T) {
	industry := "Другое"
	other := "Банки"
	m := &memory{
		client:   domain.Client{ID: "u", Group: domain.B2B, Industry: &industry},
		cutoff:   time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		sets:     map[contract.CandidateKey][]domain.Candidate{},
		products: map[domain.ItemID]Product{},
	}
	m.products["group"] = Product{Title: "Group hit", Available: true, OfferID: offerID("group")}
	m.products["industry"] = Product{Title: "Industry hit", Available: true, OfferID: offerID("industry")}
	m.add(contract.CandidateKey{Signal: "popularity", ClientGroup: domain.B2B}, "group")
	m.add(contract.CandidateKey{Signal: "popularity", ClientGroup: domain.B2B, Industry: other}, "industry")
	online, err := New(m).Recommend(context.Background(), contract.RecommendationQuery{ClientID: "u", Mode: contract.Online})
	if err != nil {
		t.Fatal(err)
	}
	if got := itemIDs(online); !reflect.DeepEqual(got, []domain.ItemID{"group"}) || online.PartialReason == nil {
		t.Fatalf("cold start: %+v", online.Items)
	}
	for _, edge := range online.Graph.Edges {
		if strings.HasPrefix(edge.From, "item:") {
			t.Fatalf("cold start invented a purchase: %+v", edge)
		}
	}
	m.client.Group = domain.B2C
	m.products["b2c"] = Product{Title: "B2C hit", Available: true, OfferID: offerID("b2c")}
	m.add(contract.CandidateKey{Signal: "popularity", ClientGroup: domain.B2C}, "b2c")
	online, err = New(m).Recommend(context.Background(), contract.RecommendationQuery{ClientID: "u", Mode: contract.Online})
	if err != nil {
		t.Fatal(err)
	}
	if got := itemIDs(online); !reflect.DeepEqual(got, []domain.ItemID{"b2c"}) {
		t.Fatalf("b2c cold start: %v", got)
	}
}

type similarReader struct {
	*memory
	rows  []domain.Candidate
	err   error
	calls int
}

func (s *similarReader) SimilarItems(context.Context, string, []domain.ItemID, int) ([]domain.Candidate, error) {
	s.calls++
	return s.rows, s.err
}

func TestSimilarItemsStayOnlineAndOptional(t *testing.T) {
	m := &memory{
		client:  domain.Client{ID: "u", Group: domain.B2B},
		cutoff:  time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		history: []domain.Sale{{ItemID: "owned"}},
		sets:    map[contract.CandidateKey][]domain.Candidate{},
		products: map[domain.ItemID]Product{
			"owned":   {Title: "Owned"},
			"popular": {Title: "Popular", Available: true, InSales: true, OfferID: offerID("popular")},
			"near":    {Title: "Near", Available: true, InSales: true, OfferID: offerID("near")},
		},
	}
	m.add(contract.CandidateKey{Signal: "catalog"}, "popular")
	m.add(contract.CandidateKey{Signal: "sales"}, "popular")
	reader := &similarReader{memory: m, rows: []domain.Candidate{{
		ItemID: "near", Score: 1,
		Evidence: []domain.Evidence{{ID: "sim", From: "cohort:similar", To: "item:near", Type: "similar", Reason: "Похоже по каталогу", Source: "catalog_embedding", SnapshotID: "s"}},
	}}}
	online, err := New(reader).Recommend(context.Background(), contract.RecommendationQuery{ClientID: "u", Mode: contract.Online})
	if err != nil {
		t.Fatal(err)
	}
	if got := itemIDs(online); !reflect.DeepEqual(got, []domain.ItemID{"near", "popular"}) || reader.calls == 0 {
		t.Fatalf("online similar: %v calls=%d", got, reader.calls)
	}
	reader.calls = 0
	back, err := New(reader).Recommend(context.Background(), contract.RecommendationQuery{ClientID: "u", Mode: contract.Backtest, SnapshotID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if got := itemIDs(back); !reflect.DeepEqual(got, []domain.ItemID{"popular"}) || reader.calls != 0 {
		t.Fatalf("backtest similar: %v calls=%d", got, reader.calls)
	}
	reader.err = errors.New("index down")
	online, err = New(reader).Recommend(context.Background(), contract.RecommendationQuery{ClientID: "u", Mode: contract.Online})
	if err != nil {
		t.Fatal(err)
	}
	if got := itemIDs(online); !reflect.DeepEqual(got, []domain.ItemID{"popular"}) {
		t.Fatalf("similar failure: %v", got)
	}
}
