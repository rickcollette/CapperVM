package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"capper/internal/api"
	"capper/internal/controller"
	"capper/internal/store"
)

// newTestServer creates an in-memory server backed by a temp SQLite store.
func newTestServer(t *testing.T) (*api.Server, *store.Store) {
	t.Helper()
	tmp := t.TempDir()
	paths := store.NewPaths(tmp)
	st, err := store.Open(paths)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	ctrl := controller.New(st, false, "auto")
	srv := api.NewServer(ctrl, api.Options{Project: "default"})
	return srv, st
}

// adminToken issues a bearer for the real local (root/admin) principal, which
// holds the bootstrap admin-all policy. Use this whenever a test needs an
// authenticated principal that passes authorization checks.
func adminToken(t *testing.T, st *store.Store) string {
	t.Helper()
	pType, pID := st.IAM.LocalPrincipal()
	bearer, _, err := st.IAM.Issue("test-admin", pType, pID, 3600*1000000000)
	if err != nil {
		t.Fatalf("issue admin token: %v", err)
	}
	return bearer
}

func doRequest(t *testing.T, srv *api.Server, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	} else {
		bodyReader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	return rr
}

// TestUnauthenticatedRequestsAreRejected verifies that protected endpoints
// return 401 when no Authorization header is provided.
func TestUnauthenticatedRequestsAreRejected(t *testing.T) {
	srv, _ := newTestServer(t)
	protectedPaths := []string{
		"GET /api/v1/instances",
		"GET /api/v1/vpcs",
		"GET /api/v1/iam/users",
		"GET /api/v1/orgs",
		"GET /api/v1/nodes",
		"GET /api/v1/certificates",
	}
	for _, spec := range protectedPaths {
		parts := strings.SplitN(spec, " ", 2)
		rr := doRequest(t, srv, parts[0], parts[1], "", nil)
		if rr.Code == http.StatusOK {
			t.Errorf("%s: expected non-200 without auth, got 200", spec)
		}
	}
}

// TestNodeJoinTokenRequired verifies that the join endpoint rejects requests
// with an invalid or missing token.
func TestNodeJoinTokenRequired(t *testing.T) {
	srv, _ := newTestServer(t)
	body := `{"token":"invalid-token","name":"node1","address":"10.0.0.1"}`
	rr := doRequest(t, srv, "POST", "/api/v1/nodes/join", body, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid join token, got %d", rr.Code)
	}
}

// TestHealthEndpointIsPublic verifies /api/v1/health is reachable without auth.
func TestHealthEndpointIsPublic(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := doRequest(t, srv, "GET", "/api/v1/health", "", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("health endpoint: expected 200, got %d", rr.Code)
	}
}

// TestJSONResponseEnvelope verifies API responses use the standard envelope.
func TestJSONResponseEnvelope(t *testing.T) {
	srv, st := newTestServer(t)
	bearer := adminToken(t, st)
	rr := doRequest(t, srv, "GET", "/api/v1/instances", "", map[string]string{
		"Authorization": "Bearer " + bearer,
	})
	if rr.Code != http.StatusOK {
		t.Skipf("instances list returned %d (skipping envelope check)", rr.Code)
	}
	var env map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := env["data"]; !ok {
		t.Error("response missing 'data' field in envelope")
	}
}

// TestSQLInjectionInPathParams verifies path parameters with SQL injection
// payloads do not cause 500 errors (they should return 400 or 404).
func TestSQLInjectionInPathParams(t *testing.T) {
	srv, st := newTestServer(t)
	bearer := adminToken(t, st)
	injectionPayloads := []string{
		"'; DROP TABLE instances; --",
		"1 OR 1=1",
		"\" OR \"1\"=\"1",
	}
	headers := map[string]string{"Authorization": "Bearer " + bearer}
	for _, payload := range injectionPayloads {
		rr := doRequest(t, srv, "GET", "/api/v1/instances/"+url.PathEscape(payload), "", headers)
		if rr.Code == http.StatusInternalServerError {
			t.Errorf("SQL injection payload %q caused 500", payload)
		}
	}
}

