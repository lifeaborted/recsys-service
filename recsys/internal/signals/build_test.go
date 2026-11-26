package signals

import (
	"context"
	"reflect"
	"testing"
	"time"

	"recsys/internal/contract"
	"recsys/internal/domain"
)

type capture struct {
	contract.Snapshot
	calls int
}

func (c *capture) PublishSnapshot(_ context.Context, s contract.Snapshot) error {
	c.Snapshot = s
	c.calls++
	return nil
}

func TestTrainCutoff(t *testing.T) {
	cut := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	sale := func(buyer, order string, id domain.ItemID, delta int) domain.Sale {
		return domain.Sale{ClientID: domain.ClientID(buyer), OrderID: order, ItemID: id, OrderWeek: cut.AddDate(0, 0, delta)}
	}
	industry := "IT"
	data := contract.DatasetBatch{Clients: []domain.Client{{ID: "u", Group: domain.B2B, Industry: &industry}, {ID: "v", Group: domain.B2B, Industry: &industry}}, Sales: []domain.Sale{
		sale("u", "same", "A", -7), sale("u", "same", "A", -7), sale("u", "same", "B", -7),
		sale("u", "same", "C", 0), sale("u", "later", "F", 7), sale("v", "other", "A", 0),
	}}
	meta := contract.Snapshot{ID: "s", DatasetVersion: "v", TrainThrough: cut}
	build := func() contract.Snapshot {
		t.Helper()
		var got capture
		if err := Build(context.Background(), meta, data, &got); err != nil || got.calls != 1 {
			t.Fatalf("publish: %d %v", got.calls, err)
		}
		return got.Snapshot
	}
	got := build()
	find := func(signal string, from, to domain.ItemID) float64 {
		t.Helper()
		for _, set := range got.CandidateSets {
			if set.Key != (contract.CandidateKey{Signal: signal, SourceItemID: from}) {
				continue
			}
			for _, c := range set.Candidates {
				if c.ItemID == to {
					return c.Score
				}
			}
		}
		return 0
	}
	if find("popularity", "", "A") != 2 || find("jaccard", "A", "B") != 0.5 || find("basket", "A", "B") != 1 || find("basket", "A", "C") != 0 || find("transition", "A", "C") != 1 || find("transition", "C", "A") != 0 || find("transition", "A", "B") != 0 {
		t.Fatal("incorrect signal counts")
	}
	for _, set := range got.CandidateSets {
		if set.Key.Industry != "" {
			t.Fatal("small industry cohort")
		}
	}
	if find("transition", "A", "F") != 0 || find("popularity", "", "F") != 0 {
		t.Fatal("future sale leaked")
	}
	if again := build(); !reflect.DeepEqual(got, again) {
		t.Fatal("non-deterministic build")
	}
	meta.TrainThrough = cut.AddDate(0, 0, 7)
	got = build()
	if find("transition", "A", "F") != 1 {
		t.Fatal("future edge did not appear at later cutoff")
	}
}
