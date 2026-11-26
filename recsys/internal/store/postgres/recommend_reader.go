package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"recsys/internal/contract"
	"recsys/internal/domain"
	"recsys/internal/recommend"
)

var _ recommend.Reader = (*Store)(nil)

type SnapshotMetadata struct {
	recommend.SnapshotInfo
	DatasetVersion  string
	WorkbookSHA256  string
	FeedGeneratedAt *time.Time
}

func (s *Store) SnapshotMetadata(ctx context.Context, id string) (SnapshotMetadata, error) {
	var m SnapshotMetadata
	err := s.pool.QueryRow(ctx, `
  SELECT m.snapshot_id,m.train_through,m.dataset_version,m.workbook_sha256,f.feed_generated_at
  FROM recommendations.model_snapshots m
  LEFT JOIN recommendations.catalog_feed_versions f USING (catalog_version)
  WHERE ($1='' AND m.is_active) OR m.snapshot_id=NULLIF($1,'')
 `, id).Scan(&m.ID, &m.TrainThrough, &m.DatasetVersion, &m.WorkbookSHA256, &m.FeedGeneratedAt)
	return m, err
}

func (s *Store) SnapshotInfo(ctx context.Context, id string) (recommend.SnapshotInfo, error) {
	m, err := s.SnapshotMetadata(ctx, id)
	return m.SnapshotInfo, err
}

func (s *Store) Products(ctx context.Context, snapshot string, ids []domain.ItemID) (map[domain.ItemID]recommend.Product, error) {
	result := make(map[domain.ItemID]recommend.Product, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = string(id)
	}
	rows, err := s.pool.Query(ctx, `
  SELECT i.item_id,COALESCE(f.title,i.item_id),f.offer_id,f.sku,f.url,COALESCE(f.platform,''),
   COALESCE(f.audience,''),COALESCE(f.available,false),
   COALESCE(f.in_sales, EXISTS(SELECT 1 FROM recommendations.sales sales WHERE sales.item_id=i.item_id)),
   COALESCE(f.family_id,''),COALESCE(f.family_source,''),COALESCE(f.differences,'[]'::jsonb),COALESCE(f.organization_type,'')
  FROM recommendations.model_snapshots m
  JOIN recommendations.historical_items i ON i.item_id=ANY($2::text[])
  LEFT JOIN recommendations.item_feature_versions f ON f.feature_version=m.feature_version AND f.item_id=i.item_id
  WHERE m.snapshot_id=$1
 `, snapshot, keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id domain.ItemID
		var p recommend.Product
		if err := rows.Scan(&id, &p.Title, &p.OfferID, &p.SKU, &p.URL, &p.Platform, &p.Audience, &p.Available, &p.InSales,
			&p.FamilyID, &p.FamilySource, &p.Differences, &p.OrganizationType); err != nil {
			return nil, err
		}
		result[id] = p
	}
	return result, rows.Err()
}

// OfflineDataset reads the historical tables together. Online purchases are excluded.
func (s *Store) OfflineDataset(ctx context.Context) (contract.DatasetBatch, error) {
	var batch contract.DatasetBatch
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return batch, err
	}
	defer tx.Rollback(ctx)
	clients, err := tx.Query(ctx, `SELECT client_id,COALESCE(client_group,''),COALESCE(client_type,''),industry,segment,
  lower(COALESCE(lk_access,''))='да' FROM recommendations.clients ORDER BY client_id`)
	if err != nil {
		return batch, err
	}
	batch.Clients, err = pgx.CollectRows(clients, func(row pgx.CollectableRow) (domain.Client, error) {
		var c domain.Client
		err := row.Scan(&c.ID, &c.Group, &c.Type, &c.Industry, &c.Segment, &c.HasLKAccess)
		return c, err
	})
	if err != nil {
		return batch, err
	}
	sales, err := tx.Query(ctx, `SELECT excel_row,order_id,client_id,order_week,item_id,sku,COALESCE(item_name,''),COALESCE(product_group,'')
  FROM recommendations.sales ORDER BY excel_row`)
	if err != nil {
		return batch, err
	}
	batch.Sales, err = pgx.CollectRows(sales, func(row pgx.CollectableRow) (domain.Sale, error) {
		var sale domain.Sale
		err := row.Scan(&sale.SourceRow, &sale.OrderID, &sale.ClientID, &sale.OrderWeek, &sale.ItemID, &sale.SKU, &sale.ItemName, &sale.ProductGroup)
		return sale, err
	})
	if err != nil {
		return batch, err
	}
	return batch, tx.Commit(ctx)
}
