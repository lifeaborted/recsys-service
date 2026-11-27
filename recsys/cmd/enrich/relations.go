package main

import (
	"context"
	"fmt"

	"recsys/internal/domain"
	"recsys/internal/enrichment"
	"recsys/internal/explain"
)

type relationRequest struct {
	ID        string        `json:"id"`
	From      domain.ItemID `json:"from"`
	To        domain.ItemID `json:"to"`
	SourceURL string        `json:"source_url"`
	Approved  bool          `json:"approved"`
}

type relationDraft struct {
	ID string `json:"id"`
	explain.RelationProposal
	ReviewNote string `json:"review_note,omitempty"`
}

func extractRelations(ctx context.Context, requests []relationRequest, documents []enrichment.Document, result enrichment.Result) ([]relationDraft, []enrichment.Rejection, error) {
	offers := make(map[string]enrichment.Offer, len(result.Offers))
	for _, offer := range result.Offers {
		offers[offer.ID] = offer
	}
	names := make(map[domain.ItemID]string, len(result.Matches))
	for _, match := range result.Matches {
		if match.Status == "selected" {
			names[match.ItemID] = offers[match.OfferID].Name
		}
	}
	sources := make(map[string]explain.PublicText, len(documents))
	for _, document := range documents {
		url := document.Source.URL
		if _, duplicate := sources[url]; duplicate {
			return nil, nil, fmt.Errorf("ambiguous relation source %q", url)
		}
		sources[url] = explain.PublicText{
			Text: document.Text, SourceURL: url, Version: document.Source.Version,
		}
	}
	ids := make(map[string]bool, len(requests))
	for _, request := range requests {
		if request.ID == "" || ids[request.ID] {
			return nil, nil, fmt.Errorf("empty or duplicate relation request ID %q", request.ID)
		}
		ids[request.ID] = true
		if !request.Approved {
			return nil, nil, fmt.Errorf("request %q: source excerpt must be approved", request.ID)
		}
		if _, ok := sources[request.SourceURL]; !ok {
			return nil, nil, fmt.Errorf("request %q: unknown relation source %q", request.ID, request.SourceURL)
		}
		if names[request.From] == "" || names[request.To] == "" || request.From == request.To {
			return nil, nil, fmt.Errorf("request %q: endpoints must match separate named offers", request.ID)
		}
	}

	chat := explain.ChatFromEnv()
	var drafts []relationDraft
	var rejections []enrichment.Rejection
	for _, request := range requests {
		source := sources[request.SourceURL]
		source.Approved = request.Approved
		proposal, err := chat.ProposeRelation(ctx, explain.RelationInput{
			From: names[request.From], To: names[request.To], Source: source,
		})
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		if err != nil {
			rejections = append(rejections, enrichment.Rejection{ID: request.ID, Reason: err.Error()})
			continue
		}
		draft := relationDraft{ID: request.ID, RelationProposal: proposal}
		if proposal.Status == "review" {
			draft.ReviewNote = "Связь требует проверки."
			if proposal.Kind == domain.Complement {
				draft.ReviewNote = "Совместимость требует проверки."
			}
		}
		drafts = append(drafts, draft)
	}
	return drafts, rejections, nil
}
