package signals

import (
	"cmp"
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"recsys/internal/contract"
	"recsys/internal/domain"
)

const MinIndustryBuyers = 5

var descriptions = map[string]string{"popularity": "Покупатели в группе", "basket": "Покупали в одной корзине", "transition": "Покупали в более позднюю неделю", "jaccard": "Общие покупатели"}

func Industry(c domain.Client) string {
	if c.Industry == nil {
		return ""
	}
	s := strings.TrimSpace(*c.Industry)
	if strings.EqualFold(s, "Другое") {
		return ""
	}
	return s
}

// PopulationKeys returns cohorts in decreasing order of specificity.
func PopulationKeys(c domain.Client) []contract.CandidateKey {
	k := contract.CandidateKey{Signal: "popularity", ClientGroup: c.Group}
	var keys []contract.CandidateKey
	if c.Group == domain.B2B {
		if s := Industry(c); s != "" {
			t := k
			t.Industry = s
			keys = append(keys, t)
		}
		if c.Segment != nil && strings.TrimSpace(*c.Segment) != "" {
			t := k
			t.Segment = strings.TrimSpace(*c.Segment)
			keys = append(keys, t)
		}
		if c.Type != "" {
			t := k
			t.ClientType = c.Type
			keys = append(keys, t)
		}
	}
	return append(keys, k, contract.CandidateKey{Signal: "popularity"})
}

type counts map[contract.CandidateKey]map[domain.ItemID]int

func (c counts) add(k contract.CandidateKey, id domain.ItemID) {
	if c[k] == nil {
		c[k] = make(map[domain.ItemID]int)
	}
	c[k][id]++
}

type basket struct{ order, week string }
type weeks struct{ first, last string }
type itemSet map[domain.ItemID]bool

// Build expects whole-week dates and publishes once, after all train rows are aggregated.
func Build(ctx context.Context, meta contract.Snapshot, data contract.DatasetBatch, writer contract.SnapshotWriter) error {
	if meta.ID == "" || meta.DatasetVersion == "" || meta.TrainThrough.IsZero() {
		return fmt.Errorf("snapshot ID, dataset version and cutoff required")
	}
	clients := map[domain.ClientID]domain.Client{}
	for _, c := range data.Clients {
		clients[c.ID] = c
	}
	byBuyer := map[domain.ClientID][]domain.Sale{}
	cutoff := meta.TrainThrough.Format("2006-01-02")
	for _, s := range data.Sales {
		if s.OrderWeek.IsZero() {
			return fmt.Errorf("missing week at row %d", s.SourceRow)
		}
		if s.OrderWeek.Format("2006-01-02") > cutoff {
			continue
		}
		if _, ok := clients[s.ClientID]; !ok || s.ItemID == "" || s.OrderID == "" {
			return fmt.Errorf("invalid sale at row %d", s.SourceRow)
		}
		byBuyer[s.ClientID] = append(byBuyer[s.ClientID], s)
	}
	totals := counts{}
	basketFrequency := map[[2]domain.ItemID]int{}
	cohortSize := map[contract.CandidateKey]int{}
	for buyer, rows := range byBuyer {
		if err := ctx.Err(); err != nil {
			return err
		}
		seen := map[domain.ItemID]weeks{}
		baskets := map[basket]itemSet{}
		for _, s := range rows {
			week := s.OrderWeek.Format("2006-01-02")
			w := seen[s.ItemID]
			if w.first == "" || week < w.first {
				w.first = week
			}
			w.last = max(w.last, week)
			seen[s.ItemID] = w
			k := basket{s.OrderID, week}
			if baskets[k] == nil {
				baskets[k] = itemSet{}
			}
			baskets[k][s.ItemID] = true
		}
		for _, k := range PopulationKeys(clients[buyer]) {
			cohortSize[k]++
			for id := range seen {
				totals.add(k, id)
			}
		}
		for a, wa := range seen {
			for b, wb := range seen {
				if a == b {
					continue
				}
				totals.add(contract.CandidateKey{Signal: "jaccard", SourceItemID: a}, b)
				if wa.first < wb.last {
					totals.add(contract.CandidateKey{Signal: "transition", SourceItemID: a}, b)
				}
			}
		}
		// A buyer contributes support once even if they repeat the basket.
		pairs := map[[2]domain.ItemID]bool{}
		for _, items := range baskets {
			for a := range items {
				for b := range items {
					if a != b {
						pair := [2]domain.ItemID{a, b}
						pairs[pair] = true
						basketFrequency[pair]++
					}
				}
			}
		}
		for p := range pairs {
			totals.add(contract.CandidateKey{Signal: "basket", SourceItemID: p[0]}, p[1])
		}
	}
	pop := totals[contract.CandidateKey{Signal: "popularity"}]
	meta.CandidateSets = nil
	for k, items := range totals {
		if k.Industry != "" && cohortSize[k] < MinIndustryBuyers {
			continue
		}
		set := contract.CandidateSet{Key: k}
		for id, support := range items {
			score := float64(support)
			if k.Signal == "basket" {
				score = float64(basketFrequency[[2]domain.ItemID{k.SourceItemID, id}])
			}
			if k.Signal == "jaccard" {
				score /= float64(pop[k.SourceItemID] + pop[id] - support)
			}
			kind := domain.Behavioral
			from := "item:" + string(k.SourceItemID)
			if k.Signal == "popularity" {
				kind = domain.Popular
				from = "cohort:" + keyID(k)
			}
			e := domain.Evidence{ID: keyID(k) + "/" + url.PathEscape(string(id)), From: from, To: "item:" + string(id), Type: k.Signal, Weight: float64(support), Reason: descriptions[k.Signal] + ": " + strconv.Itoa(support), Source: "sales/" + meta.DatasetVersion + "/signals-v1", SnapshotID: meta.ID}
			set.Candidates = append(set.Candidates, domain.Candidate{ItemID: id, Kind: kind, Score: score, Evidence: []domain.Evidence{e}})
		}
		slices.SortFunc(set.Candidates, func(a, b domain.Candidate) int {
			if v := cmp.Compare(b.Score, a.Score); v != 0 {
				return v
			}
			return cmp.Compare(a.ItemID, b.ItemID)
		})
		meta.CandidateSets = append(meta.CandidateSets, set)
	}
	slices.SortFunc(meta.CandidateSets, func(a, b contract.CandidateSet) int { return cmp.Compare(keyID(a.Key), keyID(b.Key)) })
	if err := ctx.Err(); err != nil {
		return err
	}
	return writer.PublishSnapshot(ctx, meta)
}

func keyID(k contract.CandidateKey) string {
	parts := []string{k.Signal, string(k.SourceItemID), string(k.ClientGroup), k.ClientType, k.Industry, k.Segment}
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}
