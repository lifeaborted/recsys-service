package recommend

import (
	"recsys/internal/contract"
	"recsys/internal/domain"
	"recsys/internal/signals"
)

type pool struct {
	key  contract.CandidateKey
	tier int
	kind domain.RecommendationKind
}

func pools(c domain.Client, history []domain.ItemID, mode contract.Mode) []pool {
	var result []pool
	for _, id := range history {
		add := func(signal string, tier int, kind domain.RecommendationKind) {
			result = append(result, pool{contract.CandidateKey{Signal: signal, SourceItemID: id}, tier, kind})
		}
		if c.Group == domain.B2B {
			add("renewal", 10, domain.Renewal)
			add("transition", 9, domain.Behavioral)
		} else {
			add("complement", 10, domain.Complement)
		}
		add("jaccard", 8, domain.Behavioral)
		add("basket", 7, domain.Behavioral)
	}
	for _, key := range signals.PopulationKeys(c) {
		tier := 2
		switch {
		case key.Industry != "":
			tier = 6
		case key.Segment != "":
			tier = 5
		case key.ClientType != "":
			tier = 4
		case key.ClientGroup != "":
			tier = 3
		}
		result = append(result, pool{key, tier, domain.Popular})
	}
	universe := "catalog"
	if mode == contract.Backtest {
		universe = "sales"
	}
	return append(result, pool{contract.CandidateKey{Signal: universe}, 1, domain.Popular})
}
