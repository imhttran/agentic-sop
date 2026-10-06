package promptcache

import (
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

func id() Identity {
	return Identity{
		Schema:     SchemaVersion,
		Capability: string(agent.Plan),
		Prompt:     "digest-p",
		Context:    "digest-c",
		Model:      "ollama/qwen",
		Parameters: "temp=0",
		Compiler:   "prompt-compiler/1",
	}
}

// TestEligibleExcludesMutations proves only read-only capabilities are cacheable.
func TestEligibleExcludesMutations(t *testing.T) {
	for _, c := range []agent.Capability{agent.Plan, agent.Review, agent.DiagnoseFailure, agent.DesignTests} {
		if !Eligible(c) {
			t.Errorf("%s must be eligible", c)
		}
	}
	for _, c := range []agent.Capability{agent.Implement, agent.Fix} {
		if Eligible(c) {
			t.Errorf("%s must not be eligible", c)
		}
	}
}

// TestStoreRefusesMutatingCapability proves repository work is never cached.
func TestStoreRefusesMutatingCapability(t *testing.T) {
	c := New()
	in := id()
	in.Capability = string(agent.Implement)
	if _, err := c.Store(in, "x", "agent"); err == nil {
		t.Error("storing a mutating capability must fail")
	}
	if c.Len() != 0 {
		t.Errorf("cache len = %d, want 0", c.Len())
	}
}

// TestDigestSensitiveToEveryField proves any changed input is a different key.
func TestDigestSensitiveToEveryField(t *testing.T) {
	base := id().Digest()
	for name, mutate := range map[string]func(*Identity){
		"schema":     func(i *Identity) { i.Schema = "other" },
		"capability": func(i *Identity) { i.Capability = string(agent.Review) },
		"prompt":     func(i *Identity) { i.Prompt = "other" },
		"context":    func(i *Identity) { i.Context = "other" },
		"model":      func(i *Identity) { i.Model = "other" },
		"parameters": func(i *Identity) { i.Parameters = "other" },
		"compiler":   func(i *Identity) { i.Compiler = "other" },
	} {
		in := id()
		mutate(&in)
		if in.Digest() == base {
			t.Errorf("changing %s did not change the digest", name)
		}
	}
}

// TestRunCachesReadOnlyAndAlwaysRunsMutating proves mutation work is never replayed.
func TestRunCachesReadOnlyAndAlwaysRunsMutating(t *testing.T) {
	c := New()
	calls := 0
	gen := func() (string, error) { calls++; return "out", nil }
	if _, hit, _ := c.Run(id(), gen); hit {
		t.Error("first read-only run must miss")
	}
	if s, hit, _ := c.Run(id(), gen); !hit || s != "out" {
		t.Errorf("second read-only run = %q,%v, want hit", s, hit)
	}
	if calls != 1 {
		t.Errorf("read-only generate ran %d times, want 1", calls)
	}
	m := id()
	m.Capability = string(agent.Implement)
	for i := 0; i < 2; i++ {
		if _, hit, _ := c.Run(m, gen); hit {
			t.Error("a mutating capability must never hit")
		}
	}
	if calls != 3 {
		t.Errorf("generate ran %d times total, want 3", calls)
	}
	if c.Len() != 1 {
		t.Errorf("cache len = %d, want 1 (read-only only)", c.Len())
	}
}

// TestRunDoesNotCacheErrors proves a failed generation is never cached.
func TestRunDoesNotCacheErrors(t *testing.T) {
	c := New()
	calls := 0
	gen := func() (string, error) { calls++; return "", errors.New("boom") }
	for i := 0; i < 2; i++ {
		if _, _, err := c.Run(id(), gen); err == nil {
			t.Error("expected error")
		}
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
	if c.Len() != 0 {
		t.Errorf("len = %d, want 0", c.Len())
	}
}

// TestStaleStateRejected proves a changed identity is a miss.
func TestStaleStateRejected(t *testing.T) {
	c := New()
	if _, err := c.Store(id(), "out", "agent"); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Identity){
		"prompt":   func(i *Identity) { i.Prompt = "other" },
		"context":  func(i *Identity) { i.Context = "other" },
		"model":    func(i *Identity) { i.Model = "other" },
		"compiler": func(i *Identity) { i.Compiler = "other" },
	} {
		in := id()
		mutate(&in)
		if got := c.Lookup(in); got.Hit {
			t.Errorf("%s: stale identity produced a hit", name)
		}
	}
}

// TestMarshalDeterministicAndRoundTrip proves order-independent, stable serialization.
func TestMarshalDeterministicAndRoundTrip(t *testing.T) {
	a, b := New(), New()
	if _, err := a.Store(id(), "one", "agent"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Store(id(), "one", "agent"); err != nil {
		t.Fatal(err)
	}
	da, _ := a.Marshal()
	db, _ := b.Marshal()
	if string(da) != string(db) {
		t.Errorf("marshal is not deterministic:\n%s\n%s", da, db)
	}
	loaded, err := Unmarshal(da)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got := loaded.Lookup(id()); !got.Hit || got.Entry.Content != "one" {
		t.Errorf("round-trip lookup = %+v", got)
	}
}

// TestUnmarshalRejectsIncompatibleSchema proves a schema bump invalidates entries.
func TestUnmarshalRejectsIncompatibleSchema(t *testing.T) {
	c, err := Unmarshal([]byte(`{"schema":"other","entries":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Len() != 0 {
		t.Errorf("incompatible schema must load empty, got %d", c.Len())
	}
}

// TestUnmarshalRecomputesCorruptDigest proves a corrupt digest is not trusted.
func TestUnmarshalRecomputesCorruptDigest(t *testing.T) {
	c, err := Unmarshal([]byte(`{"schema":"promptcache/1","entries":[{"identity":{"schema":"promptcache/1","capability":"PLAN","prompt":"digest-p","context":"digest-c","model":"ollama/qwen","parameters":"temp=0","compiler":"prompt-compiler/1"},"digest":"deadbeef","content":"x","source":"agent"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Lookup(id()); !got.Hit || got.Entry.Digest == "deadbeef" || got.Entry.Digest != id().Digest() {
		t.Errorf("corrupt digest was trusted: %+v", got)
	}
}

// TestUnmarshalDropsMutatingEntries proves a crafted file cannot reintroduce mutations.
func TestUnmarshalDropsMutatingEntries(t *testing.T) {
	c, err := Unmarshal([]byte(`{"schema":"promptcache/1","entries":[{"identity":{"schema":"promptcache/1","capability":"IMPLEMENT","prompt":"digest-p","context":"digest-c","model":"ollama/qwen","parameters":"temp=0","compiler":"prompt-compiler/1"},"digest":"x","content":"x","source":"agent"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Len() != 0 {
		t.Errorf("mutating entry was loaded, len = %d", c.Len())
	}
}
