package toolharness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRewriteFileIsExactAndIdempotent proves the deterministic, idempotent import
// rewrite: an exact string is replaced everywhere, a second run is a no-op, and a
// file containing neither string is refused.
func TestRewriteFileIsExactAndIdempotent(t *testing.T) {
	root := t.TempDir()
	h := newTestHarness(t, root, nil)
	ctx := context.Background()

	const before = "package sopclient\n\nimport (\n\t\"testing\"\n\n\t\"sop-controller/internal/config\"\n)\n"
	writeRepoFile(t, root, "internal/controller/sopclient/x_test.go", before)

	const from = `"sop-controller/internal/config"`
	const to = `"github.com/imhttran/agentic-sop/internal/controller/config"`

	out, err := h.Run(ctx, ToolRewriteFile, map[string]any{"path": "internal/controller/sopclient/x_test.go", "from": from, "to": to})
	if err != nil || !strings.Contains(out, "rewrote 1") {
		t.Fatalf("first rewrite = (%q, %v)", out, err)
	}
	got, _ := os.ReadFile(filepath.Join(root, "internal/controller/sopclient/x_test.go"))
	if strings.Contains(string(got), from) || !strings.Contains(string(got), to) {
		t.Fatalf("rewrite did not apply exactly: %q", string(got))
	}
	if out, err := h.Run(ctx, ToolRewriteFile, map[string]any{"path": "internal/controller/sopclient/x_test.go", "from": from, "to": to}); err != nil || !strings.Contains(out, "already rewritten") {
		t.Fatalf("second rewrite = (%q, %v), want an idempotent no-op", out, err)
	}
	after, _ := os.ReadFile(filepath.Join(root, "internal/controller/sopclient/x_test.go"))
	if string(after) != string(got) {
		t.Errorf("the second run changed the file")
	}
	writeRepoFile(t, root, "other.go", "package other\n")
	if _, err := h.Run(ctx, ToolRewriteFile, map[string]any{"path": "other.go", "from": from, "to": to}); err == nil {
		t.Error("rewriting a file containing neither string must fail")
	}
}

