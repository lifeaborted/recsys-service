package postgres

import (
	"context"
	"fmt"
	"strings"

	"recsys/internal/domain"
)

// ListProductsPage returns a page of the storefront catalog plus the total
// number of rows matching the optional filters.
func (s *Store) ListProductsPage(ctx context.Context, limit, offset int, query string, withHistory bool) ([]CatalogProduct, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	countParts := []string{}
	dataParts := []string{}
	countArgs := []any{}
	dataArgs := []any{limit, offset}
	if query != "" {
		countParts = append(countParts, "(sc.name ILIKE '%' || $1 || '%' OR COALESCE(sc.vendor_code, '') ILIKE '%' || $1 || '%')")
		dataParts = append(dataParts, "(sc.name ILIKE '%' || $3 || '%' OR COALESCE(sc.vendor_code, '') ILIKE '%' || $3 || '%')")
		countArgs = append(countArgs, query)
		dataArgs = append(dataArgs, query)
	}
	if s.closedPreview {
		const scope = "EXISTS (SELECT 1 FROM recommendations.model_snapshots m JOIN recommendations.item_feature_versions f USING(feature_version) WHERE m.snapshot_id = $%d AND f.available AND f.offer_id=sc.offer_id AND f.item_id=hio.item_id)"
		countParts = append(countParts, fmt.Sprintf(scope, len(countArgs)+1))
		dataParts = append(dataParts, fmt.Sprintf(scope, len(dataArgs)+1))
		countArgs = append(countArgs, s.onlineSnapshot)
		dataArgs = append(dataArgs, s.onlineSnapshot)
	}
	if withHistory {
		countParts = append(countParts, "EXISTS (SELECT 1 FROM recommendations.recommendation_items ri WHERE ri.source_item_id = COALESCE(hio.item_id, sc.offer_id))")
		dataParts = append(dataParts, "EXISTS (SELECT 1 FROM recommendations.recommendation_items ri WHERE ri.source_item_id = COALESCE(hio.item_id, sc.offer_id))")
	}

	countWhere := ""
	if len(countParts) > 0 {
		countWhere = "WHERE " + strings.Join(countParts, " AND ")
	}
	dataWhere := ""
	if len(dataParts) > 0 {
		dataWhere = "WHERE " + strings.Join(dataParts, " AND ")
	}

	var total int64
	if err := s.pool.QueryRow(ctx,
		"SELECT count(*) FROM recommendations.storefront_catalog sc LEFT JOIN recommendations.historical_item_offers hio ON hio.offer_id = sc.offer_id AND hio.is_selected "+countWhere,
		countArgs...,
	).Scan(&total); err != nil {
		return nil, 0, err
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
			EXISTS (SELECT 1 FROM recommendations.recommendation_items ri WHERE ri.source_item_id = COALESCE(hio.item_id, sc.offer_id)) AS has_history,
			sc.url
		FROM recommendations.storefront_catalog sc
		LEFT JOIN recommendations.historical_item_offers hio
			ON hio.offer_id = sc.offer_id AND hio.is_selected
		`+dataWhere+`
		ORDER BY sc.available DESC, item_id COLLATE "C" ASC
		LIMIT $1 OFFSET $2
	`, dataArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]CatalogProduct, 0, limit)
	for rows.Next() {
		var p CatalogProduct
		var itemID, offerID string
		var urlStr *string
		if err = rows.Scan(&itemID, &offerID, &p.SKU, &p.Title, &p.Category, &p.Vendor,
			&p.Price, &p.CurrencyCode, &p.Available, &p.HasHistory, &urlStr); err != nil {
			return nil, 0, err
		}
		p.ItemID = domain.ItemID(itemID)
		p.OfferID = &offerID
		p.URL = urlStr
		out = append(out, p)
	}
	return out, total, rows.Err()
}
