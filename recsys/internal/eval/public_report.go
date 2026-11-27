package eval

import (
	"fmt"
	"math"
	"reflect"
	"strings"
)

const (
	// PublicVisitGapNote explains the difference between unengaged visits and offline Top-K metrics.
	PublicVisitGapNote = "Около 55% визитов уходят без целевого действия; это доля визитов без целевого действия в поведении, а не Precision@3 и не Fill Rate протокола."
)

// PublicProtocolNotes explicitly documents evaluation boundaries inside JSON outputs.
type PublicProtocolNotes struct {
	CutoffRule        string `json:"cutoff_rule"`
	BasketKey         string `json:"basket_key"`
	DenominatorScope  string `json:"denominator_scope"`
	MetricTopK        int    `json:"metric_top_k"`
	TopKScope         string `json:"top_k_scope"`
	SalesUniverseOnly bool   `json:"sales_universe_only"`
	FamilyBasis       string `json:"family_basis"`
}

// PublicSummary contains aggregate dataset counts without any client_id, item_id lists, or raw rows.
type PublicSummary struct {
	Rows           int `json:"rows"`
	UniqueClients  int `json:"unique_clients"`
	Baskets        int `json:"baskets"`
	TotalItems     int `json:"total_items"`
	ProductGroups  int `json:"product_groups"`
	TrainItems     int `json:"train_items"`
	ColdItemsCount int `json:"cold_items_count"`
	CohortAClients int `json:"cohort_a_clients"`
	CohortAB2B     int `json:"cohort_a_b2b_clients"`
	CohortAB2C     int `json:"cohort_a_b2c_clients"`
	CohortBClients int `json:"cohort_b_clients"`
	CohortBB2B     int `json:"cohort_b_b2b_clients"`
	CohortBB2C     int `json:"cohort_b_b2c_clients"`
}

// PublicMethodMetrics stores Top-3 accuracy, coverage, and product_group hit split.
// Pointer fields preserve explicit JSON null when an aggregate sub-cell was not published.
type PublicMethodMetrics struct {
	PrecisionAt3      *float64 `json:"precision_at_3"`
	FillRate          *float64 `json:"fill_rate"`
	Full3Rate         *float64 `json:"full3_rate"`
	Hits              *int     `json:"hits"`
	VariantHits       *int     `json:"variant_hits"`
	NewProductHits    *int     `json:"new_product_hits"`
	UnknownFamilyHits *int     `json:"unknown_family_hits"`
	VariantShare      *float64 `json:"variant_share"`
}

// PublicCell holds popularity, copurchase, and hybrid metrics for one cohort x client_group slice.
type PublicCell struct {
	Dataset         string               `json:"dataset,omitempty"`
	PublicAggregate bool                 `json:"public_aggregate,omitempty"`
	Demo            bool                 `json:"demo,omitempty"`
	Cohort          string               `json:"cohort"`
	Group           string               `json:"group"`
	Clients         int                  `json:"clients"`
	Popularity      *PublicMethodMetrics `json:"popularity"`
	CoPurchase      *PublicMethodMetrics `json:"copurchase"`
	Hybrid          *PublicMethodMetrics `json:"hybrid"`
}

// PublicMetricsReport is the sanitized single-JSON output emitted by cmd/backtest -format json.
type PublicMetricsReport struct {
	Dataset               string              `json:"dataset"`
	PublicAggregate       bool                `json:"public_aggregate,omitempty"`
	Demo                  bool                `json:"demo,omitempty"`
	SnapshotID            *string             `json:"snapshot_id"`
	WorkbookSHA256        string              `json:"workbook_sha256,omitempty"`
	HasProductGroupColumn bool                `json:"has_product_group_column"`
	TrainThrough          string              `json:"train_through"`
	TestFrom              string              `json:"test_from"`
	FeedGeneratedAt       string              `json:"feed_generated_at,omitempty"`
	Protocol              PublicProtocolNotes `json:"protocol"`
	Summary               PublicSummary       `json:"summary"`
	Cells                 []PublicCell        `json:"cells"`
	VisitGap              string              `json:"visit_gap,omitempty"`
	HybridStatus          string              `json:"hybrid_status"`
	LeakageWarnings       []string            `json:"leakage_warnings,omitempty"`
}

