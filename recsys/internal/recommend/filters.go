package recommend

import (
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
	Available, InSales     bool
	FamilyID, FamilySource string
	Differences            []domain.Difference
}

func eligible(p Product, group domain.ClientGroup, platforms map[string]bool, mode contract.Mode) bool {
	if p.Audience != "" && p.Audience != group {
		return false
	}
	if p.Platform != "" && len(platforms) > 0 && !platforms[p.Platform] {
		return false
	}
	if mode == contract.Backtest {
		return p.InSales
	}
	return p.Available && p.OfferID != nil && *p.OfferID != ""
}
