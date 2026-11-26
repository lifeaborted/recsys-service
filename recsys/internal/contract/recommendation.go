package contract

import (
	"context"

	"recsys/internal/domain"
)

type Mode string

const (
	Online   Mode = "online"
	Backtest Mode = "backtest"
)

type RecommendationQuery struct {
	ClientID domain.ClientID
	// Empty selects the active snapshot for Online mode. Backtest pins a snapshot.
	SnapshotID string
	Mode       Mode
}

type RecommendationResult struct {
	SnapshotID    string                  `json:"snapshot_id"`
	Items         []domain.Recommendation `json:"items"`
	Graph         domain.Graph            `json:"graph"`
	PartialReason *string                 `json:"partial_reason"`
}

type RecommendationResponse struct {
	RequestID string `json:"request_id"`
	RecommendationResult
}

// Recommender is shared by HTTP and historical evaluation.
type Recommender interface {
	Recommend(ctx context.Context, query RecommendationQuery) (RecommendationResult, error)
}
