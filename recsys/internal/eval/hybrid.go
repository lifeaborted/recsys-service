package eval

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"recsys/internal/contract"
	"recsys/internal/domain"
)

// Backtest mirrors the target contract.Backtest interface for historical Recommender evaluation.
type Backtest interface {
	Run(ctx context.Context, rec contract.Recommender, cfg BacktestConfig) (HybridReport, error)
}

// BacktestConfig defines snapshot pinning, dataset labeling, candidate restriction, and family metadata.
// ProductGroups holds the canonical v5 item_id -> product_group mapping (takes precedence over FamilyID).
type BacktestConfig struct {
	DatasetLabel    string
	SnapshotID      string
	AllowedItems    map[domain.ItemID]struct{}
	ProductGroups   map[domain.ItemID]string
	ItemFamilies    map[domain.ItemID]string
	FeedGeneratedAt *time.Time
}

// FamilyBreakdown tracks hit counts across known-family variants, new products, and unknown families.
type FamilyBreakdown struct {
	TotalHits         int
	KnownFamilyHits   int
	NewFamilyHits     int
	UnknownFamilyHits int
}

// KnownFamilyShare returns the percentage of all hits belonging to a family already bought in train.
func (f FamilyBreakdown) KnownFamilyShare() float64 {
	return sharePct(f.KnownFamilyHits, f.TotalHits)
}

// NewFamilyShare returns the percentage of all hits belonging to genuinely new product families.
func (f FamilyBreakdown) NewFamilyShare() float64 {
	return sharePct(f.NewFamilyHits, f.TotalHits)
}

// UnknownFamilyShare returns the percentage of all hits where the family could not be determined.
func (f FamilyBreakdown) UnknownFamilyShare() float64 {
	return sharePct(f.UnknownFamilyHits, f.TotalHits)
}

// ColdItemStats tracks ground-truth targets and hits on items absent from the train window.
type ColdItemStats struct {
	ColdItems      []domain.ItemID
	CohortATargets int
	CohortAHits    int
	CohortBTargets int
	CohortBHits    int
}

// HybridCohortResult stores evaluated metrics and hit breakdown for one cohort/segment slice.
type HybridCohortResult struct {
	Cohort   string
	Group    string
	Clients  int
	Metrics  Metrics
	Families FamilyBreakdown
}

// HybridReport contains complete hybrid evaluation results, metadata, and leakage disclosures.
type HybridReport struct {
	DatasetLabel    string
	SnapshotID      string
	TrainThrough    string
	FeedGeneratedAt *time.Time
	LeakageWarnings []string
	Summary         DatasetSummary
	Results         []HybridCohortResult
	Cold            ColdItemStats
}

// DefaultLeakageWarnings lists features that can leak post-cutoff information when evaluating on current snapshots.
func DefaultLeakageWarnings() []string {
	return []string{
		"Исторические снимки каталога, наличия (available) и профилей клиентов на дату отсечения 2026-08-31 отсутствуют.",
		"Атрибуты catalog_offers, available и сопоставления historical_item_offers отражают дату выгрузки фида и могут содержать сведения после окончания train.",
		"Поля профиля clients (industry, segment, lk_access) отражают состояние на момент выгрузки датасета, а не исторический срез.",
	}
}

// AllowedItemsFromSales returns the set of all distinct item_id values observed in sales (e.g. 817 items for v4).
func AllowedItemsFromSales(sales []domain.Sale) map[domain.ItemID]struct{} {
	allowed := make(map[domain.ItemID]struct{})
	for _, s := range sales {
		allowed[s.ItemID] = struct{}{}
	}
	return allowed
}

// MemoryBacktester implements Backtest over in-memory domain clients and sales slices.
type MemoryBacktester struct {
	Clients []domain.Client
	Sales   []domain.Sale
}

// Run executes historical evaluation against rec with the pinned snapshot configuration.
func (b MemoryBacktester) Run(ctx context.Context, rec contract.Recommender, cfg BacktestConfig) (HybridReport, error) {
	return EvaluateRecommender(ctx, rec, b.Clients, b.Sales, cfg)
}

