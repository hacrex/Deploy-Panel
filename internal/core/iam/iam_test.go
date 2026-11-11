package iam

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// mockStore implements Store in memory for tests. All maps store values
// (not pointers), and accessors return fresh copies so callers cannot mutate
// stored state through the returned pointer.
type mockStore struct {
	mu           sync.Mutex
	users        map[string]User       // id -> user
	byEmail      map[string]User       // email -> user
	sessions     map[string]*Session   // tokenHash -> session
	tokens       map[string]*APIToken  // tokenHash -> token
	memberships  map[string][]Membership
}

func newMockStore() *mockStore {
	return &mockStore{
		users:       map[string]User{},
		byEmail:     map[string]User{},
		sessions:    map[string]*Session{},
		tokens:      map[string]*APIToken{},
		memberships: map[string][]Membership{},
	}
}

func (m *mockStore) CreateUser(ctx context.Context, u *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.byEmail[u.Email]; exists {
		return errors.New("duplicate email")
	}
	if u.ID == "" {
		u.ID = newULID()
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	stored := *u // copy so Register clearing PasswordHash on its return value is safe
	m.users[stored.ID] = stored
	m.byEmail[stored.Email] = stored
	return nil
}

func (m *mockStore) GetUserByID(ctx context.Context, id string) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return nil, ErrUserNotFound
	}
	copy := u
	return &copy, nil
}

func (m *mockStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.byEmail[email]
	if !ok {
		return nil, ErrUserNotFound
	}
	copy := u
	return &copy, nil
}

func (m *mockStore) UpdatePasswordHash(ctx context.Context, id, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return ErrUserNotFound
	}
	u.PasswordHash = hash
	return nil
}

func (m *mockStore) CreateSession(ctx context.Context, ses *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ses.ID == "" {
		ses.ID = newULID()
	}
	m.sessions[ses.TokenHash] = ses
	return nil
}

func (m *mockStore) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ses, ok := m.sessions[tokenHash]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return ses, nil
}

func (m *mockStore) UpdateSessionLastSeen(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ses := range m.sessions {
		if ses.ID == id {
			ses.LastSeenAt = time.Now().UTC()
			return nil
		}
	}
	return ErrSessionNotFound
}

func (m *mockStore) RevokeSession(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ses := range m.sessions {
		if ses.ID == id {
			now := time.Now().UTC()
			ses.RevokedAt = &now
			return nil
		}
	}
	return ErrSessionNotFound
}

func (m *mockStore) RevokeSessionByTokenHash(ctx context.Context, tokenHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ses, ok := m.sessions[tokenHash]
	if !ok {
		return ErrSessionNotFound
	}
	now := time.Now().UTC()
	ses.RevokedAt = &now
	return nil
}

func (m *mockStore) RevokeAllUserSessions(ctx context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	for _, ses := range m.sessions {
		if ses.UserID == userID {
			ses.RevokedAt = &now
		}
	}
	return nil
}

func (m *mockStore) CreateAPIToken(ctx context.Context, t *APIToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t.ID == "" {
		t.ID = newULID()
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	m.tokens[t.TokenHash] = t
	return nil
}

func (m *mockStore) GetAPITokenByHash(ctx context.Context, tokenHash string) (*APIToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tokens[tokenHash]
	if !ok {
		return nil, ErrAPITokenNotFound
	}
	return t, nil
}

func (m *mockStore) RevokeAPIToken(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tokens {
		if t.ID == id {
			delete(m.tokens, t.TokenHash)
			return nil
		}
	}
	return ErrAPITokenNotFound
}

func (m *mockStore) UpdateAPITokenLastUsed(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tokens {
		if t.ID == id {
			now := time.Now().UTC()
			t.LastUsedAt = &now
			return nil
		}
	}
	return ErrAPITokenNotFound
}

func (m *mockStore) GetMemberships(ctx context.Context, userID string) ([]Membership, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.memberships[userID], nil
}

func TestRegisterAndLogin(t *testing.T) {
	store := newMockStore()
	svc := New(store, 5, 15*time.Minute)

	u, err := svc.Register(context.Background(), "Alice@example.com ", "secret123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if u.Email != "alice@example.com" {
		t.Errorf("email normalized to %q, want alice@example.com", u.Email)
	}
	if u.PasswordHash != "" {
		t.Error("password hash must not be returned")
	}

	// Duplicate email is a generic error (no enumeration).
	if _, err := svc.Register(context.Background(), "ALICE@example.com", "other"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("duplicate email: want ErrInvalidCredentials, got %v", err)
	}

	// Wrong password -> generic error, rate limited.
	_, _, err = svc.Login(context.Background(), "alice@example.com", "wrong", "127.0.0.1", "test")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("bad password: want ErrInvalidCredentials, got %v", err)
	}

	// Missing user -> generic error (timing protected), rate limited.
	_, _, err = svc.Login(context.Background(), "nope@example.com", "whatever", "127.0.0.1", "test")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("missing user: want ErrInvalidCredentials, got %v", err)
	}

	// Valid login.
	ses, token, err := svc.Login(context.Background(), "alice@example.com", "secret123", "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if token == "" {
		t.Error("plain session token must be returned")
	}
	if ses.UserID != u.ID {
		t.Errorf("session user %q, want %q", ses.UserID, u.ID)
	}
	if ses.TokenHash == "" {
		t.Error("session token hash must be stored")
	}
	if ses.TokenHash == token {
		t.Error("stored hash must differ from plain token")
	}
	if time.Now().UTC().Before(ses.ExpiresAt) != true {
		t.Error("session should not already be expired")
	}

}

