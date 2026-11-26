package auth_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"recsys/internal/auth"
	"recsys/internal/domain"
)

func TestAuthRegisterCreatesDistinctProfilesHashesPasswordAndExpiresTokens(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 26, 16, 0, 0, 0, time.UTC)
	repo := auth.NewMemoryRepository()
	svc := auth.NewService(repo,
		auth.WithClock(func() time.Time { return now }),
		auth.WithSessionTTL(30*time.Minute),
		auth.WithBcryptCost(bcrypt.MinCost),
	)

	industry := "IT"
	prof1, sess1, err := svc.Register(ctx, auth.RegisterInput{
		Login:       "alice",
		Password:    "correct-horse-battery",
		ClientGroup: domain.B2B,
		Industry:    &industry,
	})
	if err != nil {
		t.Fatalf("register alice: %v", err)
	}
	prof2, _, err := svc.Register(ctx, auth.RegisterInput{
		Login:       "bob",
		Password:    "staple-99-hunter",
		ClientGroup: domain.B2C,
	})
	if err != nil {
		t.Fatalf("register bob: %v", err)
	}

	if prof1.ID == "" || prof2.ID == "" || prof1.ID == prof2.ID {
		t.Fatalf("expected distinct non-empty profile IDs, got %q and %q", prof1.ID, prof2.ID)
	}
	if prof1.ClientID == "" || prof2.ClientID == "" || prof1.ClientID == prof2.ClientID {
		t.Fatalf("expected distinct stand ClientIDs, got %q and %q", prof1.ClientID, prof2.ClientID)
	}
	if strings.HasPrefix(string(prof1.ClientID), "C0") {
		t.Fatalf("stand account must not attach to anonymized historical client_id: %q", prof1.ClientID)
	}

	stored, err := repo.AccountByLogin(ctx, "alice")
	if err != nil {
		t.Fatalf("AccountByLogin: %v", err)
	}
	if stored.PasswordHash == "correct-horse-battery" || !strings.HasPrefix(stored.PasswordHash, "$2") {
		t.Fatalf("expected bcrypt adaptive hash, got %q", stored.PasswordHash)
	}

	if _, _, err := svc.Register(ctx, auth.RegisterInput{
		Login:       "alice",
		Password:    "another",
		ClientGroup: domain.B2B,
	}); !errors.Is(err, auth.ErrLoginExists) {
		t.Fatalf("duplicate login error = %v, want ErrLoginExists", err)
	}

	if _, _, err := svc.Login(ctx, "alice", "wrong-password"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("wrong password error = %v, want ErrInvalidCredentials", err)
	}

	loginProf, loginSess, err := svc.Login(ctx, "alice", "correct-horse-battery")
	if err != nil {
		t.Fatalf("login alice: %v", err)
	}
	if loginProf.ID != prof1.ID || loginSess.AccessToken == "" || loginSess.AccessToken == sess1.AccessToken {
		t.Fatalf("unexpected login session: prof=%+v sess=%+v", loginProf, loginSess)
	}

	authed, err := svc.Authenticate(ctx, loginSess.AccessToken)
	if err != nil || authed.ID != prof1.ID {
		t.Fatalf("Authenticate before expiry: prof=%+v err=%v", authed, err)
	}

	// Advance clock past TTL -> token must be rejected.
	now = now.Add(31 * time.Minute)
	if _, err := svc.Authenticate(ctx, loginSess.AccessToken); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("Authenticate after expiry error = %v, want ErrUnauthorized", err)
	}
}
