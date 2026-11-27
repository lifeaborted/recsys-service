package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"recsys/internal/auth"
	"recsys/internal/contract"
	"recsys/internal/domain"
	"recsys/internal/store/postgres"
)

// Compile-time assertions that *postgres.Store satisfies platform contracts.
var (
	_ contract.DatasetWriter  = (*postgres.Store)(nil)
	_ contract.FactsReader    = (*postgres.Store)(nil)
	_ contract.SnapshotWriter = (*postgres.Store)(nil)
	_ auth.Repository         = (*postgres.Store)(nil)
)

func TestValidateV4ThresholdsRejectsIncompleteDataset(t *testing.T) {
	err := postgres.ValidateClosedV4Counts(postgres.DatasetCounts{
		SalesRows:     100,
		Clients:       50,
		ObservedWeeks: 80,
		DistinctItems: 10,
		TrainItems:    10,
	})
	if err == nil {
		t.Fatal("expected threshold validation error for incomplete v4 import")
	}

	if err := postgres.ValidateClosedV4Counts(postgres.DatasetCounts{
		SalesRows:     31753,
		Clients:       19008,
		ObservedWeeks: 27603,
		DistinctItems: 817,
		TrainItems:    813,
	}); err != nil {
		t.Fatalf("expected exact v4 counts to pass, got %v", err)
	}
}

