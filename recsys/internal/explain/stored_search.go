package explain

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"recsys/internal/domain"
	"recsys/internal/enrichment"
)

// StoredSearch embeds only the approved query and reads catalog vectors already stored in pgvector.
type StoredSearch struct {
	provider *Embeddings
	version  string
	queries  map[domain.ItemID]PublicText
	nearest  func(context.Context, []float64, int) ([]enrichment.SearchHit, error)
}

func NewStoredSearch(provider *Embeddings, feed enrichment.FeedVersion, approval SearchApproval, nearest func(context.Context, []float64, int) ([]enrichment.SearchHit, error)) (*StoredSearch, error) {
	if provider == nil || nearest == nil {
		return nil, fmt.Errorf("stored search requires an embedding provider and a vector index")
	}
	if !approval.Approved || approval.FeedURL != feed.URL || approval.CatalogVersion != feed.ID || len(approval.Queries) == 0 {
		return nil, fmt.Errorf("semantic search requires approval for this feed URL and version and at least one approved query")
	}
	search := &StoredSearch{provider: provider, version: feed.ID, queries: make(map[domain.ItemID]PublicText), nearest: nearest}
	for _, id := range slices.Sorted(maps.Keys(approval.Queries)) {
		input := approval.Queries[id]
		if strings.TrimSpace(string(id)) == "" {
			return nil, fmt.Errorf("semantic query requires an item ID")
		}
		if err := input.validate(); err != nil {
			return nil, fmt.Errorf("semantic query for item %q: %w", id, err)
		}
		search.queries[id] = input
	}
	return search, nil
}

func (s *StoredSearch) Search(ctx context.Context, version string, query enrichment.SearchQuery, limit int) ([]enrichment.SearchHit, error) {
	if version != s.version {
		return nil, fmt.Errorf("semantic search catalog version mismatch")
	}
	input, approved := s.queries[query.ItemID]
	if !approved || limit <= 0 {
		return nil, nil
	}
	vectors, err := s.provider.Embed(ctx, []PublicText{input})
	if err != nil {
		return nil, fmt.Errorf("embed semantic query for item %q: %w", query.ItemID, err)
	}
	return s.nearest(ctx, vectors[0].Vector, limit)
}
