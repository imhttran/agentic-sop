package toolharness

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixture builds a primary root and a sibling root, both git-free temp trees, and
// returns a harness that authorizes the sibling with the given mode. The concrete
// paths are fixtures, never a real repository or task name.
func rootsFixture(t *testing.T, mode RootMode) (h *Harness, primary, sibling string) {
	t.Helper()
	base := t.TempDir()
	primary = filepath.Join(base, "primary")
	sibling = filepath.Join(base, "sibling")
	for _, d := range []string{primary, sibling} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(primary, "a.txt"), []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sibling, "b.txt"), []byte("beta"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{CommandTimeout: 30 * time.Second, MaxOutputBytes: 1 << 20, Roots: []Root{{Path: sibling, Mode: mode}}}
	return New(primary, cfg, nil), primary, sibling
}

func TestAuthorizedRootRead(t *testing.T) {
	h, _, sibling := rootsFixture(t, RootReadOnly)
	ctx := context.Background()

	out, err := h.Run(ctx, ToolListFiles, map[string]any{"root": sibling})
	if err != nil || !strings.Contains(out, "b.txt") {
		t.Fatalf("list_files on authorized root: out=%q err=%v", out, err)
	}
	out, err = h.Run(ctx, ToolReadFile, map[string]any{"path": "b.txt", "root": sibling})
	if err != nil || strings.TrimSpace(out) != "beta" {
		t.Fatalf("read_file on authorized root: out=%q err=%v", out, err)
	}
	out, err = h.Run(ctx, ToolSearchFiles, map[string]any{"pattern": "beta", "root": sibling})
	if err != nil || !strings.Contains(out, "b.txt:1: beta") {
		t.Fatalf("search_files on authorized root: out=%q err=%v", out, err)
	}
	// A command with cwd in the sibling runs there (gofmt is allow-listed).
	if err := os.WriteFile(filepath.Join(sibling, "z.go"), []byte("package b\nfunc  F(){}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = h.Run(ctx, ToolRunCommand, map[string]any{"command": "gofmt -l .", "cwd": sibling})
	if err != nil || !strings.Contains(out, "z.go") {
		t.Fatalf("run_command with cwd in authorized root: out=%q err=%v", out, err)
	}
}

func TestUnauthorizedRootFailsClosed(t *testing.T) {
	h, primary, _ := rootsFixture(t, RootReadOnly)
	ctx := context.Background()
	stranger := filepath.Join(filepath.Dir(primary), "stranger")
	if err := os.MkdirAll(stranger, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stranger, "s.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Run(ctx, ToolReadFile, map[string]any{"path": "s.txt", "root": stranger}); !errors.Is(err, errUnauthorizedRoot) {
		t.Fatalf("reading an unauthorized root: err=%v, want errUnauthorizedRoot", err)
	}
	// An absolute path outside every authorized root is refused even without a root selector.
	if _, err := h.Run(ctx, ToolReadFile, map[string]any{"path": filepath.Join(stranger, "s.txt")}); err == nil {
		t.Fatal("absolute path outside the primary root should be refused")
	}
}

func TestRootTraversalEscapeDenied(t *testing.T) {
	h, _, sibling := rootsFixture(t, RootReadOnly)
	ctx := context.Background()
	// ../ from the sibling reaches into the primary root: it must not.
	if _, err := h.Run(ctx, ToolReadFile, map[string]any{"path": "../primary/a.txt", "root": sibling}); err == nil {
		t.Fatal("traversal from an authorized root escaped its boundary")
	}
	// Absolute sibling path inside the primary root is likewise out of bounds.
	if _, err := h.Run(ctx, ToolReadFile, map[string]any{"path": "/etc/hosts", "root": sibling}); err == nil {
		t.Fatal("absolute path escaped the authorized root")
	}
}

func TestReadOnlyRootWriteDenied(t *testing.T) {
	h, _, sibling := rootsFixture(t, RootReadOnly)
	ctx := context.Background()
	if _, err := h.Run(ctx, ToolWriteFile, map[string]any{"path": "new.txt", "content": "x", "root": sibling}); !errors.Is(err, errReadOnlyRoot) {
		t.Fatalf("write to read-only root: err=%v, want errReadOnlyRoot", err)
	}
	if _, err := os.Stat(filepath.Join(sibling, "new.txt")); err == nil {
		t.Fatal("read-only root was modified")
	}
	// A denied external write must not be observed as progress.
	before, _ := h.RepositoryFingerprint(ctx)
	if _, err := h.Run(ctx, ToolWriteFile, map[string]any{"path": "new.txt", "content": "x", "root": sibling}); err == nil {
		t.Fatal("second write should also be denied")
	}
	after, _ := h.RepositoryFingerprint(ctx)
	if before != after {
		t.Fatal("denied external write changed the primary repository")
	}
}

func TestPrimaryWriteStillWorks(t *testing.T) {
	h, primary, _ := rootsFixture(t, RootReadOnly)
	ctx := context.Background()
	if _, err := h.Run(ctx, ToolWriteFile, map[string]any{"path": "out.txt", "content": "ok"}); err != nil {
		t.Fatalf("primary write failed: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(primary, "out.txt")); err != nil || string(b) != "ok" {
		t.Fatalf("primary file: %q err=%v", b, err)
	}
	obs := &MutationObservation{}
	_, obs2, err := h.RunObservedMutation(ctx, ToolWriteFile, map[string]any{"path": "out2.txt", "content": "ok"})
	if err != nil {
		t.Fatalf("RunObservedMutation: %v", err)
	}
	_ = obs
	if !obs2.Succeeded || !obs2.Verified || !obs2.Changed {
		t.Fatalf("primary mutation not observed: %+v", obs2)
	}
}

func TestSummarizeRequestRecordsRootAndCwd(t *testing.T) {
	_, _, sibling := rootsFixture(t, RootReadOnly)
	got := SummarizeRequest(ToolRunCommand, map[string]any{"command": "go list ./...", "cwd": sibling})
	if !strings.Contains(got, "cwd="+sibling) || !strings.Contains(got, "go list") {
		t.Fatalf("run_command summary = %q, want cwd and command", got)
	}
	got = SummarizeRequest(ToolReadFile, map[string]any{"path": "b.txt", "root": sibling})
	if !strings.Contains(got, "root="+sibling) || !strings.Contains(got, "path=b.txt") {
		t.Fatalf("read_file summary = %q, want root and path", got)
	}
}

func TestExternalCommandDoesNotMutatePrimaryRepository(t *testing.T) {
	h, _, sibling := rootsFixture(t, RootReadOnly)
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(sibling, "z.go"), []byte("package b\nfunc  F(){}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, obs, err := h.RunObservedMutation(ctx, ToolRunCommand, map[string]any{"command": "gofmt -l .", "cwd": sibling})
	if err != nil {
		t.Fatalf("RunObservedMutation: %v", err)
	}
	if !obs.Succeeded {
		t.Fatalf("external read command should succeed: %+v", obs)
	}
	if obs.Changed {
		t.Fatalf("an external-root command must not be attributed as a primary mutation: %+v", obs)
	}
}
