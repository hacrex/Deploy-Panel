package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/OWNER/deploypanel/internal/core/iam"
)

// loginRequest matches api/openapi.yaml LoginRequest.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TOTP     string `json:"totp,omitempty"`
}

// meResponse matches api/openapi.yaml Me.
type meResponse struct {
	ID           string             `json:"id"`
	Email        string             `json:"email"`
	Memberships  []membership       `json:"memberships"`
	PasswordHash string             `json:"-"` // never serialized
}

type membership struct {
	TeamID string `json:"team_id"`
	Role   string `json:"role"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": "0.0.0-dev"})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "unable to parse body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "email and password are required")
		return
	}

	ip := clientIP(r)
	ses, plainToken, err := s.svc.Login(r.Context(), req.Email, req.Password, ip, r.UserAgent())
	if err != nil {
		switch {
		case errors.Is(err, iam.ErrInvalidCredentials):
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		case errors.Is(err, iam.ErrUserDisabled):
			writeError(w, http.StatusForbidden, "user_disabled", "account is disabled")
		default:
			writeError(w, http.StatusInternalServerError, "login_failed", err.Error())
		}
		return
	}

	expires := ses.ExpiresAt
	setSessionCookie(w, s.cfg, plainToken, expires)

	u, ms, err := s.svc.Me(r.Context(), ses)
	if err != nil {
		// Session was created but we can't load the user — log out and error.
		s.svc.Logout(r.Context(), ses.TokenHash)
		clearSessionCookie(w, s.cfg.SessionCookieName)
		writeError(w, http.StatusInternalServerError, "login_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, meResponse{
		ID:         u.ID,
		Email:      u.Email,
		Memberships: memberships(ms),
	})
}

func memberships(ms []iam.Membership) []membership {
	out := make([]membership, 0, len(ms))
	for _, m := range ms {
		out = append(out, membership{TeamID: m.TeamID, Role: m.Role})
	}
	return out
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	tokenHash, kind := s.extractToken(r)
	if tokenHash == "" || kind != "session" {
		writeError(w, http.StatusUnauthorized, "authentication_required", "no session to log out")
		return
	}
	if err := s.svc.Logout(r.Context(), tokenHash); err != nil {
		writeError(w, http.StatusInternalServerError, "logout_failed", err.Error())
		return
	}
	clearSessionCookie(w, s.cfg.SessionCookieName)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u := UserFromCtx(r)
	ms := MembershipsFromCtx(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "authentication_required", "no user on request")
		return
	}
	writeJSON(w, http.StatusOK, meResponse{
		ID:           u.ID,
		Email:        u.Email,
		Memberships:  memberships(ms),
	})
}
