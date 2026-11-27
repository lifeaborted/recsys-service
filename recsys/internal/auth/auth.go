package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"recsys/internal/domain"
)

var (
	ErrInvalidInput       = errors.New("invalid auth input")
	ErrLoginExists        = errors.New("login already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthorized       = errors.New("missing, invalid, or expired session")
)

// Profile represents an isolated personal-account (ЛК) profile bound to a stand client_id.
type Profile struct {
	Role     string             `json:"role"`
	ID       string             `json:"profile_id"`
	ClientID domain.ClientID    `json:"client_id"`
	Group    domain.ClientGroup `json:"client_group"`
	Industry *string            `json:"industry,omitempty"`
	Login    string             `json:"login,omitempty"`
}

// AccountRecord represents stored credentials and stand profile linkage.
type AccountRecord struct {
	AccountID    int64
	Login        string
	PasswordHash string
	Profile      Profile
	CreatedAt    time.Time
}

// SessionRecord represents a stored session looked up by SHA-256 token hash.
type SessionRecord struct {
	TokenHash []byte
	AccountID int64
	Profile   Profile
	CreatedAt time.Time
	ExpiresAt time.Time
}

// Session is returned to callers upon registration or login.
type Session struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// CreateAccountParams holds normalized inputs for creating a stand account and profile.
type CreateAccountParams struct {
	Login        string
	PasswordHash string
	ProfileID    string
	ClientID     domain.ClientID
	ClientGroup  domain.ClientGroup
	Industry     *string
	CreatedAt    time.Time
}

// Repository abstracts persistence of stand accounts and sessions (PostgreSQL or in-memory test fake).
type Repository interface {
	CreateAccountWithProfile(ctx context.Context, params CreateAccountParams) (AccountRecord, error)
	AccountByLogin(ctx context.Context, login string) (AccountRecord, error)
	CreateSession(ctx context.Context, tokenHash []byte, accountID int64, createdAt, expiresAt time.Time) error
	SessionByTokenHash(ctx context.Context, tokenHash []byte) (SessionRecord, error)
}

// RegisterInput holds user-supplied registration fields.
type RegisterInput struct {
	Login       string
	Password    string
	ClientGroup domain.ClientGroup
	Industry    *string
}

// Option configures an auth Service.
type Option func(*Service)

// WithClock overrides the time source (used for deterministic expiry testing).
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// WithSessionTTL configures issued session token lifetime.
func WithSessionTTL(ttl time.Duration) Option {
	return func(s *Service) {
		if ttl > 0 {
			s.ttl = ttl
		}
	}
}

// WithBcryptCost sets the adaptive bcrypt work factor.
func WithBcryptCost(cost int) Option {
	return func(s *Service) {
		if cost >= bcrypt.MinCost && cost <= bcrypt.MaxCost {
			s.bcryptCost = cost
		}
	}
}

// Service manages local stand accounts, adaptive password hashes, and expiring Bearer sessions.
type Service struct {
	repo       Repository
	now        func() time.Time
	ttl        time.Duration
	bcryptCost int
}

func NewService(repo Repository, opts ...Option) *Service {
	svc := &Service{
		repo:       repo,
		now:        func() time.Time { return time.Now().UTC() },
		ttl:        24 * time.Hour,
		bcryptCost: bcrypt.DefaultCost,
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// HashPassword hashes a plaintext password with bcrypt.
func (s *Service) HashPassword(password string) (string, error) {
	if password == "" {
		return "", fmt.Errorf("%w: password is required", ErrInvalidInput)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	return string(hash), nil
}

// Register creates a new account linked to a brand-new stand profile and issues an initial session.
func (s *Service) Register(ctx context.Context, in RegisterInput) (Profile, Session, error) {
	login := normalizeLogin(in.Login)
	if login == "" || len(login) > 128 {
		return Profile{}, Session{}, fmt.Errorf("%w: invalid login", ErrInvalidInput)
	}
	if in.Password == "" {
		return Profile{}, Session{}, fmt.Errorf("%w: empty password", ErrInvalidInput)
	}
	if in.ClientGroup != domain.B2B && in.ClientGroup != domain.B2C {
		return Profile{}, Session{}, fmt.Errorf("%w: client_group must be B2B or B2C", ErrInvalidInput)
	}

	var industry *string
	if in.Industry != nil {
		trimmed := strings.TrimSpace(*in.Industry)
		if trimmed != "" {
			industry = &trimmed
		}
	}

	hash, err := s.HashPassword(in.Password)
	if err != nil {
		return Profile{}, Session{}, err
	}

	suffix, err := randomHex(8)
	if err != nil {
		return Profile{}, Session{}, err
	}
	profileID := "prof-" + suffix
	clientID := domain.ClientID("STAND-" + strings.ToUpper(suffix))
	now := s.now().UTC()

	rec, err := s.repo.CreateAccountWithProfile(ctx, CreateAccountParams{
		Login:        login,
		PasswordHash: hash,
		ProfileID:    profileID,
		ClientID:     clientID,
		ClientGroup:  in.ClientGroup,
		Industry:     industry,
		CreatedAt:    now,
	})
	if err != nil {
		return Profile{}, Session{}, err
	}

	sess, err := s.issueSession(ctx, rec.AccountID, now)
	if err != nil {
		return Profile{}, Session{}, err
	}
	return rec.Profile, sess, nil
}

// Login verifies credentials against the adaptive hash and issues a fresh expiring session.
func (s *Service) Login(ctx context.Context, rawLogin, password string) (Profile, Session, error) {
	login := normalizeLogin(rawLogin)
	if login == "" || password == "" {
		return Profile{}, Session{}, ErrInvalidCredentials
	}
	rec, err := s.repo.AccountByLogin(ctx, login)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return Profile{}, Session{}, ErrInvalidCredentials
		}
		return Profile{}, Session{}, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(rec.PasswordHash), []byte(password)); err != nil {
		return Profile{}, Session{}, ErrInvalidCredentials
	}
	sess, err := s.issueSession(ctx, rec.AccountID, s.now().UTC())
	if err != nil {
		return Profile{}, Session{}, err
	}
	return rec.Profile, sess, nil
}

// Authenticate validates a raw Bearer token and checks its expiration timestamp.
func (s *Service) Authenticate(ctx context.Context, rawToken string) (Profile, error) {
	token := strings.TrimSpace(rawToken)
	if token == "" {
		return Profile{}, ErrUnauthorized
	}
	sum := HashToken(token)
	rec, err := s.repo.SessionByTokenHash(ctx, sum)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return Profile{}, ErrUnauthorized
		}
		return Profile{}, err
	}
	if !s.now().UTC().Before(rec.ExpiresAt.UTC()) {
		return Profile{}, ErrUnauthorized
	}
	return rec.Profile, nil
}