// PublicMetricsDocument packages the published v5 public aggregates alongside a synthetic demo example.
type PublicMetricsDocument struct {
	Dataset               string               `json:"dataset"`
	PublicAggregate       bool                 `json:"public_aggregate"`
	SnapshotID            string               `json:"snapshot_id"`
	SourceNotes           string               `json:"source_notes"`
	HasProductGroupColumn bool                 `json:"has_product_group_column"`
	TrainThrough          string               `json:"train_through"`
	TestFrom              string               `json:"test_from"`
	Protocol              PublicProtocolNotes  `json:"protocol"`
	Summary               PublicSummary        `json:"summary"`
	VisitGap              string               `json:"visit_gap"`
	Cells                 []PublicCell         `json:"cells"`
	Synthetic             *PublicMetricsReport `json:"synthetic,omitempty"`
}

// BuildPublicReport constructs a sanitized public JSON report without client IDs, raw rows, cold item IDs, or explanation texts.
func BuildPublicReport(
	datasetLabel, workbookSHA, feedGeneratedAt string,
	hasProductGroupCol bool,
	summary DatasetSummary,
	baselines []Result,
	hybrid *HybridReport,
	hybridStatus string,
	leakageWarnings []string,
	demo bool,
) PublicMetricsReport {
	var snapID *string
	if hybrid != nil && strings.TrimSpace(hybrid.SnapshotID) != "" {
		v := hybrid.SnapshotID
		snapID = &v
	}

	familyBasis := "product_group"
	if !hasProductGroupCol {
		familyBasis = "item_id_fallback"
	}

	baselineByKey := make(map[string]Result, len(baselines))
	for _, r := range baselines {
		baselineByKey[r.Cohort+"|"+r.Group] = r
	}
	hybridByKey := make(map[string]HybridCohortResult)
	if hybrid != nil {
		for _, r := range hybrid.Results {
			hybridByKey[r.Cohort+"|"+r.Group] = r
		}
	}

	order := [][2]string{
		{"A", "all"}, {"A", "B2B"}, {"A", "B2C"},
		{"B", "all"}, {"B", "B2B"}, {"B", "B2C"},
	}

	cells := make([]PublicCell, 0, len(order))
	for _, pair := range order {
		key := pair[0] + "|" + pair[1]
		b := baselineByKey[key]
		cell := PublicCell{
			Demo:       demo,
			Cohort:     pair[0],
			Group:      pair[1],
			Clients:    b.Clients,
			Popularity: methodMetricsFromEval(b.Popularity, 0),
			CoPurchase: methodMetricsFromEval(b.CoPurchase, 0),
		}
		if h, ok := hybridByKey[key]; ok {
			cell.Hybrid = methodMetricsFromEval(h.Metrics, h.Families.UnknownFamilyHits)
			if cell.Clients == 0 {
				cell.Clients = h.Clients
			}
		}
		cells = append(cells, cell)
	}

	return PublicMetricsReport{
		Dataset:               datasetLabel,
		Demo:                  demo,
		SnapshotID:            snapID,
		WorkbookSHA256:        workbookSHA,
		HasProductGroupColumn: hasProductGroupCol,
		TrainThrough:          TrainThrough,
		TestFrom:              ">" + TrainThrough,
		FeedGeneratedAt:       feedGeneratedAt,
		Protocol: PublicProtocolNotes{
			CutoffRule:        "train order_week <= 2026-08-31; test order_week > 2026-08-31",
			BasketKey:         "(order_id, order_week)",
			DenominatorScope:  "all clients in cohorts A and B (no sampling; B is a subset of A)",
			MetricTopK:        TopK,
			TopKScope:         "only the first 3 top-level recommendations count toward Precision@3 (denominator always 3 per client)",
			SalesUniverseOnly: true,
			FamilyBasis:       familyBasis,
		},
		Summary: PublicSummary{
			Rows:           summary.Rows,
			UniqueClients:  summary.UniqueClients,
			Baskets:        summary.Baskets,
			TotalItems:     summary.TotalItems,
			ProductGroups:  summary.ProductGroups,
			TrainItems:     summary.TrainItems,
			ColdItemsCount: len(summary.ColdItems),
			CohortAClients: summary.CohortAClients,
			CohortAB2B:     baselineByKey["A|B2B"].Clients,
			CohortAB2C:     baselineByKey["A|B2C"].Clients,
			CohortBClients: summary.CohortBClients,
			CohortBB2B:     baselineByKey["B|B2B"].Clients,
			CohortBB2C:     baselineByKey["B|B2C"].Clients,
		},
		Cells:           cells,
		HybridStatus:    hybridStatus,
		LeakageWarnings: append([]string(nil), leakageWarnings...),
	}
}

