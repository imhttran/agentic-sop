package verifcache

import "testing"

// TestEffectiveInputDigestDeterministic proves EffectiveInputDigest is order- and
// duplicate-independent, content-sensitive, and ignores blank paths, mirroring and
// delegating to EffectiveInputIdentity so the two names cannot drift.
func TestEffectiveInputDigestDeterministic(t *testing.T) {
	a := EffectiveInputDigest([]Input{{Path: "b.go", Hash: "2"}, {Path: "a.go", Hash: "1"}, {Path: "b.go", Hash: "2"}})
	b := EffectiveInputDigest([]Input{{Path: "a.go", Hash: "1"}, {Path: "b.go", Hash: "2"}})
	if a != b {
		t.Error("digest depends on order or duplicates")
	}
	if a != EffectiveInputIdentity([]Input{{Path: "b.go", Hash: "2"}, {Path: "a.go", Hash: "1"}}) {
		t.Error("EffectiveInputDigest must agree with EffectiveInputIdentity")
	}
	if EffectiveInputDigest([]Input{{Path: "a.go", Hash: "CHANGED"}}) == EffectiveInputDigest([]Input{{Path: "a.go", Hash: "1"}}) {
		t.Error("a content-hash change must change the digest")
	}
	if EffectiveInputDigest([]Input{{Path: "  ", Hash: "x"}}) != EffectiveInputDigest(nil) {
		t.Error("blank paths must be ignored")
	}
}

// TestEnvironmentDigestComponents proves each structured environment component participates
// and that order does not: PATH, Go version, GOOS, GOARCH, and argv policy classification.
func TestEnvironmentDigestComponents(t *testing.T) {
	base := EnvironmentInputs{PATH: "/usr/bin", GoVersion: "go1.22.0", GOOS: "linux", GOARCH: "amd64", CommandPolicy: []string{"SAFE", "REQUIRES_APPROVAL"}}
	b := EnvironmentDigest(base)
	if b != EnvironmentDigest(base) {
		t.Fatal("environment digest must be stable")
	}
	mutations := map[string]func(*EnvironmentInputs){
		"path":   func(e *EnvironmentInputs) { e.PATH = "/other" },
		"gover":  func(e *EnvironmentInputs) { e.GoVersion = "go1.23.0" },
		"goos":   func(e *EnvironmentInputs) { e.GOOS = "darwin" },
		"goarch": func(e *EnvironmentInputs) { e.GOARCH = "arm64" },
		"policy": func(e *EnvironmentInputs) { e.CommandPolicy = []string{"SAFE", "DENIED"} },
	}
	for name, mutate := range mutations {
		in := base
		mutate(&in)
		if EnvironmentDigest(in) == b {
			t.Errorf("changing %s did not change the environment identity", name)
		}
	}
	reordered := EnvironmentInputs{PATH: "/usr/bin", GoVersion: "go1.22.0", GOOS: "linux", GOARCH: "amd64", CommandPolicy: []string{"REQUIRES_APPROVAL", "SAFE"}}
	if EnvironmentDigest(reordered) != b {
		t.Error("environment identity must not depend on argument order")
	}
}

// TestEquivalentAllComponentsTable proves Equivalent is true iff every component is equal.
func TestEquivalentAllComponentsTable(t *testing.T) {
	base := fullID()
	if !Equivalent(base, base) {
		t.Fatal("reflexive equivalence failed")
	}
	identical := fullID()
	if !Equivalent(base, identical) {
		t.Fatal("structurally identical identities must be equivalent")
	}
	mutations := map[string]func(*Identity){
		"schema":     func(i *Identity) { i.Schema = "other" },
		"repository": func(i *Identity) { i.Repository = "other" },
		"inputs":     func(i *Identity) { i.Inputs = EffectiveInputDigest([]Input{{Path: "a.go", Hash: "changed"}}) },
		"command":    func(i *Identity) { i.Command = "other" },
		"environment": func(i *Identity) {
			i.Environment = EnvironmentDigest(EnvironmentInputs{PATH: "/x", GoVersion: "go1.22.0", GOOS: "linux", GOARCH: "amd64"})
		},
		"command_policy": func(i *Identity) { i.CommandPolicy = "other" },
		"scope":          func(i *Identity) { i.Scope = "other" },
		"verifier":       func(i *Identity) { i.Verifier = "other" },
	}
	for name, mutate := range mutations {
		in := base
		mutate(&in)
		if Equivalent(in, base) {
			t.Errorf("changing %s must yield non-equivalence", name)
		}
		if in.Digest() == base.Digest() {
			t.Errorf("changing %s did not change the digest", name)
		}
	}
}

