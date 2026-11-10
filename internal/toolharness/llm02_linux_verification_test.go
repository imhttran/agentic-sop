package toolharness

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	llm02LinuxCanary   = "sk-llm02-linux-verification-not-a-real-secret"
	llm02LinuxOrdinary = "llm02-linux-ordinary-marker"
)

// TestLLM02LinuxPositiveControlsAndSymlinkDenial supplements the approved A+B
// enforcement test with the Linux verification cases that were not already
// covered: a permitted copy, a permitted SAFE command output, and symlink
// variants of the search and copy bypasses. The synthetic canary is a
// disposable placeholder, not a credential.
func TestLLM02LinuxPositiveControlsAndSymlinkDenial(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write(".env", "SOP_TEST_API_KEY="+llm02LinuxCanary+"\n")
	write(".env.example", "SOP_TEST_API_KEY=replace-me\n")
	write("notes.txt", llm02LinuxOrdinary+" ordinary repository content\n")

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init", "-q")
	runGit("add", "notes.txt")

	h := New(root, DefaultConfig(), NewAuditLog(0))
	ctx := context.Background()

	// Positive control: an ordinary copy still works and produces exact bytes.
	if _, err := h.Run(ctx, ToolCopyFile, map[string]any{"source": "notes.txt", "dest": "copied-notes.txt"}); err != nil {
		t.Fatalf("copy_file ordinary source: %v", err)
	}
	copied, err := os.ReadFile(filepath.Join(root, "copied-notes.txt"))
	if err != nil || !strings.Contains(string(copied), llm02LinuxOrdinary) {
		t.Fatalf("copied ordinary content = %q, err=%v", copied, err)
	}

	// Positive control: a permitted SAFE command still returns ordinary output.
	out, err := h.Run(ctx, ToolRunCommand, map[string]any{"command": "git show :notes.txt"})
	if err != nil || !strings.Contains(out, llm02LinuxOrdinary) {
		t.Fatalf("run_command ordinary SAFE output = %q, err=%v", out, err)
	}

	// Positive control: the documented template file remains readable.
	out, err = h.Run(ctx, ToolReadFile, map[string]any{"path": ".env.example"})
	if err != nil || !strings.Contains(out, "replace-me") {
		t.Fatalf("read_file .env.example = %q, err=%v", out, err)
	}

	// Symlink bypass: a search must not surface a credential through an
	// ordinary-looking link name. The walk resolves the file target, so the
	// credential path remains excluded.
	if err := os.Symlink(".env", filepath.Join(root, "env-link.txt")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	out, err = h.Run(ctx, ToolSearchFiles, map[string]any{"pattern": "SOP_TEST_API_KEY"})
	if err != nil {
		t.Fatalf("search_files after symlink: %v", err)
	}
	if strings.Contains(out, llm02LinuxCanary) {
		t.Fatalf("search_files leaked a credential through a symlink:\n%s", out)
	}

	// Symlink bypass: copy_file resolves the source, so a link to .env is denied
	// and no ordinary-name copy is created.
	if _, err := h.Run(ctx, ToolCopyFile, map[string]any{"source": "env-link.txt", "dest": "copied-env.txt"}); !errors.Is(err, ErrSensitivePath) {
		t.Fatalf("copy_file through a .env symlink err = %v, want ErrSensitivePath", err)
	}
	if _, err := os.Stat(filepath.Join(root, "copied-env.txt")); !os.IsNotExist(err) {
		t.Fatalf("copy_file created copied-env.txt through a symlink despite the denial")
	}
}