func methodMetricsFromEval(m Metrics, unknownHits int) *PublicMethodMetrics {
	return &PublicMethodMetrics{
		PrecisionAt3:      floatPtr(round2(m.Precision)),
		FillRate:          floatPtr(round2(m.FillRate)),
		Full3Rate:         floatPtr(round2(m.Full3Rate)),
		Hits:              intPtr(m.Hits),
		VariantHits:       intPtr(m.VariantHits),
		NewProductHits:    intPtr(m.NewProductHits),
		UnknownFamilyHits: intPtr(unknownHits),
		VariantShare:      floatPtr(round2(m.VariantShare)),
	}
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func floatPtr(v float64) *float64 { return &v }
func intPtr(v int) *int           { return &v }

// V5PublicAggregateDocument returns the verified aggregates of v5-train-20260831.
func V5PublicAggregateDocument(synthetic *PublicMetricsReport) PublicMetricsDocument {
	var synCopy *PublicMetricsReport
	if synthetic != nil {
		cp := *synthetic
		cp.Demo = true
		synCells := make([]PublicCell, len(cp.Cells))
		for i, c := range cp.Cells {
			c.Demo = true
			synCells[i] = c
		}
		cp.Cells = synCells
		synCopy = &cp
	}

	makeMetrics := func(p, fill, full3 float64, hits, variant, newProd, unknown int, varShare float64) *PublicMethodMetrics {
		return &PublicMethodMetrics{
			PrecisionAt3:      floatPtr(p),
			FillRate:          floatPtr(fill),
			Full3Rate:         floatPtr(full3),
			Hits:              intPtr(hits),
			VariantHits:       intPtr(variant),
			NewProductHits:    intPtr(newProd),
			UnknownFamilyHits: intPtr(unknown),
			VariantShare:      floatPtr(varShare),
		}
	}

	cell := func(cohort, group string, clients int, pop, co, hyb *PublicMethodMetrics) PublicCell {
		return PublicCell{
			Dataset:         "v5",
			PublicAggregate: true,
			Cohort:          cohort,
			Group:           group,
			Clients:         clients,
			Popularity:      pop,
			CoPurchase:      co,
			Hybrid:          hyb,
		}
	}

	return PublicMetricsDocument{
		Dataset:               "v5",
		PublicAggregate:       true,
		SnapshotID:            "v5-train-20260831",
		SourceNotes:           "Агрегаты закрытого прогона v5 на снимке v5-train-20260831. Исторический топ-3 использует 817 товаров продаж. Попадания по вариантам рассчитаны по product_group.",
		HasProductGroupColumn: true,
		TrainThrough:          TrainThrough,
		TestFrom:              ">" + TrainThrough,
		Protocol: PublicProtocolNotes{
			CutoffRule:        "train order_week <= 2026-08-31; test order_week > 2026-08-31",
			BasketKey:         "(order_id, order_week)",
			DenominatorScope:  "all clients in cohorts A and B (no sampling; B is a subset of A)",
			MetricTopK:        TopK,
			TopKScope:         "only the first 3 top-level recommendations count toward Precision@3 (denominator always 3 per client); online UI may show up to 36 cards without changing the metric",
			SalesUniverseOnly: true,
			FamilyBasis:       "product_group",
		},
		Summary: PublicSummary{
			Rows:           31753,
			UniqueClients:  19008,
			Baskets:        27603,
			TotalItems:     817,
			ProductGroups:  419,
			TrainItems:     813,
			ColdItemsCount: 4,
			CohortAClients: 337,
			CohortAB2B:     302,
			CohortAB2C:     35,
			CohortBClients: 172,
			CohortBB2B:     147,
			CohortBB2C:     25,
		},
		VisitGap: PublicVisitGapNote,
		Cells: []PublicCell{
			cell("A", "all", 337,
				makeMetrics(5.44, 100, 100, 55, 0, 55, 0, 0),
				makeMetrics(5.64, 92.28, 81.31, 57, 6, 51, 0, 10.53),
				makeMetrics(12.27, 100, 100, 124, 41, 83, 0, 33.06),
			),
			cell("A", "B2B", 302,
				makeMetrics(3.75, 100, 100, 34, 0, 34, 0, 0),
				makeMetrics(4.53, 91.39, 79.8, 41, 5, 36, 0, 12.2),
				makeMetrics(11.26, 100, 100, 102, 39, 63, 0, 38.24),
			),
			cell("A", "B2C", 35,
				makeMetrics(20, 100, 100, 21, 0, 21, 0, 0),
				makeMetrics(15.24, 100, 94.29, 16, 1, 15, 0, 6.25),
				makeMetrics(20.95, 100, 100, 22, 2, 20, 0, 9.09),
			),
			cell("B", "all", 172,
				makeMetrics(7.36, 100, 100, 38, 0, 38, 0, 0),
				makeMetrics(6.01, 87.21, 69.19, 31, 2, 29, 0, 6.45),
				makeMetrics(14.73, 100, 100, 76, 27, 49, 0, 35.53),
			),
			cell("B", "B2B", 147,
				makeMetrics(5.22, 100, 100, 23, 0, 23, 0, 0),
				makeMetrics(4.54, 85.03, 65.31, 20, 2, 18, 0, 10),
				makeMetrics(13.61, 100, 100, 60, 25, 35, 0, 41.67),
			),
			cell("B", "B2C", 25,
				makeMetrics(20, 100, 100, 15, 0, 15, 0, 0),
				makeMetrics(14.67, 100, 92, 11, 0, 11, 0, 0),
				makeMetrics(21.33, 100, 100, 16, 2, 14, 0, 12.5),
			),
		},
		Synthetic: synCopy,
	}
}

// ValidateV5PublicAggregateDocument checks the published figures against the verified run.
func ValidateV5PublicAggregateDocument(doc PublicMetricsDocument) error {
	if doc.Dataset != "v5" || !doc.PublicAggregate {
		return fmt.Errorf("expected dataset=v5 and public_aggregate=true, got dataset=%q public_aggregate=%t", doc.Dataset, doc.PublicAggregate)
	}
	if doc.TrainThrough != TrainThrough || doc.Protocol.MetricTopK != TopK || doc.Protocol.FamilyBasis != "product_group" {
		return fmt.Errorf("unexpected protocol metadata: train_through=%q protocol=%+v", doc.TrainThrough, doc.Protocol)
	}
	if !strings.Contains(doc.VisitGap, "55%") {
		return fmt.Errorf("visit_gap must mention 55%% visits note, got %q", doc.VisitGap)
	}
	if doc.Summary.CohortAClients != 337 || doc.Summary.CohortAB2B != 302 || doc.Summary.CohortAB2C != 35 ||
		doc.Summary.CohortBClients != 172 || doc.Summary.CohortBB2B != 147 || doc.Summary.CohortBB2C != 25 {
		return fmt.Errorf("unexpected cohort summary counts: %+v", doc.Summary)
	}
	if len(doc.Cells) != 6 {
		return fmt.Errorf("expected 6 cells, got %d", len(doc.Cells))
	}

	want := V5PublicAggregateDocument(nil)
	if doc.SnapshotID != want.SnapshotID {
		return fmt.Errorf("unexpected snapshot: %q", doc.SnapshotID)
	}
	for i, cell := range doc.Cells {
		if !reflect.DeepEqual(cell, want.Cells[i]) {
			return fmt.Errorf("cell %d (%s/%s) differs from the verified v5 run", i, cell.Cohort, cell.Group)
		}
	}

	if doc.Synthetic != nil {
		if !doc.Synthetic.Demo {
			return fmt.Errorf("synthetic section must set demo=true")
		}
		for i, sc := range doc.Synthetic.Cells {
			if !sc.Demo {
				return fmt.Errorf("synthetic cell %d must set demo=true", i)
			}
		}
	}
	return nil
}
