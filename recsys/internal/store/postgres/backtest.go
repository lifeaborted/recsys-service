package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"recsys/internal/eval"
)

// EvaluationV4 reads the release marker and the sales from the same database snapshot.
func (s *Store) EvaluationV4(ctx context.Context) (string, []eval.Sale, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback()

	var sha string
	err = tx.QueryRowContext(ctx, `SELECT workbook_sha256 FROM recommendations.dataset_release WHERE version = 'v4'`).Scan(&sha)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, fmt.Errorf("в базе отсутствует релиз v4")
	}
	if err != nil {
		return "", nil, fmt.Errorf("проверить релиз v4: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT s.order_id, s.client_id, to_char(s.order_week, 'YYYY-MM-DD'), s.item_id, c.client_group
		FROM recommendations.sales s
		JOIN recommendations.clients c USING (client_id)
	`)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	var sales []eval.Sale
	products := make(map[string]struct{})
	for rows.Next() {
		var sale eval.Sale
		if err := rows.Scan(&sale.OrderID, &sale.ClientID, &sale.Week, &sale.ItemID, &sale.Group); err != nil {
			return "", nil, err
		}
		sales = append(sales, sale)
		products[sale.ItemID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	if len(sales) != 31753 || len(products) != 817 {
		return "", nil, fmt.Errorf("неполный или изменённый релиз v4: %d строк, %d товаров", len(sales), len(products))
	}
	return sha, sales, tx.Commit()
}
