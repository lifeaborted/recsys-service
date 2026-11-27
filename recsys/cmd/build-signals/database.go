package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"recsys/internal/config"
	"recsys/internal/contract"
	"recsys/internal/domain"
	"recsys/internal/enrichment"
	"recsys/internal/signals"
	"recsys/internal/store/postgres"
)

type databaseWriter struct {
	store    *postgres.Store
	evidence *enrichment.Result
	data     contract.DatasetBatch
}

func (w databaseWriter) PublishSnapshot(ctx context.Context, snapshot contract.Snapshot) error {
	if w.evidence == nil {
		return w.store.PublishSnapshot(ctx, snapshot)
	}
	features, err := attachEvidence(&snapshot, w.data, *w.evidence)
	if err != nil {
		return err
	}
	feed := w.evidence.Feed
	return w.store.PublishSnapshotWithFeatures(ctx, snapshot, features, postgres.CatalogFeedBatch{
		CatalogVersion: feed.ID, SourceURL: feed.URL, GeneratedAt: feed.GeneratedAt,
	})
}

func attachEvidence(snapshot *contract.Snapshot, data contract.DatasetBatch, result enrichment.Result) ([]postgres.ItemFeature, error) {
	if result.SnapshotID != snapshot.ID || result.Feed.ID == "" || result.Feed.GeneratedAt.IsZero() {
		return nil, fmt.Errorf("evidence artifact has invalid feed or belongs to another snapshot")
	}
	if (snapshot.DatasetVersion == "v4" || snapshot.DatasetVersion == "v5") && result.Feed.GeneratedAt.Format(time.DateOnly) > snapshot.TrainThrough.Format(time.DateOnly) {
		return nil, fmt.Errorf("historical snapshot cannot use a feed generated after train-through")
	}
	snapshot.CatalogVersion = result.Feed.ID
	products := make(map[domain.ItemID]postgres.ItemFeature)
	for _, sale := range data.Sales {
		products[sale.ItemID] = postgres.ItemFeature{ItemID: sale.ItemID, Title: sale.ItemName, SKU: sale.SKU, InSales: true}
	}
	offers := make(map[string]enrichment.Offer)
	for _, offer := range result.Offers {
		offers[offer.ID] = offer
	}
	for _, match := range result.Matches {
		if match.Status != "selected" {
			continue
		}
		offer, ok := offers[match.OfferID]
		if !ok {
			return nil, fmt.Errorf("selected offer is missing from evidence artifact")
		}
		p := products[match.ItemID]
		p.ItemID, p.Title, p.OfferID = match.ItemID, offer.Name, &offer.ID
		if offer.URL != "" {
			p.URL = &offer.URL
		}
		if offer.SKU != "" {
			p.SKU = &offer.SKU
		}
		if fact := offer.Facts["available"]; fact.Value != nil && fact.Source != nil {
			p.Available = *fact.Value == "true"
		}
		if fact := offer.Facts["platform"]; fact.Value != nil && fact.Source != nil {
			p.Platform = *fact.Value
		}
		if fact := offer.Facts["organization_type"]; fact.Value != nil && fact.Source != nil {
			p.OrganizationType = strings.ToLower(strings.TrimSpace(*fact.Value))
		}
		if fact := offer.Facts["user_type"]; fact.Value != nil && fact.Source != nil {
			switch strings.ToLower(strings.TrimSpace(*fact.Value)) {
			case "юрлицо":
				p.Audience = domain.B2B
			case "физлицо":
				p.Audience = domain.B2C
			case "нко":
				p.Audience = domain.B2B
				if p.OrganizationType == "" || p.OrganizationType == "не определено" || p.OrganizationType == "некоммерческая" {
					p.OrganizationType = "некоммерческая"
				} else {
					p.Available = false
				}
			case "", "не определено":
			default:
				p.Available = false
			}
		}
		products[p.ItemID] = p
	}
	for _, family := range result.Families {
		p, ok := products[family.ItemID]
		if !ok || family.Family.Value == nil || family.Family.Source == nil {
			continue
		}
		source := family.Family.Source
		p.FamilyID = *family.Family.Value
		p.FamilySource = source.URL + "@" + source.Version + "#" + source.Locator
		for _, variant := range result.Variants[p.FamilyID] {
			if variant.ItemID == p.ItemID {
				p.Differences = variant.Differences
				break
			}
		}
		products[p.ItemID] = p
	}
	type pool struct {
		index int
		items map[domain.ItemID]int
	}
	pools := make(map[contract.CandidateKey]pool)
	for i, set := range snapshot.CandidateSets {
		p := pool{index: i, items: make(map[domain.ItemID]int, len(set.Candidates))}
		for j, c := range set.Candidates {
			p.items[c.ItemID] = j
		}
		pools[set.Key] = p
	}
	add := func(key contract.CandidateKey, c domain.Candidate) {
		p, ok := pools[key]
		if !ok {
			p = pool{index: len(snapshot.CandidateSets), items: make(map[domain.ItemID]int)}
			pools[key] = p
			snapshot.CandidateSets = append(snapshot.CandidateSets, contract.CandidateSet{Key: key})
		}
		set := &snapshot.CandidateSets[p.index]
		if i, ok := p.items[c.ItemID]; ok {
			existing := &set.Candidates[i]
			existing.Score = max(existing.Score, c.Score)
			existing.Evidence = append(existing.Evidence, c.Evidence...)
			return
		}
		p.items[c.ItemID] = len(set.Candidates)
		set.Candidates = append(set.Candidates, c)
	}
	for _, relation := range result.Relations {
		ev := relation.Evidence
		from, fromOK := strings.CutPrefix(ev.From, "item:")
		to, toOK := strings.CutPrefix(ev.To, "item:")
		kind := domain.RecommendationKind(ev.Type)
		if !fromOK || !toOK || ev.SnapshotID != snapshot.ID || ev.Source == "" || ev.Reason == "" || relation.Source.Version == "" ||
			(kind != domain.Renewal && kind != domain.Upgrade && kind != domain.Complement) {
			return nil, fmt.Errorf("invalid verified relation %q", ev.ID)
		}
		if snapshot.DatasetVersion == "v4" || snapshot.DatasetVersion == "v5" {
			if relation.Source.ObservedAt.Format(time.DateOnly) > snapshot.TrainThrough.Format(time.DateOnly) {
				return nil, fmt.Errorf("historical snapshot cannot use evidence observed after train-through")
			}
			if !products[domain.ItemID(from)].InSales || !products[domain.ItemID(to)].InSales {
				continue
			}
		}
		if products[domain.ItemID(from)].ItemID == "" || products[domain.ItemID(to)].ItemID == "" {
			return nil, fmt.Errorf("verified relation %q has no product facts for an endpoint", ev.ID)
		}
		add(contract.CandidateKey{Signal: ev.Type, SourceItemID: domain.ItemID(from)},
			domain.Candidate{ItemID: domain.ItemID(to), Kind: kind, Score: ev.Weight, Evidence: []domain.Evidence{ev}})
	}
	features := make([]postgres.ItemFeature, 0, len(products))
	for id, p := range products {
		features = append(features, p)
		if p.Available {
			ev := domain.Evidence{ID: "catalog/" + string(id), From: "cohort:catalog", To: "item:" + string(id), Type: "catalog",
				Reason: "Доступно в снимке каталога.", Weight: 1, Source: result.Feed.URL + "@" + result.Feed.ID, SnapshotID: snapshot.ID}
			add(contract.CandidateKey{Signal: "catalog"}, domain.Candidate{ItemID: id, Kind: domain.Popular, Score: 1, Evidence: []domain.Evidence{ev}})
		}
	}
	return features, nil
}

