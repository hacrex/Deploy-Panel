package api

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/OWNER/deploypanel/internal/config"
	"github.com/OWNER/deploypanel/internal/core/iam"
)

// mockAuthService implements authService with controllable behavior.
type mockAuthService struct {
	loginSess        *iam.Session
	loginToken       string
	loginErr         error
	logoutErr        error
	validateSess     *iam.Session
	validateSessErr  error
	meUser           *iam.User
	meMemberships    []iam.Membership
	meErr            error
	validateToken    *iam.APIToken
	validateTokenErr error
	loginCalled      bool
	logoutCalled     bool
	lastLoginEmail   string
	lastLoginIP      string
}

func (m *mockAuthService) Login(ctx context.Context, email, password, ip, userAgent string) (*iam.Session, string, error) {
	m.loginCalled = true
	m.lastLoginEmail = email
	m.lastLoginIP = ip
	return m.loginSess, m.loginToken, m.loginErr
}
func (m *mockAuthService) Logout(ctx context.Context, tokenHash string) error {
	m.logoutCalled = true
	return m.logoutErr
}
func (m *mockAuthService) ValidateSession(ctx context.Context, tokenHash string) (*iam.Session, error) {
	return m.validateSess, m.validateSessErr
}
func (m *mockAuthService) Me(ctx context.Context, ses *iam.Session) (*iam.User, []iam.Membership, error) {
	return m.meUser, m.meMemberships, m.meErr
}
func (m *mockAuthService) GetUserByID(ctx context.Context, id string) (*iam.User, error) {
	return m.meUser, nil
}
func (m *mockAuthService) GetMemberships(ctx context.Context, userID string) ([]iam.Membership, error) {
	return m.meMemberships, nil
}
func (m *mockAuthService) ValidateAPIToken(ctx context.Context, tokenHash string) (*iam.APIToken, error) {
	return m.validateToken, m.validateTokenErr
}

type responseRecorder struct {
	header http.Header
	code   int
	body   *strings.Builder
}

func (rr *responseRecorder) Header() http.Header { return rr.header }
func (rr *responseRecorder) Write(b []byte) (int, error) {
	return rr.body.Write(b)
}
func (rr *responseRecorder) WriteHeader(code int) { rr.code = code }

// serve runs the request through the server mux and returns the response.
func serve(s *Server, method, path, body string, cookies []*http.Cookie, auth string) *http.Response {
	r, _ := http.NewRequest(method, path, strings.NewReader(body))
	for _, c := range cookies {
		r.AddCookie(c)
	}
	if auth != "" {
		r.Header.Set("Authorization", auth)
	}
	r.RemoteAddr = "127.0.0.1:54321"
	rr := &responseRecorder{header: make(http.Header), body: &strings.Builder{}}
	s.mux.ServeHTTP(rr, r)
	resp := &http.Response{
		StatusCode: rr.code,
		Header:     rr.header,
		Body:       io.NopCloser(strings.NewReader(rr.body.String())),
		Request:    r,
	}
	// Capture Set-Cookie headers for the jar.
	resp.Header = rr.header
	return resp
}

