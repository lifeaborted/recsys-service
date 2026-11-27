package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"recsys/internal/contract"
)

// ProductDetail reads the full catalog card, first via the active feature
// version, then directly by offer_id/vendor_code as a fallback.
func (s *Store) ProductDetail(ctx context.Context, itemID string) (contract.ProductDetail, error) {
	var out contract.ProductDetail
	var params []byte

	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(f.item_id, o.vendor_code, o.offer_id),
		       o.offer_id,
		       COALESCE(f.sku, o.vendor_code, ''),
		       COALESCE(f.title, o.name, ''),
		       COALESCE(c.name, ''),
		       COALESCE(v.name, ''),
		       o.price,
		       o.old_price,
		       COALESCE(o.currency_code, ''),
		       COALESCE(o.available, false),
		       COALESCE(f.url, o.url, ''),
		       COALESCE(o.description, ''),
		       COALESCE(o.picture_url, ''),
		       COALESCE(o.vendor_url, ''),
		       COALESCE(o.delivery, false),
		       COALESCE(o.pickup, false),
		       COALESCE(o.sales_notes, ''),
		       o.parameters
		FROM recommendations.item_feature_versions f
		JOIN recommendations.model_snapshots m
		  ON m.feature_version = f.feature_version
		  AND (($2 = '' AND m.is_active) OR m.snapshot_id = NULLIF($2, ''))
		JOIN recommendations.catalog_offers o ON o.offer_id = f.offer_id
		LEFT JOIN recommendations.catalog_categories c ON c.category_id = o.category_id
		LEFT JOIN recommendations.catalog_vendors v ON v.vendor_id = o.vendor_id
		WHERE f.item_id = $1 AND (NOT $3 OR f.available)
		LIMIT 1
	`, itemID, s.onlineSnapshot, s.closedPreview).Scan(
		&out.ItemID, &out.OfferID, &out.SKU, &out.Title, &out.Category, &out.Vendor,
		&out.Price, &out.OldPrice, &out.CurrencyCode, &out.Available, &out.URL,
		&out.Description, &out.PictureURL, &out.VendorURL, &out.Delivery, &out.Pickup,
		&out.SalesNotes, &params,
	)
	if errors.Is(err, pgx.ErrNoRows) && !s.closedPreview {
		err = s.pool.QueryRow(ctx, `
			SELECT COALESCE(o.vendor_code, o.offer_id),
			       o.offer_id,
			       COALESCE(o.vendor_code, ''),
			       COALESCE(o.name, ''),
			       COALESCE(c.name, ''),
			       COALESCE(v.name, ''),
			       o.price,
			       o.old_price,
			       COALESCE(o.currency_code, ''),
			       COALESCE(o.available, false),
			       COALESCE(o.url, ''),
			       COALESCE(o.description, ''),
			       COALESCE(o.picture_url, ''),
			       COALESCE(o.vendor_url, ''),
			       COALESCE(o.delivery, false),
			       COALESCE(o.pickup, false),
			       COALESCE(o.sales_notes, ''),
			       o.parameters
			FROM recommendations.catalog_offers o
			LEFT JOIN recommendations.catalog_categories c ON c.category_id = o.category_id
			LEFT JOIN recommendations.catalog_vendors v ON v.vendor_id = o.vendor_id
			WHERE o.offer_id = $1 OR o.vendor_code = $1
			ORDER BY (o.offer_id = $1) DESC
			LIMIT 1
		`, itemID).Scan(
			&out.ItemID, &out.OfferID, &out.SKU, &out.Title, &out.Category, &out.Vendor,
			&out.Price, &out.OldPrice, &out.CurrencyCode, &out.Available, &out.URL,
			&out.Description, &out.PictureURL, &out.VendorURL, &out.Delivery, &out.Pickup,
			&out.SalesNotes, &params,
		)
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, contract.ErrProductNotFound
		}
		return out, err
	}
	out.Parameters = []contract.Parameter{}
	if len(params) > 0 && string(params) != "null" {
		if err = json.Unmarshal(params, &out.Parameters); err != nil {
			out.Parameters = []contract.Parameter{{Name: "value", Value: string(params)}}
		}
	}
	return out, nil
}
