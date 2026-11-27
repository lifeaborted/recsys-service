package recommendgrpc

import (
	"fmt"

	recommendationv1 "recsys/api/recommendation/v1"
	"recsys/internal/contract"
	"recsys/internal/domain"

	"google.golang.org/protobuf/types/known/structpb"
)

func encodeResult(result contract.RecommendationResult) (*recommendationv1.GetRecommendationsResponse, error) {
	response := &recommendationv1.GetRecommendationsResponse{
		SnapshotId:    result.SnapshotID,
		PartialReason: result.PartialReason,
		Graph:         &recommendationv1.Graph{},
	}
	for _, item := range result.Items {
		card := &recommendationv1.Recommendation{
			ItemId: string(item.ItemID), OfferId: item.OfferID, Sku: item.SKU,
			RecommendationKind: string(item.Kind), FamilyId: item.FamilyID, Title: item.Title,
			Url: item.URL, Price: item.Price, CurrencyCode: item.CurrencyCode, Category: item.Category,
			Score: item.Score, Explanation: item.Explanation, EvidenceIds: item.EvidenceIDs,
		}
		if item.Status != "" {
			card.Status = &item.Status
		}
		for _, variant := range item.Variants {
			wire := &recommendationv1.Variant{ItemId: string(variant.ItemID), Url: variant.URL}
			for _, difference := range variant.Differences {
				value, err := structpb.NewValue(difference.Value)
				if err != nil {
					return nil, fmt.Errorf("encode variant difference %q: %w", difference.Field, err)
				}
				wire.Differences = append(wire.Differences, &recommendationv1.Difference{
					Field: difference.Field, Value: value, Unit: difference.Unit, Source: difference.Source,
				})
			}
			card.Variants = append(card.Variants, wire)
		}
		response.Items = append(response.Items, card)
	}
	for _, node := range result.Graph.Nodes {
		response.Graph.Nodes = append(response.Graph.Nodes, &recommendationv1.Node{
			Id: node.ID, Type: node.Type, Label: node.Label,
		})
	}
	for _, edge := range result.Graph.Edges {
		response.Graph.Edges = append(response.Graph.Edges, &recommendationv1.Evidence{
			Id: edge.ID, From: edge.From, To: edge.To, Type: edge.Type, Weight: edge.Weight,
			Reason: edge.Reason, Source: edge.Source, SnapshotId: edge.SnapshotID,
		})
	}
	return response, nil
}

func decodeResult(response *recommendationv1.GetRecommendationsResponse) contract.RecommendationResult {
	result := contract.RecommendationResult{
		SnapshotID:    response.GetSnapshotId(),
		PartialReason: response.PartialReason,
	}
	for _, item := range response.GetItems() {
		card := domain.Recommendation{
			ItemID: domain.ItemID(item.GetItemId()), OfferID: item.OfferId, SKU: item.Sku,
			Kind: domain.RecommendationKind(item.GetRecommendationKind()), FamilyID: item.FamilyId, Title: item.GetTitle(),
			URL: item.Url, Price: item.Price, CurrencyCode: item.CurrencyCode, Category: item.Category,
			Score: item.GetScore(), Explanation: item.GetExplanation(), EvidenceIDs: item.GetEvidenceIds(),
			Status: item.GetStatus(),
		}
		for _, variant := range item.GetVariants() {
			decoded := domain.Variant{ItemID: domain.ItemID(variant.GetItemId()), URL: variant.Url}
			for _, difference := range variant.GetDifferences() {
				decoded.Differences = append(decoded.Differences, domain.Difference{
					Field: difference.GetField(), Value: difference.GetValue().AsInterface(),
					Unit: difference.GetUnit(), Source: difference.GetSource(),
				})
			}
			card.Variants = append(card.Variants, decoded)
		}
		result.Items = append(result.Items, card)
	}
	for _, node := range response.GetGraph().GetNodes() {
		result.Graph.Nodes = append(result.Graph.Nodes, domain.Node{
			ID: node.GetId(), Type: node.GetType(), Label: node.GetLabel(),
		})
	}
	for _, edge := range response.GetGraph().GetEdges() {
		result.Graph.Edges = append(result.Graph.Edges, domain.Evidence{
			ID: edge.GetId(), From: edge.GetFrom(), To: edge.GetTo(), Type: edge.GetType(), Weight: edge.GetWeight(),
			Reason: edge.GetReason(), Source: edge.GetSource(), SnapshotID: edge.GetSnapshotId(),
		})
	}
	return result
}
