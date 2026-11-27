package domain

type RecommendationKind string

const (
	Renewal    RecommendationKind = "renewal"
	Upgrade    RecommendationKind = "upgrade"
	Complement RecommendationKind = "complement"
	Behavioral RecommendationKind = "behavioral"
	Popular    RecommendationKind = "popular"
)

type Candidate struct {
	ItemID   ItemID
	Kind     RecommendationKind
	Score    float64
	Evidence []Evidence
}

type Evidence struct {
	ID         string  `json:"id"`
	From       string  `json:"from"`
	To         string  `json:"to"`
	Type       string  `json:"type"`
	Weight     float64 `json:"weight"`
	Reason     string  `json:"reason"`
	Source     string  `json:"source"`
	SnapshotID string  `json:"snapshot_id"`
}

type Node struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Label string `json:"label"`
}

type Graph struct {
	Nodes []Node     `json:"nodes"`
	Edges []Evidence `json:"edges"`
}

type Difference struct {
	Field  string `json:"field"`
	Value  any    `json:"value"`
	Unit   string `json:"unit"`
	Source string `json:"source"`
}

type Variant struct {
	ItemID      ItemID       `json:"item_id"`
	URL         *string      `json:"url,omitempty"`
	Differences []Difference `json:"differences"`
}

type Recommendation struct {
	ItemID       ItemID             `json:"item_id"`
	OfferID      *string            `json:"offer_id,omitempty"`
	SKU          *string            `json:"sku"`
	Kind         RecommendationKind `json:"recommendation_kind"`
	FamilyID     *string            `json:"family_id"`
	Title        string             `json:"title"`
	URL          *string            `json:"url,omitempty"`
	Price        *float64           `json:"price,omitempty"`
	CurrencyCode *string            `json:"currency_code,omitempty"`
	Category     *string            `json:"category,omitempty"`
	Score        float64            `json:"score"`
	Explanation  string             `json:"explanation"`
	Variants     []Variant          `json:"variants"`
	EvidenceIDs  []string           `json:"evidence_ids"`
}
