package verifcache

import "testing"

func id() Identity {
	return Identity{
		Schema:      SchemaVersion,
		Repository:  "repo-abc",
		Command:     "go test ./...",
		Environment: "env-1",
		Verifier:    "sop/testrunner@1",
	}
}

// TestIdentityDigestStableAndSensitive proves each field participates in the key.
func TestIdentityDigestStableAndSensitive(t *testing.T) {
	base := id().Digest()
	if base != id().Digest() {
		t.Fatal("digest is not stable")
	}
	mutations := map[string]func(*Identity){
		"schema":      func(i *Identity) { i.Schema = "other" },
		"repository":  func(i *Identity) { i.Repository = "repo-def" },
		"command":     func(i *Identity) { i.Command = "go build ./..." },
		"environment": func(i *Identity) { i.Environment = "env-2" },
		"verifier":    func(i *Identity) { i.Verifier = "sop/testrunner@2" },
	}
	for name, mutate := range mutations {
		in := id()
		mutate(&in)
		if in.Digest() == base {
			t.Errorf("changing %s did not change the digest", name)
		}
	}
}

// TestLookupMissThenHit proves an exact identity match is the only hit.
func TestLookupMissThenHit(t *testing.T) {
	c := New()
	if got := c.Lookup(id()); got.Hit {
		t.Fatal("empty cache returned a hit")
	}
	c.Store(id(), Result{Passed: true, Output: "ok", Source: "validate"})
	got := c.Lookup(id())
	if !got.Hit || !got.Entry.Passed || got.Entry.Source != "validate" || got.Entry.Digest != id().Digest() {
		t.Fatalf("lookup = %+v", got)
	}
}

// TestStaleStateRejected proves any changed field is a miss, never a stale hit.
func TestStaleStateRejected(t *testing.T) {
	c := New()
	c.Store(id(), Result{Passed: true})
	for name, mutate := range map[string]func(*Identity){
		"repository":  func(i *Identity) { i.Repository = "other" },
		"command":     func(i *Identity) { i.Command = "other" },
		"environment": func(i *Identity) { i.Environment = "other" },
		"verifier":    func(i *Identity) { i.Verifier = "other" },
		"schema":      func(i *Identity) { i.Schema = "other" },
	} {
		in := id()
		mutate(&in)
		if got := c.Lookup(in); got.Hit {
			t.Errorf("%s: stale identity produced a hit", name)
		}
	}
}

// TestRunCachesAndReexecutes proves verification runs once per distinct state.
func TestRunCachesAndReexecutes(t *testing.T) {
	c := New()
	calls := 0
	verify := func() Result { calls++; return Result{Passed: true, Source: "validate"} }
	if _, hit := c.Run(id(), verify); hit {
		t.Error("first run must not be a hit")
	}
	if _, hit := c.Run(id(), verify); !hit {
		t.Error("second run must be a hit")
	}
	if calls != 1 {
		t.Errorf("verify ran %d times, want 1", calls)
	}
	other := id()
	other.Repository = "changed"
	if _, hit := c.Run(other, verify); hit {
		t.Error("a changed repository must re-run verification")
	}
	if calls != 2 {
		t.Errorf("verify ran %d times, want 2", calls)
	}
}

// TestMarshalDeterministicAndRoundTrip proves order-independent, stable serialization.
func TestMarshalDeterministicAndRoundTrip(t *testing.T) {
	a, b := New(), New()
	a.Store(id(), Result{Passed: true, Output: "x", Source: "s"})
	i2 := id()
	i2.Command = "go build ./..."
	a.Store(i2, Result{Passed: false})
	b.Store(i2, Result{Passed: false})
	b.Store(id(), Result{Passed: true, Output: "x", Source: "s"})
	da, _ := a.Marshal()
	db, _ := b.Marshal()
	if string(da) != string(db) {
		t.Errorf("marshal is not order-independent:\n%s\n%s", da, db)
	}
	loaded, err := Unmarshal(da)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if loaded.Len() != 2 {
		t.Errorf("loaded %d entries, want 2", loaded.Len())
	}
	if got := loaded.Lookup(id()); !got.Hit || !got.Entry.Passed || got.Entry.Source != "s" {
		t.Errorf("round-trip lookup = %+v", got)
	}
}

// TestUnmarshalRejectsIncompatibleSchema proves a schema bump invalidates every entry.
func TestUnmarshalRejectsIncompatibleSchema(t *testing.T) {
	c, err := Unmarshal([]byte(`{"schema":"other","entries":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Len() != 0 {
		t.Errorf("incompatible schema must load empty, got %d", c.Len())
	}
}

// TestUnmarshalCannotFabricateAHit proves a corrupt digest is recomputed, not trusted.
func TestUnmarshalCannotFabricateAHit(t *testing.T) {
	c, err := Unmarshal([]byte(`{"schema":"verifcache/1","entries":[{"identity":{"schema":"verifcache/1","repository":"repo-abc","command":"go test ./...","environment":"env-1","verifier":"sop/testrunner@1"},"digest":"deadbeef","passed":true,"source":"s"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Lookup(id()); !got.Hit || got.Entry.Digest == "deadbeef" || got.Entry.Digest != id().Digest() {
		t.Errorf("corrupt digest was trusted: %+v", got)
	}
}

// TestRepositoryIdentitySensitive proves state changes are detected.
func TestRepositoryIdentitySensitive(t *testing.T) {
	base := RepositoryIdentity("head1", false, "c1")
	if base != RepositoryIdentity("head1", false, "c1") {
		t.Fatal("not stable")
	}
	if RepositoryIdentity("head2", false, "c1") == base {
		t.Error("head change not detected")
	}
	if RepositoryIdentity("head1", true, "c1") == base {
		t.Error("dirty change not detected")
	}
	if RepositoryIdentity("head1", false, "c2") == base {
		t.Error("content change not detected")
	}
}

// TestCommandIdentityIgnoresOrderAndDuplicates proves set semantics.
func TestCommandIdentityIgnoresOrderAndDuplicates(t *testing.T) {
	a := CommandIdentity([]string{"go build ./...", "go test ./...", "go build ./..."})
	b := CommandIdentity([]string{"go test ./...", "go build ./..."})
	if a != b {
		t.Error("command identity depends on order or duplicates")
	}
	if CommandIdentity([]string{"go build ./..."}) == a {
		t.Error("different command sets must differ")
	}
	if CommandIdentity(nil) != CommandIdentity([]string{"  "}) {
		t.Error("blank commands must be ignored")
	}
}

// TestEnvironmentIdentitySorted proves iteration order does not leak.
func TestEnvironmentIdentitySorted(t *testing.T) {
	a := EnvironmentIdentity(map[string]string{"A": "1", "B": "2"})
	b := EnvironmentIdentity(map[string]string{"B": "2", "A": "1"})
	if a != b {
		t.Error("environment identity depends on iteration order")
	}
	if EnvironmentIdentity(map[string]string{"A": "2", "B": "2"}) == a {
		t.Error("different environments must differ")
	}
}
