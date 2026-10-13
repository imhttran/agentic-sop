package verifcache

import "testing"

// fullID is a fully-populated identity exercising every component.
func fullID() Identity {
	return Identity{
		Schema:        SchemaVersion,
		Repository:    RepositoryIdentity("head1", false, ""),
		Inputs:        EffectiveInputIdentity([]Input{{Path: "internal/verifcache/verifcache.go", Hash: "h1"}}),
		Command:       CommandIdentity([]string{"go test ./internal/verifcache/..."}),
		Environment:   EnvironmentIdentity(map[string]string{"PATH": "/usr/bin", "toolchain": ToolchainIdentity("go1.27.1", "linux", "amd64")}),
		CommandPolicy: CommandPolicyIdentity([]string{"SAFE"}),
		Scope:         ScopeIdentity([]string{"VER-001", "internal/verifcache"}),
		Verifier:      "sop/validate@test",
	}
}

// TestIdentityNewComponentsParticipate proves each added component participates in the key
// and in equivalence.
func TestIdentityNewComponentsParticipate(t *testing.T) {
	base := fullID()
	mutations := map[string]func(*Identity){
		"repository":     func(i *Identity) { i.Repository = "other" },
		"inputs":         func(i *Identity) { i.Inputs = "other" },
		"command":        func(i *Identity) { i.Command = "other" },
		"environment":    func(i *Identity) { i.Environment = "other" },
		"command_policy": func(i *Identity) { i.CommandPolicy = "other" },
		"scope":          func(i *Identity) { i.Scope = "other" },
		"verifier":       func(i *Identity) { i.Verifier = "other" },
	}
	for name, mutate := range mutations {
		in := base
		mutate(&in)
		if in.Digest() == base.Digest() {
			t.Errorf("changing %s did not change the digest", name)
		}
		if Equivalent(in, base) {
			t.Errorf("changing %s did not break equivalence", name)
		}
	}
}

// TestEquivalentIsReflexiveAndPure proves equivalence is reflexive and sensitive.
func TestEquivalentIsReflexiveAndPure(t *testing.T) {
	a, b := fullID(), fullID()
	if !Equivalent(a, a) || !Equivalent(a, b) {
		t.Fatal("identical identities must be equivalent")
	}
	b.Scope = "different"
	if Equivalent(a, b) {
		t.Fatal("a different scope must not be equivalent")
	}
}

// TestCrossWorktreeEquivalenceWhenInputsIdentical proves two worktrees are equivalent when
// revision and effective inputs are demonstrably identical, and not otherwise. The identity
// carries no worktree path, so location never enters the comparison.
func TestCrossWorktreeEquivalenceWhenInputsIdentical(t *testing.T) {
	inputs := []Input{{Path: "pkg/a.go", Hash: "abc"}, {Path: "pkg/b.go", Hash: "def"}}
	a, b := fullID(), fullID()
	a.Repository = RepositoryIdentity("head1", false, "")
	b.Repository = RepositoryIdentity("head1", false, "")
	a.Inputs = EffectiveInputIdentity(inputs)
	b.Inputs = EffectiveInputIdentity(inputs)
	if !Equivalent(a, b) {
		t.Fatal("identical revision + effective inputs must be cross-worktree equivalent")
	}
	b.Inputs = EffectiveInputIdentity([]Input{{Path: "pkg/a.go", Hash: "CHANGED"}, {Path: "pkg/b.go", Hash: "def"}})
	if Equivalent(a, b) {
		t.Fatal("a changed effective input must break cross-worktree equivalence")
	}
}

