package iam

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Service is the IAM business logic. It is backed by a Store and a Hasher,
// with an in-memory login attempt limiter for rate limiting.
type Service struct {
	store   Store
	hasher  *Hasher
	limiter *Limiter

	sessionLifetime time.Duration // absolute session lifetime; default 24h
}

// New builds a Service over the given store, with argon2id password hashing,
// the given login rate limit, and a default 24h session lifetime.
func New(store Store, maxAttempts int, lockoutFor time.Duration) *Service {
	return &Service{
		store:           store,
		hasher:          NewHasher(Cost),
		limiter:         NewLimiter(maxAttempts, lockoutFor),
		sessionLifetime: 24 * time.Hour,
	}
}

// WithSessionLifetime sets the absolute session lifetime and returns the service for chaining.
func (s *Service) WithSessionLifetime(d time.Duration) *Service {
	s.sessionLifetime = d
	return s
}

// ---- errors ---------------------------------------------------------------

var (
	// ErrInvalidCredentials is returned for a bad email/password pair. It does
	// not distinguish between a missing user and a wrong password.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrUserDisabled is returned when the user account is disabled.
	ErrUserDisabled = errors.New("user is disabled")
	// ErrSessionExpired is returned when a session's expires_at has passed.
	ErrSessionExpired = errors.New("session expired")
	// ErrSessionRevoked is returned when a session has been revoked.
	ErrSessionRevoked = errors.New("session revoked")
	// ErrAPITokenExpired is returned when an API token has expired.
	ErrAPITokenExpired = errors.New("api token expired")
)

// dummyHash is an argon2id hash of a random password, used to keep the timing
// of a missing-user login attempt similar to a real one.
var dummyHash string

func init() {
	h, err := NewHasher(Cost).Generate(strings.Repeat("x", 64))
	if err != nil {
		panic(err)
	}
	dummyHash = h
}

// Register creates a new user with a hashed password. It fails with
// ErrInvalidCredentials if the email already exists (message hides which).
func (s *Service) Register(ctx context.Context, email, password string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return nil, ErrInvalidCredentials
	}
	if _, err := s.store.GetUserByEmail(ctx, email); err == nil {
		return nil, ErrInvalidCredentials // email taken: don't say which
	} else if !errors.Is(err, ErrUserNotFound) {
		return nil, err
	}
	hash, err := s.hasher.Generate(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	u := &User{Email: email, PasswordHash: hash}
	if err := s.store.CreateUser(ctx, u); err != nil {
		return nil, err
	}
	u.PasswordHash = "" // never return the hash
	return u, nil
}

// Login verifies email/password (with rate limiting and timing protection),
// creates a session, and returns the session plus the plain session token to
// set as the dp_session cookie. The plain token is only returned here.
func (s *Service) Login(ctx context.Context, email, password, ip, userAgent string) (*Session, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if ip == "" {
		ip = "0.0.0.0"
	}

	// Always run a full hash compare so a missing user takes similar time.
	user, lookupErr := s.store.GetUserByEmail(ctx, email)
	hashToCompare := dummyHash
	if lookupErr == nil {
		hashToCompare = user.PasswordHash
	}
	if err := s.hasher.Compare(password, hashToCompare); err != nil {
		s.limiter.Record(email, ip)
		return nil, "", ErrInvalidCredentials
	}
	if errors.Is(lookupErr, ErrUserNotFound) {
		// User didn't exist; we already spent the hash time. Record a failed
		// attempt to slow enumeration, then return the generic error.
		s.limiter.Record(email, ip)
		return nil, "", ErrInvalidCredentials
	}
	if lookupErr != nil {
		return nil, "", lookupErr
	}

	if user.Disabled {
		s.limiter.Record(email, ip)
		return nil, "", ErrUserDisabled
	}

	// Success: clear failure record and issue a session.
	s.limiter.Clear(email, ip)

	plainToken := newSessionToken()
	tokenHash := hashToken(plainToken)
	ses := &Session{
		UserID:      user.ID,
		TokenHash:   tokenHash,
		MFAVerified: false,
		IP:          ip,
		UserAgent:   userAgent,
		ExpiresAt:   time.Now().UTC().Add(s.sessionLifetime),
	}
	if err := s.store.CreateSession(ctx, ses); err != nil {
		return nil, "", err
	}
	return ses, plainToken, nil
}

