package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSessionStoreTTLCap pins the default remembered-device lifetime and the hard
// ceiling: a remembered session may not outlive 30 days.
func TestSessionStoreTTLCap(t *testing.T) {
	if got := newSessionStore("tok", 0, "").ttl; got != SessionTTLDefault {
		t.Errorf("default TTL = %s, want %s", got, SessionTTLDefault)
	}
	if got := newSessionStore("tok", 90*24*time.Hour, "").ttl; got != SessionTTLMax {
		t.Errorf("TTL above the cap = %s, want %s", got, SessionTTLMax)
	}
	if SessionTTLMax != 30*24*time.Hour {
		t.Errorf("SessionTTLMax = %s, want 30 days", SessionTTLMax)
	}
}

// TestSessionPersistsAcrossRestart proves a remembered device survives a controller
// restart when the store is file-backed.
func TestSessionPersistsAcrossRestart(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sessions.json")
	a := newSessionStore("token-A", time.Hour, file)
	id := a.create()
	if !a.valid(id) {
		t.Fatal("a fresh session must be valid")
	}

	// A second store over the same file and token models a restart: the session must
	// survive and still authenticate.
	b := newSessionStore("token-A", time.Hour, file)
	if !b.valid(id) {
		t.Error("a remembered session must survive a restart")
	}
}

// TestSessionFileStoresHashedIDs proves the store keeps only hashes: the raw session id
// and the raw token never reach disk, and the file is owner-only.
func TestSessionFileStoresHashedIDs(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sessions.json")
	s := newSessionStore("token-A", time.Hour, file)
	id := s.create()

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read session file: %v", err)
	}
	if strings.Contains(string(data), id) {
		t.Error("the session file must not contain the raw session id")
	}
	h := sha256.Sum256([]byte(id))
	if !strings.Contains(string(data), hex.EncodeToString(h[:])) {
		t.Error("the session file must contain the hashed session id")
	}
	if strings.Contains(string(data), "token-A") {
		t.Error("the session file must not contain the raw token")
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("session file mode = %o, want 600", perm)
	}
}

// TestSessionFileTokenChangeInvalidates proves a restart with a different token
// un-trusts every remembered device.
func TestSessionFileTokenChangeInvalidates(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sessions.json")
	a := newSessionStore("token-A", time.Hour, file)
	id := a.create()

	b := newSessionStore("token-B", time.Hour, file)
	if b.valid(id) {
		t.Error("a session must not survive a token change across a restart")
	}
	if !b.valid(b.create()) {
		t.Error("a session created under the new token must be valid")
	}
}

// TestLogoutSurvivesRestart proves revocation is persisted, so a logged-out device cannot
// reauthenticate after a restart.
func TestLogoutSurvivesRestart(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sessions.json")
	a := newSessionStore("tok", time.Hour, file)
	id := a.create()
	a.delete(id)

	b := newSessionStore("tok", time.Hour, file)
	if b.valid(id) {
		t.Error("a revoked (logged-out) session must not survive a restart")
	}
}

// TestExpiredSessionNotRestored proves an already-expired persisted session is dropped on
// load rather than resurrected.
func TestExpiredSessionNotRestored(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sessions.json")
	fp := sha256.Sum256([]byte("tok"))
	idHash := sha256.Sum256([]byte("stale-id"))
	data, err := json.Marshal(sessionFile{
		Version:     sessionFileVersion,
		Fingerprint: hex.EncodeToString(fp[:]),
		Sessions: []persistedSession{{
			IDHash:  hex.EncodeToString(idHash[:]),
			Expires: time.Now().Add(-time.Minute),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}

	s := newSessionStore("tok", time.Hour, file)
	if s.valid("stale-id") {
		t.Error("an expired persisted session must not be restored")
	}
}
