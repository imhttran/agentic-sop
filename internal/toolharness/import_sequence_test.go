package toolharness

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestControllerImportSequenceIsIdempotent is the end-to-end regression for the
// controller-import task sequence: copy_file (byte-exact) -> rewrite_file (the
// documented import rewrite) -> go build/test/vet. It runs the whole sequence TWICE
// in a disposable fixture and asserts the second execution lands in exactly the same,
// still-passing state (the copy restores the original bytes; the idempotent rewrite
// reapplies), so re-running the import is safe.
func TestControllerImportSequenceIsIdempotent(t *testing.T) {
	src := t.TempDir()
	writeRepoFile(t, src, "internal/config/config.go", "package config\n\nconst Name = \"ok\"\n")
	writeRepoFile(t, src, "internal/sopclient/x_test.go",
		"package sopclient\n\nimport (\n\t\"testing\"\n\n\t\"sop-controller/internal/config\"\n)\n\nfunc TestX(t *testing.T) {\n\tif config.Name != \"ok\" {\n\t\tt.Fatal(\"bad\")\n\t}\n}\n")

	primary := t.TempDir()
	writeRepoFile(t, primary, "go.mod", "module example.com/ctrlimport\n\ngo 1.27\n")

	cfg := DefaultConfig()
	cfg.Roots = []Root{{Path: src, Mode: RootReadOnly}}
	h := New(primary, cfg, nil)
	ctx := context.Background()

	const from = `"sop-controller/internal/config"`
	const to = `"example.com/ctrlimport/internal/config"`

	run := func() {
		t.Helper()
		for _, f := range []string{"internal/config/config.go", "internal/sopclient/x_test.go"} {
			if _, err := h.Run(ctx, ToolCopyFile, map[string]any{"source": f, "source_root": src, "dest": f}); err != nil {
				t.Fatalf("copy %s: %v", f, err)
			}
		}
		if _, err := h.Run(ctx, ToolRewriteFile, map[string]any{"path": "internal/sopclient/x_test.go", "from": from, "to": to}); err != nil {
			t.Fatalf("rewrite: %v", err)
		}
	}

	verify := func(label string) string {
		t.Helper()
		for _, cmd := range [][]string{{"go", "build", "./..."}, {"go", "test", "./..."}, {"go", "vet", "./..."}} {
			c := exec.Command(cmd[0], cmd[1:]...)
			c.Dir = primary
			c.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
			if b, err := c.CombinedOutput(); err != nil {
				t.Fatalf("%s: %v %v: %s", label, cmd, err, b)
			}
		}
		b, _ := os.ReadFile(filepath.Join(primary, "internal", "sopclient", "x_test.go"))
		return string(b)
	}

	run()
	first := verify("run1")
	if strings.Contains(first, from) || !strings.Contains(first, to) {
		t.Fatalf("run1: rewrite not applied: %q", first)
	}

	run()
	second := verify("run2")
	if first != second {
		t.Errorf("the second execution changed the result:\n%q\n%q", first, second)
	}
}
