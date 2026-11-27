package contract

import (
	"context"
	"errors"
	"time"

	"recsys/internal/domain"
)

var ErrRunNotFound = errors.New("run or offer not found")

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
	CreateEvent(ctx context.Context, clientID domain.ClientID, runID int64, offerID, eventType string) error
}

const PreviewScenarioPrefix = "SCENARIO-"

var ErrPreviewSessionUnavailable = errors.New("no active preview session")

type PreviewSession struct {
	ClientID      domain.ClientID `json:"-"`
	StartedAt     time.Time       `json:"started_at"`
	PurchaseCount int             `json:"purchase_count"`
}

type PurchaseHistoryItem struct {
	OrderID     string    `json:"order_id"`
	Title       string    `json:"title"`
	PurchasedAt time.Time `json:"purchased_at"`
	Total       *float64  `json:"total,omitempty"`
	Currency    *string   `json:"currency,omitempty"`
}
