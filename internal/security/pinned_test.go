package security

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
)

type countingExecutor struct{ runs int }

func (c *countingExecutor) Run(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
	c.runs++
	return nil, nil, 0, nil
}

type recordingExecutor struct{ argv0 string }

func (r *recordingExecutor) Run(_ context.Context, _ string, argv []string, _ []byte, _ int) ([]byte, []byte, int, error) {
	r.argv0 = argv[0]
	return nil, nil, 0, nil
}

// LLM04 (PROPOSAL-SCANNER-BINARY-PINS option B): a configured binary or rules
// pin is checked before every execution, and a failed check never starts the
// scanner. Without pins BuildAdapter keeps today's executor.
func TestPinnedExecutor(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "scanner")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	binHash, _ := fileSHA256(bin)
	rules := filepath.Join(dir, "rules")
	if err := os.MkdirAll(rules, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rules, "r.yaml"), []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rulesHash, _ := rulesIdentity(rules)

	run := func(p pinnedExecutor, argv0 string) (int, error) {
		inner := &countingExecutor{}
		p.inner = inner
		_, _, _, err := p.Run(context.Background(), dir, []string{argv0, "--version"}, nil, 0)
		return inner.runs, err
	}

	if n, err := run(pinnedExecutor{binarySHA256: binHash, rules: rules, rulesSHA256: rulesHash}, bin); err != nil || n != 1 {
		t.Fatalf("matching pins: runs=%d err=%v, want one run", n, err)
	}
	if n, err := run(pinnedExecutor{binarySHA256: binHash}, filepath.Join(dir, "missing")); !errors.Is(err, ErrUnavailable) || n != 0 {
		t.Fatalf("missing executable: runs=%d err=%v, want ErrUnavailable and no run", n, err)
	}

	// The rules path resolves against the scan dir, like the adapters do.
	if n, err := run(pinnedExecutor{rules: "rules", rulesSHA256: rulesHash}, bin); err != nil || n != 1 {
		t.Fatalf("relative rules path: runs=%d err=%v, want one run", n, err)
	}
	// A symlink the digest would skip, but the scanner would follow, fails closed.
	if err := os.Symlink(bin, filepath.Join(rules, "extra.yaml")); err != nil {
		t.Fatal(err)
	}
	if n, err := run(pinnedExecutor{rules: rules, rulesSHA256: rulesHash}, bin); err == nil || !strings.Contains(err.Error(), "not a regular file") || n != 0 {
		t.Fatalf("symlink in rules: runs=%d err=%v, want refusal and no run", n, err)
	}
	if err := os.Remove(filepath.Join(rules, "extra.yaml")); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(rules, "r.yaml"), []byte("rules: [] \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if n, err := run(pinnedExecutor{rules: rules, rulesSHA256: rulesHash}, bin); err == nil || !strings.Contains(err.Error(), "rules pin mismatch") || n != 0 {
		t.Fatalf("changed rules: runs=%d err=%v, want mismatch and no run", n, err)
	}

	// A relative command resolves against the scan dir (the child's Dir), not
	// this process's cwd, and the hashed absolute path is what runs.
	rec := &recordingExecutor{}
	if _, _, _, err := (pinnedExecutor{inner: rec, binarySHA256: binHash}).Run(context.Background(), dir, []string{"./scanner"}, nil, 0); err != nil || rec.argv0 != bin {
		t.Fatalf("relative command: argv0=%q err=%v, want %q", rec.argv0, err, bin)
	}

	// A same-size one-byte replacement of the binary fails closed.
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho OK\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if n, err := run(pinnedExecutor{binarySHA256: binHash}, bin); err == nil || !strings.Contains(err.Error(), "binary pin mismatch") || n != 0 {
		t.Fatalf("replaced binary: runs=%d err=%v, want mismatch and no run", n, err)
	}

	// BuildAdapter wraps only when a pin is set, and the pin moves the resume identity.
	raw := config.SecurityAdapterConfig{ID: "gitleaks", Category: "secrets", Command: []string{bin}}
	plain := adapterConfigDigest(raw)
	a, err := BuildAdapter(raw, BuildOptions{Executor: &countingExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, wrapped := a.(*gitleaksAdapter).exec.(pinnedExecutor); wrapped {
		t.Fatal("an unpinned adapter must keep the plain executor")
	}
	raw.BinarySHA256 = binHash
	if adapterConfigDigest(raw) == plain {
		t.Fatal("a binary pin must change the adapter config digest")
	}
	if a, err = BuildAdapter(raw, BuildOptions{Executor: &countingExecutor{}}); err != nil {
		t.Fatal(err)
	}
	if _, wrapped := a.(*gitleaksAdapter).exec.(pinnedExecutor); !wrapped {
		t.Fatal("a pinned adapter must use the pinned executor")
	}
}
