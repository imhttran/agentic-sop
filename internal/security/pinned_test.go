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

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	writeFile(t, path, content)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

// LLM04 (PROPOSAL-SCANNER-BINARY-PINS option B): a configured binary pin is
// checked before every execution and a failed check never starts the scanner.
// Without a pin BuildAdapter keeps today's executor.
func TestPinnedExecutor(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "scanner")
	writeExecutable(t, bin, "#!/bin/sh\necho ok\n")
	binHash, _ := fileSHA256(bin)

	run := func(argv0 string) (int, error) {
		inner := &countingExecutor{}
		_, _, _, err := (pinnedExecutor{inner: inner, binarySHA256: binHash}).Run(context.Background(), dir, []string{argv0, "--version"}, nil, 0)
		return inner.runs, err
	}
	if n, err := run(bin); err != nil || n != 1 {
		t.Fatalf("matching pin: runs=%d err=%v, want one run", n, err)
	}
	if n, err := run(filepath.Join(dir, "missing")); !errors.Is(err, ErrUnavailable) || n != 0 {
		t.Fatalf("missing executable: runs=%d err=%v, want ErrUnavailable and no run", n, err)
	}

	// A relative command resolves against the scan dir (the child's Dir), not
	// this process's cwd, and the hashed absolute path is what runs.
	rec := &recordingExecutor{}
	if _, _, _, err := (pinnedExecutor{inner: rec, binarySHA256: binHash}).Run(context.Background(), dir, []string{"./scanner"}, nil, 0); err != nil || rec.argv0 != bin {
		t.Fatalf("relative command: argv0=%q err=%v, want %q", rec.argv0, err, bin)
	}

	// A same-size one-byte replacement fails closed.
	writeExecutable(t, bin, "#!/bin/sh\necho OK\n")
	if n, err := run(bin); err == nil || !strings.Contains(err.Error(), "binary pin mismatch") || n != 0 {
		t.Fatalf("replaced binary: runs=%d err=%v, want mismatch and no run", n, err)
	}

	// BuildAdapter wraps only when a binary pin is set, and the pin moves the resume identity.
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

// The rules pin is checked inside the adapter against the exact path it hands
// the scanner (relative paths resolve against the workspace root, not the
// process cwd or the executor dir), and a symlink the digest would skip fails.
func TestRulesPin(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "rules", "r.yaml"), "rules: []\n")
	pin, _ := rulesIdentity(filepath.Join(root, "rules"))

	scan := func(rulesPin string) (Execution, int) {
		inner := &countingExecutor{}
		a := &semgrepAdapter{id: "semgrep", category: CategorySAST, rules: "rules", rulesPin: rulesPin, exec: inner}
		exec, _ := a.Scan(context.Background(), Request{WorkspaceRoot: root})
		return exec, inner.runs
	}
	pinFailed := func(e Execution) bool {
		return e.Status == StatusFailed && len(e.Diagnostics) > 0 && strings.Contains(e.Diagnostics[0], "rules pin")
	}

	if e, _ := scan(pin); pinFailed(e) {
		t.Fatalf("matching relative rules pin was refused: %+v", e.Diagnostics)
	}
	if e, n := scan(strings.Repeat("0", 64)); !pinFailed(e) || n != 0 {
		t.Fatalf("wrong pin: status=%s diag=%v runs=%d, want pin failure and no run", e.Status, e.Diagnostics, n)
	}
	writeFile(t, filepath.Join(root, "outside.yaml"), "rules: [x]\n")
	if err := os.Symlink(filepath.Join(root, "outside.yaml"), filepath.Join(root, "rules", "extra.yaml")); err != nil {
		t.Fatal(err)
	}
	if e, n := scan(pin); !pinFailed(e) || !strings.Contains(e.Diagnostics[0], "not a regular file") || n != 0 {
		t.Fatalf("symlinked rule: status=%s diag=%v runs=%d, want refusal and no run", e.Status, e.Diagnostics, n)
	}
}
