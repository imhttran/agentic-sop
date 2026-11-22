package egmmanifest_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/egmmanifest"
)

func TestCanonicalKeyOrderStable(t *testing.T) {
	a := map[string]any{"a": 1, "b": map[string]any{"y": 2, "x": 3}}
	b := map[string]any{"b": map[string]any{"x": 3, "y": 2}, "a": 1}
	ba, err := egmmanifest.Canonical(a)
	if err != nil {
		t.Fatal(err)
	}
	bb, err := egmmanifest.Canonical(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ba, bb) {
		t.Fatalf("key order changed canonical bytes:\n%s\n%s", ba, bb)
	}
}

func TestCanonicalDistinguishesContent(t *testing.T) {
	a, _ := egmmanifest.Digest(map[string]any{"a": 1})
	b, _ := egmmanifest.Digest(map[string]any{"a": 2})
	if a == b {
		t.Fatal("different content produced the same digest")
	}
}

func TestCanonicalNumberLiteral(t *testing.T) {
	b, err := egmmanifest.Canonical(map[string]any{"n": 1})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"n":1}` {
		t.Fatalf("canonical = %s, want {\"n\":1}", b)
	}
}

func TestCanonicalNoHTMLEscape(t *testing.T) {
	b, err := egmmanifest.Canonical(map[string]any{"s": "a<b&c"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "a<b&c") {
		t.Fatalf("HTML characters were escaped: %s", b)
	}
}

func TestSealAndCaseDigest(t *testing.T) {
	m, _ := newFixture(t)
	c := m.Cases[0]
	d, err := egmmanifest.CaseDigest(c)
	if err != nil {
		t.Fatal(err)
	}
	if d != c.SealedSHA256 {
		t.Fatalf("case digest %s != sealed %s", d, c.SealedSHA256)
	}
	changed := c
	changed.DeterminabilityRationale = "different"
	d2, _ := egmmanifest.CaseDigest(changed)
	if d2 == d {
		t.Fatal("changed content produced the same digest")
	}
}
