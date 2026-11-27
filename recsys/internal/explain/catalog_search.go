package explain

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"sync"

	"recsys/internal/domain"
	"recsys/internal/enrichment"
)

// SearchApproval records permission to embed offer names from one feed and
// separately approved query texts keyed by item ID.
type SearchApproval struct {
	FeedURL        string                       `json:"feed_url"`
	CatalogVersion string                       `json:"catalog_version"`
	Approved       bool                         `json:"approved"`
	Queries        map[domain.ItemID]PublicText `json:"queries"`
}

type CatalogSearch struct {
	provider *Embeddings
	version  string
	queries  map[domain.ItemID]PublicText
	offerIDs []string
	texts    []PublicText

	mu    sync.Mutex
	index []Embedding
}

var _ enrichment.Searcher = (*CatalogSearch)(nil)

func NewCatalogSearch(provider *Embeddings, feed enrichment.FeedVersion, offers []enrichment.Offer, approval SearchApproval) (*CatalogSearch, error) {
	if provider == nil {
		return nil, fmt.Errorf("semantic search requires an embedding provider")
	}
	if !approval.Approved || approval.FeedURL != feed.URL || approval.CatalogVersion != feed.ID || len(approval.Queries) == 0 {
		return nil, fmt.Errorf("semantic search requires approval for this feed URL and version and at least one approved query")
	}
	search := &CatalogSearch{provider: provider, version: feed.ID, queries: make(map[domain.ItemID]PublicText)}
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
	ordered := slices.Clone(offers)
	slices.SortFunc(ordered, func(a, b enrichment.Offer) int { return strings.Compare(a.ID, b.ID) })
	for _, offer := range ordered {
		if strings.TrimSpace(offer.Name) == "" {
			continue
		}
		search.offerIDs = append(search.offerIDs, offer.ID)
		search.texts = append(search.texts, PublicText{
			Text: offer.Name, SourceURL: feed.URL, Version: feed.ID, Approved: approval.Approved,
		})
	}
	return search, nil
}

func (s *CatalogSearch) Search(ctx context.Context, version string, query enrichment.SearchQuery, limit int) ([]enrichment.SearchHit, error) {
	if version != s.version {
		return nil, fmt.Errorf("semantic search catalog version mismatch")
	}
	input, approved := s.queries[query.ItemID]
	if !approved || limit <= 0 || len(s.texts) == 0 {
		return nil, nil
	}
	index, err := s.catalogIndex(ctx)
	if err != nil {
		return nil, err
	}
	vectors, err := s.provider.Embed(ctx, []PublicText{input})
	if err != nil {
		return nil, fmt.Errorf("embed semantic query for item %q: %w", query.ItemID, err)
	}
	queryVector := vectors[0].Vector
	if err := normalizeVector(queryVector); err != nil {
		return nil, fmt.Errorf("semantic query for item %q: %w", query.ItemID, err)
	}

	type candidate struct {
		id    string
		score float64
	}
	compare := func(a, b candidate) int {
		if byScore := cmp.Compare(b.score, a.score); byScore != 0 {
			return byScore
		}
		return strings.Compare(a.id, b.id)
	}
	ranked := make([]candidate, 0, min(limit, len(index)))
	for i, offer := range index {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var score float64
		for j, value := range offer.Vector {
			score += queryVector[j] * value
		}
		hit := candidate{id: s.offerIDs[i], score: score}
		position, _ := slices.BinarySearchFunc(ranked, hit, compare)
		if position == limit {
			continue
		}
		if len(ranked) == limit {
			ranked = ranked[:limit-1]
		}
		ranked = slices.Insert(ranked, position, hit)
	}
	hits := make([]enrichment.SearchHit, 0, len(ranked))
	for _, candidate := range ranked {
		hits = append(hits, enrichment.SearchHit{OfferID: candidate.id,
			Basis: fmt.Sprintf("offer name cosine similarity %.6f (model %s)", candidate.score, vectors[0].Model)})
	}
	return hits, nil
}

func (s *CatalogSearch) catalogIndex(ctx context.Context) ([]Embedding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.index != nil {
		return s.index, nil
	}
	vectors, err := s.provider.Embed(ctx, s.texts)
	if err != nil {
		return nil, fmt.Errorf("embed catalog %q: %w", s.version, err)
	}
	for i, offer := range vectors {
		if err := normalizeVector(offer.Vector); err != nil {
			return nil, fmt.Errorf("catalog offer %q: %w", s.offerIDs[i], err)
		}
	}
	s.index = vectors
	return s.index, nil
}

func normalizeVector(vector []float64) error {
	var squared float64
	for _, value := range vector {
		squared += value * value
	}
	if squared == 0 || math.IsNaN(squared) || math.IsInf(squared, 0) {
		return fmt.Errorf("invalid embedding norm")
	}
	length := math.Sqrt(squared)
	for i := range vector {
		vector[i] /= length
	}
	return nil
}
