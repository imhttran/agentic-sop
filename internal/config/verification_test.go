package config

import "testing"

// The verification block is additive and OFF by default (HARDEN-001-c): an
// omitted block leaves existing behavior unchanged.
func TestVerificationDefaultsOff(t *testing.T) {
	c, err := Parse([]byte("project:\n  name: x\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Verification.Enforce || len(c.Verification.Bindings) != 0 {
		t.Fatalf("default verification = %+v, want off/empty", c.Verification)
	}
}

// The operator-owned bindings and the temporary enforce switch parse.
func TestVerificationParses(t *testing.T) {
	doc := "project:\n  name: x\n" +
		"verification:\n" +
		"  enforce: true\n" +
		"  bindings:\n" +
		"    - id: app-starts\n" +
		"      command: \"go test ./...\"\n" +
		"      output_must_contain: ok\n"
	c, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if !c.Verification.Enforce || len(c.Verification.Bindings) != 1 {
		t.Fatalf("verification = %+v", c.Verification)
	}
	b := c.Verification.Bindings[0]
	if b.ID != "app-starts" || b.Command != "go test ./..." || b.OutputMustContain != "ok" {
		t.Fatalf("binding = %+v", b)
	}
}
