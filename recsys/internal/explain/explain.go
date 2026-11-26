package explain

import (
	"context"
	"fmt"
	"strings"

	"recsys/internal/domain"
	"recsys/internal/enrichment"
)

type Rendered struct {
	Text          string `json:"text"`
	Provider      string `json:"provider"`
	InputHash     string `json:"input_sha256,omitempty"`
	PromptVersion string `json:"prompt_version,omitempty"`
}

// Render accepts verified edges and variants selected for the card. A model
// can choose presentation order from an explicitly approved source excerpt.
func Render(ctx context.Context, chat *Chat, public *PublicText, edges []domain.Evidence, variants []domain.Variant) Rendered {
	reasons := []string{}
	comparisons := []string{}
	for _, edge := range edges {
		if edge.Source != "" && edge.Reason != "" {
			reasons = append(reasons, edge.Reason)
		}
	}
	for _, variant := range variants {
		for _, delta := range variant.Differences {
			if delta.Field == "term" && delta.Unit == "month" && delta.Source != "" {
				comparisons = append(comparisons, fmt.Sprintf("%s: %v мес.", variant.ItemID, delta.Value))
			}
		}
	}
	if len(reasons)+len(comparisons) == 0 {
		return Rendered{Text: "Подтверждённых сведений для сравнения нет.", Provider: "template"}
	}
	template := strings.Join(append(reasons, comparisons...), " ")
	if chat == nil || public == nil || len(reasons) == 0 || len(comparisons) == 0 {
		return Rendered{Text: template, Provider: "template"}
	}
	reply, err := chat.Complete(ctx, *public)
	if err == nil {
		result := Rendered{Text: template, Provider: reply.Model, InputHash: reply.InputHash, PromptVersion: reply.PromptVersion}
		switch strings.TrimSpace(reply.Choice) {
		case "1":
			return result
		case "2":
			result.Text = strings.Join(append(comparisons, reasons...), " ")
			return result
		}
	}
	return Rendered{Text: template, Provider: "template"}
}

// EdgesFor returns the incident verified relations for one product card.
func EdgesFor(item domain.ItemID, relations []enrichment.VerifiedRelation) []domain.Evidence {
	edges := []domain.Evidence{}
	node := "item:" + string(item)
	for _, relation := range relations {
		if relation.Evidence.To == node || relation.Evidence.From == node {
			edges = append(edges, relation.Evidence)
		}
	}
	return edges
}
