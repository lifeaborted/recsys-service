package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"recsys/internal/domain"
	"recsys/internal/eval"
)

type EvaluationRelease struct {
	Version       string
	SHA           string
	Sales         []eval.Sale
	ProductGroups map[domain.ItemID]string
}

func (s *Store) EvaluationV4(ctx context.Context) (string, []eval.Sale, error) {
	release, err := s.EvaluationDataset(ctx, "v4")
	return release.SHA, release.Sales, err
}

// EvaluationDataset reads release metadata and sales from the same database snapshot.
func (s *Store) EvaluationDataset(ctx context.Context, version string) (EvaluationRelease, error) {
	var release EvaluationRelease
	if version != "" && version != "v4" && version != "v5" {
		return release, fmt.Errorf("неподдерживаемый релиз %q", version)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return release, err
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		SELECT version, workbook_sha256 FROM recommendations.dataset_release
		WHERE version IN ('v4', 'v5') AND ($1 = '' OR version = $1)
		ORDER BY version DESC LIMIT 1
	`, version).Scan(&release.Version, &release.SHA)
	if errors.Is(err, pgx.ErrNoRows) {
		return release, fmt.Errorf("в базе отсутствует запрошенный закрытый релиз %q", version)
	}
	if err != nil {
		return release, fmt.Errorf("проверить закрытый релиз: %w", err)
	}

	const salesSQL = `
		SELECT s.order_id, s.client_id, to_char(s.order_week, 'YYYY-MM-DD'), s.item_id, COALESCE(c.client_group, ''), %s
		FROM recommendations.sales s
		JOIN recommendations.clients c USING (client_id)
	`
	groupColumn := "''::text"
	if release.Version == "v5" {
		groupColumn = "COALESCE(s.product_group, '')"
		release.ProductGroups = make(map[domain.ItemID]string)
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(salesSQL, groupColumn))
	if err != nil {
		return release, err
	}
	defer rows.Close()
	for rows.Next() {
		var sale eval.Sale
		var group string
		if err := rows.Scan(&sale.OrderID, &sale.ClientID, &sale.Week, &sale.ItemID, &sale.Group, &group); err != nil {
			return release, err
		}
		if release.Version == "v5" {
			id := domain.ItemID(sale.ItemID)
			prev, exists := release.ProductGroups[id]
			if group == "" || (exists && prev != group) {
				return release, fmt.Errorf("v5: missing or conflicting product_group for item %q", id)
			}
			release.ProductGroups[id] = group
		}
		release.Sales = append(release.Sales, sale)
	}
	if err := rows.Err(); err != nil {
		return release, err
	}
	return release, tx.Commit(ctx)
}
