package eval

import (
	"fmt"

	"recsys/internal/data"
	"recsys/internal/domain"
)

// CheckEvalAnonymization runs v4 anonymization threshold checks on evaluated sales slices.
func CheckEvalAnonymization(sales []Sale) (data.AnonymizationReport, error) {
	seenClients := make(map[string]struct{})
	clients := make([]domain.Client, 0)
	domainSales := make([]domain.Sale, 0, len(sales))
	for i, s := range sales {
		if _, ok := seenClients[s.ClientID]; !ok {
			seenClients[s.ClientID] = struct{}{}
			clients = append(clients, domain.Client{
				ID:    domain.ClientID(s.ClientID),
				Group: domain.ClientGroup(s.Group),
			})
		}
		domainSales = append(domainSales, domain.Sale{
			SourceRow: i + 1,
			ClientID:  domain.ClientID(s.ClientID),
			ItemID:    domain.ItemID(s.ItemID),
		})
	}
	return data.CheckAnonymization(clients, domainSales)
}

// VerifyV4Evaluation validates closed v4 release metadata, expected dimensions, and anonymization thresholds.
func VerifyV4Evaluation(sha string, sales []Sale) (DatasetSummary, data.AnonymizationReport, error) {
	return VerifyClosedEvaluation("v4", sha, sales, nil)
}

func VerifyClosedEvaluation(version, sha string, sales []Sale, groups map[domain.ItemID]string) (DatasetSummary, data.AnonymizationReport, error) {
	summary := SummarizeDatasetWithGroups(sales, ProductGroupStrings(groups))
	var err error
	switch version {
	case data.ExpectedV4Version:
		err = data.VerifyV4Release(version, sha, summary.Rows, summary.TotalItems)
	case data.ExpectedV5Version:
		err = data.VerifyV5Release(version, sha, summary.Rows, summary.TotalItems, summary.ProductGroups)
	default:
		err = fmt.Errorf("unsupported closed dataset %q", version)
	}
	if err != nil {
		return summary, data.AnonymizationReport{}, err
	}
	if summary.Baskets != data.ExpectedV4Baskets ||
		summary.TrainItems != data.ExpectedV4TrainItems ||
		len(summary.ColdItems) != data.ExpectedV4ColdItems {
		return summary, data.AnonymizationReport{}, fmt.Errorf(
			"unexpected closed dataset split counts: baskets=%d train_items=%d cold_items=%d (want %d/%d/%d)",
			summary.Baskets, summary.TrainItems, len(summary.ColdItems),
			data.ExpectedV4Baskets, data.ExpectedV4TrainItems, data.ExpectedV4ColdItems,
		)
	}
	anon, err := CheckEvalAnonymization(sales)
	if err != nil {
		return summary, anon, err
	}
	return summary, anon, nil
}
