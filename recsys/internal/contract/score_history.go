package contract

import (
	"context"
	"time"
)

// ScorePoint is the average recommendation score for one time bucket.
type ScorePoint struct {
	At    time.Time `json:"at"`
	Score float64   `json:"score"`
	Runs  int       `json:"runs"`
}

// ScoreHistory shows how the average score of one item changed over time
// across all stand users.
type ScoreHistory struct {
	ItemID  string       `json:"item_id"`
	Average float64      `json:"average"`
	Points  []ScorePoint `json:"points"`
}

// ScoreHistoryStore reads the recommendation score timeline.
type ScoreHistoryStore interface {
	ScoreHistory(ctx context.Context, itemID string) (ScoreHistory, error)
}
