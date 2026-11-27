package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"recsys/internal/contract"
	"recsys/internal/domain"
)

// CatalogOfferInput describes a single offer in a versioned catalog feed batch.
type CatalogOfferInput struct {
	OfferID      string
	CategoryID   string
	CategoryName string
	VendorName   string
	GroupID      string
	Name         string
	VendorCode   string
	URL          string
	Description  string
	PictureURL   string
	Available    bool
	Price        float64
	OldPrice     float64
	Currency     string
	Parameters   json.RawMessage
}

// CatalogFeedBatch groups offers belonging to one immutable catalog_version.
type CatalogFeedBatch struct {
	CatalogVersion string
	SourceURL      string
	GeneratedAt    time.Time
	Offers         []CatalogOfferInput
}

// CatalogOfferRecord represents an offer read back for a specific catalog_version.
type CatalogOfferRecord struct {
	CatalogVersion string
	OfferID        string
	CategoryID     string
	VendorName     string
	Name           string
	VendorCode     string
	URL            string
	Available      bool
	Price          float64
	Currency       string
}

// ItemFeature holds versioned product attributes and variant differences keyed by item_id.
type ItemFeature struct {
	ItemID           domain.ItemID
	OfferID          *string
	Title            string
	SKU              *string
	URL              *string
	Platform         string
	Audience         domain.ClientGroup
	OrganizationType string
	Available        bool
	InSales          bool
	FamilyID         string
	FamilySource     string
	Differences      []domain.Difference
}