func bodyString(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func newServer(t *testing.T, svc authService) *Server {
	t.Helper()
	if svc == nil {
		svc = &mockAuthService{}
	}
	return New(svc, &config.Config{
		SessionCookieName: "dp_session",
		SessionLifetime:   24 * time.Hour,
		SessionInsecure:   true,
	})
}

func TestLoginSuccess(t *testing.T) {
	m := &mockAuthService{
		loginSess: &iam.Session{ID: "s1", UserID: "u1", ExpiresAt: time.Now().UTC().Add(time.Hour)},
		loginToken: "abc123",
		meUser: &iam.User{ID: "u1", Email: "alice@example.com"},
		meMemberships: []iam.Membership{{TeamID: "t1", Role: "admin"}},
	}
	s := newServer(t, m)

	resp := serve(s, "POST", "/v1/auth/login", `{"email":"alice@example.com","password":"secret"}`, nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: status %d, body %s", resp.StatusCode, bodyString(t, resp))
	}
	if !m.loginCalled {
		t.Error("Login not called")
	}
	if m.lastLoginEmail != "alice@example.com" || m.lastLoginIP != "127.0.0.1" {
		t.Errorf("login called with email=%q ip=%q", m.lastLoginEmail, m.lastLoginIP)
	}
	// Cookie set.
	cookies := resp.Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "dp_session" && c.Value == "abc123" && c.HttpOnly && c.Path == "/" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected dp_session cookie with abc123, got %+v", cookies)
	}
	body := bodyString(t, resp)
	if !strings.Contains(body, `"id":"u1"`) || !strings.Contains(body, `"email":"alice@example.com"`) {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	m := &mockAuthService{loginErr: iam.ErrInvalidCredentials}
	s := newServer(t, m)

	resp := serve(s, "POST", "/v1/auth/login", `{"email":"alice@example.com","password":"wrong"}`, nil, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d body %s", resp.StatusCode, bodyString(t, resp))
	}
	body := bodyString(t, resp)
	if !strings.Contains(body, `"code":"invalid_credentials"`) {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestLoginMissingFields(t *testing.T) {
	m := &mockAuthService{}
	s := newServer(t, m)

	resp := serve(s, "POST", "/v1/auth/login", `{"email":"alice@example.com"}`, nil, "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d body %s", resp.StatusCode, bodyString(t, resp))
	}
}

func TestLogoutSuccess(t *testing.T) {
	m := &mockAuthService{}
	s := newServer(t, m)

	resp := serve(s, "POST", "/v1/auth/logout", "", []*http.Cookie{{Name: "dp_session", Value: "token-xyz"}}, "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout: status %d body %s", resp.StatusCode, bodyString(t, resp))
	}
	if !m.logoutCalled {
		t.Error("Logout not called")
	}
	// Cookie cleared.
	cleared := false
	for _, c := range resp.Cookies() {
		if c.Name == "dp_session" && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Errorf("expected cleared dp_session cookie, got %+v", resp.Cookies())
	}
}

func TestLogoutNoSession(t *testing.T) {
	m := &mockAuthService{}
	s := newServer(t, m)

	resp := serve(s, "POST", "/v1/auth/logout", "", nil, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d body %s", resp.StatusCode, bodyString(t, resp))
	}
}

func TestMeWithSession(t *testing.T) {
	m := &mockAuthService{
		validateSess: &iam.Session{UserID: "u1"},
		meUser: &iam.User{ID: "u1", Email: "bob@example.com"},
		meMemberships: []iam.Membership{{TeamID: "t1", Role: "developer"}},
	}
	s := newServer(t, m)

	resp := serve(s, "GET", "/v1/me", "", []*http.Cookie{{Name: "dp_session", Value: "sess-tok"}}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("me: status %d body %s", resp.StatusCode, bodyString(t, resp))
	}
	body := bodyString(t, resp)
	if !strings.Contains(body, `"email":"bob@example.com"`) || !strings.Contains(body, `"role":"developer"`) {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestMeNoAuth(t *testing.T) {
	m := &mockAuthService{}
	s := newServer(t, m)

	resp := serve(s, "GET", "/v1/me", "", nil, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d body %s", resp.StatusCode, bodyString(t, resp))
	}
}

func TestMeWithBearerToken(t *testing.T) {
	m := &mockAuthService{
		validateToken: &iam.APIToken{ID: "tk1", UserID: "u1"},
		meUser: &iam.User{ID: "u1", Email: "carol@example.com"},
		meMemberships: []iam.Membership{{TeamID: "t1", Role: "billing"}},
	}
	s := newServer(t, m)

	resp := serve(s, "GET", "/v1/me", "", nil, "Bearer plain-token")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("me bearer: status %d body %s", resp.StatusCode, bodyString(t, resp))
	}
}

func TestMeSessionExpired(t *testing.T) {
	m := &mockAuthService{validateSessErr: iam.ErrSessionExpired}
	s := newServer(t, m)

	resp := serve(s, "GET", "/v1/me", "", []*http.Cookie{{Name: "dp_session", Value: "sess-tok"}}, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d body %s", resp.StatusCode, bodyString(t, resp))
	}
	body := bodyString(t, resp)
	if !strings.Contains(body, `"code":"session_invalid"`) {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestHealthz(t *testing.T) {
	s := newServer(t, nil)
	resp := serve(s, "GET", "/healthz", "", nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz: status %d body %s", resp.StatusCode, bodyString(t, resp))
	}
	body := bodyString(t, resp)
	if !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("unexpected body: %s", body)
	}
}

// Compile-time check that mockAuthService implements authService.
var _ authService = (*mockAuthService)(nil)
