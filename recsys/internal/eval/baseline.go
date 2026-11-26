package eval

import (
	"cmp"
	"slices"
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
}

type Result struct {
	Cohort, Group          string
	Clients                int
	Popularity, CoPurchase Metrics
}

type itemSet map[string]struct{}

type basketKey struct {
	order, week string
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
	for _, item := range ranked {
		if _, hit := bought[item]; hit {
			m.Precision++
		}
	}
}
