package iam

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Store is the persistence port used by the IAM service. Implementations run
// SQL against the control plane database. Core never imports mod/*, so this
// stays in core and is the only place auth SQL lives.
type Store interface {
	CreateUser(ctx context.Context, u *User) error
	GetUserByID(ctx context.Context, id string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	UpdatePasswordHash(ctx context.Context, id, hash string) error

	CreateSession(ctx context.Context, s *Session) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error)
	UpdateSessionLastSeen(ctx context.Context, id string) error
	RevokeSession(ctx context.Context, id string) error
	RevokeSessionByTokenHash(ctx context.Context, tokenHash string) error
	RevokeAllUserSessions(ctx context.Context, userID string) error

	CreateAPIToken(ctx context.Context, t *APIToken) error
	GetAPITokenByHash(ctx context.Context, tokenHash string) (*APIToken, error)
	RevokeAPIToken(ctx context.Context, id string) error
	UpdateAPITokenLastUsed(ctx context.Context, id string) error
	GetMemberships(ctx context.Context, userID string) ([]Membership, error)
}

// Membership links a user to a team with a role.
type Membership struct {
	TeamID string
	Role   string // owner | admin | developer | viewer | billing
}

// DBStore implements Store over *sql.DB.
type DBStore struct{ db *sql.DB }

func NewDBStore(db *sql.DB) *DBStore { return &DBStore{db: db} }

// User domain model.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	TOTPSecret   string // encrypted
	Disabled     bool
	CreatedAt    time.Time
}

// Session is a login session. token_hash is the hash of the cookie value.
type Session struct {
	ID          string
	UserID      string
	TokenHash   string
	MFAVerified bool
	IP          string
	UserAgent   string
	CreatedAt   time.Time
	LastSeenAt  time.Time
	ExpiresAt   time.Time
	RevokedAt   *time.Time
}

// APIToken is a bearer token scoped to a user and optional team.
type APIToken struct {
	ID         string
	UserID     string
	TeamID     string
	Name       string
	TokenHash  string
	Scopes     string // JSON array
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

func (s *DBStore) CreateUser(ctx context.Context, u *User) error {
	if u.ID == "" {
		u.ID = newULID()
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, totp_secret, disabled, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		u.ID, u.Email, u.PasswordHash, u.TOTPSecret, boolToInt(u.Disabled), u.CreatedAt.Format(time.RFC3339))
	return err
}

func (s *DBStore) GetUserByID(ctx context.Context, id string) (*User, error) {
	return s.getUser(ctx, `SELECT id, email, password_hash, totp_secret, disabled, created_at FROM users WHERE id=?`, id)
}

func (s *DBStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return s.getUser(ctx, `SELECT id, email, password_hash, totp_secret, disabled, created_at FROM users WHERE email=?`, email)
}

func (s *DBStore) getUser(ctx context.Context, query string, arg any) (*User, error) {
	var u User
	var disabled int
	err := s.db.QueryRowContext(ctx, query, arg).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.TOTPSecret, &disabled, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	u.Disabled = disabled != 0
	return &u, nil
}

func (s *DBStore) UpdatePasswordHash(ctx context.Context, id, hash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash=? WHERE id=?`, hash, id)
	return err
}

func (s *DBStore) CreateSession(ctx context.Context, ses *Session) error {
	if ses.ID == "" {
		ses.ID = newULID()
	}
	now := time.Now().UTC()
	if ses.CreatedAt.IsZero() {
		ses.CreatedAt = now
	}
	if ses.LastSeenAt.IsZero() {
		ses.LastSeenAt = now
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, token_hash, mfa_verified, ip, user_agent, created_at, last_seen_at, expires_at, revoked_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		ses.ID, ses.UserID, ses.TokenHash, boolToInt(ses.MFAVerified), ses.IP, ses.UserAgent,
		ses.CreatedAt.Format(time.RFC3339), ses.LastSeenAt.Format(time.RFC3339), ses.ExpiresAt.Format(time.RFC3339))
	return err
}

func (s *DBStore) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error) {
	return s.getSession(ctx, `SELECT id, user_id, token_hash, mfa_verified, ip, user_agent, created_at, last_seen_at, expires_at, revoked_at FROM sessions WHERE token_hash=?`, tokenHash)
}

func (s *DBStore) getSession(ctx context.Context, query string, arg any) (*Session, error) {
	var ses Session
	var mfa int64
	var revokedAt sql.NullString
	err := s.db.QueryRowContext(ctx, query, arg).Scan(
		&ses.ID, &ses.UserID, &ses.TokenHash, &mfa, &ses.IP, &ses.UserAgent,
		&ses.CreatedAt, &ses.LastSeenAt, &ses.ExpiresAt, &revokedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	ses.MFAVerified = mfa != 0
	if revokedAt.Valid {
		t, perr := time.Parse(time.RFC3339, revokedAt.String)
		if perr != nil {
			return nil, perr
		}
		ses.RevokedAt = &t
	}
	return &ses, nil
}

func (s *DBStore) UpdateSessionLastSeen(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

func (s *DBStore) RevokeSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET revoked_at=? WHERE id=? AND revoked_at IS NULL`, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

func (s *DBStore) RevokeSessionByTokenHash(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET revoked_at=? WHERE token_hash=? AND revoked_at IS NULL`, time.Now().UTC().Format(time.RFC3339), tokenHash)
	return err
}

func (s *DBStore) RevokeAllUserSessions(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL`, time.Now().UTC().Format(time.RFC3339), userID)
	return err
}

func (s *DBStore) CreateAPIToken(ctx context.Context, t *APIToken) error {
	if t.ID == "" {
		t.ID = newULID()
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO api_tokens (id, user_id, team_id, name, token_hash, scopes, expires_at, last_used_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?)`,
		t.ID, t.UserID, t.TeamID, t.Name, t.TokenHash, t.Scopes, expiresAtStr(t.ExpiresAt), t.CreatedAt.Format(time.RFC3339))
	return err
}

func (s *DBStore) GetAPITokenByHash(ctx context.Context, tokenHash string) (*APIToken, error) {
	var t APIToken
	var expiresAt sql.NullString
	var lastUsedAt sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, team_id, name, token_hash, scopes, expires_at, last_used_at, created_at FROM api_tokens WHERE token_hash=?`,
		tokenHash).Scan(&t.ID, &t.UserID, &t.TeamID, &t.Name, &t.TokenHash, &t.Scopes, &expiresAt, &lastUsedAt, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAPITokenNotFound
		}
		return nil, err
	}
	if expiresAt.Valid {
		tm, perr := time.Parse(time.RFC3339, expiresAt.String)
		if perr != nil {
			return nil, perr
		}
		t.ExpiresAt = &tm
	}
	if lastUsedAt.Valid {
		tm, perr := time.Parse(time.RFC3339, lastUsedAt.String)
		if perr != nil {
			return nil, perr
		}
		t.LastUsedAt = &tm
	}
	return &t, nil
}