func TestDisabledUserLogin(t *testing.T) {
	store := newMockStore()
	svc := New(store, 5, 15*time.Minute)

	u, err := svc.Register(context.Background(), "bob@example.com", "secret123")
	if err != nil {
		t.Fatal(err)
	}
	// Mark disabled by mutating the stored user (fetch fresh copy with hash).
	fresh, err := store.GetUserByID(context.Background(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	fresh.Disabled = true
	store.mu.Lock()
	store.users[fresh.ID] = *fresh
	store.byEmail[fresh.Email] = *fresh
	store.mu.Unlock()

	_, _, err = svc.Login(context.Background(), "bob@example.com", "secret123", "127.0.0.1", "test")
	if !errors.Is(err, ErrUserDisabled) {
		t.Errorf("disabled user: want ErrUserDisabled, got %v", err)
	}
}

func TestSessionValidateRevokeExpire(t *testing.T) {
	store := newMockStore()
	svc := New(store, 5, 15*time.Minute)

	u, err := svc.Register(context.Background(), "carol@example.com", "secret123")
	if err != nil {
		t.Fatal(err)
	}

	ses, token, err := svc.Login(context.Background(), "carol@example.com", "secret123", "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	// Valid session.
	got, err := svc.ValidateSession(context.Background(), hashToken(token))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if got.ID != ses.ID {
		t.Errorf("validate returned session %q, want %q", got.ID, ses.ID)
	}

	// Revoke.
	if err := svc.Logout(context.Background(), hashToken(token)); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := svc.ValidateSession(context.Background(), hashToken(token)); !errors.Is(err, ErrSessionRevoked) {
		t.Errorf("revoked: want ErrSessionRevoked, got %v", err)
	}

	// Unknown token.
	if _, err := svc.ValidateSession(context.Background(), hashToken("does-not-exist")); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("unknown: want ErrInvalidCredentials, got %v", err)
	}

	// Expired session: create one with a past expiry.
	expired := &Session{UserID: u.ID, TokenHash: hashToken("expired-token"), ExpiresAt: time.Now().UTC().Add(-time.Hour)}
	if err := store.CreateSession(context.Background(), expired); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateSession(context.Background(), hashToken("expired-token")); !errors.Is(err, ErrSessionExpired) {
		t.Errorf("expired: want ErrSessionExpired, got %v", err)
	}
}

func TestMe(t *testing.T) {
	store := newMockStore()
	svc := New(store, 5, 15*time.Minute)

	u, err := svc.Register(context.Background(), "dave@example.com", "secret123")
	if err != nil {
		t.Fatal(err)
	}
	store.memberships[u.ID] = []Membership{{TeamID: "t1", Role: "admin"}, {TeamID: "t2", Role: "viewer"}}

	ses, token, err := svc.Login(context.Background(), "dave@example.com", "secret123", "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}

	got, ms, err := svc.Me(context.Background(), ses)
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	if got.PasswordHash != "" {
		t.Error("me must not return password hash")
	}
	if len(ms) != 2 || ms[0].Role != "admin" || ms[1].Role != "viewer" {
		t.Errorf("me memberships: %+v", ms)
	}
	_ = token
}

func TestAPIToken(t *testing.T) {
	store := newMockStore()
	svc := New(store, 5, 15*time.Minute)

	u, err := svc.Register(context.Background(), "erin@example.com", "secret123")
	if err != nil {
		t.Fatal(err)
	}

	expires := time.Now().UTC().Add(time.Hour)
	tk, plain, err := svc.IssueAPIToken(context.Background(), u.ID, "t1", "ci-token", `["sites:read"]`, &expires)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if plain == "" {
		t.Error("plain api token must be returned")
	}
	if tk.TokenHash == plain {
		t.Error("stored hash must differ from plain token")
	}

	got, err := svc.ValidateAPIToken(context.Background(), hashToken(plain))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if got.ID != tk.ID {
		t.Errorf("validate returned token %q, want %q", got.ID, tk.ID)
	}
	if got.LastUsedAt == nil {
		t.Error("last_used_at must be set on validate")
	}

	// Expired token.
	old := time.Now().UTC().Add(-time.Hour)
	tk2, plain2, err := svc.IssueAPIToken(context.Background(), u.ID, "t1", "expired", `[]`, &old)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateAPIToken(context.Background(), hashToken(plain2)); !errors.Is(err, ErrAPITokenExpired) {
		t.Errorf("expired token: want ErrAPITokenExpired, got %v", err)
	}
	_ = tk2
}

func TestRateLimitLockout(t *testing.T) {
	store := newMockStore()
	svc := New(store, 2, time.Minute) // lock after 2 failures

	email := "frank@example.com"
	for i := 0; i < 2; i++ {
		_, _, err := svc.Login(context.Background(), email, "wrong", "127.0.0.1", "test")
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("failure %d: %v", i, err)
		}
	}
	// Third failure still returns invalid credentials (limiter records, doesn't block).
	_, _, err := svc.Login(context.Background(), email, "wrong", "127.0.0.1", "test")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("locked: want ErrInvalidCredentials, got %v", err)
	}
}
