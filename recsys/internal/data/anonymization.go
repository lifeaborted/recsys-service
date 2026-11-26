package data

import (
	"fmt"

	"recsys/internal/domain"
)

const (
	// MinItemBuyers is the minimum number of distinct buyers per item_id required by v4 anonymization.
	MinItemBuyers = 3
	// MinB2CItemBuyers is the minimum number of distinct B2C buyers for any item_id appearing in B2C sales.
	MinB2CItemBuyers = 5
)

// AnonymizationViolation describes a threshold violation for one item_id.
type AnonymizationViolation struct {
	ItemID    domain.ItemID
	Buyers    int
	B2CBuyers int
	Reason    string
}

// AnonymizationReport summarizes anonymization compliance across all item_id values.
type AnonymizationReport struct {
	ItemsChecked    int
	B2CItemsChecked int
	MinItemBuyers   int
	MinB2CBuyers    int
	Violations      []AnonymizationViolation
}

// CheckAnonymization verifies v4 anonymization thresholds (>=3 buyers per item_id,
// and >=5 distinct B2C buyers for any item_id purchased by B2C clients).
func CheckAnonymization(clients []domain.Client, sales []domain.Sale) (AnonymizationReport, error) {
	groups := make(map[domain.ClientID]domain.ClientGroup, len(clients))
	for _, c := range clients {
		groups[c.ID] = c.Group
	}

	buyers := make(map[domain.ItemID]map[domain.ClientID]struct{})
	b2cBuyers := make(map[domain.ItemID]map[domain.ClientID]struct{})
	for _, s := range sales {
		group, ok := groups[s.ClientID]
		if !ok {
			return AnonymizationReport{}, fmt.Errorf("anonymization check: sale row %d references unknown client %q", s.SourceRow, s.ClientID)
		}
		if buyers[s.ItemID] == nil {
			buyers[s.ItemID] = make(map[domain.ClientID]struct{})
		}
		buyers[s.ItemID][s.ClientID] = struct{}{}
		if group == domain.B2C {
			if b2cBuyers[s.ItemID] == nil {
				b2cBuyers[s.ItemID] = make(map[domain.ClientID]struct{})
			}
			b2cBuyers[s.ItemID][s.ClientID] = struct{}{}
		}
	}

	report := AnonymizationReport{
		ItemsChecked:    len(buyers),
		B2CItemsChecked: len(b2cBuyers),
	}
	for itemID, itemBuyers := range buyers {
		total := len(itemBuyers)
		b2c := len(b2cBuyers[itemID])
		if report.MinItemBuyers == 0 || total < report.MinItemBuyers {
			report.MinItemBuyers = total
		}
		if b2c > 0 && (report.MinB2CBuyers == 0 || b2c < report.MinB2CBuyers) {
			report.MinB2CBuyers = b2c
		}
		if total < MinItemBuyers {
			report.Violations = append(report.Violations, AnonymizationViolation{
				ItemID:    itemID,
				Buyers:    total,
				B2CBuyers: b2c,
				Reason:    fmt.Sprintf("item_id has %d distinct buyers (< %d)", total, MinItemBuyers),
			})
			continue
		}
		if b2c > 0 && b2c < MinB2CItemBuyers {
			report.Violations = append(report.Violations, AnonymizationViolation{
				ItemID:    itemID,
				Buyers:    total,
				B2CBuyers: b2c,
				Reason:    fmt.Sprintf("B2C item_id has %d distinct B2C buyers (< %d)", b2c, MinB2CItemBuyers),
			})
		}
	}
	if len(report.Violations) > 0 {
		first := report.Violations[0]
		return report, fmt.Errorf("anonymization check failed (%d violations; first %s: %s)",
			len(report.Violations), first.ItemID, first.Reason)
	}
	return report, nil
}
