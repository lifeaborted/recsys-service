package enrichment

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"recsys/internal/domain"
)

type Item struct {
	ID   domain.ItemID `json:"item_id"`
	SKU  string        `json:"sku"`
	Name string        `json:"name"`
}

type SearchQuery struct {
	SKU  string
	Name string
}

type SearchHit struct {
	OfferID string `json:"offer_id"`
	Basis   string `json:"basis"`
}

// Searcher returns suggestions scoped to the requested catalog version.
type Searcher interface {
	Search(ctx context.Context, version string, query SearchQuery, limit int) ([]SearchHit, error)
}

type Match struct {
	SearchHits []SearchHit   `json:"search_hits,omitempty"`
	ItemID     domain.ItemID `json:"item_id"`
	OfferID    string        `json:"offer_id,omitempty"`
	Method     string        `json:"method,omitempty"`
	Status     string        `json:"status"`
	Candidates []string      `json:"candidates"`
	Basis      string        `json:"basis"`
}

type Matcher struct {
	version        string
	sku, url, name map[string][]string
	offers         map[string]struct{}
	search         Searcher
}

func NewMatcher(version string, offers []Offer, search Searcher) *Matcher {
	matcher := &Matcher{version: version, sku: make(map[string][]string), url: make(map[string][]string),
		name: make(map[string][]string), offers: make(map[string]struct{}), search: search}
	for _, offer := range offers {
		matcher.offers[offer.ID] = struct{}{}
		for _, entry := range []struct {
			index map[string][]string
			key   string
		}{
			{matcher.sku, normalizeSKU(offer.SKU)}, {matcher.url, urlSKU(offer.URL)}, {matcher.name, normalizeName(offer.Name)},
		} {
			if entry.key != "" {
				entry.index[entry.key] = append(entry.index[entry.key], offer.ID)
			}
		}
	}
	return matcher
}

func normalizeSKU(s string) string  { return strings.ToUpper(strings.TrimSpace(s)) }
func normalizeName(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

func (m *Matcher) Match(ctx context.Context, item Item) (Match, error) {
	result := Match{ItemID: item.ID, Status: "unmatched", Candidates: []string{}, Basis: "no exact match"}
	if item.ID == "" {
		return result, fmt.Errorf("item without id")
	}
	sku, name := normalizeSKU(item.SKU), normalizeName(item.Name)
	if sku != "" {
		if ids := m.sku[sku]; len(ids) > 0 {
			return exact(result, "vendor_code", sku, ids), nil
		}
		if ids := m.url[sku]; len(ids) > 0 {
			return exact(result, "url_code", sku, ids), nil
		}
	} else if ids := m.name[name]; name != "" && len(ids) > 0 {
		result = exact(result, "name", name, ids)
		if result.Status == "selected" {
			result.Status, result.OfferID, result.Basis = "review", "", "exact name without SKU requires verification"
		}
		return result, nil
	}
	if m.search == nil || (sku == "" && name == "") {
		return result, nil
	}
	hits, err := m.search.Search(ctx, m.version, SearchQuery{SKU: item.SKU, Name: item.Name}, 10)
	if err != nil {
		return result, err
	}
	for _, hit := range hits {
		if _, ok := m.offers[hit.OfferID]; !ok {
			return result, fmt.Errorf("search returned offer %q outside catalog %q", hit.OfferID, m.version)
		}
		result.Candidates = append(result.Candidates, hit.OfferID)
	}
	if len(result.Candidates) > 0 {
		result.SearchHits = slices.Clone(hits)
		slices.SortFunc(result.SearchHits, func(a, b SearchHit) int {
			if a.OfferID == b.OfferID {
				return strings.Compare(a.Basis, b.Basis)
			}
			return strings.Compare(a.OfferID, b.OfferID)
		})
		result.SearchHits = slices.Compact(result.SearchHits)
		slices.Sort(result.Candidates)
		result.Candidates = slices.Compact(result.Candidates)
		result.Status, result.Method, result.Basis = "review", "search", "search candidates require verification"
	}
	return result, nil
}

func exact(result Match, method, key string, ids []string) Match {
	result.Method, result.Basis = method, key
	result.Candidates = slices.Clone(ids)
	slices.Sort(result.Candidates)
	result.Candidates = slices.Compact(result.Candidates)
	if len(result.Candidates) == 1 {
		result.Status, result.OfferID = "selected", result.Candidates[0]
	} else {
		result.Status, result.Basis = "ambiguous", "multiple offers match "+key
	}
	return result
}