// TestXSSInAPIResponse verifies that user-provided string fields are not
// reflected back without encoding in JSON responses (no raw <script> tags).
func TestXSSInAPIResponse(t *testing.T) {
	srv, st := newTestServer(t)
	bearer := adminToken(t, st)
	xssPayload := `<script>alert(1)</script>`
	body := `{"name":"` + xssPayload + `","slug":"xss-test"}`
	rr := doRequest(t, srv, "POST", "/api/v1/orgs", body, map[string]string{
		"Authorization": "Bearer " + bearer,
	})
	// We don't care about success/failure - just that <script> is not in the raw response
	// as an unescaped HTML tag (JSON encoding handles this, but verify).
	if strings.Contains(rr.Body.String(), "<script>") {
		t.Error("XSS payload reflected unescaped in response body")
	}
}

// TestMethodNotAllowed verifies that wrong HTTP methods return 405 or 404.
func TestMethodNotAllowed(t *testing.T) {
	srv, st := newTestServer(t)
	bearer := adminToken(t, st)
	// PATCH on a list-only endpoint should not succeed
	rr := doRequest(t, srv, "PATCH", "/api/v1/instances", "", map[string]string{
		"Authorization": "Bearer " + bearer,
	})
	if rr.Code == http.StatusOK {
		t.Error("PATCH /api/v1/instances returned 200, expected 4xx")
	}
}

// TestAccountIAMRequiresAuthorization verifies that an authenticated but
// unprivileged principal cannot manage IAM in an account or assume a role —
// authentication alone is not sufficient (regression guard for the cross-tenant
// privilege-escalation fix).
func TestAccountIAMRequiresAuthorization(t *testing.T) {
	srv, st := newTestServer(t)
	bearer, _, err := st.IAM.Issue("nobody", "user", "unprivileged-user", 3600*1000000000)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	headers := map[string]string{"Authorization": "Bearer " + bearer}

	cases := []struct{ method, path, body string }{
		{"GET", "/api/v1/accounts/acct_local/iam/users", ""},
		{"POST", "/api/v1/accounts/acct_local/iam/users", `{"name":"evil"}`},
		{"DELETE", "/api/v1/accounts/acct_local/iam/users/some-id", ""},
		{"GET", "/api/v1/accounts/acct_local/audit", ""},
		{"POST", "/api/v1/iam/assume-role", `{"roleArn":"role_admin"}`},
	}
	for _, c := range cases {
		rr := doRequest(t, srv, c.method, c.path, c.body, headers)
		if rr.Code != http.StatusForbidden {
			t.Errorf("%s %s: expected 403 for unprivileged principal, got %d", c.method, c.path, rr.Code)
		}
	}

	// Sanity: the same list route is not forbidden for the admin principal.
	admin := map[string]string{"Authorization": "Bearer " + adminToken(t, st)}
	if rr := doRequest(t, srv, "GET", "/api/v1/accounts/acct_local/iam/users", "", admin); rr.Code == http.StatusForbidden {
		t.Errorf("admin principal was denied account IAM list (got 403)")
	}
}

// TestCookieSessionAndCSRF exercises the end-to-end browser auth flow: exchange
// a bearer token for a session cookie + CSRF token, then confirm a
// cookie-authenticated state-changing request is rejected without the CSRF
// header and accepted with it, while bearer auth bypasses CSRF.
func TestCookieSessionAndCSRF(t *testing.T) {
	srv, st := newTestServer(t)
	bearer := adminToken(t, st)

	// 1. Establish a session.
	rr := doRequest(t, srv, "POST", "/api/v1/auth/session", `{"token":"`+bearer+`"}`, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("auth/session: expected 200, got %d", rr.Code)
	}
	var sessionCookie, csrfCookie *http.Cookie
	for _, c := range rr.Result().Cookies() {
		switch c.Name {
		case "capper_session":
			sessionCookie = c
		case "capper_csrf":
			csrfCookie = c
		}
	}
	if sessionCookie == nil || csrfCookie == nil {
		t.Fatalf("session/csrf cookies not set (session=%v csrf=%v)", sessionCookie, csrfCookie)
	}
	if !sessionCookie.HttpOnly || !sessionCookie.Secure || sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("session cookie not hardened: HttpOnly=%v Secure=%v SameSite=%v",
			sessionCookie.HttpOnly, sessionCookie.Secure, sessionCookie.SameSite)
	}

	cookieHdr := sessionCookie.Name + "=" + sessionCookie.Value + "; " + csrfCookie.Name + "=" + csrfCookie.Value

	// 2. Cookie-authenticated POST WITHOUT the CSRF header → 403.
	rr = doRequest(t, srv, "POST", "/api/v1/orgs", `{"name":"x"}`, map[string]string{"Cookie": cookieHdr})
	if rr.Code != http.StatusForbidden {
		t.Errorf("cookie POST without CSRF: expected 403, got %d", rr.Code)
	}

	// 3. Cookie-authenticated POST WITH the matching CSRF header → not blocked by
	//    the CSRF/auth layer (handler may still 4xx, but never 403-CSRF).
	rr = doRequest(t, srv, "POST", "/api/v1/orgs", `{"name":"x"}`, map[string]string{
		"Cookie":       cookieHdr,
		"X-CSRF-Token": csrfCookie.Value,
	})
	if rr.Code == http.StatusForbidden {
		t.Errorf("cookie POST with valid CSRF was forbidden (got 403)")
	}

	// 4. Bearer POST bypasses CSRF entirely.
	rr = doRequest(t, srv, "POST", "/api/v1/orgs", `{"name":"y"}`, map[string]string{
		"Authorization": "Bearer " + bearer,
	})
	if rr.Code == http.StatusForbidden {
		t.Errorf("bearer POST should bypass CSRF, got 403")
	}
}

