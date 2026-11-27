package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"recsys/internal/contract"
	"recsys/internal/data"
	"recsys/internal/domain"
)

// Expected canonical counts for the closed v4 workbook import.
const (
	V4ExpectedSalesRows     = 31753
	V4ExpectedClients       = 19008
	V4ExpectedObservedWeeks = 27603
	V4ExpectedDistinctItems = 817
	V4ExpectedTrainItems    = 813
	V4TrainThroughDate      = "2026-08-31"
)

// DatasetCounts summarizes imported historical dataset cardinality.
type DatasetCounts struct {
	SalesRows     int
	Clients       int
	ObservedWeeks int
	DistinctItems int
	TrainItems    int
}

// ValidateClosedV4Counts checks final item_id, basket, client, and row thresholds for closed v4 imports.
func ValidateClosedV4Counts(c DatasetCounts) error {
	if c.DistinctItems != V4ExpectedDistinctItems {
		return fmt.Errorf("v4 distinct item_id count = %d, want %d", c.DistinctItems, V4ExpectedDistinctItems)
	}
	if c.TrainItems != V4ExpectedTrainItems {
		return fmt.Errorf("v4 train item_id count = %d, want %d", c.TrainItems, V4ExpectedTrainItems)
	}
	if c.SalesRows != V4ExpectedSalesRows {
		return fmt.Errorf("v4 sales rows = %d, want %d", c.SalesRows, V4ExpectedSalesRows)
	}
	if c.Clients != V4ExpectedClients {
		return fmt.Errorf("v4 clients = %d, want %d", c.Clients, V4ExpectedClients)
	}
	if c.ObservedWeeks != V4ExpectedObservedWeeks {
		return fmt.Errorf("v4 (order_id, order_week) baskets = %d, want %d", c.ObservedWeeks, V4ExpectedObservedWeeks)
	}
	return nil
}

// WriteDataset publishes a complete normalized dataset version via temp staging + pgx CopyFrom in a single transaction.
// Re-running with the same or updated dataset is idempotent; any failure rolls back leaving the previous version intact.
func (s *Store) WriteDataset(ctx context.Context, version string, batches []contract.DatasetBatch) error {
	return s.WriteDatasetWithChecksum(ctx, version, "", batches)
}

