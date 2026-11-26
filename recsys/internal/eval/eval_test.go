package eval

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"recsys/internal/contract"
	"recsys/internal/data"
	"recsys/internal/domain"
)

type stubRecommender struct {
	responses map[domain.ClientID]contract.RecommendationResult
	queries   []contract.RecommendationQuery
	err       error
}

func (s *stubRecommender) Recommend(_ context.Context, query contract.RecommendationQuery) (contract.RecommendationResult, error) {
	s.queries = append(s.queries, query)
	if s.err != nil {
		return contract.RecommendationResult{}, s.err
	}
	if res, ok := s.responses[query.ClientID]; ok {
		return res, nil
	}
	return contract.RecommendationResult{SnapshotID: query.SnapshotID}, nil
}

type stubDatasetWriter struct {
	version string
	batches []contract.DatasetBatch
}

func (w *stubDatasetWriter) WriteDataset(_ context.Context, version string, batches []contract.DatasetBatch) error {
	w.version = version
	w.batches = append([]contract.DatasetBatch(nil), batches...)
	return nil
}

func assertNear(t *testing.T, got, want float64, label string) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("%s: got %.6f, want %.6f", label, got, want)
	}
}

func strPtr(v string) *string { return &v }

func TestDataModule(t *testing.T) {
	t.Run("CohortsAAndBManualFractions", func(t *testing.T) {
		// Hand-crafted set:
		// c1 (B2B, lk_access=false): train={i1}, test={i2, i3} -> A & B
		// c2 (B2B, lk_access=true):  train={i1, i4}, test={i3} -> A only (2 train items)
		// c3 (B2C, lk_access=false): train={i1}, test={i2} -> A & B
		// c4 (B2B): train={i1}, test={i1} -> excluded from A (no new item)
		// c5 (B2C): train={}, test={i2} -> excluded from A (no train history)
		// Peer train baskets: (o_p1, 2026-08-24)={i1,i2}, (o_p2, 2026-08-24)={i1,i3}, (o_p3, 2026-08-24)={i4,i5}
		// Train popularity: i1=5 (c1,c2,c3,c4,p1?), let's make peers explicit:
		// p1: {i1, i2}, p2: {i1, i3}, p3: {i4, i5}
		// Popularity in train: i1: 6 (c1,c2,c3,c4,p1,p2), i2: 1 (p1), i3: 1 (p2), i4: 2 (c2,p3), i5: 1 (p3)
		// For c1 (seen={i1}):
		//   pop top3 (tie at 1: i2,i3,i5): [i4(2), i2(1), i3(1)] -> bought={i2,i3} -> 2/3 hits
		//   co top3: neighbors of i1 are i2(1), i3(1) -> [i2, i3] (short list of 2!) -> 2/3 hits, Fill=100%, Full3=0%
		// For c2 (seen={i1,i4}):
		//   pop top3: [i2(1), i3(1), i5(1)] -> bought={i3} -> 1/3 hits
		//   co top3: i2(1), i3(1), i5(1) -> [i2, i3, i5] -> 1/3 hits, Fill=100%, Full3=100%
		// For c3 (seen={i1}):
		//   pop top3: [i4(2), i2(1), i3(1)] -> bought={i2} -> 1/3 hits
		//   co top3: [i2, i3] -> 1/3 hits, Fill=100%, Full3=0%
		clients := []domain.Client{
			{ID: "c1", Group: domain.B2B, HasLKAccess: false},
			{ID: "c2", Group: domain.B2B, HasLKAccess: true},
			{ID: "c3", Group: domain.B2C, HasLKAccess: false},
			{ID: "c4", Group: domain.B2B, HasLKAccess: true},
			{ID: "c5", Group: domain.B2C, HasLKAccess: true},
			{ID: "p1", Group: domain.B2B, HasLKAccess: false},
			{ID: "p2", Group: domain.B2B, HasLKAccess: false},
			{ID: "p3", Group: domain.B2B, HasLKAccess: false},
		}
		wTrain1 := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
		wTrain2 := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
		wTest := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
		sales := []domain.Sale{
			{SourceRow: 2, OrderID: "o1", ClientID: "c1", OrderWeek: wTrain1, ItemID: "i1"},
			{SourceRow: 3, OrderID: "o2a", ClientID: "c2", OrderWeek: wTrain1, ItemID: "i1"},
			{SourceRow: 4, OrderID: "o2b", ClientID: "c2", OrderWeek: wTrain2, ItemID: "i4"},
			{SourceRow: 5, OrderID: "o3", ClientID: "c3", OrderWeek: wTrain1, ItemID: "i1"},
			{SourceRow: 6, OrderID: "o4", ClientID: "c4", OrderWeek: wTrain2, ItemID: "i1"},
			{SourceRow: 7, OrderID: "op1", ClientID: "p1", OrderWeek: wTrain1, ItemID: "i1"},
			{SourceRow: 8, OrderID: "op1", ClientID: "p1", OrderWeek: wTrain1, ItemID: "i2"},
			{SourceRow: 9, OrderID: "op2", ClientID: "p2", OrderWeek: wTrain1, ItemID: "i1"},
			{SourceRow: 10, OrderID: "op2", ClientID: "p2", OrderWeek: wTrain1, ItemID: "i3"},
			{SourceRow: 11, OrderID: "op3", ClientID: "p3", OrderWeek: wTrain1, ItemID: "i4"},
			{SourceRow: 12, OrderID: "op3", ClientID: "p3", OrderWeek: wTrain1, ItemID: "i5"},
			{SourceRow: 13, OrderID: "ot1", ClientID: "c1", OrderWeek: wTest, ItemID: "i2"},
			{SourceRow: 14, OrderID: "ot1", ClientID: "c1", OrderWeek: wTest, ItemID: "i3"},
			{SourceRow: 15, OrderID: "ot2", ClientID: "c2", OrderWeek: wTest, ItemID: "i3"},
			{SourceRow: 16, OrderID: "ot3", ClientID: "c3", OrderWeek: wTest, ItemID: "i2"},
			{SourceRow: 17, OrderID: "ot4", ClientID: "c4", OrderWeek: wTest, ItemID: "i1"},
			{SourceRow: 18, OrderID: "ot5", ClientID: "c5", OrderWeek: wTest, ItemID: "i2"},
		}
		evalSales, err := SalesFromDomain(clients, sales)
		if err != nil {
			t.Fatalf("SalesFromDomain failed: %v", err)
		}
		res := Baselines(evalSales)
		// A all: 3 clients, hits = (2+1+1)/9 = 4/9 = 44.444444%
		if res[0].Cohort != "A" || res[0].Group != "all" || res[0].Clients != 3 {
			t.Fatalf("unexpected A all header: %+v", res[0])
		}
		assertNear(t, res[0].Popularity.Precision, 400.0/9.0, "A all pop precision")
		assertNear(t, res[0].Popularity.Full3Rate, 100.0, "A all pop full3")
		assertNear(t, res[0].CoPurchase.Precision, 400.0/9.0, "A all co precision")
		assertNear(t, res[0].CoPurchase.FillRate, 100.0, "A all co fill")
		assertNear(t, res[0].CoPurchase.Full3Rate, 100.0/3.0, "A all co full3")

		// A B2B: 2 clients (c1, c2), hits = (2+1)/6 = 50%
		if res[1].Clients != 2 {
			t.Fatalf("expected 2 B2B clients in A, got %d", res[1].Clients)
		}
		assertNear(t, res[1].CoPurchase.Precision, 50.0, "A B2B co precision")
		assertNear(t, res[1].CoPurchase.Full3Rate, 50.0, "A B2B co full3")

		// A B2C: 1 client (c3, lk_access=false retained), hits = 1/3 = 33.333333%
		if res[2].Clients != 1 {
			t.Fatalf("expected 1 B2C client in A, got %d", res[2].Clients)
		}
		assertNear(t, res[2].CoPurchase.Precision, 100.0/3.0, "A B2C co precision")

		// B all: 2 clients (c1, c3), hits = (2+1)/6 = 50%, co full3 = 0%
		if res[3].Cohort != "B" || res[3].Clients != 2 {
			t.Fatalf("expected 2 clients in B all, got %+v", res[3])
		}
		assertNear(t, res[3].CoPurchase.Precision, 50.0, "B all co precision")
		assertNear(t, res[3].CoPurchase.Full3Rate, 0.0, "B all co full3")
	})

	t.Run("EmptyListAndZeroNeighbors", func(t *testing.T) {
		sales := []Sale{
			{OrderID: "o1", ClientID: "solo", Week: "2026-08-31", ItemID: "only_item", Group: "B2C"},
			{OrderID: "o2", ClientID: "solo", Week: "2026-09-07", ItemID: "future_item", Group: "B2C"},
		}
		res := Baselines(sales)
		// solo has no co-purchase neighbors -> empty list -> Precision=0, FillRate=0, Full3Rate=0
		// popularity also has no other train items -> empty list!
		if res[0].Clients != 1 || res[3].Clients != 1 {
			t.Fatalf("expected 1 client in A and B, got %+v", res)
		}
		if res[0].CoPurchase.Precision != 0 || res[0].CoPurchase.FillRate != 0 || res[0].CoPurchase.Full3Rate != 0 {
			t.Fatalf("expected all-zero metrics for empty recommendation list, got %+v", res[0].CoPurchase)
		}
	})

	t.Run("EmptySKUAndBoundaryDates", func(t *testing.T) {
		clients := []domain.Client{{ID: "c1", Group: domain.B2B, HasLKAccess: false}, {ID: "p1", Group: domain.B2B}}
		wOnBoundary := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
		wAfterBoundary := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
		wNextMonday := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
		sales := []domain.Sale{
			{SourceRow: 2, OrderID: "o1", ClientID: "c1", OrderWeek: wOnBoundary, ItemID: "N-nosku", SKU: nil},
			{SourceRow: 3, OrderID: "o2", ClientID: "p1", OrderWeek: wOnBoundary, ItemID: "N-nosku", SKU: nil},
			{SourceRow: 4, OrderID: "o2", ClientID: "p1", OrderWeek: wOnBoundary, ItemID: "N-target", SKU: nil},
			{SourceRow: 5, OrderID: "o3", ClientID: "c1", OrderWeek: wAfterBoundary, ItemID: "N-target", SKU: nil},
			{SourceRow: 6, OrderID: "o4", ClientID: "c1", OrderWeek: wNextMonday, ItemID: "N-later", SKU: nil},
		}
		evalSales, err := SalesFromDomain(clients, sales)
		if err != nil {
			t.Fatalf("SalesFromDomain failed: %v", err)
		}
		res := Baselines(evalSales)
		// N-target bought on 2026-09-01 (> 2026-08-31) is in test and matched by co-purchase with N-nosku on 2026-08-31.
		assertNear(t, res[0].CoPurchase.Precision, 100.0/3.0, "boundary hit on empty SKU item")
	})

	t.Run("SameOrderIDDifferentWeeksAndUniqueBuyers", func(t *testing.T) {
		// Peer p1 has SUB-1 across two weeks: (SUB-1, 2026-08-24)={a, b}, (SUB-1, 2026-08-31)={c}.
		// c1 buys {a} in train and {b, c} in test.
		// Because baskets split by (order_id, order_week), only b co-occurs with a; c must NOT co-occur with a!
		// Also p2 buys {c} twice in two different orders: popularity[c] must be 2 (p1 and p2), not 3.
		sales := []Sale{
			{OrderID: "SUB-1", ClientID: "p1", Week: "2026-08-24", ItemID: "a", Group: "B2B"},
			{OrderID: "SUB-1", ClientID: "p1", Week: "2026-08-24", ItemID: "b", Group: "B2B"},
			{OrderID: "SUB-1", ClientID: "p1", Week: "2026-08-31", ItemID: "c", Group: "B2B"},
			{OrderID: "O-p2-1", ClientID: "p2", Week: "2026-08-17", ItemID: "c", Group: "B2B"},
			{OrderID: "O-p2-2", ClientID: "p2", Week: "2026-08-24", ItemID: "c", Group: "B2B"},
			{OrderID: "O-p3", ClientID: "p3", Week: "2026-08-24", ItemID: "b", Group: "B2B"},
			{OrderID: "O-c1-train", ClientID: "c1", Week: "2026-08-31", ItemID: "a", Group: "B2B"},
			{OrderID: "O-c1-test", ClientID: "c1", Week: "2026-09-07", ItemID: "b", Group: "B2B"},
			{OrderID: "O-c1-test", ClientID: "c1", Week: "2026-09-07", ItemID: "c", Group: "B2B"},
		}
		res := Baselines(sales)
		// For c1: co-purchase recommends ONLY [b] (1/3 hit), NOT c!
		assertNear(t, res[0].CoPurchase.Precision, 100.0/3.0, "same order_id across weeks must not link")
		// Popularity among unseen items for c1: b has 2 unique buyers (p1, p3), c has 2 unique buyers (p1, p2) -> tie broken by item_id: [b, c] -> 2/3 hits.
		assertNear(t, res[0].Popularity.Precision, 200.0/3.0, "unique buyers popularity")
	})

	t.Run("DeterministicUnderRowPermutation", func(t *testing.T) {
		sales := []Sale{
			{OrderID: "o1", ClientID: "p1", Week: "2026-08-24", ItemID: "seed", Group: "B2B"},
			{OrderID: "o1", ClientID: "p1", Week: "2026-08-24", ItemID: "z_item", Group: "B2B"},
			{OrderID: "o2", ClientID: "p2", Week: "2026-08-24", ItemID: "seed", Group: "B2B"},
			{OrderID: "o2", ClientID: "p2", Week: "2026-08-24", ItemID: "a_item", Group: "B2B"},
			{OrderID: "o3", ClientID: "p3", Week: "2026-08-24", ItemID: "a_item", Group: "B2B"},
			{OrderID: "o4", ClientID: "c1", Week: "2026-08-31", ItemID: "seed", Group: "B2B"},
			{OrderID: "o5", ClientID: "c1", Week: "2026-09-07", ItemID: "a_item", Group: "B2B"},
		}
		reversed := slices.Clone(sales)
		slices.Reverse(reversed)

		r1 := Baselines(sales)
		r2 := Baselines(reversed)
		if !slices.Equal(r1, r2) {
			t.Fatalf("results changed under row permutation:\n%#v\nvs\n%#v", r1, r2)
		}
	})

	t.Run("HybridRecommenderAndFamilies", func(t *testing.T) {
		wTrain := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
		wTest := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
		feedAt := time.Date(2026, 11, 26, 11, 45, 0, 0, time.UTC)

		clients := []domain.Client{
			{ID: "c1", Group: domain.B2B, HasLKAccess: false},
			{ID: "c2", Group: domain.B2C, HasLKAccess: true},
			{ID: "peer", Group: domain.B2B, HasLKAccess: false},
		}
		sales := []domain.Sale{
			{SourceRow: 2, OrderID: "o1", ClientID: "c1", OrderWeek: wTrain, ItemID: "hist-base"},
			{SourceRow: 3, OrderID: "o2", ClientID: "c2", OrderWeek: wTrain, ItemID: "hist-b2c"},
			// Peer buys warm items in train so only hist-cold is cold (absent from train).
			{SourceRow: 4, OrderID: "op1", ClientID: "peer", OrderWeek: wTrain, ItemID: "hist-renew"},
			{SourceRow: 5, OrderID: "op1", ClientID: "peer", OrderWeek: wTrain, ItemID: "hist-nosku"},
			{SourceRow: 6, OrderID: "op1", ClientID: "peer", OrderWeek: wTrain, ItemID: "hist-newfam"},
			// test purchases:
			// c1 buys: hist-renew (same family FAM-A), hist-cold (new family FAM-B, cold item!), hist-nosku (unknown family)
			{SourceRow: 7, OrderID: "o3", ClientID: "c1", OrderWeek: wTest, ItemID: "hist-renew"},
			{SourceRow: 8, OrderID: "o3", ClientID: "c1", OrderWeek: wTest, ItemID: "hist-cold"},
			{SourceRow: 9, OrderID: "o3", ClientID: "c1", OrderWeek: wTest, ItemID: "hist-nosku"},
			// c2 buys: hist-newfam (FAM-C)
			{SourceRow: 10, OrderID: "o4", ClientID: "c2", OrderWeek: wTest, ItemID: "hist-newfam"},
		}
		allowed := map[domain.ItemID]struct{}{
			"hist-base":   {},
			"hist-b2c":    {},
			"hist-renew":  {},
			"hist-cold":   {},
			"hist-nosku":  {},
			"hist-newfam": {},
		}
		families := map[domain.ItemID]string{
			"hist-base":   "FAM-A",
			"hist-renew":  "FAM-A",
			"hist-b2c":    "FAM-X",
			"hist-cold":   "FAM-B",
			"hist-newfam": "FAM-C",
		}

		rec := &stubRecommender{
			responses: map[domain.ClientID]contract.RecommendationResult{
				"c1": {
					SnapshotID: "snap-1",
					Items: []domain.Recommendation{
						// Slot 1: hit on same family (with extra variant that must NOT become slot 4!)
						{
							ItemID: "hist-renew", FamilyID: strPtr("FAM-A"),
							Variants: []domain.Variant{{ItemID: "hist-nosku"}},
						},
						// Slot 2: feed item outside 817 allowed sales items -> occupies slot and counts as miss!
						{ItemID: "feed:999999", FamilyID: strPtr("FAM-B")},
						// Slot 3: cold item hit with new family
						{ItemID: "hist-cold", FamilyID: strPtr("FAM-B")},
						// 4th card returned by buggy recommender must be truncated
						{ItemID: "hist-nosku"},
					},
				},
				"c2": {
					SnapshotID: "snap-1",
					Items: []domain.Recommendation{
						// Unknown family hit (FamilyID == nil and not in ItemFamilies)
						{ItemID: "hist-nosku"}, // miss for c2
						{ItemID: "hist-newfam", FamilyID: nil},
					},
				},
			},
		}
		// Remove hist-newfam from ItemFamilies to test unknown family classification on c2's hit while keeping c1's hits known!
		delete(families, "hist-newfam")

		bt := MemoryBacktester{Clients: clients, Sales: sales}
		report, err := bt.Run(context.Background(), rec, BacktestConfig{
			DatasetLabel:    "v4-test",
			SnapshotID:      "snap-1",
			AllowedItems:    allowed,
			ItemFamilies:    families,
			FeedGeneratedAt: &feedAt,
		})
		if err != nil {
			t.Fatalf("EvaluateRecommender failed: %v", err)
		}
		if len(rec.queries) != 2 || rec.queries[0].Mode != contract.Backtest || rec.queries[0].SnapshotID != "snap-1" {
			t.Fatalf("unexpected queries sent to recommender: %+v", rec.queries)
		}
		// c1: 2 hits out of 3 slots (feed:999999 missed; 4th card ignored); c2: 1 hit out of 2 items (short list, denom=3)
		// Total A all: (2 + 1) / (3 * 2) = 50%, FillRate = 100%, Full3Rate = 50% (c1 had 3 top cards, c2 had 2)
		allRes := report.Results[0]
		assertNear(t, allRes.Metrics.Precision, 50.0, "hybrid A all precision")
		assertNear(t, allRes.Metrics.FillRate, 100.0, "hybrid A all fill rate")
		assertNear(t, allRes.Metrics.Full3Rate, 50.0, "hybrid A all full3 rate")

		// Hits breakdown across A all: total=3 (hist-renew -> known, hist-cold -> new, hist-newfam -> unknown)
		if allRes.Families.TotalHits != 3 ||
			allRes.Families.KnownFamilyHits != 1 ||
			allRes.Families.NewFamilyHits != 1 ||
			allRes.Families.UnknownFamilyHits != 1 {
			t.Fatalf("unexpected family breakdown: %+v", allRes.Families)
		}
		assertNear(t, allRes.Families.KnownFamilyShare()+allRes.Families.NewFamilyShare()+allRes.Families.UnknownFamilyShare(), 100.0, "family shares sum")

		if len(report.Cold.ColdItems) != 1 || report.Cold.ColdItems[0] != "hist-cold" ||
			report.Cold.CohortATargets != 1 || report.Cold.CohortAHits != 1 {
			t.Fatalf("unexpected cold stats: %+v", report.Cold)
		}
		if report.FeedGeneratedAt == nil || len(report.LeakageWarnings) == 0 {
			t.Fatalf("expected feed timestamp and leakage warnings")
		}

		// When canonical v5 ProductGroups are supplied, they take precedence over card.FamilyID / nil.
		pgReport, err := bt.Run(context.Background(), rec, BacktestConfig{
			DatasetLabel: "v5-test",
			SnapshotID:   "snap-1",
			AllowedItems: allowed,
			ProductGroups: map[domain.ItemID]string{
				"hist-base":   "G-A",
				"hist-renew":  "G-A",
				"hist-b2c":    "G-X",
				"hist-cold":   "G-B",
				"hist-newfam": "G-C",
			},
		})
		if err != nil {
			t.Fatalf("EvaluateRecommender with v5 ProductGroups failed: %v", err)
		}
		pgFam := pgReport.Results[0].Families
		if pgFam.TotalHits != 3 || pgFam.KnownFamilyHits != 1 || pgFam.NewFamilyHits != 2 || pgFam.UnknownFamilyHits != 0 {
			t.Fatalf("expected 1 known-family variant and 2 new-product hits via canonical ProductGroups, got %+v", pgFam)
		}

		// Recommender errors must propagate.
		rec.err = errors.New("snapshot missing")
		if _, err := bt.Run(context.Background(), rec, BacktestConfig{SnapshotID: "snap-1"}); err == nil {
			t.Fatalf("expected error from failing recommender")
		}
	})

	t.Run("SmokeFixtureImportAndEval", func(t *testing.T) {
		ctx := context.Background()
		ds, err := data.LoadCSV(
			ctx,
			filepath.Join("..", "..", "testdata", "smoke", "clients.csv"),
			filepath.Join("..", "..", "testdata", "smoke", "sales.csv"),
		)
		if err != nil {
			t.Fatalf("LoadCSV smoke failed: %v", err)
		}
		writer := &stubDatasetWriter{}
		batches := data.BuildBatches(ds, 10)
		if err := writer.WriteDataset(ctx, "demo", batches); err != nil {
			t.Fatalf("WriteDataset failed: %v", err)
		}
		if len(ds.Clients) != 2 || len(ds.Sales) != 1 || writer.version != "demo" {
			t.Fatalf("unexpected smoke import: clients=%d sales=%d version=%q", len(ds.Clients), len(ds.Sales), writer.version)
		}

		evalSales, err := SalesFromDomain(ds.Clients, ds.Sales)
		if err != nil {
			t.Fatalf("SalesFromDomain failed: %v", err)
		}
		summary := SummarizeDataset(evalSales)
		if summary.Rows != 1 || summary.Baskets != 1 || summary.TrainItems != 1 || summary.CohortAClients != 0 {
			t.Fatalf("unexpected smoke summary: %+v", summary)
		}
	})

	t.Run("V5ProductGroupHitSplit", func(t *testing.T) {
		// Peer p1 links base_v1 with base_v2 (same product_group G100) and new_prod (G200).
		// Target c1 buys base_v1 in train, then buys base_v2 (variant hit) and new_prod (new product hit) in test.
		sales := []Sale{
			{OrderID: "op1", ClientID: "p1", Week: "2026-08-24", ItemID: "base_v1", Group: "B2B"},
			{OrderID: "op1", ClientID: "p1", Week: "2026-08-24", ItemID: "base_v2", Group: "B2B"},
			{OrderID: "op1", ClientID: "p1", Week: "2026-08-24", ItemID: "new_prod", Group: "B2B"},
			{OrderID: "oc1", ClientID: "c1", Week: "2026-08-31", ItemID: "base_v1", Group: "B2B"},
			{OrderID: "oc2", ClientID: "c1", Week: "2026-09-07", ItemID: "base_v2", Group: "B2B"},
			{OrderID: "oc2", ClientID: "c1", Week: "2026-09-07", ItemID: "new_prod", Group: "B2B"},
		}
		pgroups := map[string]string{
			"base_v1":  "G100",
			"base_v2":  "G100",
			"new_prod": "G200",
		}
		summary := SummarizeDatasetWithGroups(sales, pgroups)
		if summary.TotalItems != 3 || summary.ProductGroups != 2 {
			t.Fatalf("expected 3 items and 2 product_groups, got %+v", summary)
		}
		res := BaselinesWithGroups(sales, pgroups)[0]
		if res.CoPurchase.Hits != 2 || res.CoPurchase.VariantHits != 1 || res.CoPurchase.NewProductHits != 1 {
			t.Fatalf("expected 2 hits (1 variant, 1 new), got %+v", res.CoPurchase)
		}
		assertNear(t, res.CoPurchase.VariantShare, 50.0, "variant share %")
	})
}