func (s *DBStore) RevokeAPIToken(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM api_tokens WHERE id=?`, id)
	return err
}

func (s *DBStore) UpdateAPITokenLastUsed(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE api_tokens SET last_used_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

func (s *DBStore) GetMemberships(ctx context.Context, userID string) ([]Membership, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT team_id, role FROM memberships WHERE user_id=? ORDER BY team_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Membership
	for rows.Next() {
		var m Membership
		if err := rows.Scan(&m.TeamID, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func expiresAtStr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

var (
	// ErrUserNotFound is returned when no user matches the lookup.
	ErrUserNotFound = errors.New("user not found")
	// ErrSessionNotFound is returned when no session matches the token hash.
	ErrSessionNotFound = errors.New("session not found")
	// ErrAPITokenNotFound is returned when no API token matches the token hash.
	ErrAPITokenNotFound = errors.New("api token not found")
)

// newULID generates a sortable 26-char ULID ([0-9A-HJKMNP-TV-Z]) matching the
// API ID pattern: 48-bit timestamp (ms) + 80-bit randomness, Crockford base32.
func newULID() string {
	var ms uint64 = uint64(time.Now().UnixMilli())
	var b [16]byte
	for i := 5; i >= 0; i-- {
		b[i] = byte(ms)
		ms >>= 8
	}
	if _, err := rand.Read(b[6:]); err != nil {
		panic(fmt.Sprintf("ulid: cannot read randomness: %v", err))
	}
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	var out [26]byte
	var bitBuf uint32
	var bits int
	var oi int
	for _, v := range b {
		bitBuf = (bitBuf << 8) | uint32(v)
		bits += 8
		for bits >= 5 {
			bits -= 5
			out[oi] = alphabet[(bitBuf>>bits)&0x1F]
			oi++
		}
	}
	if bits > 0 {
		out[oi] = alphabet[(bitBuf<<(5-bits))&0x1F]
	}
	return string(out[:])
}
