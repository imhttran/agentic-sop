package web

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Secure token login. The controller keeps its SOP_CONTROLLER_TOKEN model; the token is
// never placed in a URL, response body, log, or browser storage. A successful sign-in
// mints an opaque session id held server-side (in memory, not a database) and delivered
// in a Secure, HttpOnly, SameSite cookie. A restart, a changed token, or expiry ends the
// session.

const sessionCookie = "sop_ctrl_session"

// Session lifetime and login throttling.
const (
	// SessionTTLDefault remembers a device for 30 days when the operator sets no TTL.
	SessionTTLDefault = 30 * 24 * time.Hour
	// SessionTTLMax is the hard ceiling on a remembered session, enforced on the cookie
	// and the server-side record alike.
	SessionTTLMax = 30 * 24 * time.Hour
	// loginMaxAttempts and loginWindow throttle repeated failed sign-ins.
	loginMaxAttempts = 10
	loginWindow      = time.Minute
	// maxSessions caps the store so a long-lived process cannot grow unbounded.
	maxSessions = 512
	// sessionFileVersion identifies the on-disk session format.
	sessionFileVersion = 1
)

// session is one authenticated session: when it expires and the fingerprint of the token
// it was created under, so a changed token invalidates it.
type session struct {
	expires     time.Time
	fingerprint [sha256.Size]byte
}

// hashID derives the server-side identifier for a session cookie. Only this hash is
// stored — in memory and on disk — never the cookie's own opaque value, so a leaked
// session file cannot be replayed as a cookie.
func hashID(id string) [sha256.Size]byte { return sha256.Sum256([]byte(id)) }

// sessionStore keeps remembered-device sessions. It persists hashed identifiers to a
// file (when configured) so a signed-in device survives a controller restart, and drops
// everything when the configured token changes.
type sessionStore struct {
	mu       sync.Mutex
	ttl      time.Duration
	fp       [sha256.Size]byte
	sessions map[[sha256.Size]byte]session
	// file is the persistence path; empty keeps the store in memory only.
	file string
	now  func() time.Time
}

// sessionFile is the on-disk shape: no raw token, no raw session id — only the token's
// fingerprint and the hashes of the live session ids.
type sessionFile struct {
	Version     int                `json:"version"`
	Fingerprint string             `json:"fingerprint"`
	Sessions    []persistedSession `json:"sessions"`
}

type persistedSession struct {
	IDHash  string    `json:"id_hash"`
	Expires time.Time `json:"expires"`
}

// EffectiveSessionTTL clamps a configured session lifetime: zero uses the default, and
// anything above the maximum is capped. It is the single source of truth for both the
// store and the startup banner.
func EffectiveSessionTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return SessionTTLDefault
	}
	if ttl > SessionTTLMax {
		return SessionTTLMax
	}
	return ttl
}

func newSessionStore(token string, ttl time.Duration, file string) *sessionStore {
	s := &sessionStore{
		ttl:      EffectiveSessionTTL(ttl),
		fp:       sha256.Sum256([]byte(token)),
		sessions: map[[sha256.Size]byte]session{},
		file:     file,
		now:      time.Now,
	}
	s.load()
	return s
}

// create returns a fresh opaque session id — only the caller (to set the cookie) ever
// sees it; the store keeps its hash. The configured token is never part of it.
func (s *sessionStore) create() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.evictLocked(now)
	s.sessions[hashID(id)] = session{expires: now.Add(s.ttl), fingerprint: s.fp}
	s.persistLocked()
	return id
}