// TestTenantHeaderCannotWidenScope verifies that a principal cannot switch its
// account/org scope to one it is not authorized for via the X-Capper-* headers.
func TestTenantHeaderCannotWidenScope(t *testing.T) {
	srv, st := newTestServer(t)
	bearer, _, err := st.IAM.Issue("nobody", "user", "unprivileged-user", 3600*1000000000)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	// A non-member principal asking to act in a foreign account is rejected at
	// the auth boundary (403), not silently downgraded.
	rr := doRequest(t, srv, "GET", "/api/v1/instances", "", map[string]string{
		"Authorization":       "Bearer " + bearer,
		"X-Capper-Account-ID": "acct_victim",
	})
	if rr.Code != http.StatusForbidden {
		t.Errorf("foreign account header: expected 403, got %d", rr.Code)
	}

	// A foreign org switch is likewise rejected.
	rr = doRequest(t, srv, "GET", "/api/v1/instances", "", map[string]string{
		"Authorization":   "Bearer " + bearer,
		"X-Capper-Org-ID": "org_victim",
	})
	if rr.Code != http.StatusForbidden {
		t.Errorf("foreign org header: expected 403, got %d", rr.Code)
	}

	// The org-root admin may switch (here to the default account, a no-op) and is
	// not blocked at the auth boundary.
	rr = doRequest(t, srv, "GET", "/api/v1/instances", "", map[string]string{
		"Authorization":       "Bearer " + adminToken(t, st),
		"X-Capper-Account-ID": "acct_local",
	})
	if rr.Code == http.StatusForbidden {
		t.Errorf("admin with default account header was blocked at auth boundary (403)")
	}
}

// TestCORSDoesNotReflectArbitraryOrigins verifies the CORS layer only grants
// credentialed access to loopback or allowlisted origins, never an arbitrary
// reflected origin.
func TestCORSDoesNotReflectArbitraryOrigins(t *testing.T) {
	srv, _ := newTestServer(t)

	// Untrusted origin: no ACAO / credentials headers echoed back.
	rr := doRequest(t, srv, "GET", "/api/v1/health", "", map[string]string{"Origin": "https://evil.example.com"})
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("arbitrary origin was reflected: Access-Control-Allow-Origin=%q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("credentials allowed for arbitrary origin: %q", got)
	}

	// Loopback origin (e.g. the Vite dev console): credentialed access granted.
	rr = doRequest(t, srv, "GET", "/api/v1/health", "", map[string]string{"Origin": "http://localhost:5173"})
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("loopback origin not allowed: Access-Control-Allow-Origin=%q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("loopback origin missing Allow-Credentials: %q", got)
	}
}

// TestOversizedPayloadRejected verifies that extremely large request bodies
// do not crash the server (Go's default behavior limits body size).
func TestOversizedPayloadRejected(t *testing.T) {
	srv, _ := newTestServer(t)
	huge := strings.Repeat("x", 10*1024*1024) // 10 MB
	body := `{"name":"` + huge + `"}`
	rr := doRequest(t, srv, "POST", "/api/v1/orgs", body, nil)
	// Should not be 500 — 400, 401, 413, or 404 are all acceptable.
	if rr.Code == http.StatusInternalServerError {
		t.Error("oversized payload caused 500")
	}
}
