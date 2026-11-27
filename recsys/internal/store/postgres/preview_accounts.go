package postgres

import (
	"context"
	"fmt"

	"recsys/internal/auth"
)

// ConfigurePreviewAccounts rotates the test credentials exclusively on a synthetic stand.
func (s *Store) ConfigurePreviewAccounts(ctx context.Context, userPassword, adminPassword string, bcryptCost int) error {
	if userPassword == "" || adminPassword == "" || userPassword == adminPassword {
		return fmt.Errorf("set different PREVIEW_ACCOUNT_PASSWORD and PREVIEW_ADMIN_PASSWORD")
	}
	passwords := auth.NewService(nil, auth.WithBcryptCost(bcryptCost))
	toSeed := []struct {
		login, password, profile, client, role, group string
	}{
		{"notver", userPassword, "preview-notver", "DEMO-NOTVER", "unverified", "B2C"},
		{"verb2b", userPassword, "preview-verb2b", "DEMO-VERB2B", "verified", "B2B"},
		{"verb2c", userPassword, "preview-verb2c", "DEMO-VERB2C", "verified", "B2C"},
		{"admin", adminPassword, "preview-admin", "DEMO-ADMIN", "admin", "B2C"},
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var synthetic bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM recommendations.model_snapshots WHERE snapshot_id='demo-v1')
    AND NOT EXISTS(SELECT 1 FROM recommendations.model_snapshots WHERE snapshot_id<>'demo-v1')
    AND NOT EXISTS(SELECT 1 FROM recommendations.sales)`).Scan(&synthetic); err != nil {
		return err
	}
	if !synthetic {
		return fmt.Errorf("preview accounts require a synthetic database with demo-v1")
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO recommendations.clients(excel_row,client_id,client_group,client_type,lk_access)
VALUES (nextval('recommendations.stand_client_excel_row_seq'),'DEMO-GUEST','B2C','ФЛ','да')
ON CONFLICT(client_id) DO NOTHING`); err != nil {
		return err
	}

	// Removing these accounts also invalidates sessions. Their clients and test history remain available.
	if _, err := tx.Exec(ctx, `
DELETE FROM recommendations.stand_accounts
WHERE (client_id='DEMO-BUYER' AND login IN ('demo-b2b','verb2b'))
   OR (client_id='DEMO-NEW' AND login IN ('demo-b2c','verb2c'))`); err != nil {
		return err
	}

	for _, account := range toSeed {
		hash, err := passwords.HashPassword(account.password)
		if err != nil {
			return err
		}
		clientType := "ФЛ"
		if account.group == "B2B" {
			clientType = "ЮЛ"
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO recommendations.clients(excel_row,client_id,client_group,client_type,lk_access)
VALUES (nextval('recommendations.stand_client_excel_row_seq'),$1,$2,$3,'да')
ON CONFLICT(client_id) DO UPDATE SET client_group=EXCLUDED.client_group, client_type=EXCLUDED.client_type
`, account.client, account.group, clientType); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO recommendations.account_profiles(client_id,company_name)
VALUES ($1,$2) ON CONFLICT(client_id) DO NOTHING`, account.client, "Профиль стенда "+account.login); err != nil {
			return err
		}

		var id int64
		if err := tx.QueryRow(ctx, `
INSERT INTO recommendations.stand_accounts(login,password_hash,profile_id,client_id,role)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT(client_id) DO UPDATE SET login=EXCLUDED.login,password_hash=EXCLUDED.password_hash,role=EXCLUDED.role
RETURNING account_id
`, account.login, hash, account.profile, account.client, account.role).Scan(&id); err != nil {
			return fmt.Errorf("configure preview profile %s: %w", account.login, err)
		}
		if _, err := tx.Exec(ctx, "DELETE FROM recommendations.stand_sessions WHERE account_id=$1", id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
