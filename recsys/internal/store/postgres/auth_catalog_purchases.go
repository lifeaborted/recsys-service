package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"recsys/internal/auth"
	"recsys/internal/contract"
	"recsys/internal/domain"
)

// CreateAccountWithProfile atomically creates a new isolated stand client_id, account_profile, and stand_account.
func (s *Store) CreateAccountWithProfile(ctx context.Context, p auth.CreateAccountParams) (auth.AccountRecord, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return auth.AccountRecord{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ctype := "ФЛ"
	if p.ClientGroup == domain.B2B {
		ctype = "ЮЛ"
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO recommendations.clients (
			excel_row, client_id, client_group, client_type, industry, lk_access
		) VALUES (
			nextval('recommendations.stand_client_excel_row_seq'),
			$1, $2, $3, $4, 'да'
		)
	`, string(p.ClientID), string(p.ClientGroup), ctype, p.Industry); err != nil {
		return auth.AccountRecord{}, fmt.Errorf("insert stand client: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO recommendations.account_profiles (client_id, company_name)
		VALUES ($1, $2)
		ON CONFLICT (client_id) DO NOTHING
	`, string(p.ClientID), "Профиль стенда "+p.Login); err != nil {
		return auth.AccountRecord{}, fmt.Errorf("insert account_profile: %w", err)
	}

	var accountID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO recommendations.stand_accounts (
			login, password_hash, profile_id, client_id, created_at
		) VALUES ($1, $2, $3, $4, $5)
		RETURNING account_id
	`, p.Login, p.PasswordHash, p.ProfileID, string(p.ClientID), p.CreatedAt.UTC()).Scan(&accountID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return auth.AccountRecord{}, auth.ErrLoginExists
		}
		return auth.AccountRecord{}, fmt.Errorf("insert stand_account: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return auth.AccountRecord{}, err
	}
	return auth.AccountRecord{
		AccountID:    accountID,
		Login:        p.Login,
		PasswordHash: p.PasswordHash,
		Profile: auth.Profile{
			ID:       p.ProfileID,
			ClientID: p.ClientID,
			Group:    p.ClientGroup,
			Industry: p.Industry,
			Login:    p.Login,
		},
		CreatedAt: p.CreatedAt,
	}, nil
}

// AccountByLogin fetches a stand account and its linked client profile.
func (s *Store) AccountByLogin(ctx context.Context, login string) (auth.AccountRecord, error) {
	var rec auth.AccountRecord
	var clientID, group string
	err := s.pool.QueryRow(ctx, `
		SELECT a.account_id, a.login, a.password_hash, a.profile_id, a.client_id,
		       COALESCE(c.client_group, 'B2B'), c.industry, a.created_at
		FROM recommendations.stand_accounts a
		JOIN recommendations.clients c ON c.client_id = a.client_id
		WHERE a.login = $1
	`, strings.ToLower(strings.TrimSpace(login))).Scan(
		&rec.AccountID, &rec.Login, &rec.PasswordHash,
		&rec.Profile.ID, &clientID, &group, &rec.Profile.Industry, &rec.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.AccountRecord{}, auth.ErrInvalidCredentials
	}
	if err != nil {
		return auth.AccountRecord{}, err
	}
	rec.Profile.ClientID = domain.ClientID(clientID)
	rec.Profile.Group = domain.ClientGroup(group)
	rec.Profile.Login = rec.Login
	return rec, nil
}

// CreateSession stores a SHA-256 session token digest with explicit creation and expiration timestamps.
func (s *Store) CreateSession(ctx context.Context, tokenHash []byte, accountID int64, createdAt, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO recommendations.stand_sessions (token_hash, account_id, created_at, expires_at)
		VALUES ($1, $2, $3, $4)
	`, tokenHash, accountID, createdAt.UTC(), expiresAt.UTC())
	return err
}

