package verifcache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sharedID builds a fully-populated identity for a worktree at head with the
// given effective-inputs digest and scope.
func sharedID(head, inputs, scope string) Identity {
	id := fullID()
	id.Repository = RepositoryIdentity(head, false, inputs)
	id.Inputs = inputs
	id.Scope = ScopeIdentity([]string{scope})
	return id
}

// TestStoreSharedCrossWorktreeReuseOnExactIdentity proves evidence written to a
// shared store by one worktree is found by an identically-identified worktree,
// and never when any component diverges.
func TestStoreSharedCrossWorktreeReuseOnExactIdentity(t *testing.T) {
	shared := SharedStore{Path: filepath.Join(t.TempDir(), "shared", "verify-cache.json")}

	// Worktree A computes an identity and stores VERIFIED evidence.
	a := New()
	idA := sharedID("head1", "inputs-abc", "VER-002")
	if _, err := shared.Store(a, idA, Result{Passed: true, Output: "ok", Source: "run/A", Verdict: VerdictVerified}); err != nil {
		t.Fatalf("store: %v", err)
	}

	// Worktree B has an empty local cache but the identical identity: the shared
	// store answers the hit.
	b := New()
	idB := sharedID("head1", "inputs-abc", "VER-002")
	if !Equivalent(idA, idB) {
		t.Fatal("identical worktree identities must be Equivalent")
	}
	hit, source := shared.Lookup(b, idB)
	if !hit.Hit || source != SourceShared {
		t.Fatalf("cross-worktree reuse expected, got hit=%v source=%q", hit.Hit, source)
	}
	if hit.Entry.Source != "run/A" {
		t.Errorf("reused evidence should carry its origin, got %q", hit.Entry.Source)
	}

	// A diverged revision must miss: main vs recovery/rag025 never share
	// evidence by default.
	diverged := sharedID("recovery/rag025", "inputs-abc", "VER-002")
	if hit, _ := shared.Lookup(New(), diverged); hit.Hit {
		t.Error("a diverged revision must not reuse shared evidence")
	}

	// Every other component must also break reuse.
	for name, mutate := range map[string]func(*Identity){
		"inputs":      func(i *Identity) { i.Inputs = "inputs-changed" },
		"environment": func(i *Identity) { i.Environment = "env-changed" },
		"command":     func(i *Identity) { i.Command = "command-changed" },
		"policy":      func(i *Identity) { i.CommandPolicy = "policy-changed" },
		"scope":       func(i *Identity) { i.Scope = ScopeIdentity([]string{"VER-OTHER"}) },
	} {
		in := idB
		mutate(&in)
		if hit, _ := shared.Lookup(New(), in); hit.Hit {
			t.Errorf("%s divergence must not reuse shared evidence", name)
		}
	}
}

// TestStoreLocalTakesPrecedence proves the local cache answers first without
// consulting the shared store, and that a disabled shared store is local-only.
func TestStoreLocalTakesPrecedence(t *testing.T) {
	shared := SharedStore{Path: filepath.Join(t.TempDir(), "shared", "verify-cache.json")}
	id := sharedID("head1", "inputs-abc", "VER-002")

	local := New()
	local.Store(id, Result{Passed: true, Source: "run/local", Verdict: VerdictVerified})
	hit, source := shared.Lookup(local, id)
	if !hit.Hit || source != SourceLocal {
		t.Fatalf("local cache must answer first, got hit=%v source=%q", hit.Hit, source)
	}
	if hit.Entry.Source != "run/local" {
		t.Errorf("expected local evidence, got %q", hit.Entry.Source)
	}

	// Disabled store: lookup is unchanged from the current local-only behavior.
	if !(SharedStore{}).Disabled() {
		t.Fatal("a zero shared store must be disabled")
	}
	empty := New()
	if hit, src := (SharedStore{}).Lookup(empty, id); hit.Hit || src != SourceLocal {
		t.Fatalf("disabled store must miss locally, got hit=%v src=%q", hit.Hit, src)
	}
}

