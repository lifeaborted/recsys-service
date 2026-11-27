package eval

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"recsys/internal/data"
)

func TestV5PublicAggregateConstants(t *testing.T) {
	synBytes, err := os.ReadFile(filepath.Join("..", "..", "testdata", "public-eval", "expected-cli-public.json"))
	if err != nil {
		t.Fatalf("read synthetic fixture report: %v", err)
	}
	var syn PublicMetricsReport
	if err := json.Unmarshal(synBytes, &syn); err != nil {
		t.Fatalf("decode synthetic report: %v", err)
	}

	canonical := V5PublicAggregateDocument(&syn)
	if err := ValidateV5PublicAggregateDocument(canonical); err != nil {
		t.Fatalf("canonical v5 report failed validation: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join("..", "..", "tests", "e2e", "data", "public-metrics.json"))
	if err != nil {
		t.Fatalf("read public-metrics.json: %v", err)
	}
	for _, forbidden := range []string{"client_id", "item_name", "\"cold_items\"", "SYN-A1", "SYN-B1", "SYN-B2"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("public-metrics.json contains forbidden identifier %q", forbidden)
		}
	}

	var doc PublicMetricsDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal public-metrics.json: %v", err)
	}
	if err := ValidateV5PublicAggregateDocument(doc); err != nil {
		t.Fatalf("public-metrics.json failed v5 public constant validation: %v", err)
	}
	if doc.Synthetic == nil || !doc.Synthetic.Demo {
		t.Fatalf("public-metrics.json must include synthetic section with demo=true")
	}

	t.Run("RejectRounding1197To12", func(t *testing.T) {
		mutated := V5PublicAggregateDocument(&syn)
		v := 12.0
		mutated.Cells[0].Hybrid.PrecisionAt3 = &v
		if err := ValidateV5PublicAggregateDocument(mutated); err == nil {
			t.Fatalf("expected validation error when hybrid A Precision@3 11.97 is replaced with 12.0")
		}
	})

	t.Run("RejectCohortASwapWithB", func(t *testing.T) {
		mutated := V5PublicAggregateDocument(&syn)
		mutated.Cells[0], mutated.Cells[3] = mutated.Cells[3], mutated.Cells[0]
		if err := ValidateV5PublicAggregateDocument(mutated); err == nil {
			t.Fatalf("expected validation error when Cohort A and Cohort B rows are swapped")
		}
	})

	t.Run("RejectV4ZeroesOrFabricatedHybridSubcell", func(t *testing.T) {
		mutated := V5PublicAggregateDocument(&syn)
		zero := 0
		mutated.Cells[1].Hybrid.VariantHits = &zero
		if err := ValidateV5PublicAggregateDocument(mutated); err == nil {
			t.Fatalf("expected validation error when unpublished B2B hybrid variant_hits is non-nil")
		}
	})
}

