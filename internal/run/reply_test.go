package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A known-format secret in a model reply is redacted before it is written; an
// ordinary reply is written unchanged, and Write keeps evidence verbatim.
func TestWriteReplyRedactsKnownFormatSecret(t *testing.T) {
	const shaped = "sk-reply-canary-not-a-real-secret"
	r, err := New(t.TempDir(), "reply-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.WriteReply("implementation.md", "token="+shaped+" done"); err != nil {
		t.Fatal(err)
	}
	if err := r.WriteReply("result.md", "plain reply"); err != nil {
		t.Fatal(err)
	}
	if err := r.Write("diff.patch", "token="+shaped); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(r.Dir(), name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if strings.Contains(read("implementation.md"), shaped) {
		t.Fatal("a known-format secret was written to a reply file")
	}
	if read("result.md") != "plain reply" {
		t.Fatalf("an ordinary reply changed: %q", read("result.md"))
	}
	if !strings.Contains(read("diff.patch"), shaped) {
		t.Fatal("evidence written with Write was altered")
	}
}
