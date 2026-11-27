package recommend

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"recsys/internal/contract"
	"recsys/internal/domain"
)

type SnapshotInfo struct {
	ID           string
	TrainThrough time.Time
}

type Reader interface {
	contract.FactsReader
	SnapshotInfo(context.Context, string) (SnapshotInfo, error)
	Products(context.Context, string, []domain.ItemID) (map[domain.ItemID]Product, error)
}

type Engine struct {
	reader   Reader
	reranker Reranker
}

func New(r Reader) *Engine { return &Engine{reader: r} }

// UseReranker reorders the online shortlist. Backtest order stays on the cascade.
func (e *Engine) UseReranker(r Reranker) { e.reranker = r }

var _ contract.Recommender = (*Engine)(nil)

type ranked struct {
	id    domain.ItemID
	tier  int
	score float64
	kind  domain.RecommendationKind
	proof []domain.Evidence
}

func compare(a, b ranked) int {
	if n := cmp.Compare(b.tier, a.tier); n != 0 {
		return n
	}
	if n := cmp.Compare(b.score, a.score); n != 0 {
		return n
	}
	return cmp.Compare(a.id, b.id)
}

func evidence(p pool, c domain.Candidate, snapshot string) []domain.Evidence {
	var valid []domain.Evidence
	for _, e := range c.Evidence {
		if e.ID == "" || e.From == "" || e.To != "item:"+string(c.ItemID) || e.Source == "" || e.Reason == "" || e.SnapshotID != snapshot || e.Type != p.key.Signal {
			continue
		}
		if p.key.SourceItemID != "" && e.From != "item:"+string(p.key.SourceItemID) {
			continue
		}
		valid = append(valid, e)
	}
	return valid
}

func (e *Engine) Recommend(ctx context.Context, q contract.RecommendationQuery) (contract.RecommendationResult, error) {
	var zero contract.RecommendationResult
	if q.Mode != contract.Online && q.Mode != contract.Backtest || q.ClientID == "" || q.Mode == contract.Backtest && q.SnapshotID == "" {
		return zero, fmt.Errorf("invalid recommendation query")
	}
	want := cardLimit(q.Mode)
	if q.Mode == contract.Online && q.Limit > 0 {
		want = min(q.Limit, want)
	}
	s, err := e.reader.SnapshotInfo(ctx, q.SnapshotID)
	if err != nil {
		return zero, fmt.Errorf("snapshot: %w", err)
	}
	if s.ID == "" || q.SnapshotID != "" && s.ID != q.SnapshotID {
		return zero, fmt.Errorf("snapshot mismatch")
	}
	var through time.Time
	if q.Mode == contract.Backtest {
		through = s.TrainThrough
		if through.IsZero() {
			return zero, fmt.Errorf("snapshot has no cutoff")
		}
	}
	client, err := e.reader.Profile(ctx, q.ClientID)
	if err != nil {
		return zero, fmt.Errorf("profile: %w", err)
	}
	hist, err := e.reader.History(ctx, q.ClientID, through)
	if err != nil {
		return zero, fmt.Errorf("history: %w", err)
	}
	bought := map[domain.ItemID]bool{}
	var ids []domain.ItemID
	for _, sale := range hist {
		if !bought[sale.ItemID] {
			bought[sale.ItemID] = true
			ids = append(ids, sale.ItemID)
		}
	}
	slices.Sort(ids)
	products, err := e.reader.Products(ctx, s.ID, ids)
	if err != nil {
		return zero, fmt.Errorf("history products: %w", err)
	}
	platforms := map[string]bool{}
	ownedOffers := map[string]bool{}
	for _, id := range ids {
		product := products[id]
		if product.Platform != "" {
			platforms[product.Platform] = true
		}
		if product.OfferID != nil {
			ownedOffers[*product.OfferID] = true
		}
	}
	active := pools(client, ids, q.Mode)
	best := map[domain.ItemID]ranked{}
	cards := map[domain.ItemID]Product{}
	owned := slices.Clone(ids)
	finish := func(ordered []ranked) contract.RecommendationResult {
		if q.Mode == contract.Online {
			ordered = rerankOnline(ctx, e.reranker, ordered, cards, products)
		}
		return render(s.ID, q.Mode, ordered, cards, products, want)
	}
	similarSeen := false
	for poolLimit := 64; ; poolLimit *= 2 {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		var more []pool
		pending := map[domain.ItemID]bool{}
		if q.Mode == contract.Online && !similarSeen {
			similarSeen = true
			collectSimilar(ctx, e.reader, s.ID, owned, bought, best, pending)
		}
		for _, p := range active {
			rows, err := e.reader.Candidates(ctx, s.ID, p.key, poolLimit)
			if err != nil {
				return zero, fmt.Errorf("candidate pool %s: %w", p.key.Signal, err)
			}
			if len(rows) == poolLimit {
				more = append(more, p)
			}
			for _, c := range rows {
				if c.ItemID == "" || bought[c.ItemID] {
					continue
				}
				proof := evidence(p, c, s.ID)
				if len(proof) == 0 {
					continue
				}
				r := ranked{c.ItemID, p.tier, c.Score, p.kind, proof}
				if old, ok := best[c.ItemID]; ok && compare(old, r) <= 0 {
					continue
				}
				best[c.ItemID] = r
				if _, ok := cards[c.ItemID]; !ok {
					pending[c.ItemID] = true
				}
			}
		}
		ids = ids[:0]
		for id := range pending {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		if len(ids) > 0 {
			found, err := e.reader.Products(ctx, s.ID, ids)
			if err != nil {
				return zero, fmt.Errorf("candidate products: %w", err)
			}
			for _, id := range ids {
				p, ok := found[id]
				if !ok {
					return zero, fmt.Errorf("missing product %s", id)
				}
				cards[id] = p
			}
		}
		var ordered []ranked
		for id, r := range best {
			product := cards[id]
			if !eligible(product, client, platforms, q.Mode) {
				continue
			}
			if q.Mode == contract.Online && product.OfferID != nil && ownedOffers[*product.OfferID] {
				continue
			}
			if q.Mode == contract.Online && r.kind == domain.Renewal {
				continue
			}
			if q.Mode == contract.Online && sameCategoryRival(cards[id], products, r.kind) {
				continue
			}
			ordered = append(ordered, r)
		}
		slices.SortFunc(ordered, compare)
		if len(more) == 0 {
			return finish(ordered), nil
		}
		result := render(s.ID, q.Mode, ordered, cards, products, want)
		// A truncated higher-priority pool may still displace a fallback card.
		if len(result.Items) == want {
			lastTier := best[result.Items[want-1].ItemID].tier
			active = nil
			for _, p := range more {
				if p.tier >= lastTier {
					active = append(active, p)
				}
			}
			if len(active) == 0 {
				return finish(ordered), nil
			}
		} else {
			active = more
		}
	}
}
