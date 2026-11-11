// Package api serves the DeployPanel control plane HTTP API. It is transport
// logic only: routing, middleware, request decoding and response encoding.
// Business logic lives in internal/core/iam. See docs/API_CONVENTIONS.md.
package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/OWNER/deploypanel/internal/config"
	"github.com/OWNER/deploypanel/internal/core/iam"
)

// authService is the IAM surface the API handlers need. iam.Service satisfies it.
type authService interface {
	Login(ctx context.Context, email, password, ip, userAgent string) (*iam.Session, string, error)
	Logout(ctx context.Context, tokenHash string) error
	ValidateSession(ctx context.Context, tokenHash string) (*iam.Session, error)
	Me(ctx context.Context, ses *iam.Session) (*iam.User, []iam.Membership, error)
	GetUserByID(ctx context.Context, id string) (*iam.User, error)
	GetMemberships(ctx context.Context, userID string) ([]iam.Membership, error)
	ValidateAPIToken(ctx context.Context, tokenHash string) (*iam.APIToken, error)
}

// Server wires auth routes with middleware. It returns an http.Handler that
// cmd/dpd mounts on its mux.
type Server struct {
	mux *http.ServeMux
	svc authService
	cfg *config.Config
}

// New builds the API server. It does not bind a listener; cmd/dpd does that.
func New(svc authService, cfg *config.Config) *Server {
	s := &Server{svc: svc, cfg: cfg, mux: http.NewServeMux()}
	s.routes()
	return s
}

// Handler returns the http.Handler (with middleware applied).
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	// Public (no auth)
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("POST /v1/auth/login", s.handleLogin)
	// Auth required
	s.mux.HandleFunc("POST /v1/auth/logout", s.requireAuth(s.handleLogout))
	s.mux.HandleFunc("GET /v1/me", s.requireAuth(s.handleMe))
}

// ctxKey is a private type for context values.
type ctxKey int

const (
	userCtxKey ctxKey = iota
	sessionCtxKey
)

// UserFromCtx returns the authenticated user, or nil if none.
func UserFromCtx(r *http.Request) *iam.User {
	u, _ := r.Context().Value(userCtxKey).(*iam.User)
	return u
}

// SessionFromCtx returns the authenticated session, or nil.
func SessionFromCtx(r *http.Request) *iam.Session {
	ses, _ := r.Context().Value(sessionCtxKey).(*iam.Session)
	return ses
}

// ---- middleware -----------------------------------------------------------

// requireAuth wraps a handler so it only runs for an authenticated caller.
// It accepts either the dp_session cookie or a Bearer API token.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenHash, kind := s.extractToken(r)
		if tokenHash == "" {
			writeError(w, http.StatusUnauthorized, "authentication_required", "log in or provide an API token")
			return
		}

		switch kind {
		case "session":
			ses, err := s.svc.ValidateSession(r.Context(), tokenHash)
			if err != nil {
				clearSessionCookie(w, s.cfg.SessionCookieName)
				if errors.Is(err, iam.ErrSessionExpired) || errors.Is(err, iam.ErrSessionRevoked) {
					writeError(w, http.StatusUnauthorized, "session_invalid", err.Error())
				} else {
					writeError(w, http.StatusUnauthorized, "authentication_required", "invalid session")
				}
				return
			}
			u, ms, err := s.svc.Me(r.Context(), ses)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "authentication_required", err.Error())
				return
			}
			r = r.WithContext(context.WithValue(context.WithValue(r.Context(), userCtxKey, u), sessionCtxKey, ses))
			// attach memberships for route handlers that need them
			r = r.WithContext(context.WithValue(r.Context(), membershipsCtxKey, ms))
			next(w, r)
		case "bearer":
			t, err := s.svc.ValidateAPIToken(r.Context(), tokenHash)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "authentication_required", "invalid api token")
				return
			}
			u, err := s.svc.GetUserByID(r.Context(), t.UserID)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "authentication_required", "token user not found")
				return
			}
			if u.Disabled {
				writeError(w, http.StatusUnauthorized, "authentication_required", "user disabled")
				return
			}
			ms, err := s.svc.GetMemberships(r.Context(), u.ID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "authentication_required", "could not load memberships")
				return
			}
			r = r.WithContext(context.WithValue(context.WithValue(r.Context(), userCtxKey, u), tokenCtxKey, t))
			r = r.WithContext(context.WithValue(r.Context(), membershipsCtxKey, ms))
			next(w, r)
		}
	}
}

// extractToken returns the token hash to validate and its kind ("session" or "bearer").
// Cookie takes precedence when both are present.
func (s *Server) extractToken(r *http.Request) (string, string) {
	cookie, err := r.Cookie(s.cfg.SessionCookieName)
	if err == nil && cookie.Value != "" {
		return hashToken(cookie.Value), "session"
	}
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return hashToken(strings.TrimPrefix(auth, "Bearer ")), "bearer"
	}
	return "", ""
}

// hashToken reuses iam's SHA-256 so cookie and bearer tokens are compared against
// the stored token_hash. (iam.hashToken is unexported; duplicate here.)
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// membershipsCtxKey carries the caller's memberships on the request context.
type membershipsCtxKeyT int

const membershipsCtxKey membershipsCtxKeyT = iota

// MembershipsFromCtx returns the caller's team memberships, or nil.
func MembershipsFromCtx(r *http.Request) []iam.Membership {
	ms, _ := r.Context().Value(membershipsCtxKey).([]iam.Membership)
	return ms
}

// tokenCtxKey carries the validated API token.
type tokenCtxKeyT int

const tokenCtxKey tokenCtxKeyT = 100

// APITokenFromCtx returns the validated API token, or nil.
func APITokenFromCtx(r *http.Request) *iam.APIToken {
	t, _ := r.Context().Value(tokenCtxKey).(*iam.APIToken)
	return t
}

// ---- response helpers -----------------------------------------------------

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, code int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	return json.NewEncoder(w).Encode(v)
}

// writeError writes the standard Error envelope.
func writeError(w http.ResponseWriter, code int, codeID, message string) {
	writeJSON(w, code, map[string]any{
		"code":    codeID,
		"message": message,
	})
}

// setSessionCookie issues the dp_session cookie with the plain token.
func setSessionCookie(w http.ResponseWriter, cfg *config.Config, token string, expires time.Time) {
	maxAge := int(time.Until(expires).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	secure := !cfg.SessionInsecure
	cookie := http.Cookie{
		Name:     cfg.SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
		Expires:  expires,
	}
	http.SetCookie(w, &cookie)
}

// clearSessionCookie removes the dp_session cookie.
func clearSessionCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

// clientIP returns the request's IP without port.
func clientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