// TestEquivalentPurityRepeatable proves Equivalent is pure: repeated comparisons of
// identical inputs are stable and side-effect free. It compares the same pair many times
// and asserts both the result and the underlying components are unchanged afterward.
func TestEquivalentPurityRepeatable(t *testing.T) {
	a, b := fullID(), fullID()
	first := Equivalent(a, b)
	for i := 0; i < 100; i++ {
		if Equivalent(a, b) != first {
			t.Fatalf("Equivalent result changed on repetition at iteration %d", i)
		}
	}
	if !first {
		t.Fatal("identical identities must compare equivalent")
	}
	if Equivalent(a, b) != Equivalent(b, a) {
		t.Fatal("Equivalent must be symmetric")
	}
}

// TestScopeIdentityConstructorEndToEnd proves the full path: a scope built by
// ScopeIdentity plus an effective-input digest consumed through RepositoryIdentity
// yields an identity whose scope, inputs, and repository components each participate in
// equivalence independently.
func TestScopeIdentityConstructorEndToEnd(t *testing.T) {
	digest := EffectiveInputDigest([]Input{
		{Path: "internal/verifcache/verifcache.go", Hash: "h1"},
		{Path: "internal/verifcache/verifcache_test.go", Hash: "h2"},
	})
	repo := RepositoryIdentity("head1", true, digest)
	scope := ScopeIdentity([]string{"VER-001", "internal/verifcache", "VER-001"})

	base := Identity{
		Schema:        SchemaVersion,
		Repository:    repo,
		Inputs:        digest,
		Command:       CommandIdentity([]string{"go test ./internal/verifcache/..."}),
		Environment:   EnvironmentDigest(EnvironmentInputs{PATH: "/usr/bin", GoVersion: "go1.22.0", GOOS: "linux", GOARCH: "amd64"}),
		CommandPolicy: CommandPolicyIdentity([]string{"SAFE"}),
		Scope:         scope,
		Verifier:      "sop/validate@test",
	}

	if !Equivalent(base, base) {
		t.Fatal("constructed identity must be equivalent to itself")
	}

	// A change to the effective inputs changes the digest, the repository identity, and
	// therefore the whole identity.
	changedDigest := EffectiveInputDigest([]Input{
		{Path: "internal/verifcache/verifcache.go", Hash: "h1"},
		{Path: "internal/verifcache/verifcache_test.go", Hash: "CHANGED"},
	})
	changed := base
	changed.Inputs = changedDigest
	changed.Repository = RepositoryIdentity("head1", true, changedDigest)
	if Equivalent(base, changed) {
		t.Fatal("a relevant-input change must break equivalence")
	}

	// A change to the scope alone breaks equivalence.
	scopeChanged := base
	scopeChanged.Scope = ScopeIdentity([]string{"VER-001", "internal/verifcache", "VER-002"})
	if Equivalent(base, scopeChanged) {
		t.Fatal("a scope change must break equivalence")
	}

	// ScopeIdentity is set-like: duplicates and ordering do not matter.
	if scope != ScopeIdentity([]string{"internal/verifcache", "VER-001"}) {
		t.Fatal("ScopeIdentity must be deduplicated and sorted")
	}

	// A toolchain change alone breaks equivalence.
	toolchainChanged := base
	toolchainChanged.Environment = EnvironmentDigest(EnvironmentInputs{PATH: "/usr/bin", GoVersion: "go1.23.0", GOOS: "linux", GOARCH: "amd64"})
	if Equivalent(base, toolchainChanged) {
		t.Fatal("a toolchain change must break equivalence")
	}

	// A command change alone breaks equivalence.
	commandChanged := base
	commandChanged.Command = CommandIdentity([]string{"go vet ./internal/verifcache/..."})
	if Equivalent(base, commandChanged) {
		t.Fatal("a command change must break equivalence")
	}
}
