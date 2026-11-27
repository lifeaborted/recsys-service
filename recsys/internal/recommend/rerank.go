package recommend

import (
	"context"
	"slices"
	"strings"

	"recsys/internal/domain"
)

// RerankItem is a card the cascade already accepted. The model may only reorder these ids.
type RerankItem struct {
	Signal string `json:"signal"`
	ID     domain.ItemID
	Title  string
	Kind   domain.RecommendationKind
}

// Reranker reorders the online shortlist. A failure leaves the cascade order in place.
type Reranker interface {
	Order(ctx context.Context, owned []string, items []RerankItem) ([]domain.ItemID, error)
}

func rerankOnline(ctx context.Context, reranker Reranker, ordered []ranked, cards, owned map[domain.ItemID]Product) []ranked {
	if reranker == nil || len(ordered) < 2 {
		return ordered
	}
	head := ordered
	if len(head) > 8 {
		head = ordered[:8]
	}
	items := make([]RerankItem, len(head))
	for i, row := range head {
		items[i] = RerankItem{ID: row.id, Title: cards[row.id].Title, Kind: row.kind, Signal: row.proof[0].Type}
	}
	ids, err := reranker.Order(ctx, ownedTitles(owned), items)
	if err != nil || len(ids) != len(head) {
		return ordered
	}
	index := make(map[domain.ItemID]ranked, len(head))
	for _, row := range head {
		index[row.id] = row
	}
	seen := map[domain.ItemID]bool{}
	next := make([]ranked, 0, len(ordered))
	for _, id := range ids {
		row, ok := index[id]
		if !ok || seen[id] {
			return ordered
		}
		seen[id] = true
		next = append(next, row)
	}
	return append(next, ordered[len(head):]...)
}

func ownedTitles(owned map[domain.ItemID]Product) []string {
	titles := make([]string, 0, len(owned))
	for _, product := range owned {
		if title := strings.TrimSpace(product.Title); title != "" {
			titles = append(titles, title)
		}
	}
	slices.Sort(titles)
	return titles
}
