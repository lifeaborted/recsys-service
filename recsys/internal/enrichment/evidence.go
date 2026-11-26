package enrichment

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"recsys/internal/domain"
)

// ReviewedStatement binds a verified excerpt to a directional product claim.
// Only curated records, never raw provider output, belong in the review file.
type ReviewedStatement struct {
	ID          string                    `json:"id"`
	From        domain.ItemID             `json:"from"`
	To          domain.ItemID             `json:"to"`
	Kind        domain.RecommendationKind `json:"kind"`
	Source      Source                    `json:"source"`
	Constraints []Constraint              `json:"constraints"`
}

type Constraint struct {
	Endpoint string `json:"endpoint"`
	Field    string `json:"field"`
	Value    string `json:"value"`
	Unit     string `json:"unit,omitempty"`
}

type Document struct {
	Source     Source              `json:"source"`
	Text       string              `json:"text"`
	Statements []ReviewedStatement `json:"statements"`
}

type Rule struct {
	ID    string                    `json:"id"`
	From  domain.ItemID             `json:"from"`
	To    domain.ItemID             `json:"to"`
	Kind  domain.RecommendationKind `json:"kind"`
	Quote string                    `json:"quote"`
}

type Review struct {
	Documents []Document `json:"documents"`
	Rules     []Rule     `json:"rules"`
}

type VerifiedRelation struct {
	Evidence    domain.Evidence `json:"evidence"`
	Source      Source          `json:"source"`
	Constraints []Constraint    `json:"constraints"`
}

type FamilyFact struct {
	ItemID domain.ItemID `json:"item_id"`
	Family Fact          `json:"family"`
}

type Rejection struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

type Result struct {
	Feed       FeedVersion                 `json:"feed"`
	SnapshotID string                      `json:"snapshot_id"`
	Cutoff     *time.Time                  `json:"cutoff"`
	PostCutoff bool                        `json:"post_cutoff"`
	Offers     []Offer                     `json:"offers"`
	Matches    []Match                     `json:"matches"`
	Families   []FamilyFact                `json:"families"`
	Variants   map[string][]domain.Variant `json:"variants_by_family"`
	Relations  []VerifiedRelation          `json:"relations"`
	Rejected   []Rejection                 `json:"rejected"`
}

// Build assembles one complete batch for the supplied catalog and recommendation snapshot.
func Build(ctx context.Context, feed FeedVersion, snapshotID string, cutoff *time.Time, offers []Offer, items []Item, review Review, search Searcher) (Result, error) {
	result := Result{Feed: feed, SnapshotID: snapshotID, Cutoff: cutoff,
		Offers: append([]Offer{}, offers...), Matches: []Match{}, Families: []FamilyFact{},
		Variants: make(map[string][]domain.Variant), Relations: []VerifiedRelation{}, Rejected: []Rejection{}}
	if feed.ID == "" || snapshotID == "" || feed.GeneratedAt.IsZero() {
		return result, fmt.Errorf("feed version, date and snapshot ID required")
	}
	result.PostCutoff = cutoff != nil && feed.GeneratedAt.After(*cutoff)
	slices.SortFunc(result.Offers, func(a, b Offer) int { return strings.Compare(a.ID, b.ID) })
	items = slices.Clone(items)
	slices.SortFunc(items, func(a, b Item) int { return strings.Compare(string(a.ID), string(b.ID)) })
	byOffer := make(map[string]Offer)
	for _, offer := range result.Offers {
		if _, ok := byOffer[offer.ID]; ok || offer.ID == "" {
			return result, fmt.Errorf("empty or duplicate offer ID %q", offer.ID)
		}
		byOffer[offer.ID] = offer
	}
	matcher := NewMatcher(feed.ID, result.Offers, search)
	claimed := make(map[string]int)
	for i, item := range items {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if i > 0 && item.ID == items[i-1].ID {
			return result, fmt.Errorf("duplicate item ID %q", item.ID)
		}
		match, err := matcher.Match(ctx, item)
		if err != nil {
			return result, err
		}
		result.Matches = append(result.Matches, match)
		if match.OfferID != "" {
			claimed[match.OfferID]++
		}
	}
	selected := make(map[domain.ItemID]Offer)
	for i, match := range result.Matches {
		if match.OfferID == "" {
			continue
		}
		if claimed[match.OfferID] > 1 {
			result.Matches[i].Status, result.Matches[i].OfferID, result.Matches[i].Basis = "ambiguous", "", "multiple item IDs claim the same offer"
			continue
		}
		offer := byOffer[match.OfferID]
		selected[match.ItemID] = offer
		family := offer.Facts["family"]
		if family.Value == nil || family.Source == nil {
			result.Families = append(result.Families, FamilyFact{ItemID: match.ItemID})
			continue
		}
		result.Families = append(result.Families, FamilyFact{ItemID: match.ItemID, Family: family})
		variant := domain.Variant{ItemID: match.ItemID, Differences: []domain.Difference{}}
		if webURL(offer.URL) {
			variant.URL = &offer.URL
		}
		for _, field := range []string{"license_type", "edition", "platform", "seats", "term"} {
			fact := offer.Facts[field]
			if fact.Value != nil && fact.Source != nil {
				variant.Differences = append(variant.Differences, difference(field, fact))
			}
		}
		key := *family.Value
		result.Variants[key] = append(result.Variants[key], variant)
	}
	for key, variants := range result.Variants {
		if len(variants) < 2 {
			delete(result.Variants, key)
		}
	}
	if err := result.verify(review, selected); err != nil {
		return result, err
	}
	return result, nil
}