// TestStoreSharedFromEnvDefaultOff proves the opt-in shared location is off
// unless SOP_VERIFY_CACHE_SHARED is set, and resolves a directory to the
// shared verify-cache.json.
func TestStoreSharedFromEnvDefaultOff(t *testing.T) {
	getenv := func(k string) string {
		if k == SharedEnv {
			return ""
		}
		return ""
	}
	if s := SharedFromEnv(getenv); !s.Disabled() {
		t.Errorf("shared store must be off by default, got %q", s.Path)
	}

	dir := t.TempDir()
	set := func(k string) string {
		if k == SharedEnv {
			return dir
		}
		return ""
	}
	s := SharedFromEnv(set)
	if s.Disabled() {
		t.Fatal("configured shared store must be enabled")
	}
	if filepath.Base(s.Path) != SharedStoreFileName {
		t.Errorf("directory location should resolve to %s, got %q", SharedStoreFileName, s.Path)
	}
}

// TestStoreSharedFailClosed covers the three fail-closed modes: an unreadable
// store, a schema mismatch, and a corrupt entry each yield a miss, never a hit.
func TestStoreSharedFailClosed(t *testing.T) {
	dir := t.TempDir()

	// Unreadable: the path names a directory, so os.ReadFile fails.
	unreadable := SharedStore{Path: dir}
	if hit, _ := unreadable.Lookup(New(), sharedID("head1", "inputs-abc", "VER-002")); hit.Hit {
		t.Error("an unreadable shared store must yield a miss")
	}

	// Schema mismatch: the store declares an older schema, so Unmarshal drops it.
	schemaPath := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(schemaPath, []byte(`{"schema":"verifcache/1","entries":[{"identity":{"schema":"verifcache/1","repository":"r","command":"c","environment":"e","verifier":"v"},"digest":"d","passed":true,"verdict":"VERIFIED"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if hit, _ := (SharedStore{Path: schemaPath}).Lookup(New(), sharedID("head1", "inputs-abc", "VER-002")); hit.Hit {
		t.Error("a schema mismatch must yield a miss")
	}

	// Corrupt entry: unparseable JSON must be a miss.
	corruptPath := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corruptPath, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if hit, _ := (SharedStore{Path: corruptPath}).Lookup(New(), sharedID("head1", "inputs-abc", "VER-002")); hit.Hit {
		t.Error("a corrupt shared store must yield a miss")
	}

	// A corrupted digest inside an otherwise-readable entry must also miss rather
	// than fabricate a hit for the requested identity.
	fabricated := SharedStore{Path: filepath.Join(dir, "fabricated.json")}
	id := sharedID("head1", "inputs-abc", "VER-002")
	data := `{"schema":"verifcache/2","entries":[{"identity":{"schema":"verifcache/2","repository":"x","inputs":"y","command":"z","environment":"e","command_policy":"p","scope":"s","verifier":"v"},"digest":"deadbeef","passed":true,"verdict":"VERIFIED"}]}`
	if err := os.WriteFile(fabricated.Path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if hit, _ := fabricated.Lookup(New(), id); hit.Hit {
		t.Error("a mismatched shared entry must yield a miss")
	}
}

// TestReuseDecisionRecordAppendedForHitAndMiss proves exactly one append-only
// JSONL record is written per lookup decision, carrying hit/miss, key digest,
// scope, and source run.
func TestReuseDecisionRecordAppendedForHitAndMiss(t *testing.T) {
	dir := t.TempDir()
	path := DecisionPath(dir, "S001")
	id := sharedID("head1", "inputs-abc", "VER-002")

	miss := NewDecisionRecord(id, false, SourceShared, "S001", Entry{}, "identity mismatch")
	if err := AppendDecision(path, miss); err != nil {
		t.Fatalf("append miss: %v", err)
	}
	hit := NewDecisionRecord(id, true, SourceShared, "S001", Entry{Source: "run/A"}, "")
	if err := AppendDecision(path, hit); err != nil {
		t.Fatalf("append hit: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want exactly 2 append-only records, got %d: %q", len(lines), string(data))
	}
	for i, line := range lines {
		if !strings.Contains(line, `"key_digest":"`+id.Digest()+`"`) {
			t.Errorf("record %d must carry the key digest: %s", i, line)
		}
		if !strings.Contains(line, `"scope":"`) || !strings.Contains(line, `"source_run":"S001"`) {
			t.Errorf("record %d must carry scope and source run: %s", i, line)
		}
	}
	if !strings.Contains(lines[0], `"hit":false`) || !strings.Contains(lines[1], `"hit":true`) {
		t.Errorf("records must record hit/miss: %q", string(data))
	}

	// Appending again must not rewrite earlier records: the record is append-only.
	if err := AppendDecision(path, miss); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(after), string(data)) {
		t.Error("appending must preserve earlier decision records")
	}
}