// Logout revokes the session identified by tokenHash.
func (s *Service) Logout(ctx context.Context, tokenHash string) error {
	return s.store.RevokeSessionByTokenHash(ctx, tokenHash)
}

// ValidateSession looks up the session by token hash and checks expiry and
// revocation. On success it slides last_seen_at and returns the session.
func (s *Service) ValidateSession(ctx context.Context, tokenHash string) (*Session, error) {
	ses, err := s.store.GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, ErrInvalidCredentials // generic: don't say why
	}
	if ses.RevokedAt != nil {
		return nil, ErrSessionRevoked
	}
	if time.Now().UTC().After(ses.ExpiresAt) {
		return nil, ErrSessionExpired
	}
	if err := s.store.UpdateSessionLastSeen(ctx, ses.ID); err != nil {
		return nil, err
	}
	return ses, nil
}

// Me returns the user for a session plus their team memberships.
func (s *Service) Me(ctx context.Context, ses *Session) (*User, []Membership, error) {
	u, err := s.store.GetUserByID(ctx, ses.UserID)
	if err != nil {
		return nil, nil, err
	}
	if u.Disabled {
		return nil, nil, ErrUserDisabled
	}
	ms, err := s.store.GetMemberships(ctx, u.ID)
	if err != nil {
		return nil, nil, err
	}
	u.PasswordHash = ""
	return u, ms, nil
}

// IssueAPIToken creates a scoped API token and returns the plain token (shown
// once) plus the stored token record.
func (s *Service) IssueAPIToken(ctx context.Context, userID, teamID, name, scopes string, expiresAt *time.Time) (*APIToken, string, error) {
	plainToken := newSessionToken()
	tokenHash := hashToken(plainToken)
	t := &APIToken{
		UserID:     userID,
		TeamID:     teamID,
		Name:       name,
		TokenHash:  tokenHash,
		Scopes:     scopes,
		ExpiresAt:  expiresAt,
		CreatedAt:  time.Now().UTC(),
	}
	if err := s.store.CreateAPIToken(ctx, t); err != nil {
		return nil, "", err
	}
	return t, plainToken, nil
}

// GetUserByID returns a user by ID with the password hash cleared.
func (s *Service) GetUserByID(ctx context.Context, id string) (*User, error) {
	u, err := s.store.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	u.PasswordHash = ""
	return u, nil
}

// GetMemberships returns the caller's team memberships.
func (s *Service) GetMemberships(ctx context.Context, userID string) ([]Membership, error) {
	return s.store.GetMemberships(ctx, userID)
}

// ValidateAPIToken looks up a bearer token by hash and checks expiry.
func (s *Service) ValidateAPIToken(ctx context.Context, tokenHash string) (*APIToken, error) {
	t, err := s.store.GetAPITokenByHash(ctx, tokenHash)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if t.ExpiresAt != nil && time.Now().UTC().After(*t.ExpiresAt) {
		return nil, ErrAPITokenExpired
	}
	if err := s.store.UpdateAPITokenLastUsed(ctx, t.ID); err != nil {
		return nil, err
	}
	return t, nil
}

// ---- helpers --------------------------------------------------------------

// newSessionToken generates a high-entropy session/API token (64 hex chars).
func newSessionToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("session token: %v", err))
	}
	return hex.EncodeToString(b)
}

// hashToken hashes a high-entropy token with SHA-256 (tokens already have
// high entropy, so a fast hash is fine; passwords use argon2id).
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
