package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"recsys/internal/domain"
)

// OnlineRelationItem contains catalog data used to find pairs and detect changes.
type OnlineRelationItem struct {
	ID          domain.ItemID
	OfferID     string
	Title       string
	SKU         string
	Family      string
	Fingerprint string
}

func (s *Store) OnlineRelationItems(ctx context.Context, snapshot string) ([]OnlineRelationItem, error) {
	if !strings.HasPrefix(snapshot, "v5-online-") {
		return nil, fmt.Errorf("relation jobs require a v5-online-* snapshot")
	}

	var feature, catalog string
	err := s.pool.QueryRow(ctx, `
		SELECT feature_version, catalog_version
		FROM recommendations.model_snapshots
		WHERE snapshot_id=$1 AND dataset_version='v5' AND NOT is_active
		  AND feature_version<>'' AND catalog_version<>''
	`, snapshot).Scan(&feature, &catalog)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("online snapshot %q with catalog features was not found", snapshot)
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT i.item_id, o.offer_id, o.name, coalesce(o.vendor_code, i.sku,''), coalesce(i.family_id,''),
		       md5((to_jsonb(o)-'catalog_version')::text)
		FROM recommendations.item_feature_versions i
		JOIN recommendations.catalog_version_offers o ON o.catalog_version=$2 AND o.offer_id=i.offer_id
		WHERE i.feature_version=$1 AND i.available AND o.available
		ORDER BY i.item_id COLLATE "C"
	`, feature, catalog)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []OnlineRelationItem
	for rows.Next() {
		var item OnlineRelationItem
		if err := rows.Scan(&item.ID, &item.OfferID, &item.Title, &item.SKU, &item.Family, &item.Fingerprint); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
