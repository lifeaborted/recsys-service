package recommend

import (
	"strings"

	"recsys/internal/contract"
	"recsys/internal/domain"
)

func cardLimit(mode contract.Mode) int {
	if mode == contract.Online {
		return 36
	}
	return 3
}

func render(snapshot string, mode contract.Mode, ordered []ranked, cards map[domain.ItemID]Product) contract.RecommendationResult {
	out := contract.RecommendationResult{SnapshotID: snapshot, Items: []domain.Recommendation{}, Graph: domain.Graph{Nodes: []domain.Node{}, Edges: []domain.Evidence{}}}
	type group struct {
		family string
		kind   domain.RecommendationKind
	}
	groups := map[group]int{}
	nodes, edges := map[string]bool{}, map[string]bool{}
	for _, r := range ordered {
		p := cards[r.id]
		g := group{p.FamilyID, r.kind}
		grouped := mode == contract.Online && p.FamilyID != "" && p.FamilySource != ""
		idx, exists := groups[g]
		if !grouped || !exists {
			if len(out.Items) == cardLimit(mode) {
				continue
			}
			idx = len(out.Items)
			var family *string
			if p.FamilySource != "" && p.FamilyID != "" {
				family = &p.FamilyID
			}
			title := p.Title
			if title == "" {
				title = string(r.id)
			}
			out.Items = append(out.Items, domain.Recommendation{ItemID: r.id, Kind: r.kind, OfferID: p.OfferID, SKU: p.SKU, URL: p.URL, FamilyID: family, Title: title, Explanation: r.proof[0].Reason, Variants: []domain.Variant{}, EvidenceIDs: []string{}})
			if grouped {
				groups[g] = idx
			}
		}
		item := &out.Items[idx]
		if grouped {
			diffs := []domain.Difference{}
			for _, d := range p.Differences {
				if d.Source != "" {
					diffs = append(diffs, d)
				}
			}
			item.Variants = append(item.Variants, domain.Variant{ItemID: r.id, URL: p.URL, Differences: diffs})
		}
		for _, e := range r.proof {
			if edges[e.ID] {
				continue
			}
			edges[e.ID] = true
			item.EvidenceIDs = append(item.EvidenceIDs, e.ID)
			attachNode := func(id string) {
				if nodes[id] {
					return
				}
				nodes[id] = true
				kind, label := "cohort", id
				if raw, ok := strings.CutPrefix(id, "item:"); ok {
					kind, label = "product", raw
					if t := cards[domain.ItemID(raw)].Title; t != "" {
						label = t
					}
				}
				out.Graph.Nodes = append(out.Graph.Nodes, domain.Node{ID: id, Type: kind, Label: label})
			}
			attachNode(e.From)
			attachNode(e.To)
			out.Graph.Edges = append(out.Graph.Edges, e)
		}
	}
	if len(out.Items) < 3 {
		reason := "insufficient_eligible_products"
		out.PartialReason = &reason
	}
	return out
}