// SessionByTokenHash looks up a session and its bound stand profile by token SHA-256 digest.
func (s *Store) SessionByTokenHash(ctx context.Context, tokenHash []byte) (auth.SessionRecord, error) {
	var rec auth.SessionRecord
	var clientID, group string
	err := s.pool.QueryRow(ctx, `
		SELECT s.account_id, s.created_at, s.expires_at,
		       a.profile_id, a.client_id, a.login,
		       COALESCE(c.client_group, 'B2B'), c.industry
		FROM recommendations.stand_sessions s
		JOIN recommendations.stand_accounts a ON a.account_id = s.account_id
		JOIN recommendations.clients c ON c.client_id = a.client_id
		WHERE s.token_hash = $1
	`, tokenHash).Scan(
		&rec.AccountID, &rec.CreatedAt, &rec.ExpiresAt,
		&rec.Profile.ID, &clientID, &rec.Profile.Login,
		&group, &rec.Profile.Industry,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.SessionRecord{}, auth.ErrUnauthorized
	}
	if err != nil {
		return auth.SessionRecord{}, err
	}
	rec.TokenHash = append([]byte(nil), tokenHash...)
	rec.Profile.ClientID = domain.ClientID(clientID)
	rec.Profile.Group = domain.ClientGroup(group)
	return rec, nil
}

// CatalogProduct represents a storefront product row returned by ListProducts.
type CatalogProduct struct {
	ItemID       domain.ItemID `json:"item_id"`
	OfferID      *string       `json:"offer_id,omitempty"`
	SKU          *string       `json:"sku"`
	Title        string        `json:"title"`
	Category     string        `json:"category"`
	Vendor       string        `json:"vendor"`
	Price        *float64      `json:"price,omitempty"`
	CurrencyCode *string       `json:"currency_code,omitempty"`
	Available    bool          `json:"available"`
	URL          *string       `json:"url,omitempty"`
}

// ListProducts returns up to limit storefront products mapped to canonical item_ids when available.
func (s *Store) ListProducts(ctx context.Context, limit int) ([]CatalogProduct, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT
			COALESCE(hio.item_id, sc.offer_id) AS item_id,
			sc.offer_id,
			sc.vendor_code AS sku,
			sc.name AS title,
			COALESCE(sc.category, '') AS category,
			COALESCE(sc.vendor, '') AS vendor,
			sc.price::float8,
			sc.currency_code,
			sc.available,
			sc.url
		FROM recommendations.storefront_catalog sc
		LEFT JOIN recommendations.historical_item_offers hio
			ON hio.offer_id = sc.offer_id AND hio.is_selected
		ORDER BY sc.available DESC, item_id COLLATE "C" ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]CatalogProduct, 0, limit)
	for rows.Next() {
		var p CatalogProduct
		var itemID, offerID string
		var urlStr *string
		if err := rows.Scan(&itemID, &offerID, &p.SKU, &p.Title, &p.Category, &p.Vendor,
			&p.Price, &p.CurrencyCode, &p.Available, &urlStr); err != nil {
			return nil, err
		}
		p.ItemID = domain.ItemID(itemID)
		p.OfferID = &offerID
		p.URL = urlStr
		out = append(out, p)
	}
	return out, rows.Err()
}

// PurchaseRecord describes a recorded online profile purchase.
type PurchaseRecord struct {
	OrderID string        `json:"order_id"`
	ItemID  domain.ItemID `json:"item_id"`
	OfferID *string       `json:"offer_id,omitempty"`
}

