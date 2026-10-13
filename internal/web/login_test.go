package web

import (
	"database/sql"
	"io"
	"io/fs"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	assets "sop-controller"

	"sop-controller/internal/sopclient"

	_ "modernc.org/sqlite"
)

// newAuthServer builds a network-mode server (token auth + login) over a disposable
// project. ttl <= 0 uses the default session lifetime.
func newAuthServer(t *testing.T, token string, ttl time.Duration) *httptest.Server {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".agent-sdlc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("project:\n  name: \"demo\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(testSchema); err != nil {
		t.Fatalf("seed: %v", err)
	}
	db.Close()

	sop, err := sopclient.New([]string{root}, withApprovals(t, "sop"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sop.Close() })
	views, err := NewViews(assets.FS)
	if err != nil {
		t.Fatalf("views: %v", err)
	}
	staticFS, err := fs.Sub(assets.FS, "static")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(NewServer(Options{
		SOP: sop, Views: views, StaticFS: staticFS, Poll: time.Second,
		CommandTimeout: time.Minute, AllowNetwork: true, AccessToken: token, SessionTTL: ttl,
	}))
	t.Cleanup(srv.Close)
	return srv
}

// authClient returns a client with a cookie jar and no redirect following, so a 303 can
// be asserted and the CSRF/session cookies persist across requests.
func authClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// login performs the CSRF-protected sign-in and returns the client (now holding the
// session cookie) and the response.
func login(t *testing.T, srv *httptest.Server, token string) (*http.Client, *http.Response) {
	t.Helper()
	client := authClient()
	resp, err := client.Get(srv.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	csrf := ""
	for _, c := range resp.Cookies() {
		if c.Name == csrfCookie {
			csrf = c.Value
		}
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if csrf == "" {
		t.Fatal("no CSRF cookie issued by /login")
	}
	form := url.Values{"token": {token}, "csrf": {csrf}}
	res, err := client.PostForm(srv.URL+"/login", form)
	if err != nil {
		t.Fatal(err)
	}
	return client, res
}

// TestSessionStoreLifecycle covers creation, expiry, deletion, and that a changed token
// invalidates an existing session.
func TestSessionStoreLifecycle(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	s := newSessionStore("token-A", time.Hour, "")
	s.now = func() time.Time { return now }

	id := s.create()
	if !s.valid(id) {
		t.Fatal("a fresh session must be valid")
	}
	if s.valid("") || s.valid("nope") {
		t.Error("empty/unknown session ids must be invalid")
	}
	now = now.Add(2 * time.Hour)
	if s.valid(id) {
		t.Error("an expired session must be invalid")
	}

	// A changed configured token invalidates the existing session.
	id2 := s.create()
	changed := newSessionStore("token-B", time.Hour, "")
	changed.sessions = s.sessions
	if changed.valid(id2) {
		t.Error("a session must not survive a token change")
	}

	// Delete ends a session.
	id3 := s.create()
	s.delete(id3)
	if s.valid(id3) {
		t.Error("a deleted session must be invalid")
	}
}

// TestLoginFlow proves the sign-in page, the server-side validation, the secure session
// cookie, and authenticated navigation.
func TestLoginFlow(t *testing.T) {
	srv := newAuthServer(t, "s3cret-token", time.Hour)

	// The sign-in page has a masked token field and is not gated.
	code, body := get(t, srv.URL+"/login")
	if code != http.StatusOK {
		t.Fatalf("/login = %d, want 200", code)
	}
	if !strings.Contains(body, `type="password"`) || !strings.Contains(body, "Sign In") {
		t.Errorf("/login must offer a masked token field and a Sign In button")
	}
	if strings.Contains(body, "s3cret-token") {
		t.Error("the login page must never contain the configured token")
	}

	// A wrong token is rejected with a clear message and no session cookie.
	client, resp := login(t, srv, "wrong-token")
	if resp.StatusCode != http.StatusOK || !strings.Contains(readBody(t, resp), "Invalid token") {
		t.Errorf("a wrong token must be rejected with feedback, got %d", resp.StatusCode)
	}
	if hasCookie(resp, sessionCookie) {
		t.Error("a failed sign-in must not set a session cookie")
	}
	_ = client

	// The right token signs in: a Secure-shaped, HttpOnly, SameSite session cookie.
	client, resp = login(t, srv, "s3cret-token")
	c := cookieOf(resp, sessionCookie)
	if c == nil {
		t.Fatal("a successful sign-in must set a session cookie")
	}
	if !c.HttpOnly || c.SameSite == 0 {
		t.Errorf("session cookie must be HttpOnly and SameSite, got %+v", c)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Authenticated navigation now succeeds.
	req, _ := http.NewRequest("GET", srv.URL+"/projects", nil)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("authenticated /projects = %d, want 200", res.StatusCode)
	}
}

// TestLoginRequiresCSRF proves the sign-in form is CSRF-protected.
func TestLoginRequiresCSRF(t *testing.T) {
	srv := newAuthServer(t, "tok", time.Hour)
	resp, err := http.PostForm(srv.URL+"/login", url.Values{"token": {"tok"}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("login without a CSRF token = %d, want 403", resp.StatusCode)
	}
}

// TestLogoutInvalidatesSession proves logout ends the session.
func TestLogoutInvalidatesSession(t *testing.T) {
	srv := newAuthServer(t, "tok", time.Hour)
	client, resp := login(t, srv, "tok")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Find the CSRF cookie for the logout POST.
	csrf := ""
	u, _ := url.Parse(srv.URL)
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == csrfCookie {
			csrf = c.Value
		}
	}
	logout, err := client.PostForm(srv.URL+"/logout", url.Values{"csrf": {csrf}})
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, logout.Body)
	logout.Body.Close()
	if logout.StatusCode != http.StatusSeeOther {
		t.Fatalf("logout = %d, want 303", logout.StatusCode)
	}

	// The old session must no longer authenticate.
	req, _ := http.NewRequest("GET", srv.URL+"/projects", nil)
	req.Header.Set("Accept", "text/html")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusSeeOther || !strings.HasPrefix(res.Header.Get("Location"), "/login") {
		t.Errorf("after logout /projects = %d, want a redirect to /login", res.StatusCode)
	}
}

// TestSessionExpiry proves an expired session is rejected at the HTTP layer.
func TestSessionExpiry(t *testing.T) {
	srv := newAuthServer(t, "tok", 60*time.Millisecond)
	client, resp := login(t, srv, "tok")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	time.Sleep(90 * time.Millisecond)
	req, _ := http.NewRequest("GET", srv.URL+"/projects", nil)
	req.Header.Set("Accept", "text/html")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		t.Errorf("an expired session = %d, want a redirect to /login", res.StatusCode)
	}
}