// TestCopyFileIsDeterministicCrossRootImport proves the governed import primitive:
// copy_file copies one file byte-for-byte from an authorized read-only root into the
// writable root, leaves the source unchanged, and errors on a missing source (never
// an empty copy).
func TestCopyFileIsDeterministicCrossRootImport(t *testing.T) {
	base := t.TempDir()
	primary := filepath.Join(base, "primary")
	sibling := filepath.Join(base, "sibling")
	for _, d := range []string{primary, sibling} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Content a model might "improve" (double spaces, a tab, //-only lines): a
	// deterministic copy must reproduce it exactly.
	const src = "package decision\n\n//  deliberate  spacing and\ttab\n//\nfunc mustRead() {} // keep the // line\n"
	if err := os.WriteFile(filepath.Join(sibling, "request.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.Roots = []Root{{Path: sibling, Mode: RootReadOnly}}
	h := New(primary, cfg, nil)
	ctx := context.Background()

	if _, err := h.Run(ctx, ToolCopyFile, map[string]any{"source": "request.go", "source_root": sibling, "dest": "decision/request.go"}); err != nil {
		t.Fatalf("copy_file: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(primary, "decision", "request.go")); err != nil || string(b) != src {
		t.Fatalf("copied content = (%q, %v), want byte-exact", b, err)
	}
	if after, _ := os.ReadFile(filepath.Join(sibling, "request.go")); string(after) != src {
		t.Errorf("source mutated")
	}
	if _, err := h.Run(ctx, ToolCopyFile, map[string]any{"source": "missing.go", "source_root": sibling, "dest": "decision/missing.go"}); err == nil {
		t.Error("copying a missing source must fail, not write an empty file")
	}
}

// TestNoDirectoryToolFileCollisionIsActionable is the regression for the observed
// file/directory collision: a model that calls create_file expecting a directory
// learns, from an actionable error, that these tools create regular files only and
// that write_file creates missing parents. A failed write is never credited.
func TestNoDirectoryToolFileCollisionIsActionable(t *testing.T) {
	root := t.TempDir()
	h := newTestHarness(t, root, nil)
	ctx := context.Background()

	if _, err := h.Run(ctx, ToolCreateFile, map[string]any{"path": "decision", "content": ""}); err != nil {
		t.Fatalf("create_file decision: %v", err)
	}
	if info, err := os.Stat(filepath.Join(root, "decision")); err != nil || info.IsDir() {
		t.Fatalf("decision should be a regular file: info=%v err=%v", info, err)
	}

	// A directory path (trailing slash) is refused, never silently turned into a
	// regular file.
	if _, err := h.Run(ctx, ToolCreateFile, map[string]any{"path": "decision/", "content": "x"}); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("trailing-slash create_file: err=%v, want a directory-path refusal", err)
	}

	// Writing a file under the collision cannot create the directory: the error is
	// actionable and the failed write is not credited as progress.
	_, obs, err := h.RunObservedMutation(ctx, ToolWriteFile, map[string]any{"path": "decision/errors.go", "content": "package decision\n"})
	if err == nil {
		t.Fatal("write under a file-named path must fail")
	}
	if !strings.Contains(err.Error(), "a file already exists at") {
		t.Errorf("collision error = %q, want it to name the conflicting file", err)
	}
	if obs.Succeeded || obs.Changed {
		t.Errorf("a failed write must not be credited: %+v", obs)
	}
	if _, err := os.Stat(filepath.Join(root, "decision", "errors.go")); err == nil {
		t.Errorf("failed write left a side effect: the child exists")
	}
}

// TestWriteFileCreatesParentDirectories proves the supported way to add files in a
// new directory: write the file directly; no directory tool is needed.
func TestWriteFileCreatesParentDirectories(t *testing.T) {
	root := t.TempDir()
	h := newTestHarness(t, root, nil)

	if _, err := h.Run(context.Background(), ToolWriteFile, map[string]any{"path": "decision/errors.go", "content": "package decision\n"}); err != nil {
		t.Fatalf("write_file into a new directory: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(root, "decision", "errors.go"))
	if err != nil || string(b) != "package decision\n" {
		t.Fatalf("nested write = (%q, %v)", b, err)
	}
}

// TestCrossRootReadToPrimaryWrite is the regression for a cross-root import: read
// an authorized read-only sibling file and write its content into the primary
// root, with the source left byte-for-byte unchanged and the sibling still
// read-only.
func TestCrossRootReadToPrimaryWrite(t *testing.T) {
	base := t.TempDir()
	primary := filepath.Join(base, "primary")
	sibling := filepath.Join(base, "sibling")
	for _, d := range []string{primary, sibling} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	const src = "package decision\n\n// cross-root marker\n"
	if err := os.WriteFile(filepath.Join(sibling, "result.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.Roots = []Root{{Path: sibling, Mode: RootReadOnly}}
	h := New(primary, cfg, nil)
	ctx := context.Background()

	got, err := h.Run(ctx, ToolReadFile, map[string]any{"path": "result.go", "root": sibling})
	if err != nil || got != src {
		t.Fatalf("authorized sibling read = (%q, %v)", got, err)
	}
	if _, err := h.Run(ctx, ToolWriteFile, map[string]any{"path": "decision/result.go", "content": got}); err != nil {
		t.Fatalf("primary write: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(primary, "decision", "result.go"))
	if err != nil || string(b) != src {
		t.Fatalf("primary copy = (%q, %v), want the sibling content", b, err)
	}
	if after, _ := os.ReadFile(filepath.Join(sibling, "result.go")); string(after) != src {
		t.Errorf("sibling source mutated: %q", string(after))
	}
	if _, err := h.Run(ctx, ToolWriteFile, map[string]any{"path": "new.txt", "content": "x", "root": sibling}); err == nil {
		t.Error("the read-only sibling root accepted a write")
	}
}