// CreatePurchase records an online purchase for clientID under its canonical item_id.
func (s *Store) CreatePurchase(ctx context.Context, clientID domain.ClientID, rawItemID string, now time.Time) (PurchaseRecord, error) {
	target := strings.TrimSpace(rawItemID)
	if target == "" {
		return PurchaseRecord{}, ErrItemNotFound
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PurchaseRecord{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Resolve canonical item_id and optional offer metadata.
	var canonItemID string
	var offerID, sku, itemName, currency *string
	var price *float64

	err = tx.QueryRow(ctx, `
		SELECT
			hi.item_id,
			o.offer_id,
			COALESCE(o.vendor_code, f.sku) AS sku,
			COALESCE(o.name, f.title, hi.item_id) AS item_name,
			o.price::float8,
			o.currency_code
		FROM recommendations.historical_items hi
		LEFT JOIN recommendations.historical_item_offers hio
			ON hio.item_id = hi.item_id AND hio.is_selected
		LEFT JOIN recommendations.catalog_offers o
			ON o.offer_id = hio.offer_id
		LEFT JOIN LATERAL (
			SELECT sku, title
			FROM recommendations.item_feature_versions
			WHERE item_id = hi.item_id
			ORDER BY created_at DESC
			LIMIT 1
		) f ON true
		WHERE hi.item_id = $1
	`, target).Scan(&canonItemID, &offerID, &sku, &itemName, &price, &currency)

	if errors.Is(err, pgx.ErrNoRows) {
		// Check if caller supplied an offer_id that maps to a canonical item_id (or a standalone offer).
		err = tx.QueryRow(ctx, `
			SELECT
				COALESCE(hio.item_id, o.offer_id) AS item_id,
				o.offer_id,
				o.vendor_code,
				o.name,
				o.price::float8,
				o.currency_code
			FROM recommendations.catalog_offers o
			LEFT JOIN recommendations.historical_item_offers hio
				ON hio.offer_id = o.offer_id AND hio.is_selected
			WHERE o.offer_id = $1
		`, target).Scan(&canonItemID, &offerID, &sku, &itemName, &price, &currency)
		if errors.Is(err, pgx.ErrNoRows) {
			return PurchaseRecord{}, ErrItemNotFound
		}
		if err != nil {
			return PurchaseRecord{}, err
		}
		// Ensure standalone catalog offer has an entry in historical_items so canonical item_id FK holds.
		if _, err := tx.Exec(ctx, `
			INSERT INTO recommendations.historical_items (item_id)
			VALUES ($1)
			ON CONFLICT (item_id) DO NOTHING
		`, canonItemID); err != nil {
			return PurchaseRecord{}, err
		}
	} else if err != nil {
		return PurchaseRecord{}, err
	}

	weekDate := mondayOfWeek(now.UTC()).Format("2006-01-02")
	randSuffix := make([]byte, 6)
	_, _ = rand.Read(randSuffix)
	orderID := fmt.Sprintf("ORD-%s-%s", now.UTC().Format("20060102150405"), hex.EncodeToString(randSuffix))

	if _, err := tx.Exec(ctx, `
		INSERT INTO recommendations.orders (order_id, client_id, source, order_week, created_at, status)
		VALUES ($1, $2, 'account', $3::date, $4, 'paid')
	`, orderID, string(clientID), weekDate, now.UTC()); err != nil {
		return PurchaseRecord{}, fmt.Errorf("insert online order: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO recommendations.order_observed_weeks (order_id, order_week)
		VALUES ($1, $2::date)
		ON CONFLICT (order_id, order_week) DO NOTHING
	`, orderID, weekDate); err != nil {
		return PurchaseRecord{}, fmt.Errorf("insert online order_observed_weeks: %w", err)
	}

	if offerID != nil && *offerID != "" {
		unitPrice := 0.0
		if price != nil && *price >= 0 {
			unitPrice = *price
		}
		cur := "RUB"
		if currency != nil && *currency != "" {
			cur = *currency
		} else {
			_, _ = tx.Exec(ctx, `
				INSERT INTO recommendations.catalog_currencies (currency_code, rate)
				VALUES ('RUB', 1)
				ON CONFLICT (currency_code) DO NOTHING
			`)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO recommendations.store_order_items (order_id, offer_id, quantity, unit_price, currency_code)
			VALUES ($1, $2, 1, $3, $4)
		`, orderID, *offerID, unitPrice, cur); err != nil {
			return PurchaseRecord{}, fmt.Errorf("insert store_order_item: %w", err)
		}
	}

	resolvedName := canonItemID
	if itemName != nil && *itemName != "" {
		resolvedName = *itemName
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO recommendations.online_purchases (
			order_id, client_id, item_id, offer_id, sku, item_name, purchased_week, purchased_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7::date, $8)
	`, orderID, string(clientID), canonItemID, offerID, sku, resolvedName, weekDate, now.UTC()); err != nil {
		return PurchaseRecord{}, fmt.Errorf("insert online_purchase: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return PurchaseRecord{}, err
	}
	return PurchaseRecord{
		OrderID: orderID,
		ItemID:  domain.ItemID(canonItemID),
		OfferID: offerID,
	}, nil
}

func mondayOfWeek(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	d := t.AddDate(0, 0, -(wd - 1))
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
}

// SnapshotRecommender implements contract.Recommender over Store's FactsReader and item_feature_versions.
// Used by cmd/api on standalone platform setups while keeping HTTP completely decoupled via contract.Recommender.
type SnapshotRecommender struct {
	store *Store
}

func NewRecommender(store *Store) *SnapshotRecommender {
	return &SnapshotRecommender{store: store}
}

var _ contract.Recommender = (*SnapshotRecommender)(nil)

func (r *SnapshotRecommender) Recommend(ctx context.Context, q contract.RecommendationQuery) (contract.RecommendationResult, error) {
	empty := contract.RecommendationResult{
		Items: []domain.Recommendation{},
		Graph: domain.Graph{Nodes: []domain.Node{}, Edges: []domain.Evidence{}},
	}
	if r == nil || r.store == nil {
		return empty, fmt.Errorf("recommender store is nil")
	}

	snapID, err := r.store.ResolveSnapshotID(ctx, q.SnapshotID)
	if err != nil {
		return empty, err
	}
	empty.SnapshotID = snapID

	var featureVer string
	var cutoff time.Time
	if err := r.store.pool.QueryRow(ctx, `
		SELECT feature_version, train_through
		FROM recommendations.model_snapshots
		WHERE snapshot_id = $1
	`, snapID).Scan(&featureVer, &cutoff); err != nil {
		return empty, err
	}

	profile, err := r.store.Profile(ctx, q.ClientID)
	if err != nil {
		return empty, err
	}

	var through time.Time
	if q.Mode == contract.Backtest {
		through = cutoff
	}
	history, err := r.store.History(ctx, q.ClientID, through)
	if err != nil {
		return empty, err
	}

	bought := make(map[domain.ItemID]bool, len(history))
	var sourceItems []domain.ItemID
	for _, h := range history {
		if !bought[h.ItemID] {
			bought[h.ItemID] = true
			sourceItems = append(sourceItems, h.ItemID)
		}
	}

	var keys []contract.CandidateKey
	for _, src := range sourceItems {
		keys = append(keys,
			contract.CandidateKey{Signal: "renewal", SourceItemID: src},
			contract.CandidateKey{Signal: "upgrade", SourceItemID: src},
			contract.CandidateKey{Signal: "complement", SourceItemID: src},
			contract.CandidateKey{Signal: "transition", SourceItemID: src},
			contract.CandidateKey{Signal: "basket", SourceItemID: src},
		)
	}
	if profile.Group != "" {
		keys = append(keys, contract.CandidateKey{Signal: "popularity", ClientGroup: profile.Group})
	}
	keys = append(keys, contract.CandidateKey{Signal: "popularity"})

	var picked []domain.Candidate
	seenCand := make(map[domain.ItemID]bool)
	for _, key := range keys {
		cands, err := r.store.Candidates(ctx, snapID, key, 32)
		if err != nil {
			return empty, err
		}
		for _, c := range cands {
			if bought[c.ItemID] || seenCand[c.ItemID] {
				continue
			}
			seenCand[c.ItemID] = true
			picked = append(picked, c)
		}
	}

	if len(picked) == 0 {
		reason := "insufficient_eligible_products"
		empty.PartialReason = &reason
		return empty, nil
	}

	features, err := r.loadFeatures(ctx, featureVer)
	if err != nil {
		return empty, err
	}

	type familyGroupKey struct {
		family string
		kind   domain.RecommendationKind
	}
	groupIdx := make(map[familyGroupKey]int)
	nodeSeen := make(map[string]bool)
	edgeSeen := make(map[string]bool)

	for _, c := range picked {
		meta, hasMeta := features[c.ItemID]
		if q.Mode == contract.Online && hasMeta && (!meta.Available || meta.OfferID == nil || *meta.OfferID == "") {
			continue
		}
		if hasMeta && meta.Audience != "" && profile.Group != "" && meta.Audience != profile.Group {
			continue
		}

		grouped := q.Mode == contract.Online && meta.FamilyID != "" && meta.FamilySource != ""
		gk := familyGroupKey{family: meta.FamilyID, kind: c.Kind}
		idx, exists := groupIdx[gk]
		if !grouped || !exists {
			if len(empty.Items) >= 3 {
				continue
			}
			idx = len(empty.Items)
			title := meta.Title
			if title == "" {
				title = string(c.ItemID)
			}
			explanation := "Рекомендовано для вашего профиля."
			if len(c.Evidence) > 0 && c.Evidence[0].Reason != "" {
				explanation = c.Evidence[0].Reason
			}
			var famPtr *string
			if meta.FamilyID != "" && meta.FamilySource != "" {
				famCopy := meta.FamilyID
				famPtr = &famCopy
			}
			empty.Items = append(empty.Items, domain.Recommendation{
				ItemID:      c.ItemID,
				OfferID:     meta.OfferID,
				SKU:         meta.SKU,
				Kind:        c.Kind,
				FamilyID:    famPtr,
				Title:       title,
				URL:         meta.URL,
				Explanation: explanation,
				Variants:    []domain.Variant{},
				EvidenceIDs: []string{},
			})
			if grouped {
				groupIdx[gk] = idx
			}
		}

		card := &empty.Items[idx]
		if grouped {
			diffs := meta.Differences
			if diffs == nil {
				diffs = []domain.Difference{}
			}
			card.Variants = append(card.Variants, domain.Variant{
				ItemID:      c.ItemID,
				URL:         meta.URL,
				Differences: diffs,
			})
		}
		for _, ev := range c.Evidence {
			if edgeSeen[ev.ID] {
				continue
			}
			edgeSeen[ev.ID] = true
			card.EvidenceIDs = append(card.EvidenceIDs, ev.ID)
			for _, nodeID := range []string{ev.From, ev.To} {
				if nodeID == "" || nodeSeen[nodeID] {
					continue
				}
				nodeSeen[nodeID] = true
				ntype, label := "cohort", nodeID
				if rawID, ok := strings.CutPrefix(nodeID, "item:"); ok {
					ntype = "product"
					label = rawID
					if fm, ok := features[domain.ItemID(rawID)]; ok && fm.Title != "" {
						label = fm.Title
					}
				}
				empty.Graph.Nodes = append(empty.Graph.Nodes, domain.Node{ID: nodeID, Type: ntype, Label: label})
			}
			empty.Graph.Edges = append(empty.Graph.Edges, ev)
		}
	}

	if len(empty.Items) < 3 {
		reason := "insufficient_eligible_products"
		empty.PartialReason = &reason
	}
	return empty, nil
}

func (r *SnapshotRecommender) loadFeatures(ctx context.Context, featureVersion string) (map[domain.ItemID]ItemFeature, error) {
	rows, err := r.store.pool.Query(ctx, `
		SELECT item_id, offer_id, title, sku, url, platform, audience,
		       available, in_sales, COALESCE(family_id, ''), COALESCE(family_source, ''), differences
		FROM recommendations.item_feature_versions
		WHERE feature_version = $1
	`, featureVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[domain.ItemID]ItemFeature)
	for rows.Next() {
		var f ItemFeature
		var itemID, audience string
		var diffBytes []byte
		if err := rows.Scan(&itemID, &f.OfferID, &f.Title, &f.SKU, &f.URL, &f.Platform, &audience,
			&f.Available, &f.InSales, &f.FamilyID, &f.FamilySource, &diffBytes); err != nil {
			return nil, err
		}
		f.ItemID = domain.ItemID(itemID)
		f.Audience = domain.ClientGroup(audience)
		if len(diffBytes) > 0 {
			_ = json.Unmarshal(diffBytes, &f.Differences)
		}
		out[f.ItemID] = f
	}
	return out, rows.Err()
}
