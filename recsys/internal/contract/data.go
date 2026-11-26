// Package contract defines interfaces shared by the application components.
package contract

import (
	"context"
	"time"

	"recsys/internal/domain"
)

type DatasetBatch struct {
	Clients []domain.Client
	Sales   []domain.Sale
}

type DatasetWriter interface {
	// WriteDataset publishes one complete version or leaves the previous version in place.
	// Batches arrive in source order and every sale retains its SourceRow.
	WriteDataset(ctx context.Context, version string, batches []DatasetBatch) error
}

type CandidateKey struct {
	Signal       string
	SourceItemID domain.ItemID
	ClientGroup  domain.ClientGroup
	ClientType   string
	Industry     string
	Segment      string
}

type CandidateSet struct {
	Key        CandidateKey
	Candidates []domain.Candidate
}

type Snapshot struct {
	ID             string
	DatasetVersion string
	CatalogVersion string
	TrainThrough   time.Time
	CandidateSets  []CandidateSet
}

type SnapshotWriter interface {
	// PublishSnapshot makes the completed snapshot visible in one operation.
	PublishSnapshot(ctx context.Context, snapshot Snapshot) error
}

type FactsReader interface {
	Profile(ctx context.Context, clientID domain.ClientID) (domain.Client, error)
	// History returns purchases through the inclusive week. A zero time reads
	// the current history. Historical evaluation always supplies its cutoff.
	History(ctx context.Context, clientID domain.ClientID, throughWeek time.Time) ([]domain.Sale, error)
	Candidates(ctx context.Context, snapshotID string, key CandidateKey, limit int) ([]domain.Candidate, error)
}