func TestPostgresIntegration_ContractsTransactionsAndVersioning(t *testing.T) {
	dsn := os.Getenv("RECSYS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("RECSYS_TEST_DATABASE_URL is not set; skipping live PostgreSQL integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	store, err := postgres.OpenContext(ctx, dsn)
	if err != nil {
		t.Fatalf("OpenContext: %v", err)
	}
	defer store.Close()

	if _, err := store.Migrate(ctx, "../../../migrations"); err != nil {
		t.Fatalf("Migrate first run: %v", err)
	}
	if _, err := store.Migrate(ctx, "../../../migrations"); err != nil {
		t.Fatalf("Migrate second run (idempotency): %v", err)
	}
	if _, err := store.CheckHealth(ctx); err != nil {
		t.Fatalf("CheckHealth: %v", err)
	}

	// 1. DatasetWriter via staging/COPY: composite basket key (order_id, order_week) & idempotent repeat.
	week1 := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	week2 := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	weekAfterCutoff := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	skuBase := "SKU-BASE"
	industry := "Образование"

	batches := []contract.DatasetBatch{{
		Clients: []domain.Client{
			{ID: "HIST-C1", Group: domain.B2B, Type: "ЮЛ", Industry: &industry, HasLKAccess: true},
			{ID: "HIST-C2", Group: domain.B2C, Type: "ФЛ", HasLKAccess: false},
		},
		Sales: []domain.Sale{
			// Same order_id "ORD-MULTI" observed in two distinct weeks!
			{SourceRow: 2, OrderID: "ORD-MULTI", ClientID: "HIST-C1", OrderWeek: week1, ItemID: "ITEM-BASE", SKU: &skuBase, ItemName: "Base License", ProductGroup: "G-BASE"},
			{SourceRow: 3, OrderID: "ORD-MULTI", ClientID: "HIST-C1", OrderWeek: week2, ItemID: "ITEM-ADDON", ItemName: "Addon Module"},
			{SourceRow: 4, OrderID: "ORD-FUTURE", ClientID: "HIST-C1", OrderWeek: weekAfterCutoff, ItemID: "ITEM-FUTURE", ItemName: "Future Upgrade"},
		},
	}}

	if err := store.WriteDataset(ctx, "synth-test-v1", batches); err != nil {
		t.Fatalf("WriteDataset run 1: %v", err)
	}
	if err := store.WriteDataset(ctx, "synth-test-v1", batches); err != nil {
		t.Fatalf("WriteDataset run 2 (repeat idempotency): %v", err)
	}

	offline, err := store.OfflineDataset(ctx)
	if err != nil || len(offline.Sales) != 3 || offline.Sales[0].ProductGroup != "G-BASE" || offline.Sales[1].ProductGroup != "" {
		t.Fatalf("product_group round trip: %+v, err=%v", offline.Sales, err)
	}

	// Verify closed v4 import threshold check rolls back without clobbering synth-test-v1.
	if err := store.WriteDataset(ctx, "v4", batches); err == nil {
		t.Fatal("expected WriteDataset(version=v4) with 3 rows to fail threshold check and roll back")
	}

	prof, err := store.Profile(ctx, "HIST-C1")
	if err != nil || prof.Group != domain.B2B || prof.Industry == nil || *prof.Industry != "Образование" {
		t.Fatalf("Profile(HIST-C1) = %+v, err=%v", prof, err)
	}

	histCutoff, err := store.History(ctx, "HIST-C1", week2)
	if err != nil || len(histCutoff) != 2 {
		t.Fatalf("History through cutoff = %d rows (%+v), err=%v; want 2", len(histCutoff), histCutoff, err)
	}
	histAll, err := store.History(ctx, "HIST-C1", time.Time{})
	if err != nil || len(histAll) != 3 {
		t.Fatalf("History without cutoff = %d rows, err=%v; want 3", len(histAll), err)
	}

	// 2. Multi-version catalog feed despite catalog_offers.offer_id PK.
	feedT1 := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	feedT2 := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	if err := store.PublishCatalogFeed(ctx, postgres.CatalogFeedBatch{
		CatalogVersion: "feed-v1",
		SourceURL:      "https://example.com/feed.xml",
		GeneratedAt:    feedT1,
		Offers: []postgres.CatalogOfferInput{
			{OfferID: "off-100", CategoryID: "cat-1", CategoryName: "Security", Name: "Product v1", URL: "https://example.com/100", Available: true, Price: 1000, Currency: "RUB"},
		},
	}); err != nil {
		t.Fatalf("PublishCatalogFeed feed-v1: %v", err)
	}
	if err := store.PublishCatalogFeed(ctx, postgres.CatalogFeedBatch{
		CatalogVersion: "feed-v2",
		SourceURL:      "https://example.com/feed.xml",
		GeneratedAt:    feedT2,
		Offers: []postgres.CatalogOfferInput{
			{OfferID: "off-100", CategoryID: "cat-1", CategoryName: "Security", Name: "Product v2", URL: "https://example.com/100", Available: false, Price: 1200, Currency: "RUB"},
		},
	}); err != nil {
		t.Fatalf("PublishCatalogFeed feed-v2: %v", err)
	}

	v1Offers, err := store.CatalogVersionOffers(ctx, "feed-v1")
	if err != nil || len(v1Offers) != 1 || !v1Offers[0].Available || v1Offers[0].Name != "Product v1" {
		t.Fatalf("feed-v1 offer was overwritten or lost: %+v err=%v", v1Offers, err)
	}
	v2Offers, err := store.CatalogVersionOffers(ctx, "feed-v2")
	if err != nil || len(v2Offers) != 1 || v2Offers[0].Available || v2Offers[0].Name != "Product v2" {
		t.Fatalf("feed-v2 offer mismatch: %+v err=%v", v2Offers, err)
	}

	// 3. SnapshotWriter & FactsReader.Candidates including candidate WITHOUT an offer_id!
	snap := contract.Snapshot{
		ID:             "snap-test-1",
		DatasetVersion: "synth-test-v1",
		CatalogVersion: "feed-v2",
		TrainThrough:   week2,
		CandidateSets: []contract.CandidateSet{{
			Key: contract.CandidateKey{Signal: "renewal", SourceItemID: "ITEM-BASE"},
			Candidates: []domain.Candidate{
				{
					ItemID: "ITEM-NO-OFFER-CANDIDATE",
					Kind:   domain.Renewal,
					Score:  0.95,
					Evidence: []domain.Evidence{{
						ID:         "ev-no-offer",
						From:       "item:ITEM-BASE",
						To:         "item:ITEM-NO-OFFER-CANDIDATE",
						Type:       "renewal",
						Weight:     12,
						Reason:     "Verified renewal path without current storefront offer",
						Source:     "synth-rules-v1",
						SnapshotID: "snap-test-1",
					}},
				},
			},
		}},
	}
	if err := store.PublishSnapshot(ctx, snap); err != nil {
		t.Fatalf("PublishSnapshot 1: %v", err)
	}
	if err := store.PublishSnapshot(ctx, snap); err != nil {
		t.Fatalf("PublishSnapshot 2 (idempotent): %v", err)
	}

	cands, err := store.Candidates(ctx, "snap-test-1", contract.CandidateKey{Signal: "renewal", SourceItemID: "ITEM-BASE"}, 10)
	if err != nil || len(cands) != 1 || cands[0].ItemID != "ITEM-NO-OFFER-CANDIDATE" || len(cands[0].Evidence) != 1 {
		t.Fatalf("Candidates returned %+v, err=%v", cands, err)
	}

	// 4. CatalogMeta returns latest feed version and feed_generated_at timestamp.
	meta, err := store.CatalogMeta(ctx)
	if err != nil {
		t.Fatalf("CatalogMeta: %v", err)
	}
	if meta.CatalogVersion != "feed-v2" || meta.FeedGeneratedAt == nil || !meta.FeedGeneratedAt.Equal(feedT2) || meta.ImportedAt == nil {
		t.Fatalf("unexpected CatalogMeta: %+v", meta)
	}
}