// WriteDatasetWithChecksum writes a normalized dataset and records workbookSHA256 (or computes a deterministic digest if empty).
func (s *Store) WriteDatasetWithChecksum(ctx context.Context, version, workbookSHA256 string, batches []contract.DatasetBatch) error {
	ver := strings.TrimSpace(version)
	if ver == "" {
		return fmt.Errorf("dataset version is required")
	}

	type clientRow struct {
		excelRow int
		client   domain.Client
	}
	type saleRow struct {
		excelRow int
		sale     domain.Sale
	}

	seenClients := make(map[domain.ClientID]int)
	var clients []clientRow
	var sales []saleRow
	hasher := sha256.New()
	productGroups := make(map[domain.ItemID]string)

	nextClientRow := 2
	nextSaleRow := 2
	for _, batch := range batches {
		for _, c := range batch.Clients {
			id := domain.ClientID(strings.TrimSpace(string(c.ID)))
			if id == "" {
				return fmt.Errorf("client with empty client_id")
			}
			c.ID = id
			if idx, exists := seenClients[id]; exists {
				clients[idx].client = c
			} else {
				seenClients[id] = len(clients)
				clients = append(clients, clientRow{excelRow: nextClientRow, client: c})
				nextClientRow++
			}
		}
		for _, sale := range batch.Sales {
			if strings.TrimSpace(sale.OrderID) == "" || strings.TrimSpace(string(sale.ClientID)) == "" || strings.TrimSpace(string(sale.ItemID)) == "" {
				return fmt.Errorf("sale missing order_id, client_id, or item_id (source_row=%d)", sale.SourceRow)
			}
			if sale.OrderWeek.IsZero() {
				return fmt.Errorf("sale %s missing order_week", sale.OrderID)
			}
			rowNum := sale.SourceRow
			if rowNum <= 0 {
				rowNum = nextSaleRow
			}
			if rowNum >= nextSaleRow {
				nextSaleRow = rowNum + 1
			}
			sale.SourceRow = rowNum
			if ver == data.ExpectedV5Version && sale.ProductGroup == "" {
				return fmt.Errorf("v5 requires product_group for each sale")
			}
			if previous, ok := productGroups[sale.ItemID]; ok && previous != sale.ProductGroup {
				return fmt.Errorf("conflicting product_group for item %q", sale.ItemID)
			}
			productGroups[sale.ItemID] = sale.ProductGroup
			sales = append(sales, saleRow{excelRow: rowNum, sale: sale})

			if _, exists := seenClients[sale.ClientID]; !exists {
				seenClients[sale.ClientID] = len(clients)
				clients = append(clients, clientRow{
					excelRow: nextClientRow,
					client: domain.Client{
						ID:    sale.ClientID,
						Group: domain.B2B,
					},
				})
				nextClientRow++
			}
		}
	}

	for _, c := range clients {
		_, _ = fmt.Fprintf(hasher, "C|%s|%s|%s\n", c.client.ID, c.client.Group, c.client.Type)
	}
	for _, sr := range sales {
		_, _ = fmt.Fprintf(hasher, "S|%d|%s|%s|%s|%s\n",
			sr.excelRow, sr.sale.OrderID, sr.sale.ClientID, sr.sale.OrderWeek.UTC().Format("2006-01-02"), sr.sale.ItemID)
		if sr.sale.ProductGroup != "" {
			_, _ = fmt.Fprintf(hasher, "G|%q\n", sr.sale.ProductGroup)
		}
	}
	digest := strings.TrimSpace(workbookSHA256)
	if digest == "" {
		digest = hex.EncodeToString(hasher.Sum(nil))
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin dataset tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Historical rows are shared across releases. Serialize dataset publication.
	if _, err := tx.Exec(ctx, "LOCK TABLE recommendations.dataset_release IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		return fmt.Errorf("lock dataset releases: %w", err)
	}
	if ver != data.ExpectedV5Version {
		var hasV5 bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM recommendations.dataset_release WHERE version = 'v5')").Scan(&hasV5); err != nil {
			return fmt.Errorf("read dataset releases: %w", err)
		}
		if hasV5 {
			return fmt.Errorf("import %q would replace the published v5 history", ver)
		}
	}

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE stg_clients (
			excel_row integer NOT NULL,
			client_id text NOT NULL,
			client_group text,
			client_type text,
			industry text,
			segment text,
			lk_access text
		) ON COMMIT DROP;

		CREATE TEMP TABLE stg_sales (
			excel_row integer NOT NULL,
			order_id text NOT NULL,
			client_id text NOT NULL,
			order_week date NOT NULL,
			item_id text NOT NULL,
			sku text,
			item_name text,
			product_group text
		) ON COMMIT DROP;
	`); err != nil {
		return fmt.Errorf("create temp staging tables: %w", err)
	}

	if len(clients) > 0 {
		clientCopyRows := make([][]any, len(clients))
		for i, cr := range clients {
			lk := "нет"
			if cr.client.HasLKAccess {
				lk = "да"
			}
			var grp, ctype any
			if cr.client.Group != "" {
				grp = string(cr.client.Group)
			}
			if cr.client.Type != "" {
				ctype = cr.client.Type
			}
			clientCopyRows[i] = []any{
				cr.excelRow,
				string(cr.client.ID),
				grp,
				ctype,
				cr.client.Industry,
				cr.client.Segment,
				lk,
			}
		}
		if _, err := tx.CopyFrom(
			ctx,
			pgx.Identifier{"stg_clients"},
			[]string{"excel_row", "client_id", "client_group", "client_type", "industry", "segment", "lk_access"},
			pgx.CopyFromRows(clientCopyRows),
		); err != nil {
			return fmt.Errorf("COPY stg_clients: %w", err)
		}
	}

	if len(sales) > 0 {
		saleCopyRows := make([][]any, len(sales))
		for i, sr := range sales {
			var itemName any
			if strings.TrimSpace(sr.sale.ItemName) != "" {
				itemName = sr.sale.ItemName
			}
			saleCopyRows[i] = []any{
				sr.excelRow,
				sr.sale.OrderID,
				string(sr.sale.ClientID),
				sr.sale.OrderWeek.UTC().Format("2006-01-02"),
				string(sr.sale.ItemID),
				sr.sale.SKU,
				itemName,
				sr.sale.ProductGroup,
			}
		}
		if _, err := tx.CopyFrom(
			ctx,
			pgx.Identifier{"stg_sales"},
			[]string{"excel_row", "order_id", "client_id", "order_week", "item_id", "sku", "item_name", "product_group"},
			pgx.CopyFromRows(saleCopyRows),
		); err != nil {
			return fmt.Errorf("COPY stg_sales: %w", err)
		}
	}

	// Verify no single order_id belongs to multiple client_ids in staging.
	var multiOwnerOrder string
	err = tx.QueryRow(ctx, `
		SELECT order_id
		FROM stg_sales
		GROUP BY order_id
		HAVING COUNT(DISTINCT client_id) > 1
		LIMIT 1
	`).Scan(&multiOwnerOrder)
	if err == nil {
		return fmt.Errorf("order %q references multiple client_ids", multiOwnerOrder)
	}
	if err != pgx.ErrNoRows {
		return fmt.Errorf("validate order owners: %w", err)
	}

	// Replace prior historical orders/weeks/sales cleanly while preserving online/stand data.
	if _, err := tx.Exec(ctx, `
		DELETE FROM recommendations.sales s
		USING recommendations.orders o
		WHERE s.order_id = o.order_id AND o.source = 'history';

		DELETE FROM recommendations.sales s
		USING stg_sales stg
		WHERE s.excel_row = stg.excel_row OR s.order_id = stg.order_id;

		DELETE FROM recommendations.order_observed_weeks w
		USING recommendations.orders o
		WHERE w.order_id = o.order_id AND o.source = 'history';

		DELETE FROM recommendations.orders
		WHERE source = 'history';
	`); err != nil {
		return fmt.Errorf("clear prior historical sales: %w", err)
	}

	// Resolve any excel_row collisions on non-stand clients before upserting clients by client_id.
	if _, err := tx.Exec(ctx, `
		UPDATE recommendations.clients c
		SET excel_row = nextval('recommendations.stand_client_excel_row_seq')
		FROM stg_clients stg
		WHERE c.excel_row = stg.excel_row AND c.client_id <> stg.client_id;

		INSERT INTO recommendations.clients (
			excel_row, client_id, client_group, client_type, industry, segment, lk_access
		)
		SELECT excel_row, client_id, client_group, client_type, industry, segment, lk_access
		FROM stg_clients
		ON CONFLICT (client_id) DO UPDATE SET
			excel_row    = EXCLUDED.excel_row,
			client_group = EXCLUDED.client_group,
			client_type  = EXCLUDED.client_type,
			industry     = EXCLUDED.industry,
			segment      = EXCLUDED.segment,
			lk_access    = EXCLUDED.lk_access;

		INSERT INTO recommendations.historical_items (item_id)
		SELECT DISTINCT item_id FROM stg_sales
		ON CONFLICT (item_id) DO NOTHING;

		INSERT INTO recommendations.orders (order_id, client_id, source, order_week)
		SELECT
			order_id,
			MIN(client_id) AS client_id,
			'history' AS source,
			CASE WHEN COUNT(DISTINCT order_week) = 1 THEN MIN(order_week) ELSE NULL END AS order_week
		FROM stg_sales
		GROUP BY order_id
		ON CONFLICT (order_id) DO UPDATE SET
			client_id  = EXCLUDED.client_id,
			source     = 'history',
			order_week = EXCLUDED.order_week;

		INSERT INTO recommendations.order_observed_weeks (order_id, order_week)
		SELECT DISTINCT order_id, order_week
		FROM stg_sales
		ON CONFLICT (order_id, order_week) DO NOTHING;

		INSERT INTO recommendations.sales (
			excel_row, order_id, client_id, order_week, item_id, sku, item_name, product_group
		)
		SELECT excel_row, order_id, client_id, order_week, item_id, sku, item_name, NULLIF(product_group, '')
		FROM stg_sales;
	`); err != nil {
		return fmt.Errorf("publish staged dataset: %w", err)
	}

	if ver == "v4" || ver == "v5" {
		var counts DatasetCounts
		if err := tx.QueryRow(ctx, `
			SELECT
				(SELECT COUNT(*) FROM stg_sales),
				(SELECT COUNT(*) FROM stg_clients),
				(SELECT COUNT(*) FROM (SELECT DISTINCT order_id, order_week FROM stg_sales) b),
				(SELECT COUNT(DISTINCT item_id) FROM stg_sales),
				(SELECT COUNT(DISTINCT item_id) FROM stg_sales WHERE order_week <= $1::date)
		`, V4TrainThroughDate).Scan(
			&counts.SalesRows,
			&counts.Clients,
			&counts.ObservedWeeks,
			&counts.DistinctItems,
			&counts.TrainItems,
		); err != nil {
			return fmt.Errorf("count staged v4 dataset: %w", err)
		}
		if err := ValidateClosedV4Counts(counts); err != nil {
			return err
		}
		if ver == data.ExpectedV5Version {
			var groups int
			if err := tx.QueryRow(ctx, "SELECT count(DISTINCT product_group) FROM stg_sales").Scan(&groups); err != nil {
				return fmt.Errorf("count v5 product groups: %w", err)
			}
			if err := data.VerifyV5Release(ver, digest, counts.SalesRows, counts.DistinctItems, groups); err != nil {
				return err
			}
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO recommendations.dataset_release (version, workbook_sha256, imported_at)
		VALUES ($1, $2, now())
		ON CONFLICT (version) DO UPDATE SET
			workbook_sha256 = CASE
				WHEN $3 <> '' THEN EXCLUDED.workbook_sha256
				ELSE recommendations.dataset_release.workbook_sha256
			END,
			imported_at = now()
	`, ver, digest, strings.TrimSpace(workbookSHA256)); err != nil {
		return fmt.Errorf("upsert dataset_release: %w", err)
	}

	return tx.Commit(ctx)
}