// TestTokenChangeInvalidatesSessions proves restarting with a different token rejects an
// existing session cookie.
func TestTokenChangeInvalidatesSessions(t *testing.T) {
	srvA := newAuthServer(t, "token-A", time.Hour)
	client, resp := login(t, srvA, "token-A")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	srvB := newAuthServer(t, "token-B", time.Hour)
	// Reuse the old client's cookie against the new server.
	req, _ := http.NewRequest("GET", srvB.URL+"/projects", nil)
	for _, c := range resp.Cookies() {
		req.AddCookie(c)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusOK {
		t.Error("a session from a previous token must not authenticate")
	}
}

// TestSecureCookieBehindHTTPSProxy proves the session cookie is Secure when the request
// arrived over HTTPS via the trusted loopback proxy.
func TestSecureCookieBehindHTTPSProxy(t *testing.T) {
	srv := newAuthServer(t, "tok", time.Hour)
	client := authClient()
	resp, err := client.Get(srv.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	csrf := ""
	for _, c := range resp.Cookies() {
		if c.Name == csrfCookie {
			csrf = c.Value
		}
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/login", strings.NewReader(url.Values{"token": {"tok"}, "csrf": {csrf}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Forwarded-Proto", "https") // honored: the peer is loopback
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	c := cookieOf(res, sessionCookie)
	if c == nil || !c.Secure {
		t.Errorf("behind HTTPS the session cookie must be Secure, got %+v", c)
	}
}

// TestLoginThrottling proves repeated failures are throttled.
func TestLoginThrottling(t *testing.T) {
	srv := newAuthServer(t, "tok", time.Hour)
	client := authClient()
	resp, _ := client.Get(srv.URL + "/login")
	csrf := ""
	for _, c := range resp.Cookies() {
		if c.Name == csrfCookie {
			csrf = c.Value
		}
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	throttled := false
	for i := 0; i < loginMaxAttempts+2; i++ {
		res, err := client.PostForm(srv.URL+"/login", url.Values{"token": {"nope"}, "csrf": {csrf}})
		if err != nil {
			t.Fatal(err)
		}
		body := readBody(t, res)
		if strings.Contains(body, "Too many attempts") {
			throttled = true
			break
		}
	}
	if !throttled {
		t.Error("repeated failed sign-ins must be throttled")
	}
}

// TestSafeNextRejectsOpenRedirect proves the login redirect cannot leave the site.
func TestSafeNextRejectsOpenRedirect(t *testing.T) {
	for _, bad := range []string{"https://evil.example", "//evil.example", "http://evil", "/\\evil", ""} {
		if got := safeNext(bad); got != "" {
			t.Errorf("safeNext(%q) = %q, want empty", bad, got)
		}
	}
	if got := safeNext("/approvals"); got != "/approvals" {
		t.Errorf("safeNext(/approvals) = %q, want /approvals", got)
	}
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return string(b)
}

func hasCookie(resp *http.Response, name string) bool { return cookieOf(resp, name) != nil }

func cookieOf(resp *http.Response, name string) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}
