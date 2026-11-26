package eval

import (
	"cmp"
	"fmt"
	"slices"

	"recsys/internal/domain"
)

const (
	TrainThrough = "2026-08-31"
	TopK         = 3
)

type Sale struct {
	OrderID, ClientID, Week, ItemID, Group string
}

type Metrics struct {
	Precision float64
	FillRate  float64
	Full3Rate float64
}

type Result struct {
	Cohort, Group          string
	Clients                int
	Popularity, CoPurchase Metrics
}

// DatasetSummary captures aggregate counts and cold-start split diagnostics.
type DatasetSummary struct {
	Rows           int
	UniqueClients  int
	Baskets        int
	TotalItems     int
	TrainItems     int
	ColdItems      []string
	CohortAClients int
	CohortBClients int
}

type itemSet map[string]struct{}

type basketKey struct {
	order, week string
}

// SalesFromDomain converts normalized domain clients and sales into eval.Sale rows,
// retaining clients without LK access and items without SKU or catalog match.
func SalesFromDomain(clients []domain.Client, sales []domain.Sale) ([]Sale, error) {
	groups := make(map[domain.ClientID]string, len(clients))
	for _, c := range clients {
		groups[c.ID] = string(c.Group)
	}
	out := make([]Sale, 0, len(sales))
	for _, s := range sales {
		group, ok := groups[s.ClientID]
		if !ok {
			return nil, fmt.Errorf("sale row %d references unknown client_id %q", s.SourceRow, s.ClientID)
		}
		out = append(out, Sale{
			OrderID:  s.OrderID,
			ClientID: string(s.ClientID),
			Week:     s.OrderWeek.UTC().Format("2006-01-02"),
			ItemID:   string(s.ItemID),
			Group:    group,
		})
	}
	return out, nil
}

// SummarizeDataset computes dataset dimensions, train/cold item split, and cohort counts.
func SummarizeDataset(sales []Sale) DatasetSummary {
	clients := make(map[string]struct{})
	baskets := make(map[basketKey]struct{})
	allItems := make(itemSet)
	trainItems := make(itemSet)
	testItems := make(itemSet)
	train := make(map[string]itemSet)
	test := make(map[string]itemSet)

	for _, sale := range sales {
		clients[sale.ClientID] = struct{}{}
		baskets[basketKey{sale.OrderID, sale.Week}] = struct{}{}
		allItems[sale.ItemID] = struct{}{}
		if sale.Week <= TrainThrough {
			trainItems[sale.ItemID] = struct{}{}
			add(train, sale.ClientID, sale.ItemID)
		} else {
			testItems[sale.ItemID] = struct{}{}
			add(test, sale.ClientID, sale.ItemID)
		}
	}

	var cold []string
	for item := range testItems {
		if _, inTrain := trainItems[item]; !inTrain {
			cold = append(cold, item)
		}
	}
	slices.Sort(cold)

	cohortA, cohortB := 0, 0
	for client, bought := range test {
		seen := train[client]
		if len(seen) == 0 || !hasNewItem(bought, seen) {
			continue
		}
		cohortA++
		if len(seen) == 1 {
			cohortB++
		}
	}

	return DatasetSummary{
		Rows:           len(sales),
		UniqueClients:  len(clients),
		Baskets:        len(baskets),
		TotalItems:     len(allItems),
		TrainItems:     len(trainItems),
		ColdItems:      cold,
		CohortAClients: cohortA,
		CohortBClients: cohortB,
	}
}

func Baselines(sales []Sale) []Result {
	train, test := make(map[string]itemSet), make(map[string]itemSet)
	baskets := make(map[basketKey]itemSet)
	groups := make(map[string]string)
	for _, sale := range sales {
		groups[sale.ClientID] = sale.Group
		if sale.Week <= TrainThrough {
			add(train, sale.ClientID, sale.ItemID)
			add(baskets, basketKey{sale.OrderID, sale.Week}, sale.ItemID)
		} else {
			add(test, sale.ClientID, sale.ItemID)
		}
	}

	popularity := make(map[string]int)
	for _, items := range train {
		for item := range items {
			popularity[item]++
		}
	}
	pairs := make(map[string]map[string]int)
	for _, items := range baskets {
		for item := range items {
			if pairs[item] == nil {
				pairs[item] = make(map[string]int)
			}
			for neighbor := range items {
				if neighbor != item {
					pairs[item][neighbor]++
				}
			}
		}
	}

	results := []Result{
		{Cohort: "A", Group: "all"}, {Cohort: "A", Group: "B2B"}, {Cohort: "A", Group: "B2C"},
		{Cohort: "B", Group: "all"}, {Cohort: "B", Group: "B2B"}, {Cohort: "B", Group: "B2C"},
	}
	for client, bought := range test {
		seen := train[client]
		if len(seen) == 0 || !hasNewItem(bought, seen) {
			continue
		}
		scores := make(map[string]int)
		for item := range seen {
			for neighbor, count := range pairs[item] {
				scores[neighbor] += count
			}
		}
		pop := topItems(popularity, popularity, seen)
		co := topItems(scores, popularity, seen)
		for i := range results {
			result := &results[i]
			if (result.Group != "all" && result.Group != groups[client]) ||
				(result.Cohort == "B" && len(seen) != 1) {
				continue
			}
			result.Clients++
			record(&result.Popularity, pop, bought)
			record(&result.CoPurchase, co, bought)
		}
	}
	for i := range results {
		result := &results[i]
		for _, m := range []*Metrics{&result.Popularity, &result.CoPurchase} {
			if result.Clients > 0 {
				m.Precision *= 100 / (TopK * float64(result.Clients))
				m.FillRate *= 100 / float64(result.Clients)
				m.Full3Rate *= 100 / float64(result.Clients)
			}
		}
	}
	return results
}

func add[K comparable](sets map[K]itemSet, key K, item string) {
	if sets[key] == nil {
		sets[key] = make(itemSet)
	}
	sets[key][item] = struct{}{}
}

func hasNewItem(bought, seen itemSet) bool {
	for item := range bought {
		if _, exists := seen[item]; !exists {
			return true
		}
	}
	return false
}

func topItems(scores, popularity map[string]int, seen itemSet) []string {
	items := make([]string, 0, len(scores))
	for item := range scores {
		if _, exists := seen[item]; !exists {
			items = append(items, item)
		}
	}
	slices.SortFunc(items, func(a, b string) int {
		if order := cmp.Compare(scores[b], scores[a]); order != 0 {
			return order
		}
		if order := cmp.Compare(popularity[b], popularity[a]); order != 0 {
			return order
		}
		return cmp.Compare(a, b)
	})
	return items[:min(len(items), TopK)]
}

func record(m *Metrics, ranked []string, bought itemSet) {
	if len(ranked) > 0 {
		m.FillRate++
	}
	if len(ranked) == TopK {
		m.Full3Rate++
	}
	for _, item := range ranked {
		if _, hit := bought[item]; hit {
			m.Precision++
		}
	}
}
