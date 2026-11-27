package contract

import (
	"context"
	"time"

	"recsys/internal/domain"
)

// RunItemInput is one item of a stand "Новый run" request.
type RunItemInput struct {
	ItemID      string  `json:"item_id"`
	OfferID     string  `json:"offer_id,omitempty"`
	Explanation string  `json:"explanation,omitempty"`
	Score       float64 `json:"score"`
}

type RunItemRecord struct {
	ItemID      string  `json:"item_id"`
	OfferID     string  `json:"offer_id"`
	Explanation string  `json:"explanation"`
	Score       float64 `json:"score"`
}

type RunRecord struct {
	RunID        int64           `json:"run_id"`
	ClientID     string          `json:"client_id,omitempty"`
	ModelVersion string          `json:"model_version"`
	RequestedAt  time.Time       `json:"requested_at"`
	Items        []RunItemRecord `json:"items"`
	Clicks       int             `json:"clicks"`
	Purchases    int             `json:"purchases"`
}

// RunsStore persists stand runs and their feedback events.
type RunsStore interface {
	CreateRun(ctx context.Context, clientID domain.ClientID, modelVersion string, items []RunItemInput) (RunRecord, error)
	ListRuns(ctx context.Context, clientID domain.ClientID, limit int) ([]RunRecord, error)
	CreateEvent(ctx context.Context, runID int64, offerID, eventType string) error
}