func (s *Service) issueSession(ctx context.Context, accountID int64, now time.Time) (Session, error) {
	raw, err := randomHex(32)
	if err != nil {
		return Session{}, err
	}
	token := "rs_" + raw
	expiresAt := now.Add(s.ttl)
	if err := s.repo.CreateSession(ctx, HashToken(token), accountID, now, expiresAt); err != nil {
		return Session{}, err
	}
	return Session{
		AccessToken: token,
		ExpiresAt:   expiresAt,
	}, nil
}

// HashToken computes the SHA-256 digest stored in the database for a raw Bearer token.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func normalizeLogin(login string) string {
	return strings.ToLower(strings.TrimSpace(login))
}

func randomHex(numBytes int) (string, error) {
	buf := make([]byte, numBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

type profileContextKey struct{}

// WithProfile stores the authenticated Profile in ctx.
func WithProfile(ctx context.Context, profile Profile) context.Context {
	return context.WithValue(ctx, profileContextKey{}, profile)
}

// Current extracts the authenticated Profile from ctx.
func Current(ctx context.Context) (Profile, bool) {
	if ctx == nil {
		return Profile{}, false
	}
	p, ok := ctx.Value(profileContextKey{}).(Profile)
	return p, ok
}

// MemoryRepository is a thread-safe in-memory Repository for unit and httptest suites without Docker.
type MemoryRepository struct {
	mu       sync.RWMutex
	nextID   int64
	byLogin  map[string]AccountRecord
	byID     map[int64]AccountRecord
	sessions map[string]SessionRecord
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		nextID:   1,
		byLogin:  make(map[string]AccountRecord),
		byID:     make(map[int64]AccountRecord),
		sessions: make(map[string]SessionRecord),
	}
}

func (m *MemoryRepository) CreateAccountWithProfile(_ context.Context, p CreateAccountParams) (AccountRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.byLogin[p.Login]; exists {
		return AccountRecord{}, ErrLoginExists
	}
	id := m.nextID
	m.nextID++
	rec := AccountRecord{
		AccountID:    id,
		Login:        p.Login,
		PasswordHash: p.PasswordHash,
		Profile: Profile{
			ID:       p.ProfileID,
			ClientID: p.ClientID,
			Group:    p.ClientGroup,
			Industry: p.Industry,
			Login:    p.Login,
			Role:     "unverified",
		},
		CreatedAt: p.CreatedAt,
	}
	m.byLogin[p.Login] = rec
	m.byID[id] = rec
	return rec, nil
}

func (m *MemoryRepository) AccountByLogin(_ context.Context, login string) (AccountRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec, ok := m.byLogin[login]
	if !ok {
		return AccountRecord{}, ErrInvalidCredentials
	}
	return rec, nil
}

func (m *MemoryRepository) CreateSession(_ context.Context, tokenHash []byte, accountID int64, createdAt, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.byID[accountID]
	if !ok {
		return ErrInvalidCredentials
	}
	key := hex.EncodeToString(tokenHash)
	m.sessions[key] = SessionRecord{
		TokenHash: append([]byte(nil), tokenHash...),
		AccountID: accountID,
		Profile:   acc.Profile,
		CreatedAt: createdAt,
		ExpiresAt: expiresAt,
	}
	return nil
}

func (m *MemoryRepository) SessionByTokenHash(_ context.Context, tokenHash []byte) (SessionRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec, ok := m.sessions[hex.EncodeToString(tokenHash)]
	if !ok {
		return SessionRecord{}, ErrUnauthorized
	}
	return rec, nil
}