// EvaluateRecommender evaluates a contract.Recommender in Backtest mode using only top-level cards.
func EvaluateRecommender(
	ctx context.Context,
	rec contract.Recommender,
	clients []domain.Client,
	sales []domain.Sale,
	cfg BacktestConfig,
) (HybridReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if rec == nil {
		return HybridReport{}, errors.New("recommender is required")
	}
	if strings.TrimSpace(cfg.SnapshotID) == "" {
		return HybridReport{}, errors.New("snapshot_id is required for historical backtest")
	}

	evalSales, err := SalesFromDomain(clients, sales)
	if err != nil {
		return HybridReport{}, err
	}
	summary := SummarizeDataset(evalSales)

	groups := make(map[string]string, len(clients))
	for _, c := range clients {
		groups[string(c.ID)] = string(c.Group)
	}

	train := make(map[string]itemSet)
	test := make(map[string]itemSet)
	for _, s := range evalSales {
		if s.Week <= TrainThrough {
			add(train, s.ClientID, s.ItemID)
		} else {
			add(test, s.ClientID, s.ItemID)
		}
	}

	coldSet := make(itemSet, len(summary.ColdItems))
	coldItems := make([]domain.ItemID, 0, len(summary.ColdItems))
	for _, id := range summary.ColdItems {
		coldSet[id] = struct{}{}
		coldItems = append(coldItems, domain.ItemID(id))
	}

	cohortA := make([]string, 0, summary.CohortAClients)
	for client, bought := range test {
		seen := train[client]
		if len(seen) > 0 && hasNewItem(bought, seen) {
			cohortA = append(cohortA, client)
		}
	}
	slices.Sort(cohortA)

	results := []HybridCohortResult{
		{Cohort: "A", Group: "all"}, {Cohort: "A", Group: "B2B"}, {Cohort: "A", Group: "B2C"},
		{Cohort: "B", Group: "all"}, {Cohort: "B", Group: "B2B"}, {Cohort: "B", Group: "B2C"},
	}
	coldStats := ColdItemStats{ColdItems: coldItems}

	for _, client := range cohortA {
		if err := ctx.Err(); err != nil {
			return HybridReport{}, err
		}

		seen := train[client]
		bought := test[client]
		isCohortB := len(seen) == 1

		for item := range bought {
			if _, seenAlready := seen[item]; seenAlready {
				continue
			}
			if _, isCold := coldSet[item]; isCold {
				coldStats.CohortATargets++
				if isCohortB {
					coldStats.CohortBTargets++
				}
			}
		}

		res, err := rec.Recommend(ctx, contract.RecommendationQuery{
			ClientID:   domain.ClientID(client),
			SnapshotID: cfg.SnapshotID,
			Mode:       contract.Backtest,
		})
		if err != nil {
			return HybridReport{}, fmt.Errorf("recommend for client %q: %w", client, err)
		}

		topCards := res.Items
		if len(topCards) > TopK {
			topCards = topCards[:TopK]
		}

		seenFamilies := clientSeenFamilies(seen, cfg.ProductGroups, cfg.ItemFamilies)
		countedHits := make(itemSet, TopK)
		clientHits := 0
		var clientFamilies FamilyBreakdown

		for _, card := range topCards {
			idStr := string(card.ItemID)
			if cfg.AllowedItems != nil {
				if _, allowed := cfg.AllowedItems[card.ItemID]; !allowed {
					// Out-of-sales feed item occupies one of the 3 slots and counts as a miss.
					continue
				}
			}
			if _, alreadyBought := seen[idStr]; alreadyBought {
				continue
			}
			if _, dup := countedHits[idStr]; dup {
				continue
			}
			countedHits[idStr] = struct{}{}
			if _, isHit := bought[idStr]; !isHit {
				continue
			}

			clientHits++
			if _, isCold := coldSet[idStr]; isCold {
				coldStats.CohortAHits++
				if isCohortB {
					coldStats.CohortBHits++
				}
			}

			fam := resolveCardFamily(card, cfg.ProductGroups, cfg.ItemFamilies)
			clientFamilies.TotalHits++
			switch {
			case fam == "":
				clientFamilies.UnknownFamilyHits++
			case seenFamilies[fam]:
				clientFamilies.KnownFamilyHits++
			default:
				clientFamilies.NewFamilyHits++
			}
		}

		for i := range results {
			sliceRes := &results[i]
			if (sliceRes.Group != "all" && sliceRes.Group != groups[client]) ||
				(sliceRes.Cohort == "B" && !isCohortB) {
				continue
			}
			sliceRes.Clients++
			if len(topCards) > 0 {
				sliceRes.Metrics.FillRate++
			}
			if len(topCards) == TopK {
				sliceRes.Metrics.Full3Rate++
			}
			sliceRes.Metrics.Precision += float64(clientHits)
			sliceRes.Families.TotalHits += clientFamilies.TotalHits
			sliceRes.Families.KnownFamilyHits += clientFamilies.KnownFamilyHits
			sliceRes.Families.NewFamilyHits += clientFamilies.NewFamilyHits
			sliceRes.Families.UnknownFamilyHits += clientFamilies.UnknownFamilyHits
		}
	}

	for i := range results {
		sliceRes := &results[i]
		if sliceRes.Clients > 0 {
			sliceRes.Metrics.Precision *= 100 / (TopK * float64(sliceRes.Clients))
			sliceRes.Metrics.FillRate *= 100 / float64(sliceRes.Clients)
			sliceRes.Metrics.Full3Rate *= 100 / float64(sliceRes.Clients)
		}
	}

	label := strings.TrimSpace(cfg.DatasetLabel)
	if label == "" {
		label = "demo"
	}

	return HybridReport{
		DatasetLabel:    label,
		SnapshotID:      cfg.SnapshotID,
		TrainThrough:    TrainThrough,
		FeedGeneratedAt: cfg.FeedGeneratedAt,
		LeakageWarnings: DefaultLeakageWarnings(),
		Summary:         summary,
		Results:         results,
		Cold:            coldStats,
	}, nil
}

func clientSeenFamilies(seen itemSet, productGroups, itemFamilies map[domain.ItemID]string) map[string]bool {
	out := make(map[string]bool)
	if len(productGroups) == 0 && len(itemFamilies) == 0 {
		return out
	}
	for item := range seen {
		id := domain.ItemID(item)
		if pg := strings.TrimSpace(productGroups[id]); pg != "" {
			out[pg] = true
			continue
		}
		if fam := strings.TrimSpace(itemFamilies[id]); fam != "" {
			out[fam] = true
		}
	}
	return out
}

func resolveCardFamily(card domain.Recommendation, productGroups, itemFamilies map[domain.ItemID]string) string {
	if len(productGroups) > 0 {
		if pg := strings.TrimSpace(productGroups[card.ItemID]); pg != "" {
			return pg
		}
	}
	if len(itemFamilies) > 0 {
		if fam := strings.TrimSpace(itemFamilies[card.ItemID]); fam != "" {
			return fam
		}
	}
	if card.FamilyID != nil {
		if fam := strings.TrimSpace(*card.FamilyID); fam != "" {
			return fam
		}
	}
	return ""
}

func sharePct(part, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) * 100 / float64(total)
}