func TestMissingBoundaryAndTieCases(t *testing.T) {
	t.Run("SameOrderIDAcrossTwoWeeksCountsAsTwoBaskets", func(t *testing.T) {
		sales := []Sale{
			{OrderID: "SUB-X", ClientID: "p1", Week: "2026-08-24", ItemID: "a", Group: "B2B"},
			{OrderID: "SUB-X", ClientID: "p1", Week: "2026-08-24", ItemID: "b", Group: "B2B"},
			{OrderID: "SUB-X", ClientID: "p1", Week: "2026-08-31", ItemID: "a", Group: "B2B"},
			{OrderID: "SUB-X", ClientID: "p1", Week: "2026-08-31", ItemID: "c", Group: "B2B"},
			{OrderID: "O-C1-TR", ClientID: "c1", Week: "2026-08-31", ItemID: "a", Group: "B2B"},
			{OrderID: "O-C1-TE", ClientID: "c1", Week: "2026-09-07", ItemID: "b", Group: "B2B"},
			{OrderID: "O-C1-TE", ClientID: "c1", Week: "2026-09-07", ItemID: "c", Group: "B2B"},
		}
		summary := SummarizeDataset(sales)
		if summary.Baskets != 4 {
			t.Fatalf("expected 4 distinct (order_id, order_week) baskets (2 for SUB-X), got %d", summary.Baskets)
		}
		res := Baselines(sales)[0]
		assertNear(t, res.CoPurchase.Precision, 200.0/3.0, "both weeks of SUB-X contribute co-purchases")
	})

	t.Run("ExactAug31TrainVsSep07TestBoundary", func(t *testing.T) {
		sales := []Sale{
			{OrderID: "O1", ClientID: "p1", Week: "2026-08-31", ItemID: "base", Group: "B2B"},
			{OrderID: "O1", ClientID: "p1", Week: "2026-08-31", ItemID: "sep_target", Group: "B2B"},
			{OrderID: "O2", ClientID: "p2", Week: "2026-09-07", ItemID: "late_only", Group: "B2B"},
			{OrderID: "O3", ClientID: "c1", Week: "2026-08-31", ItemID: "base", Group: "B2B"},
			{OrderID: "O4", ClientID: "c1", Week: "2026-09-07", ItemID: "sep_target", Group: "B2B"},
			{OrderID: "O4", ClientID: "c1", Week: "2026-09-07", ItemID: "late_only", Group: "B2B"},
		}
		summary := SummarizeDataset(sales)
		if summary.TrainItems != 2 || len(summary.ColdItems) != 1 || summary.ColdItems[0] != "late_only" ||
			summary.CohortAClients != 1 || summary.CohortBClients != 1 {
			t.Fatalf("unexpected Aug 31 / Sep 07 summary split: %+v", summary)
		}
		res := Baselines(sales)[0]
		// Only sep_target is in train (co-occurs on 2026-08-31); late_only from 2026-09-07 must stay invisible to train.
		assertNear(t, res.CoPurchase.Precision, 100.0/3.0, "Aug 31 hit on Sep 07 target")
		assertNear(t, res.CoPurchase.FillRate, 100.0, "fill rate")
		assertNear(t, res.CoPurchase.Full3Rate, 0.0, "short 1-item list cannot reach full3")
	})

	t.Run("TieBreakScoreThenPopularityThenItemIDE2E", func(t *testing.T) {
		// Peer train baskets around "seed":
		// - z_best co-occurs twice with seed -> score=2, pop=2 -> rank #1
		// - p_pop2 co-occurs once with seed + bought solo by two others -> score=1, pop=3 -> rank #2
		// - a_tie1 co-occurs once with seed -> score=1, pop=1 -> rank #3 (wins ascending item_id over b_tie2)
		// - b_tie2 co-occurs once with seed -> score=1, pop=1 -> rank #4 (dropped from top-3)
		baseSales := []Sale{
			{OrderID: "op1", ClientID: "p1", Week: "2026-08-24", ItemID: "seed", Group: "B2B"},
			{OrderID: "op1", ClientID: "p1", Week: "2026-08-24", ItemID: "z_best", Group: "B2B"},
			{OrderID: "op2", ClientID: "p2", Week: "2026-08-24", ItemID: "seed", Group: "B2B"},
			{OrderID: "op2", ClientID: "p2", Week: "2026-08-24", ItemID: "z_best", Group: "B2B"},
			{OrderID: "op3", ClientID: "p3", Week: "2026-08-24", ItemID: "seed", Group: "B2B"},
			{OrderID: "op3", ClientID: "p3", Week: "2026-08-24", ItemID: "p_pop2", Group: "B2B"},
			{OrderID: "op4", ClientID: "p4", Week: "2026-08-24", ItemID: "p_pop2", Group: "B2B"},
			{OrderID: "op5", ClientID: "p5", Week: "2026-08-24", ItemID: "p_pop2", Group: "B2B"},
			{OrderID: "op6", ClientID: "p6", Week: "2026-08-24", ItemID: "seed", Group: "B2B"},
			{OrderID: "op6", ClientID: "p6", Week: "2026-08-24", ItemID: "b_tie2", Group: "B2B"},
			{OrderID: "op7", ClientID: "p7", Week: "2026-08-24", ItemID: "seed", Group: "B2B"},
			{OrderID: "op7", ClientID: "p7", Week: "2026-08-24", ItemID: "a_tie1", Group: "B2B"},
			{OrderID: "oc1-tr", ClientID: "c1", Week: "2026-08-31", ItemID: "seed", Group: "B2B"},
		}

		miss4th := append(append([]Sale(nil), baseSales...),
			Sale{OrderID: "oc1-te", ClientID: "c1", Week: "2026-09-07", ItemID: "b_tie2", Group: "B2B"},
		)
		if hits := Baselines(miss4th)[0].CoPurchase.Hits; hits != 0 {
			t.Fatalf("expected b_tie2 at rank #4 to lose tie to a_tie1, got hits=%d", hits)
		}

		hitTop3 := append(append([]Sale(nil), baseSales...),
			Sale{OrderID: "oc1-te", ClientID: "c1", Week: "2026-09-07", ItemID: "z_best", Group: "B2B"},
			Sale{OrderID: "oc1-te", ClientID: "c1", Week: "2026-09-07", ItemID: "p_pop2", Group: "B2B"},
			Sale{OrderID: "oc1-te", ClientID: "c1", Week: "2026-09-07", ItemID: "a_tie1", Group: "B2B"},
		)
		res := Baselines(hitTop3)[0]
		if res.CoPurchase.Hits != 3 {
			t.Fatalf("expected exact top-3 [z_best, p_pop2, a_tie1] to hit all 3, got hits=%d", res.CoPurchase.Hits)
		}
		assertNear(t, res.CoPurchase.Precision, 100.0, "end-to-end score>pop>item_id")
	})

	t.Run("V5CSVEmptySKUPreservedIntoEval", func(t *testing.T) {
		clientsCSV := "client_id,client_group,client_type,industry,segment,lk_access\nC1,B2B,ЮЛ,,,да\nP1,B2B,ЮЛ,,,да\n"
		salesCSV := "order_id,client_id,order_week,item_id,sku,item_name,product_group\n" +
			"O1,P1,2026-08-31,BASE,,Base,\n" +
			"O1,P1,2026-08-31,NEXT,,Next,G1\n" +
			"O2,C1,2026-08-31,BASE,,Base,\n" +
			"O3,C1,2026-09-07,NEXT,,Next,G1\n"
		ds, err := data.ReadCSV(context.Background(), strings.NewReader(clientsCSV), strings.NewReader(salesCSV))
		if err != nil {
			t.Fatalf("ReadCSV failed: %v", err)
		}
		for i, s := range ds.Sales {
			if s.SKU != nil {
				t.Fatalf("sale %d expected nil SKU, got %v", i, *s.SKU)
			}
		}
		evalSales, err := SalesFromDomain(ds.Clients, ds.Sales)
		if err != nil {
			t.Fatalf("SalesFromDomain failed: %v", err)
		}
		res := BaselinesWithGroups(evalSales, ProductGroupStrings(ds.ProductGroups))[0]
		if res.CoPurchase.Hits != 1 || res.CoPurchase.NewProductHits != 1 {
			t.Fatalf("expected 1 new-product hit on empty-SKU v5 items, got %+v", res.CoPurchase)
		}
	})
}
