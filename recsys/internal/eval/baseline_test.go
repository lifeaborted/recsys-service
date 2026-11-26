package eval

import (
	"math"
	"slices"
	"testing"
)

func TestWeeklyBasketsAndShortList(t *testing.T) {
	sales := []Sale{
		{"one", "target", "2026-08-31", "base", "B2B"},
		{"sub", "peer", "2026-08-24", "base", "B2B"},
		{"sub", "peer", "2026-08-24", "variant", "B2B"},
		{"sub", "peer", "2026-08-24", "variant", "B2B"},
		{"sub", "peer", "2026-08-31", "unrelated", "B2B"},
		{"sub", "peer", "2026-09-07", "future", "B2B"},
		{"two", "target", "2026-09-07", "variant", "B2B"},
		{"two", "target", "2026-09-07", "unrelated", "B2B"},
	}
	result := Baselines(sales)[3]
	if result.Clients != 1 || result.CoPurchase.FillRate != 100 || math.Abs(result.CoPurchase.Precision-100.0/3) > 1e-9 {
		t.Fatalf("expected one hit in a short list for cohort B, got %+v", result)
	}
}

func TestTieBreakByPopularityThenItemID(t *testing.T) {
	scores := map[string]int{"a": 3, "b": 3, "c": 3, "seen": 5, "lower": 2}
	pop := map[string]int{"a": 1, "b": 2, "c": 2, "lower": 9}
	got := topItems(scores, pop, itemSet{"seen": {}})
	if !slices.Equal(got, []string{"b", "c", "a"}) {
		t.Fatalf("got %v", got)
	}
}