func difference(field string, fact Fact) domain.Difference {
	raw := *fact.Value
	value, unit := any(raw), fact.Unit
	if unit == "" {
		unit = "text"
		if field == "seats" {
			unit = "seat"
		}
	}
	if number, err := strconv.Atoi(raw); err == nil && number > 0 && unit != "text" {
		value = number
	}
	return domain.Difference{Field: field, Value: value, Unit: unit, Source: sourceKey(*fact.Source)}
}

func sourceKey(source Source) string { return source.URL + "@" + source.Version + "#" + source.Locator }

func (r *Result) verify(review Review, selected map[domain.ItemID]Offer) error {
	type record struct {
		statement ReviewedStatement
		document  Document
	}
	reviewed := make(map[string]record)
	for _, document := range review.Documents {
		for _, statement := range document.Statements {
			if _, duplicate := reviewed[statement.ID]; duplicate || statement.ID == "" {
				return fmt.Errorf("empty or duplicate reviewed statement ID %q", statement.ID)
			}
			reviewed[statement.ID] = record{statement, document}
		}
	}
	rules := slices.Clone(review.Rules)
	slices.SortFunc(rules, func(a, b Rule) int { return strings.Compare(a.ID, b.ID) })
	for i, rule := range rules {
		if rule.ID == "" || (i > 0 && rule.ID == rules[i-1].ID) {
			return fmt.Errorf("empty or duplicate rule ID %q", rule.ID)
		}
		record := reviewed[rule.ID]
		source := record.document.Source
		source.Quote = rule.Quote
		source.Locator = rule.ID
		reason := reject(rule, record.statement, record.document, selected)
		if reason != "" {
			r.Rejected = append(r.Rejected, Rejection{ID: rule.ID, Reason: reason})
			continue
		}
		if r.Cutoff != nil && source.ObservedAt.After(*r.Cutoff) {
			r.PostCutoff = true
		}
		edge := domain.Evidence{ID: rule.ID, From: "item:" + string(rule.From), To: "item:" + string(rule.To),
			Type: string(rule.Kind), Weight: 1, Reason: rule.Quote, Source: sourceKey(source), SnapshotID: r.SnapshotID}
		r.Relations = append(r.Relations, VerifiedRelation{Evidence: edge, Source: source, Constraints: record.statement.Constraints})
	}
	return nil
}

func reject(rule Rule, statement ReviewedStatement, document Document, selected map[domain.ItemID]Offer) string {
	if rule.Kind != domain.Renewal && rule.Kind != domain.Upgrade && rule.Kind != domain.Complement {
		return "unsupported relation kind"
	}
	if rule.From == rule.To || statement.From != rule.From || statement.To != rule.To || statement.Kind != rule.Kind {
		return "direction or relation kind is not confirmed"
	}
	if !webURL(document.Source.URL) || document.Source.Version == "" || document.Source.ObservedAt.IsZero() ||
		strings.TrimSpace(rule.Quote) == "" || statement.Source.Quote != rule.Quote || !strings.Contains(document.Text, rule.Quote) {
		return "quote is not confirmed by the versioned source"
	}
	from, fromOK := selected[rule.From]
	to, toOK := selected[rule.To]
	if !fromOK || !toOK || from.ID == to.ID {
		return "endpoints must resolve to separate offers"
	}
	if rule.Kind == domain.Renewal {
		fromType, toType := from.Facts["license_type"], to.Facts["license_type"]
		if fromType.Value == nil || toType.Value == nil || !strings.EqualFold(*fromType.Value, "base") || !strings.EqualFold(*toType.Value, "renewal") {
			return "renewal requires verified base and renewal offers"
		}
	}
	for _, constraint := range statement.Constraints {
		var offer Offer
		switch constraint.Endpoint {
		case "from":
			offer = from
		case "to":
			offer = to
		default:
			return "unsupported constraint endpoint"
		}
		fact := offer.Facts[constraint.Field]
		if fact.Value == nil || fact.Source == nil || *fact.Value != constraint.Value || (constraint.Unit != "" && fact.Unit != constraint.Unit) {
			return "catalog constraint is not confirmed"
		}
	}
	return ""
}