// valid reports whether id names a live session for the current token. It validates on
// every request and never slides the expiry: a remembered device lasts at most ttl from
// sign-in.
func (s *sessionStore) valid(id string) bool {
	if id == "" {
		return false
	}
	h := hashID(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.sessions[h]
	if !ok || rec.fingerprint != s.fp || !s.now().Before(rec.expires) {
		delete(s.sessions, h)
		return false
	}
	return true
}

// delete revokes one session immediately (logout).
func (s *sessionStore) delete(id string) {
	if id == "" {
		return
	}
	h := hashID(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[h]; ok {
		delete(s.sessions, h)
		s.persistLocked()
	}
}

// evictLocked drops expired sessions and, past the cap, the soonest to expire.
func (s *sessionStore) evictLocked(now time.Time) {
	for id, rec := range s.sessions {
		if !now.Before(rec.expires) {
			delete(s.sessions, id)
		}
	}
	for len(s.sessions) >= maxSessions {
		var oldestID [sha256.Size]byte
		var oldest time.Time
		found := false
		for id, rec := range s.sessions {
			if !found || rec.expires.Before(oldest) {
				oldestID, oldest, found = id, rec.expires, true
			}
		}
		if !found {
			break
		}
		delete(s.sessions, oldestID)
	}
}

// persistLocked writes the store atomically (temp file + rename) with owner-only
// permissions. It is called with the mutex held.
func (s *sessionStore) persistLocked() {
	if s.file == "" {
		return
	}
	out := sessionFile{Version: sessionFileVersion, Fingerprint: hex.EncodeToString(s.fp[:])}
	for h, rec := range s.sessions {
		out.Sessions = append(out.Sessions, persistedSession{
			IDHash:  hex.EncodeToString(h[:]),
			Expires: rec.expires,
		})
	}
	data, err := json.Marshal(out)
	if err != nil {
		log.Printf("[session] encode: %v", err)
		return
	}
	if dir := filepath.Dir(s.file); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			log.Printf("[session] create %s: %v", dir, err)
			return
		}
	}
	tmp := s.file + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		log.Printf("[session] write %s: %v", tmp, err)
		return
	}
	if err := os.Rename(tmp, s.file); err != nil {
		log.Printf("[session] rename %s: %v", s.file, err)
	}
}

// load restores sessions from disk, ignoring the file when it is missing, unreadable, or
// was written for a different token (a changed token un-trusts every remembered device).
func (s *sessionStore) load() {
	if s.file == "" {
		return
	}
	data, err := os.ReadFile(s.file)
	if err != nil {
		return
	}
	var in sessionFile
	if err := json.Unmarshal(data, &in); err != nil {
		log.Printf("[session] ignoring unreadable session file %s: %v", s.file, err)
		return
	}
	if in.Fingerprint != hex.EncodeToString(s.fp[:]) {
		return
	}
	now := s.now()
	for _, p := range in.Sessions {
		h, ok := decodeHash(p.IDHash)
		if !ok || !now.Before(p.Expires) {
			continue
		}
		s.sessions[h] = session{expires: p.Expires, fingerprint: s.fp}
	}
}

func decodeHash(s string) ([sha256.Size]byte, bool) {
	var h [sha256.Size]byte
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != sha256.Size {
		return h, false
	}
	copy(h[:], b)
	return h, true
}

// attemptLimiter throttles login attempts per client over a rolling window.
type attemptLimiter struct {
	mu     sync.Mutex
	window time.Duration
	max    int
	hits   map[string][]time.Time
	now    func() time.Time
}

func newAttemptLimiter(max int, window time.Duration) *attemptLimiter {
	return &attemptLimiter{window: window, max: max, hits: map[string][]time.Time{}, now: time.Now}
}

// allow reports whether another attempt from key is permitted right now.
func (l *attemptLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := l.now().Add(-l.window)
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	l.hits[key] = kept
	return len(kept) < l.max
}

func (l *attemptLimiter) record(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits[key] = append(l.hits[key], l.now())
}

// authGate requires an authenticated session for every route except health, static
// assets, and the login page. Tooling may present the token directly in the
// X-Access-Token header; the token is never accepted from the URL.
func authGate(token string, store *sessionStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if id, ok := sessionID(r); ok && store.valid(id) {
			next.ServeHTTP(w, r)
			return
		}
		if provided := r.Header.Get("X-Access-Token"); provided != "" &&
			subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1 {
			next.ServeHTTP(w, r)
			return
		}
		// A browser navigation goes to the login page; anything else is a 401.
		if r.Method == http.MethodGet && wantsHTML(r) {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
			return
		}
		http.Error(w, "authentication required", http.StatusUnauthorized)
	})
}

