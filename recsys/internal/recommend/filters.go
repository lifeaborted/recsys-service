package recommend

import (
	"strings"

	"recsys/internal/contract"
	"recsys/internal/domain"
)

// Product holds snapshot facts keyed by item_id, including historical items without offers.
// Empty Platform or Audience means the corresponding restriction is unknown or absent.
type Product struct {
	Title                  string
	OfferID, SKU, URL      *string
	Platform               string
	Audience               domain.ClientGroup
	OrganizationType       string
	Available, InSales     bool
	FamilyID, FamilySource string
	Differences            []domain.Difference
	Vendor, CategoryID     string
}

func eligible(p Product, client domain.Client, platforms map[string]bool, mode contract.Mode) bool {
	if mode == contract.Backtest {
		return p.InSales
	}
	if p.Audience != "" && p.Audience != client.Group {
		return false
	}
	if p.Platform != "" && len(platforms) > 0 && !platforms[p.Platform] {
		return false
	}
	if !eligibleOrganization(p.OrganizationType, client) {
		return false
	}
	return p.Available && p.OfferID != nil && *p.OfferID != ""
}

// sameCategoryRival drops a different vendor in a category the buyer already owns.
// Documented renewal, upgrade and complement edges stay: those are the same product line or an add-on.
func sameCategoryRival(candidate Product, owned map[domain.ItemID]Product, kind domain.RecommendationKind) bool {
	if kind == domain.Renewal || kind == domain.Upgrade || kind == domain.Complement {
		return false
	}
	vendor, category := normalizeVendor(candidate.Vendor), strings.TrimSpace(candidate.CategoryID)
	if vendor == "" || category == "" {
		return false
	}
	for _, have := range owned {
		if strings.TrimSpace(have.CategoryID) != category {
			continue
		}
		known := normalizeVendor(have.Vendor)
		if known != "" && known != vendor {
			return true
		}
	}
	return false
}

func normalizeVendor(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), " ")
}

func eligibleOrganization(restriction string, client domain.Client) bool {
	switch restriction {
	case "", "не определено":
		return true
	case "для дома":
		return client.Group == domain.B2C
	}
	if client.Group != domain.B2B {
		return false
	}
	// Profile segments can describe company size. Require an explicit type match.
	kind := strings.ToLower(strings.TrimSpace(client.Type))
	return kind == restriction || kind == restriction+" организация" || (kind == "нко" && restriction == "некоммерческая")
}