// PublishCatalogFeed stores a versioned feed in catalog_version_offers (keyed by (catalog_version, offer_id))
// while keeping recommendations.catalog_offers (PRIMARY KEY (offer_id)) updated for existing FKs.
func (s *Store) PublishCatalogFeed(ctx context.Context, batch CatalogFeedBatch) error {
	ver := strings.TrimSpace(batch.CatalogVersion)
	if ver == "" || strings.TrimSpace(batch.SourceURL) == "" || batch.GeneratedAt.IsZero() {
		return fmt.Errorf("catalog_version, source_url, and generated_at are required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var snapSeqID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO recommendations.catalog_snapshots (source_url, feed_generated_at)
		VALUES ($1, $2)
		ON CONFLICT (source_url, feed_generated_at) DO UPDATE SET imported_at = now()
		RETURNING id
	`, batch.SourceURL, batch.GeneratedAt.UTC()).Scan(&snapSeqID)
	if err != nil {
		return fmt.Errorf("upsert catalog_snapshots: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO recommendations.catalog_feed_versions (
			catalog_version, snapshot_seq_id, source_url, feed_generated_at, imported_at
		) VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (catalog_version) DO UPDATE SET
			snapshot_seq_id   = EXCLUDED.snapshot_seq_id,
			source_url        = EXCLUDED.source_url,
			feed_generated_at = EXCLUDED.feed_generated_at,
			imported_at       = now()
	`, ver, snapSeqID, batch.SourceURL, batch.GeneratedAt.UTC()); err != nil {
		return fmt.Errorf("upsert catalog_feed_versions: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM recommendations.catalog_version_offers WHERE catalog_version = $1`, ver); err != nil {
		return fmt.Errorf("clear catalog_version_offers: %w", err)
	}

	for _, off := range batch.Offers {
		if strings.TrimSpace(off.OfferID) == "" || strings.TrimSpace(off.Name) == "" || strings.TrimSpace(off.URL) == "" {
			return fmt.Errorf("offer requires offer_id, name, and url")
		}
		catID := strings.TrimSpace(off.CategoryID)
		if catID == "" {
			catID = "uncategorized"
		}
		catName := strings.TrimSpace(off.CategoryName)
		if catName == "" {
			catName = catID
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO recommendations.catalog_categories (category_id, name)
			VALUES ($1, $2)
			ON CONFLICT (category_id) DO NOTHING
		`, catID, catName); err != nil {
			return fmt.Errorf("upsert category %s: %w", catID, err)
		}

		var vendorID *int64
		if vName := strings.TrimSpace(off.VendorName); vName != "" {
			var vid int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO recommendations.catalog_vendors (name)
				VALUES ($1)
				ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
				RETURNING vendor_id
			`, vName).Scan(&vid); err != nil {
				return fmt.Errorf("upsert vendor %s: %w", vName, err)
			}
			vendorID = &vid
		}

		var groupID *string
		if g := strings.TrimSpace(off.GroupID); g != "" {
			if _, err := tx.Exec(ctx, `
				INSERT INTO recommendations.catalog_groups (group_id)
				VALUES ($1)
				ON CONFLICT (group_id) DO NOTHING
			`, g); err != nil {
				return fmt.Errorf("upsert group %s: %w", g, err)
			}
			groupID = &g
		}

		var currency *string
		if cur := strings.TrimSpace(off.Currency); cur != "" {
			if _, err := tx.Exec(ctx, `
				INSERT INTO recommendations.catalog_currencies (currency_code, rate)
				VALUES ($1, 1)
				ON CONFLICT (currency_code) DO NOTHING
			`, cur); err != nil {
				return fmt.Errorf("upsert currency %s: %w", cur, err)
			}
			currency = &cur
		}

		paramsJSON := off.Parameters
		if len(paramsJSON) == 0 {
			paramsJSON = []byte("[]")
		}
		vcKey := strings.ToUpper(strings.TrimSpace(off.VendorCode))
		nameKey := strings.ToLower(strings.Join(strings.Fields(off.Name), " "))

		if _, err := tx.Exec(ctx, `
			INSERT INTO recommendations.catalog_version_offers (
				catalog_version, offer_id, category_id, vendor_name, group_id, name,
				vendor_code, url, description, picture_url, available, price, old_price,
				currency_code, parameters, vendor_code_key, name_key
			) VALUES (
				$1, $2, $3, NULLIF($4, ''), $5, $6,
				NULLIF($7, ''), $8, NULLIF($9, ''), NULLIF($10, ''), $11, $12, $13,
				$14, $15::jsonb, NULLIF($16, ''), NULLIF($17, '')
			)
		`, ver, off.OfferID, catID, strings.TrimSpace(off.VendorName), groupID, off.Name,
			strings.TrimSpace(off.VendorCode), off.URL, strings.TrimSpace(off.Description),
			strings.TrimSpace(off.PictureURL), off.Available, off.Price, off.OldPrice,
			currency, string(paramsJSON), vcKey, nameKey,
		); err != nil {
			return fmt.Errorf("insert catalog_version_offers %s: %w", off.OfferID, err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO recommendations.catalog_offers (
				offer_id, snapshot_id, category_id, vendor_id, group_id, name,
				vendor_code, url, description, picture_url, available, price, old_price,
				currency_code, parameters, vendor_code_key, name_key
			) VALUES (
				$1, $2, $3, $4, $5, $6,
				NULLIF($7, ''), $8, NULLIF($9, ''), NULLIF($10, ''), $11, $12, $13,
				$14, $15::jsonb, NULLIF($16, ''), NULLIF($17, '')
			)
			ON CONFLICT (offer_id) DO UPDATE SET
				snapshot_id     = EXCLUDED.snapshot_id,
				category_id     = EXCLUDED.category_id,
				vendor_id       = EXCLUDED.vendor_id,
				group_id        = EXCLUDED.group_id,
				name            = EXCLUDED.name,
				vendor_code     = EXCLUDED.vendor_code,
				url             = EXCLUDED.url,
				description     = EXCLUDED.description,
				picture_url     = EXCLUDED.picture_url,
				available       = EXCLUDED.available,
				price           = EXCLUDED.price,
				old_price       = EXCLUDED.old_price,
				currency_code   = EXCLUDED.currency_code,
				parameters      = EXCLUDED.parameters,
				vendor_code_key = EXCLUDED.vendor_code_key,
				name_key        = EXCLUDED.name_key
		`, off.OfferID, snapSeqID, catID, vendorID, groupID, off.Name,
			strings.TrimSpace(off.VendorCode), off.URL, strings.TrimSpace(off.Description),
			strings.TrimSpace(off.PictureURL), off.Available, off.Price, off.OldPrice,
			currency, string(paramsJSON), vcKey, nameKey,
		); err != nil {
			return fmt.Errorf("upsert catalog_offers %s: %w", off.OfferID, err)
		}
	}

	return tx.Commit(ctx)
}

// CatalogVersionOffers reads offers belonging to a specific catalog_version.
func (s *Store) CatalogVersionOffers(ctx context.Context, catalogVersion string) ([]CatalogOfferRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT catalog_version, offer_id, category_id, COALESCE(vendor_name, ''),
		       name, COALESCE(vendor_code, ''), url, available,
		       COALESCE(price, 0)::float8, COALESCE(currency_code, '')
		FROM recommendations.catalog_version_offers
		WHERE catalog_version = $1
		ORDER BY offer_id
	`, catalogVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CatalogOfferRecord
	for rows.Next() {
		var r CatalogOfferRecord
		if err := rows.Scan(&r.CatalogVersion, &r.OfferID, &r.CategoryID, &r.VendorName,
			&r.Name, &r.VendorCode, &r.URL, &r.Available, &r.Price, &r.Currency); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PublishItemFeatures writes versioned item features and variant differences in one transaction.
func (s *Store) PublishItemFeatures(ctx context.Context, featureVersion string, features []ItemFeature) error {
	fv := strings.TrimSpace(featureVersion)
	if fv == "" {
		return fmt.Errorf("feature_version is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := writeItemFeatures(ctx, tx, fv, features); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func writeItemFeatures(ctx context.Context, tx pgx.Tx, fv string, features []ItemFeature) error {
	if _, err := tx.Exec(ctx, `DELETE FROM recommendations.item_feature_versions WHERE feature_version = $1`, fv); err != nil {
		return err
	}
	for _, f := range features {
		if strings.TrimSpace(string(f.ItemID)) == "" {
			return fmt.Errorf("item feature missing item_id")
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO recommendations.historical_items (item_id)
			VALUES ($1)
			ON CONFLICT (item_id) DO NOTHING
		`, string(f.ItemID)); err != nil {
			return err
		}
		diffs := f.Differences
		if diffs == nil {
			diffs = []domain.Difference{}
		}
		diffBytes, err := json.Marshal(diffs)
		if err != nil {
			return err
		}
		title := strings.TrimSpace(f.Title)
		if title == "" {
			title = string(f.ItemID)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO recommendations.item_feature_versions (
				feature_version, item_id, offer_id, title, sku, url,
				platform, audience, available, in_sales, family_id, family_source, differences, organization_type
			) VALUES (
				$1, $2, $3, $4, $5, $6,
				$7, $8, $9, $10, NULLIF($11, ''), NULLIF($12, ''), $13::jsonb, $14
			)
		`, fv, string(f.ItemID), f.OfferID, title, f.SKU, f.URL,
			f.Platform, string(f.Audience), f.Available, f.InSales,
			f.FamilyID, f.FamilySource, string(diffBytes), f.OrganizationType,
		); err != nil {
			return fmt.Errorf("insert item_feature_versions %s: %w", f.ItemID, err)
		}
	}
	return nil
}

// PublishSnapshot atomically publishes a completed snapshot bound to dataset_release (version + workbook_sha256).
// Candidates without catalog offers retain their canonical item_id.
func (s *Store) PublishSnapshot(ctx context.Context, snapshot contract.Snapshot) error {
	return s.publishSnapshot(ctx, snapshot, nil, nil)
}

func (s *Store) PublishSnapshotWithFeatures(ctx context.Context, snapshot contract.Snapshot, features []ItemFeature, feed CatalogFeedBatch) error {
	if features == nil {
		features = []ItemFeature{}
	}
	return s.publishSnapshot(ctx, snapshot, features, &feed)
}

func (s *Store) publishSnapshot(ctx context.Context, snapshot contract.Snapshot, features []ItemFeature, feed *CatalogFeedBatch) error {
	snapID := strings.TrimSpace(snapshot.ID)
	dsVer := strings.TrimSpace(snapshot.DatasetVersion)
	if snapID == "" || dsVer == "" || snapshot.TrainThrough.IsZero() {
		return fmt.Errorf("snapshot requires ID, DatasetVersion, and TrainThrough")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var workbookSHA string
	err = tx.QueryRow(ctx, `
		SELECT workbook_sha256
		FROM recommendations.dataset_release
		WHERE version = $1
	`, dsVer).Scan(&workbookSHA)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("dataset_release version %q does not exist", dsVer)
	}
	if err != nil {
		return fmt.Errorf("lookup dataset_release %q: %w", dsVer, err)
	}
	if strings.TrimSpace(workbookSHA) == "" {
		return fmt.Errorf("dataset_release %q has empty workbook_sha256", dsVer)
	}

	featureVer := strings.TrimSpace(snapshot.CatalogVersion)
	if featureVer == "" {
		featureVer = snapID
	}

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(5042026)`); err != nil {
		return err
	}
	if feed != nil {
		if feed.CatalogVersion != snapshot.CatalogVersion || feed.SourceURL == "" || feed.GeneratedAt.IsZero() {
			return fmt.Errorf("invalid snapshot feed metadata")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO recommendations.catalog_feed_versions (catalog_version,source_url,feed_generated_at)
   VALUES ($1,$2,$3) ON CONFLICT (catalog_version) DO NOTHING`, feed.CatalogVersion, feed.SourceURL, feed.GeneratedAt); err != nil {
			return err
		}
		var consistent bool
		if err := tx.QueryRow(ctx, `SELECT source_url=$2 AND feed_generated_at=$3
   FROM recommendations.catalog_feed_versions WHERE catalog_version=$1`, feed.CatalogVersion, feed.SourceURL, feed.GeneratedAt).Scan(&consistent); err != nil {
			return err
		}
		if !consistent {
			return fmt.Errorf("catalog version already has different metadata")
		}
	}

	if features != nil {
		// Key features by snapshot so publishing a new batch cannot change old readers.
		featureVer = snapID
		if err := writeItemFeatures(ctx, tx, featureVer, features); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE recommendations.model_snapshots SET is_active = false WHERE is_active`); err != nil {
		return fmt.Errorf("deactivate prior snapshots: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO recommendations.model_snapshots (
			snapshot_id, dataset_version, workbook_sha256, catalog_version, feature_version, train_through, is_active, published_at
		) VALUES ($1, $2, $3, $4, $5, $6, true, now())
		ON CONFLICT (snapshot_id) DO UPDATE SET
			dataset_version = EXCLUDED.dataset_version,
			workbook_sha256 = EXCLUDED.workbook_sha256,
			catalog_version = EXCLUDED.catalog_version,
			feature_version = EXCLUDED.feature_version,
			train_through   = EXCLUDED.train_through,
			is_active       = true,
			published_at    = now()
	`, snapID, dsVer, workbookSHA, snapshot.CatalogVersion, featureVer, snapshot.TrainThrough.UTC().Format("2006-01-02")); err != nil {
		return fmt.Errorf("upsert model_snapshots: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM recommendations.snapshot_candidates WHERE snapshot_id = $1`, snapID); err != nil {
		return fmt.Errorf("clear snapshot_candidates: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM recommendations.evidence_records WHERE snapshot_id = $1`, snapID); err != nil {
		return fmt.Errorf("clear evidence_records: %w", err)
	}

	seenEvidence := make(map[string]bool)
	var candRows [][]any

	for _, cs := range snapshot.CandidateSets {
		for idx, cand := range cs.Candidates {
			itemID := strings.TrimSpace(string(cand.ItemID))
			if itemID == "" {
				return fmt.Errorf("candidate with empty item_id in signal %q", cs.Key.Signal)
			}
			// Ensure item_id is registered in historical_items even if it has no catalog offer.
			if _, err := tx.Exec(ctx, `
				INSERT INTO recommendations.historical_items (item_id)
				VALUES ($1)
				ON CONFLICT (item_id) DO NOTHING
			`, itemID); err != nil {
				return fmt.Errorf("ensure historical_item %s: %w", itemID, err)
			}

			evIDs := make([]string, 0, len(cand.Evidence))
			for _, ev := range cand.Evidence {
				if strings.TrimSpace(ev.ID) == "" {
					continue
				}
				evIDs = append(evIDs, ev.ID)
				if !seenEvidence[ev.ID] {
					seenEvidence[ev.ID] = true
					evSnap := ev.SnapshotID
					if evSnap == "" {
						evSnap = snapID
					}
					if _, err := tx.Exec(ctx, `
						INSERT INTO recommendations.evidence_records (
							snapshot_id, evidence_id, from_node, to_node, signal_type, weight, reason, source
						) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
						ON CONFLICT (snapshot_id, evidence_id) DO UPDATE SET
							from_node   = EXCLUDED.from_node,
							to_node     = EXCLUDED.to_node,
							signal_type = EXCLUDED.signal_type,
							weight      = EXCLUDED.weight,
							reason      = EXCLUDED.reason,
							source      = EXCLUDED.source
					`, snapID, ev.ID, ev.From, ev.To, ev.Type, ev.Weight, ev.Reason, ev.Source); err != nil {
						return fmt.Errorf("upsert evidence %s: %w", ev.ID, err)
					}
				}
			}

			evJSON, err := json.Marshal(evIDs)
			if err != nil {
				return err
			}
			candRows = append(candRows, []any{
				snapID,
				cs.Key.Signal,
				string(cs.Key.SourceItemID),
				string(cs.Key.ClientGroup),
				cs.Key.ClientType,
				cs.Key.Industry,
				cs.Key.Segment,
				idx + 1,
				itemID,
				string(cand.Kind),
				cand.Score,
				string(evJSON),
			})
		}
	}

	if len(candRows) > 0 {
		if _, err := tx.CopyFrom(
			ctx,
			pgx.Identifier{"recommendations", "snapshot_candidates"},
			[]string{
				"snapshot_id", "signal", "source_item_id", "client_group",
				"client_type", "industry", "segment", "rank",
				"item_id", "recommendation_kind", "score", "evidence_ids",
			},
			pgx.CopyFromRows(candRows),
		); err != nil {
			return fmt.Errorf("COPY snapshot_candidates: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// Profile reads client metadata from recommendations.clients.
func (s *Store) Profile(ctx context.Context, clientID domain.ClientID) (domain.Client, error) {
	lookup, _, isScenario := previewScenario(clientID)
	if !isScenario {
		lookup = clientID
	}
	var grp, ctype, lk string
	var industry, segment *string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(client_group, ''), COALESCE(client_type, ''), industry, segment, COALESCE(lk_access, '')
		FROM recommendations.clients
		WHERE client_id = $1
		  AND (NOT $2 OR EXISTS (SELECT 1 FROM recommendations.model_snapshots WHERE snapshot_id = 'demo-v1'))
	`, string(lookup), isScenario).Scan(&grp, &ctype, &industry, &segment, &lk)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Client{}, fmt.Errorf("%w: client %s", ErrNotFound, clientID)
	}
	if err != nil {
		return domain.Client{}, err
	}
	return domain.Client{
		ID:          clientID,
		Group:       domain.ClientGroup(grp),
		Type:        ctype,
		Industry:    industry,
		Segment:     segment,
		HasLKAccess: strings.EqualFold(strings.TrimSpace(lk), "да"),
	}, nil
}

// History returns purchases for clientID. A zero throughWeek reads full current history
// (historical sales plus isolated online profile purchases); non-zero filters inclusively by week.
func (s *Store) History(ctx context.Context, clientID domain.ClientID, throughWeek time.Time) ([]domain.Sale, error) {
	if template, item, ok := previewScenario(clientID); ok && throughWeek.IsZero() {
		if s.closedPreview {
			var err error
			item, err = closedPreviewSeed(ctx, s.pool, template)
			if err != nil {
				return nil, err
			}
		}
		if item == "" {
			return []domain.Sale{}, nil
		}
		return []domain.Sale{{ClientID: clientID, ItemID: item}}, nil
	}
	var rows pgx.Rows
	var err error
	if throughWeek.IsZero() {
		rows, err = s.pool.Query(ctx, `
			SELECT excel_row, order_id, client_id, order_week, item_id, sku, COALESCE(item_name, '')
			FROM (
				SELECT s.excel_row, s.order_id, s.client_id, s.order_week, s.item_id, s.sku, s.item_name, 0 AS src_ord
				FROM recommendations.sales s
				WHERE s.client_id = $1
				UNION ALL
				SELECT (100000000 + p.purchase_id)::int AS excel_row, p.order_id, p.client_id, p.purchased_week AS order_week, p.item_id, p.sku, p.item_name, 1 AS src_ord
				FROM recommendations.online_purchases p
				WHERE p.client_id = $1
			) combined
			ORDER BY order_week ASC, src_ord ASC, excel_row ASC
		`, string(clientID))
	} else {
		cutoff := throughWeek.UTC().Format("2006-01-02")
		rows, err = s.pool.Query(ctx, `
			SELECT excel_row, order_id, client_id, order_week, item_id, sku, COALESCE(item_name, '')
   FROM recommendations.sales
   WHERE client_id = $1 AND order_week <= $2::date
   ORDER BY order_week ASC, excel_row ASC
  `, string(clientID), cutoff)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.Sale{}
	for rows.Next() {
		var sale domain.Sale
		var cid, iid string
		if err := rows.Scan(&sale.SourceRow, &sale.OrderID, &cid, &sale.OrderWeek, &iid, &sale.SKU, &sale.ItemName); err != nil {
			return nil, err
		}
		sale.ClientID = domain.ClientID(cid)
		sale.ItemID = domain.ItemID(iid)
		sale.OrderWeek = sale.OrderWeek.UTC()
		out = append(out, sale)
	}
	return out, rows.Err()
}

// ResolveSnapshotID resolves an empty snapshotID to the currently active snapshot.
func (s *Store) ResolveSnapshotID(ctx context.Context, snapshotID string) (string, error) {
	if id := strings.TrimSpace(snapshotID); id != "" {
		return id, nil
	}
	var active string
	err := s.pool.QueryRow(ctx, `
		SELECT snapshot_id
		FROM recommendations.model_snapshots
		WHERE is_active
		LIMIT 1
	`).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%w: no active snapshot", ErrNotFound)
	}
	return active, err
}

// Candidates reads top-N candidates for (snapshotID, key) ordered by score DESC, item_id ASC.
func (s *Store) Candidates(ctx context.Context, snapshotID string, key contract.CandidateKey, limit int) ([]domain.Candidate, error) {
	if limit <= 0 {
		limit = 64
	}
	resolvedID, err := s.ResolveSnapshotID(ctx, snapshotID)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT item_id, recommendation_kind, score, evidence_ids
		FROM recommendations.snapshot_candidates
		WHERE snapshot_id = $1
		  AND signal = $2
		  AND source_item_id = $3
		  AND client_group = $4
		  AND client_type = $5
		  AND industry = $6
		  AND segment = $7
		ORDER BY score DESC, item_id COLLATE "C" ASC
		LIMIT $8
	`, resolvedID, key.Signal, string(key.SourceItemID), string(key.ClientGroup),
		key.ClientType, key.Industry, key.Segment, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type rawCand struct {
		cand  domain.Candidate
		evIDs []string
	}
	var rawList []rawCand
	var allEvIDs []string
	seenEv := make(map[string]bool)

	for rows.Next() {
		var itemID, kind string
		var score float64
		var evBytes []byte
		if err := rows.Scan(&itemID, &kind, &score, &evBytes); err != nil {
			return nil, err
		}
		var ids []string
		if len(evBytes) > 0 {
			_ = json.Unmarshal(evBytes, &ids)
		}
		for _, id := range ids {
			if !seenEv[id] {
				seenEv[id] = true
				allEvIDs = append(allEvIDs, id)
			}
		}
		rawList = append(rawList, rawCand{
			cand: domain.Candidate{
				ItemID:   domain.ItemID(itemID),
				Kind:     domain.RecommendationKind(kind),
				Score:    score,
				Evidence: []domain.Evidence{},
			},
			evIDs: ids,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(rawList) == 0 {
		return []domain.Candidate{}, nil
	}

	evMap := make(map[string]domain.Evidence, len(allEvIDs))
	if len(allEvIDs) > 0 {
		evRows, err := s.pool.Query(ctx, `
			SELECT evidence_id, from_node, to_node, signal_type, weight, reason, source, snapshot_id
			FROM recommendations.evidence_records
			WHERE snapshot_id = $1 AND evidence_id = ANY($2)
		`, resolvedID, allEvIDs)
		if err != nil {
			return nil, err
		}
		for evRows.Next() {
			var ev domain.Evidence
			if err := evRows.Scan(&ev.ID, &ev.From, &ev.To, &ev.Type, &ev.Weight, &ev.Reason, &ev.Source, &ev.SnapshotID); err != nil {
				evRows.Close()
				return nil, err
			}
			evMap[ev.ID] = ev
		}
		evRows.Close()
		if err := evRows.Err(); err != nil {
			return nil, err
		}
	}

	out := make([]domain.Candidate, len(rawList))
	for i, rc := range rawList {
		for _, id := range rc.evIDs {
			if ev, ok := evMap[id]; ok {
				rc.cand.Evidence = append(rc.cand.Evidence, ev)
			}
		}
		out[i] = rc.cand
	}
	return out, nil
}
