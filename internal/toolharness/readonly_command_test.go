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

// readonlyFixture builds a primary root A, an external root B declared read-only,
// and an "outside" directory C that is authorized by nobody. B contains an
// unformatted Go module (so read-only gates run) and a symlink to C. None of the
// names are a real repository or task.
func readonlyFixture(t *testing.T) (h *Harness, primary, external, outside string) {
	t.Helper()
	base := t.TempDir()
	primary = filepath.Join(base, "primary")
	external = filepath.Join(base, "external")
	outside = filepath.Join(base, "outside")
	for _, d := range []string{primary, external, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFixture(t, filepath.Join(external, "go.mod"), "module fixture\n\ngo 1.21\n")
	writeFixture(t, filepath.Join(external, "bad.go"), "package fixture\n\nfunc  F(){}\n")
	writeFixture(t, filepath.Join(external, "ok.go"), "package fixture\n")
	writeFixture(t, filepath.Join(outside, "victim.go"), "package victim\n\nfunc  G(){}\n")
	if err := os.Symlink(outside, filepath.Join(external, "link")); err != nil {
		t.Fatal(err)
	}
	cfg := Config{CommandTimeout: 60 * time.Second, MaxOutputBytes: 1 << 20,
		Roots: []Root{{Path: external, Mode: RootReadOnly}}}
	return New(primary, cfg, nil), primary, external, outside
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadOnlyRootRejectsGofmtWrite(t *testing.T) {
	h, _, external, _ := readonlyFixture(t)
	ctx := context.Background()
	bad := filepath.Join(external, "bad.go")
	before, _ := os.ReadFile(bad)

	_, obs, err := h.RunObservedMutation(ctx, ToolRunCommand, map[string]any{"command": "gofmt -w bad.go", "cwd": external})
	if err == nil {
		t.Fatal("gofmt -w was allowed in a read-only root")
	}
	if !errors.Is(err, errReadOnlyRoot) {
		t.Fatalf("err = %v, want errReadOnlyRoot", err)
	}
	if after, _ := os.ReadFile(bad); string(before) != string(after) {
		t.Fatal("gofmt -w modified a file in a read-only root")
	}
	if obs.Changed {
		t.Fatal("a denied read-only command was counted as a mutation")
	}
}

func TestReadOnlyRootRejectsRelativeTraversalArgument(t *testing.T) {
	h, _, external, outside := readonlyFixture(t)
	ctx := context.Background()
	victim := filepath.Join(outside, "victim.go")
	before, _ := os.ReadFile(victim)

	_, _, err := h.RunObservedMutation(ctx, ToolRunCommand, map[string]any{"command": "gofmt -w ../outside/victim.go", "cwd": external})
	if err == nil {
		t.Fatal("a relative traversal argument escaped the read-only root")
	}
	if after, _ := os.ReadFile(victim); string(before) != string(after) {
		t.Fatal("a relative traversal argument modified a file outside the root")
	}
}

func TestReadOnlyRootRejectsAbsoluteEscapeArgument(t *testing.T) {
	h, _, external, outside := readonlyFixture(t)
	ctx := context.Background()
	victim := filepath.Join(outside, "victim.go")

	_, _, err := h.RunObservedMutation(ctx, ToolRunCommand, map[string]any{"command": "gofmt -w " + victim, "cwd": external})
	if err == nil {
		t.Fatal("an absolute path outside the read-only root was allowed")
	}
}

func TestReadOnlyRootRejectsSymlinkEscapeArgument(t *testing.T) {
	h, _, external, outside := readonlyFixture(t)
	ctx := context.Background()
	victim := filepath.Join(outside, "victim.go")
	before, _ := os.ReadFile(victim)

	_, _, err := h.RunObservedMutation(ctx, ToolRunCommand, map[string]any{"command": "gofmt -w link/victim.go", "cwd": external})
	if err == nil {
		t.Fatal("a symlink argument escaped the read-only root")
	}
	if after, _ := os.ReadFile(victim); string(before) != string(after) {
		t.Fatal("a symlink argument modified a file outside the root")
	}
}

func TestReadOnlyRootAllowsReadOnlyGoCommands(t *testing.T) {
	h, _, external, _ := readonlyFixture(t)
	ctx := context.Background()
	for _, command := range []string{"gofmt -l .", "go build ./...", "go vet ./...", "go test ./..."} {
		out, err := h.Run(ctx, ToolRunCommand, map[string]any{"command": command, "cwd": external})
		if err != nil {
			t.Fatalf("%q must be allowed in a read-only Go repository: %v", command, err)
		}
		if strings.HasPrefix(out, "exit 0") == false {
			t.Fatalf("%q did not exit 0: %q", command, out)
		}
	}
	out, err := h.Run(ctx, ToolRunCommand, map[string]any{"command": "gofmt -l .", "cwd": external})
	if err != nil || !strings.Contains(out, "bad.go") {
		t.Fatalf("gofmt -l . should report the unformatted file: %q err=%v", out, err)
	}
}

func TestPrimaryRootStillAllowsGofmtWrite(t *testing.T) {
	h, primary, _, _ := readonlyFixture(t)
	ctx := context.Background()
	writeFixture(t, filepath.Join(primary, "p.go"), "package p\n\nfunc  H(){}\n")
	_, _, err := h.RunObservedMutation(ctx, ToolRunCommand, map[string]any{"command": "gofmt -w p.go"})
	if err != nil {
		t.Fatalf("gofmt -w in the read-write primary root must stay allowed: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(primary, "p.go")); string(b) != "package p\n\nfunc H() {}\n" {
		t.Fatalf("primary gofmt -w did not format the file: %q", b)
	}
}
