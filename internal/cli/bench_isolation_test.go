package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestBenchmarkIsolationPreflightIsFailClosed proves the benchmark isolation
// preflight refuses a bad checkout target BEFORE any write, so a misconfigured
// setup can never modify the source repository (the failure mode that was
// observed once during benchmarking).
func TestBenchmarkIsolationPreflightIsFailClosed(t *testing.T) {
	script := filepath.Join("..", "..", "scripts", "bench", "isolate.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("isolation script not found: %v", err)
	}

	src := t.TempDir()
	gitEnv := append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = src
		cmd.Env = gitEnv
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(src, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	git("init", "-b", "main")
	write("sentinel.txt", "keep\n")
	git("add", "sentinel.txt")
	git("commit", "-m", "init")
	// An uncommitted change that must survive every refused preflight.
	write("dirty.txt", "dirty\n")

	before := benchTreeState(t, src)

	invoke := func(dst string) error {
		cmd := exec.Command("sh", script, src, dst)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("isolate.sh refused target (expected): %s", out)
		}
		return err
	}

	if err := invoke(filepath.Join(t.TempDir(), "missing-parent", "t")); err == nil {
		t.Error("preflight must fail when the target parent does not exist")
	}
	if err := invoke(filepath.Join(src, "inside")); err == nil {
		t.Error("preflight must refuse a target inside the source repository")
	}
	if err := invoke(src); err == nil {
		t.Error("preflight must refuse the source repository itself as the target")
	}

	if after := benchTreeState(t, src); after != before {
		t.Fatalf("a refused preflight modified the source repository:\nbefore:\n%s\nafter:\n%s", before, after)
	}

	// A valid target is copied, and the source is still unchanged.
	destParent := t.TempDir()
	dest := filepath.Join(destParent, "checkout")
	if err := invoke(dest); err != nil {
		t.Fatalf("preflight rejected a valid target: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, ".git")); err != nil {
		t.Errorf("isolated checkout is not a git repository: %v", err)
	}
	if after := benchTreeState(t, src); after != before {
		t.Fatalf("a successful preflight modified the source repository:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// benchTreeState snapshots a repository's file list and working-tree status, so a
// test can prove a refused setup left it byte-for-byte unchanged.
func benchTreeState(t *testing.T, dir string) string {
	t.Helper()
	var b []byte
	for _, args := range [][]string{
		{"-c", "core.quotepath=false", "status", "--porcelain"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		b = append(b, out...)
	}
	find := exec.Command("find", ".", "-type", "f", "-not", "-path", "./.git/*")
	find.Dir = dir
	out, err := find.CombinedOutput()
	if err != nil {
		t.Fatalf("find: %v: %s", err, out)
	}
	b = append(b, out...)
	return string(b)
}