func publicPath(p string) bool {
	if p == "/healthz" || p == "/login" || p == "/favicon.ico" {
		return true
	}
	return strings.HasPrefix(p, "/static/")
}

func sessionID(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return "", false
	}
	return c.Value, true
}

func wantsHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// isHTTPS reports whether the original request reached us over TLS. X-Forwarded-Proto is
// honored only from a loopback peer (the local reverse proxy), never blindly.
func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return isLoopbackPeer(r.RemoteAddr) &&
		strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func isLoopbackPeer(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	if host == "" || host == "localhost" {
		return true // httptest and unix sockets
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// setSessionCookie delivers the session id in a Secure, HttpOnly, SameSite cookie.
func setSessionCookie(w http.ResponseWriter, r *http.Request, id string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// clientIP identifies the caller for throttling. Behind the local proxy it prefers the
// forwarded address, trusted only because the direct peer is loopback.
func clientIP(r *http.Request) string {
	if isLoopbackPeer(r.RemoteAddr) {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			if first := strings.TrimSpace(strings.Split(fwd, ",")[0]); first != "" {
				return first
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// safeNext returns next only when it is a local path, so the login page cannot be used
// for an open redirect.
func safeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return ""
	}
	if strings.ContainsAny(next, "\r\n\\") {
		return ""
	}
	if u, err := url.Parse(next); err != nil || u.Scheme != "" || u.Host != "" {
		return ""
	}
	return next
}

func nextOrDefault(next string) string {
	if next != "" {
		return next
	}
	return "/projects"
}

// loginPageData feeds login.html.
type loginPageData struct {
	baseData
	Error string
	Next  string
}

// loginPage renders the sign-in form; an already-authenticated visitor is sent on.
func (h *Handlers) loginPage(w http.ResponseWriter, r *http.Request) {
	if h.accessToken == "" {
		http.Redirect(w, r, "/projects", http.StatusSeeOther)
		return
	}
	next := safeNext(r.URL.Query().Get("next"))
	if id, ok := sessionID(r); ok && h.sessions.valid(id) {
		http.Redirect(w, r, nextOrDefault(next), http.StatusSeeOther)
		return
	}
	h.render(w, http.StatusOK, "login.html", loginPageData{baseData: h.base(r, ""), Next: next})
}

// loginSubmit validates the submitted token server-side and opens a session.
func (h *Handlers) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if h.accessToken == "" {
		http.Redirect(w, r, "/projects", http.StatusSeeOther)
		return
	}
	next := safeNext(r.FormValue("next"))
	key := clientIP(r)
	if !h.loginLimiter.allow(key) {
		h.renderLoginError(w, r, next, "Too many attempts. Try again in a moment.")
		return
	}
	got := r.FormValue("token")
	if subtle.ConstantTimeCompare([]byte(got), []byte(h.accessToken)) != 1 {
		h.loginLimiter.record(key)
		h.renderLoginError(w, r, next, "Invalid token.")
		return
	}
	setSessionCookie(w, r, h.sessions.create(), h.sessions.ttl)
	http.Redirect(w, r, nextOrDefault(next), http.StatusSeeOther)
}

func (h *Handlers) renderLoginError(w http.ResponseWriter, r *http.Request, next, msg string) {
	h.render(w, http.StatusOK, "login.html",
		loginPageData{baseData: h.base(r, ""), Error: msg, Next: next})
}

// logout ends the session and clears its cookie.
func (h *Handlers) logout(w http.ResponseWriter, r *http.Request) {
	if id, ok := sessionID(r); ok {
		h.sessions.delete(id)
	}
	clearSessionCookie(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