func runDatabase(ctx context.Context, snapshotID, dataset, cutoff, evidencePath string) error {
	through, err := time.Parse(time.DateOnly, cutoff)
	if err != nil || snapshotID == "" || dataset == "" {
		return fmt.Errorf("supply -snapshot, -dataset and -train-through YYYY-MM-DD")
	}
	if (dataset == "v4" || dataset == "v5") && cutoff != "2026-08-31" {
		return fmt.Errorf("%s train-through must be 2026-08-31", dataset)
	}
	store, err := postgres.OpenContext(ctx, config.Load().DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	batch, err := store.OfflineDataset(ctx)
	if err != nil {
		return err
	}
	meta := contract.Snapshot{ID: snapshotID, DatasetVersion: dataset, TrainThrough: through}
	w := databaseWriter{store: store, data: batch}
	if evidencePath != "" {
		f, err := os.Open(evidencePath)
		if err != nil {
			return err
		}
		defer f.Close()
		w.evidence = new(enrichment.Result)
		if err = json.NewDecoder(f).Decode(w.evidence); err != nil {
			return err
		}
	}
	if err := signals.Build(ctx, meta, batch, w); err != nil {
		return err
	}
	fmt.Printf("published snapshot_id=%s dataset=%s train_through=%s\n", snapshotID, dataset, cutoff)
	return nil
}

func registerDatabaseFlags() (*bool, *string, *string, *string, *string) {
	return flag.Bool("database", false, "read dataset from PostgreSQL and publish there"),
		flag.String("snapshot", "", "snapshot ID"),
		flag.String("dataset", "", "dataset release version"),
		flag.String("train-through", "2026-08-31", "inclusive train cutoff"),
		flag.String("evidence", "", "verified output of cmd/enrich (database mode)")
}
