package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"recsys/internal/contract"
	"recsys/internal/domain"
)

const (
	DemoDatasetVersion = "demo-v1"
	DemoCatalogVersion = "demo-feed-v1"
	DemoSnapshotID     = "demo-v1"
)

// SeedOptions configures synthetic demo data seeding.
type SeedOptions struct {
	B2BLogin    string
	B2BPassword string
	B2CLogin    string
	B2CPassword string
	BcryptCost  int
	AllowOnV4   bool
}

// SeedDemo idempotently populates synthetic demo profiles, catalog offers, features, and snapshot demo-v1.
// By default it refuses to execute against a database containing closed dataset v4.
func (s *Store) SeedDemo(ctx context.Context, opts SeedOptions) error {
	b2bLogin := strings.ToLower(strings.TrimSpace(opts.B2BLogin))
	if b2bLogin == "" {
		b2bLogin = "demo-b2b"
	}
	b2bPass := opts.B2BPassword
	if b2bPass == "" {
		b2bPass = "demo-b2b-pass"
	}
	b2cLogin := strings.ToLower(strings.TrimSpace(opts.B2CLogin))
	if b2cLogin == "" {
		b2cLogin = "demo-b2c"
	}
	b2cPass := opts.B2CPassword
	if b2cPass == "" {
		b2cPass = "demo-b2c-pass"
	}
	cost := opts.BcryptCost
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		cost = bcrypt.DefaultCost
	}

	if !opts.AllowOnV4 {
		var hasV4 bool
		if err := s.pool.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM recommendations.dataset_release WHERE version IN ('v4', 'v5'))
		`).Scan(&hasV4); err != nil {
			return fmt.Errorf("check dataset_release: %w", err)
		}
		if hasV4 {
			return fmt.Errorf("refusing to run demo seed against closed dataset database")
		}
	}

	b2bHash, err := bcrypt.GenerateFromPassword([]byte(b2bPass), cost)
	if err != nil {
		return err
	}
	b2cHash, err := bcrypt.GenerateFromPassword([]byte(b2cPass), cost)
	if err != nil {
		return err
	}

	demoDigest := sha256.Sum256([]byte("recsys-synthetic-demo-fixture-v1"))
	demoSHA := hex.EncodeToString(demoDigest[:])

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Demo dataset release & two unlinked account_users (mirroring anonymized schema invariants).
	if _, err := tx.Exec(ctx, `
		INSERT INTO recommendations.dataset_release (version, workbook_sha256)
		VALUES ($1, $2)
		ON CONFLICT (version) DO UPDATE SET workbook_sha256 = EXCLUDED.workbook_sha256
	`, DemoDatasetVersion, demoSHA); err != nil {
		return fmt.Errorf("seed dataset_release: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO recommendations.account_users (email, display_name)
		VALUES
			('unlinked-user-1@demo.local', 'Обычный тестовый пользователь'),
			('unlinked-user-2@demo.local', 'Верифицированный тестовый клиент')
		ON CONFLICT (email) DO NOTHING;

		INSERT INTO recommendations.clients (excel_row, client_id, client_group, client_type, industry, segment, lk_access)
		VALUES
			(900003, 'DEMO-GUEST', 'B2C', 'ФЛ', NULL, NULL, 'да'),
			(900001, 'DEMO-BUYER', 'B2B', 'ЮЛ', 'Образование', 'enterprise', 'да'),
			(900002, 'DEMO-NEW',   'B2C', 'ФЛ', NULL,          NULL,         'да')
		ON CONFLICT (client_id) DO UPDATE SET
			client_group = EXCLUDED.client_group,
			client_type  = EXCLUDED.client_type,
			industry     = EXCLUDED.industry,
			segment      = EXCLUDED.segment,
			lk_access    = EXCLUDED.lk_access;

		INSERT INTO recommendations.account_profiles (client_id, company_name)
		VALUES
			('DEMO-BUYER', 'Демо-компания B2B'),
			('DEMO-NEW',   'Демо-профиль B2C')
		ON CONFLICT (client_id) DO UPDATE SET company_name = EXCLUDED.company_name;
	`); err != nil {
		return fmt.Errorf("seed demo clients: %w", err)
	}

	// Upsert stand accounts without replacing existing password_hash if password matches, keeping account_id stable!
	for _, acc := range []struct {
		login     string
		pass      string
		hash      string
		profileID string
		clientID  string
	}{
		{b2bLogin, b2bPass, string(b2bHash), "u1", "DEMO-BUYER"},
		{b2cLogin, b2cPass, string(b2cHash), "u2", "DEMO-NEW"},
	} {
		var existingHash string
		err := tx.QueryRow(ctx, `SELECT password_hash FROM recommendations.stand_accounts WHERE login = $1`, acc.login).Scan(&existingHash)
		useHash := acc.hash
		if err == nil && bcrypt.CompareHashAndPassword([]byte(existingHash), []byte(acc.pass)) == nil {
			useHash = existingHash
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO recommendations.stand_accounts (login, password_hash, profile_id, client_id, role)
			VALUES ($1, $2, $3, $4, 'verified')
			ON CONFLICT (login) DO UPDATE SET
				password_hash = EXCLUDED.password_hash,
				profile_id    = EXCLUDED.profile_id,
				client_id     = EXCLUDED.client_id
		`, acc.login, useHash, acc.profileID, acc.clientID); err != nil {
			return fmt.Errorf("seed stand_account %s: %w", acc.login, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// 2. Publish demo catalog feed version and canonical historical_item_offers mappings.
	feedGenAt := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	offers := []CatalogOfferInput{
		{OfferID: "demo-offer-base", CategoryID: "demo-security", CategoryName: "Безопасность", VendorName: "DemoSuite", Name: "Базовая корпоративная лицензия", VendorCode: "DEMO-BASE", URL: "https://store.softline.ru/demo/base", Available: true, Price: 12000, Currency: "RUB"},
		{OfferID: "demo-offer-renew-12", CategoryID: "demo-security", CategoryName: "Безопасность", VendorName: "DemoSuite", Name: "Продление на 12 месяцев", VendorCode: "DEMO-RENEW-12", URL: "https://store.softline.ru/demo/renew-12", Available: true, Price: 9600, Currency: "RUB"},
		{OfferID: "demo-offer-renew-24", CategoryID: "demo-security", CategoryName: "Безопасность", VendorName: "DemoSuite", Name: "Продление на 24 месяца", VendorCode: "DEMO-RENEW-24", URL: "https://store.softline.ru/demo/renew-24", Available: true, Price: 17200, Currency: "RUB"},
		{OfferID: "demo-offer-upgrade", CategoryID: "demo-security", CategoryName: "Безопасность", VendorName: "DemoSuite", Name: "Апгрейд редакции", VendorCode: "DEMO-UPG", URL: "https://store.softline.ru/demo/upgrade", Available: true, Price: 18500, Currency: "RUB"},
		{OfferID: "demo-offer-complement", CategoryID: "demo-security", CategoryName: "Безопасность", VendorName: "DemoSuite", Name: "Дополнительный модуль", VendorCode: "DEMO-COMP", URL: "https://store.softline.ru/demo/complement", Available: true, Price: 5400, Currency: "RUB"},
		{OfferID: "demo-offer-b2c-1", CategoryID: "demo-office", CategoryName: "Офисное ПО", VendorName: "PRO32", Name: "PRO32 Office Security Base (1 год)", VendorCode: "DEMO-B2C-1", URL: "https://store.softline.ru/demo/b2c-1", Available: true, Price: 2490, Currency: "RUB"},
		{OfferID: "demo-offer-b2c-2", CategoryID: "demo-office", CategoryName: "Офисное ПО", VendorName: "Content AI", Name: "ContentReader PDF Персональная", VendorCode: "DEMO-B2C-2", URL: "https://store.softline.ru/demo/b2c-2", Available: true, Price: 1990, Currency: "RUB"},
		{OfferID: "demo-offer-b2c-3", CategoryID: "demo-security", CategoryName: "Безопасность", VendorName: "Kaspersky", Name: "Kaspersky Standard (3 устройства)", VendorCode: "DEMO-B2C-3", URL: "https://store.softline.ru/demo/b2c-3", Available: true, Price: 1590, Currency: "RUB"},
	}
	if err := s.PublishCatalogFeed(ctx, CatalogFeedBatch{
		CatalogVersion: DemoCatalogVersion,
		SourceURL:      "https://store.softline.ru/demo/feed.xml",
		GeneratedAt:    feedGenAt,
		Offers:         offers,
	}); err != nil {
		return fmt.Errorf("seed catalog feed: %w", err)
	}

	itemMappings := []struct {
		itemID  string
		offerID string
	}{
		{"DEMO-BASE", "demo-offer-base"},
		{"DEMO-RENEW-12", "demo-offer-renew-12"},
		{"DEMO-RENEW-24", "demo-offer-renew-24"},
		{"DEMO-UPGRADE", "demo-offer-upgrade"},
		{"DEMO-COMPLEMENT", "demo-offer-complement"},
		{"DEMO-B2C-1", "demo-offer-b2c-1"},
		{"DEMO-B2C-2", "demo-offer-b2c-2"},
		{"DEMO-B2C-3", "demo-offer-b2c-3"},
	}
	for _, m := range itemMappings {
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO recommendations.historical_items (item_id) VALUES ($1)
			ON CONFLICT (item_id) DO NOTHING;
		`, m.itemID); err != nil {
			return err
		}
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO recommendations.historical_item_offers (item_id, offer_id, match_method, is_selected)
			VALUES ($1, $2, 'vendor_code', true)
			ON CONFLICT (item_id, offer_id) DO UPDATE SET match_method = 'vendor_code', is_selected = true;
		`, m.itemID, m.offerID); err != nil {
			return err
		}
	}

	// 3. Publish versioned item features (with 12/24 variant differences and null unknown expiry).
	strPtr := func(v string) *string { return &v }
	features := []ItemFeature{
		{ItemID: "DEMO-BASE", OfferID: strPtr("demo-offer-base"), Title: "Базовая корпоративная лицензия", SKU: strPtr("DEMO-BASE"), URL: strPtr("https://store.softline.ru/demo/base"), Available: true, InSales: true},
		{
			ItemID: "DEMO-RENEW-12", OfferID: strPtr("demo-offer-renew-12"), Title: "Продление на 12 месяцев", SKU: strPtr("DEMO-RENEW-12"), URL: strPtr("https://store.softline.ru/demo/renew-12"),
			Audience: domain.B2B, Available: true, InSales: true, FamilyID: "DEMO-SUITE", FamilySource: "synthetic-rule-1",
			Differences: []domain.Difference{
				{Field: "term", Value: 12, Unit: "month", Source: "synthetic-offer-12"},
				{Field: "expires_at", Value: nil, Unit: "date", Source: "synthetic-unknown-expiry"},
			},
		},
		{
			ItemID: "DEMO-RENEW-24", OfferID: strPtr("demo-offer-renew-24"), Title: "Продление на 24 месяца", SKU: strPtr("DEMO-RENEW-24"), URL: strPtr("https://store.softline.ru/demo/renew-24"),
			Audience: domain.B2B, Available: true, InSales: true, FamilyID: "DEMO-SUITE", FamilySource: "synthetic-rule-1",
			Differences: []domain.Difference{
				{Field: "term", Value: 24, Unit: "month", Source: "synthetic-offer-24"},
				{Field: "expires_at", Value: "2028-11-26", Unit: "date", Source: "synthetic-offer-24"},
			},
		},
		{ItemID: "DEMO-UPGRADE", OfferID: strPtr("demo-offer-upgrade"), Title: "Апгрейд редакции", SKU: strPtr("DEMO-UPG"), URL: strPtr("https://store.softline.ru/demo/upgrade"), Audience: domain.B2B, Available: true, InSales: true},
		{ItemID: "DEMO-COMPLEMENT", OfferID: strPtr("demo-offer-complement"), Title: "Дополнительный модуль", SKU: strPtr("DEMO-COMP"), URL: strPtr("https://store.softline.ru/demo/complement"), Audience: domain.B2B, Available: true, InSales: true},
		{ItemID: "DEMO-B2C-1", OfferID: strPtr("demo-offer-b2c-1"), Title: "PRO32 Office Security Base (1 год)", SKU: strPtr("DEMO-B2C-1"), URL: strPtr("https://store.softline.ru/demo/b2c-1"), Audience: domain.B2C, Available: true, InSales: true},
		{ItemID: "DEMO-B2C-2", OfferID: strPtr("demo-offer-b2c-2"), Title: "ContentReader PDF Персональная", SKU: strPtr("DEMO-B2C-2"), URL: strPtr("https://store.softline.ru/demo/b2c-2"), Audience: domain.B2C, Available: true, InSales: true},
		{ItemID: "DEMO-B2C-3", OfferID: strPtr("demo-offer-b2c-3"), Title: "Kaspersky Standard (3 устройства)", SKU: strPtr("DEMO-B2C-3"), URL: strPtr("https://store.softline.ru/demo/b2c-3"), Audience: domain.B2C, Available: true, InSales: true},
		{ItemID: "HIST-NO-OFFER-1", OfferID: nil, Title: "Историческая позиция без оффера", Available: false, InSales: true},
	}
	if err := s.PublishItemFeatures(ctx, DemoCatalogVersion, features); err != nil {
		return fmt.Errorf("seed item features: %w", err)
	}

	// 4. Publish active snapshot demo-v1.
	snap := contract.Snapshot{
		ID:             DemoSnapshotID,
		DatasetVersion: DemoDatasetVersion,
		CatalogVersion: DemoCatalogVersion,
		TrainThrough:   time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		CandidateSets: []contract.CandidateSet{
			{
				Key: contract.CandidateKey{Signal: "renewal", SourceItemID: "DEMO-BASE"},
				Candidates: []domain.Candidate{
					{
						ItemID: "DEMO-RENEW-12", Kind: domain.Renewal, Score: 1.0,
						Evidence: []domain.Evidence{{ID: "demo-renewal-12", From: "item:DEMO-BASE", To: "item:DEMO-RENEW-12", Type: "renewal", Weight: 1.0, Reason: "Для купленной редакции доступно продление на 12 или 24 месяца.", Source: "synthetic-rule-1", SnapshotID: DemoSnapshotID}},
					},
					{
						ItemID: "DEMO-RENEW-24", Kind: domain.Renewal, Score: 0.95,
						Evidence: []domain.Evidence{{ID: "demo-renewal-24", From: "item:DEMO-BASE", To: "item:DEMO-RENEW-24", Type: "renewal", Weight: 0.95, Reason: "Вариант продления на 24 месяца.", Source: "synthetic-rule-1", SnapshotID: DemoSnapshotID}},
					},
					{
						ItemID: "HIST-NO-OFFER-1", Kind: domain.Renewal, Score: 0.50,
						Evidence: []domain.Evidence{{ID: "demo-hist-no-offer", From: "item:DEMO-BASE", To: "item:HIST-NO-OFFER-1", Type: "renewal", Weight: 0.50, Reason: "Исторический кандидат без витринного оффера.", Source: "synthetic-history", SnapshotID: DemoSnapshotID}},
					},
				},
			},
			{
				Key: contract.CandidateKey{Signal: "upgrade", SourceItemID: "DEMO-BASE"},
				Candidates: []domain.Candidate{{
					ItemID: "DEMO-UPGRADE", Kind: domain.Upgrade, Score: 0.88,
					Evidence: []domain.Evidence{{ID: "demo-upgrade", From: "item:DEMO-BASE", To: "item:DEMO-UPGRADE", Type: "upgrade", Weight: 0.88, Reason: "Похожие клиенты переходили на старшую редакцию.", Source: "synthetic-rule-2", SnapshotID: DemoSnapshotID}},
				}},
			},
			{
				Key: contract.CandidateKey{Signal: "complement", SourceItemID: "DEMO-BASE"},
				Candidates: []domain.Candidate{{
					ItemID: "DEMO-COMPLEMENT", Kind: domain.Complement, Score: 0.76,
					Evidence: []domain.Evidence{{ID: "demo-complement", From: "item:DEMO-BASE", To: "item:DEMO-COMPLEMENT", Type: "complement", Weight: 0.76, Reason: "Дополнительный модуль совместим с базовой лицензией.", Source: "synthetic-rule-3", SnapshotID: DemoSnapshotID}},
				}},
			},
			{
				Key: contract.CandidateKey{Signal: "popularity", ClientGroup: domain.B2B},
				Candidates: []domain.Candidate{
					{ItemID: "DEMO-BASE", Kind: domain.Popular, Score: 0.70, Evidence: []domain.Evidence{{ID: "pop-b2b-base", From: "cohort:B2B", To: "item:DEMO-BASE", Type: "popularity", Weight: 42, Reason: "Популярная корпоративная лицензия в сегменте B2B.", Source: "demo-v1", SnapshotID: DemoSnapshotID}}},
					{ItemID: "DEMO-UPGRADE", Kind: domain.Popular, Score: 0.65, Evidence: []domain.Evidence{{ID: "pop-b2b-upg", From: "cohort:B2B", To: "item:DEMO-UPGRADE", Type: "popularity", Weight: 31, Reason: "Популярная старшая редакция B2B.", Source: "demo-v1", SnapshotID: DemoSnapshotID}}},
					{ItemID: "DEMO-COMPLEMENT", Kind: domain.Popular, Score: 0.60, Evidence: []domain.Evidence{{ID: "pop-b2b-comp", From: "cohort:B2B", To: "item:DEMO-COMPLEMENT", Type: "popularity", Weight: 24, Reason: "Часто выбирают корпоративные клиенты.", Source: "demo-v1", SnapshotID: DemoSnapshotID}}},
				},
			},
			{
				Key: contract.CandidateKey{Signal: "popularity", ClientGroup: domain.B2C},
				Candidates: []domain.Candidate{
					{ItemID: "DEMO-B2C-1", Kind: domain.Popular, Score: 0.80, Evidence: []domain.Evidence{{ID: "pop-b2c-1", From: "cohort:B2C", To: "item:DEMO-B2C-1", Type: "popularity", Weight: 55, Reason: "Популярно у клиентов B2C.", Source: "demo-v1", SnapshotID: DemoSnapshotID}}},
					{ItemID: "DEMO-B2C-2", Kind: domain.Popular, Score: 0.75, Evidence: []domain.Evidence{{ID: "pop-b2c-2", From: "cohort:B2C", To: "item:DEMO-B2C-2", Type: "popularity", Weight: 40, Reason: "Популярное решение для документов в B2C.", Source: "demo-v1", SnapshotID: DemoSnapshotID}}},
					{ItemID: "DEMO-B2C-3", Kind: domain.Popular, Score: 0.70, Evidence: []domain.Evidence{{ID: "pop-b2c-3", From: "cohort:B2C", To: "item:DEMO-B2C-3", Type: "popularity", Weight: 38, Reason: "Базовая защита для домашних устройств.", Source: "demo-v1", SnapshotID: DemoSnapshotID}}},
				},
			},
		},
	}
	return s.PublishSnapshot(ctx, snap)
}
