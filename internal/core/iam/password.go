// Package iam provides identity and access management: users, login sessions,
// API tokens and the RBAC checks that gate the control plane API. See docs/ARCHITECTURE.md
// section 5 (core/iam) and docs/API_CONVENTIONS.md.
package iam

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Cost is the bcrypt work factor. 10 is the OWASP-recommended minimum for
// interactive logins; raise it (11-14) on faster hardware if the latency
// budget allows. It is stored implicitly in the bcrypt output, so existing
// hashes keep verifying after the default changes.
const Cost = 10

// ErrPasswordMismatch is returned when the supplied password does not match the stored hash.
var ErrPasswordMismatch = errors.New("password does not match")

// Hasher creates and verifies bcrypt password hashes.
type Hasher struct {
	cost int
}

// NewHasher returns a Hasher using Cost by default.
func NewHasher(cost int) *Hasher {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		cost = Cost
	}
	return &Hasher{cost: cost}
}

// Generate hashes a password with bcrypt. The encoded string carries the cost,
// so stored hashes remain verifiable after the default cost changes.
func (h *Hasher) Generate(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", fmt.Errorf("bcrypt generate: %w", err)
	}
	return string(hash), nil
}

// Compare checks a password against an encoded bcrypt hash, returning nil on
// match and ErrPasswordMismatch otherwise. Comparison is constant-time.
func (h *Hasher) Compare(password, encoded string) error {
	err := bcrypt.CompareHashAndPassword([]byte(encoded), []byte(password))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return ErrPasswordMismatch
		}
		return err
	}
	return nil
}

// ---- login attempt rate limiting (in-memory, single-process) --------------

// attempt records a failed login for a (normalized email, ip) pair.
type attempt struct {
	count     int
	windowEnd time.Time
}

// Limiter enforces a max failed-login window per (email, ip). It is safe for
// concurrent use. On the first failure in a window it starts the lockout
// timer; once count reaches max, further attempts are rejected until the
// window ends. A successful login clears the record.
type Limiter struct {
	mu     sync.Mutex
	rec    map[string]attempt
	max    int
	window time.Duration
}

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{rec: make(map[string]attempt), max: max, window: window}
}

// key normalizes the email (lowercase, trimmed) and joins it with the IP.
func key(email, ip string) string {
	return strings.ToLower(strings.TrimSpace(email)) + "\x00" + ip
}

// Record registers a failed attempt and reports whether the pair is now locked out.
// It returns the remaining time in the current lockout window (0 if not locked).
func (l *Limiter) Record(email, ip string) (locked bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	k := key(email, ip)
	now := time.Now()
	a, ok := l.rec[k]
	if !ok || now.After(a.windowEnd) {
		a = attempt{count: 1, windowEnd: now.Add(l.window)}
	} else {
		a.count++
	}
	l.rec[k] = a
	if a.count >= l.max {
		return true, time.Until(a.windowEnd)
	}
	return false, 0
}

// Clear removes the failure record for a pair, called on successful login.
func (l *Limiter) Clear(email, ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.rec, key(email, ip))
}