// TestDecideFailClosed proves reuse is granted only for an exact, VERIFIED match.
func TestDecideFailClosed(t *testing.T) {
	cur := fullID()
	verified := Entry{Identity: cur, Digest: cur.Digest(), Passed: true, Verdict: VerdictVerified}

	if d := Decide(Entry{}, cur); d.Reusable {
		t.Error("no prior evidence must not be reusable")
	}
	if d := Decide(verified, cur); !d.Reusable {
		t.Errorf("exact VERIFIED match must be reusable: %+v", d)
	}
	for _, v := range []Verdict{VerdictFailed, VerdictPartial, VerdictHold, VerdictIncomplete, VerdictUntrusted, ""} {
		e := verified
		e.Verdict = v
		if d := Decide(e, cur); d.Reusable {
			t.Errorf("verdict %q must never be reusable as PASS", string(v))
		}
	}
	other := cur
	other.Inputs = "changed"
	if d := Decide(verified, other); d.Reusable {
		t.Error("an identity mismatch must not be reusable")
	}
}

// TestCacheReusableOnlyVerified proves the fail-closed read path never yields PASS for
// evidence that was not stored VERIFIED.
func TestCacheReusableOnlyVerified(t *testing.T) {
	c := New()
	id := fullID()
	c.Store(id, Result{Passed: true, Source: "s"}) // no verdict recorded
	if _, ok := c.Reusable(id); ok {
		t.Error("evidence without a VERIFIED verdict must not be reusable")
	}
	c.Store(id, Result{Passed: true, Source: "s", Verdict: VerdictVerified})
	if e, ok := c.Reusable(id); !ok || e.Verdict != VerdictVerified {
		t.Errorf("VERIFIED evidence must be reusable, got %+v ok=%v", e, ok)
	}
	other := id
	other.Scope = "other"
	if _, ok := c.Reusable(other); ok {
		t.Error("a different scope must not be reusable")
	}
}

// TestEffectiveInputIdentityDeterministic proves the effective-input identity is
// order- and duplicate-independent and content-sensitive.
func TestEffectiveInputIdentityDeterministic(t *testing.T) {
	a := EffectiveInputIdentity([]Input{{Path: "b.go", Hash: "2"}, {Path: "a.go", Hash: "1"}, {Path: "b.go", Hash: "2"}})
	b := EffectiveInputIdentity([]Input{{Path: "a.go", Hash: "1"}, {Path: "b.go", Hash: "2"}})
	if a != b {
		t.Error("order or duplicates changed the effective-input identity")
	}
	if EffectiveInputIdentity([]Input{{Path: "a.go", Hash: "CHANGED"}}) == EffectiveInputIdentity([]Input{{Path: "a.go", Hash: "1"}}) {
		t.Error("a content change must change the effective-input identity")
	}
	if EffectiveInputIdentity([]Input{{Path: "  ", Hash: "x"}}) != EffectiveInputIdentity(nil) {
		t.Error("blank paths must be ignored")
	}
}

// TestScopePolicyToolchainIdentities proves the scope, command-policy, and toolchain
// identities are deterministic set/field identities.
func TestScopePolicyToolchainIdentities(t *testing.T) {
	if ScopeIdentity([]string{"b", "a", "a"}) != ScopeIdentity([]string{"a", "b"}) {
		t.Error("scope identity must be set-like")
	}
	if ScopeIdentity([]string{"a"}) == ScopeIdentity([]string{"a", "b"}) {
		t.Error("different scopes must differ")
	}
	if CommandPolicyIdentity([]string{"SAFE", "DENIED"}) != CommandPolicyIdentity([]string{"DENIED", "SAFE"}) {
		t.Error("policy identity must be set-like")
	}
	if ToolchainIdentity("go1.27.1", "linux", "amd64") == ToolchainIdentity("go1.27.2", "linux", "amd64") {
		t.Error("a toolchain version change must change the identity")
	}
}

// TestUnmarshalDropsPriorSchemaEntries proves a schema bump fails closed: entries written
// under the older key are dropped, never reused under the wider key.
func TestUnmarshalDropsPriorSchemaEntries(t *testing.T) {
	c, err := Unmarshal([]byte(`{"schema":"verifcache/1","entries":[{"identity":{"schema":"verifcache/1","repository":"r","command":"c","environment":"e","verifier":"v"},"digest":"d","passed":true,"verdict":"VERIFIED"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Len() != 0 {
		t.Errorf("entries from an older schema must be dropped (fail closed), got %d", c.Len())
	}
}
