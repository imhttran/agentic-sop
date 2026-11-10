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
	llm08PathCanary = "sk-llm08-path-canary-not-a-real-secret"
	llm08DiffCanary = "sk-llm08-diff-canary-not-a-real-secret"
	llm08Ordinary   = "ordinary-source-marker"
)

// TestCredentialPathDenialAndBoundaryDivision is the harness-level enforcement
// test for the approved A+B policy. It proves the B half: direct reads, searches,
// and copy sources are denied before sensitive content is read. It also pins the
// division of responsibility: git_diff and SAFE command output still contain the
// raw bytes at this boundary; the model-bound redaction in ollamaagent is tested
// separately. Ordinary source, .env.example, and .env.sample remain usable.
func TestCredentialPathDenialAndBoundaryDivision(t *testing.T) {
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
	write(".env", "SOP_TEST_API_KEY="+llm08PathCanary+"\n")
	write(".env.example", "SOP_TEST_API_KEY=replace-me\n")
	write(".env.sample", "SOP_TEST_API_KEY=replace-me\n")
	write("config/.env", "SOP_TEST_API_KEY="+llm08PathCanary+"\n")
	write("credentials.json", `{"SOP_TEST_API_KEY":"`+llm08PathCanary+`"}`)
	write("service.key", llm08PathCanary+"\n")
	write(".ssh/id_rsa", llm08PathCanary+"\n")
	write("notes.txt", llm08Ordinary+" ordinary repository content\n")
	write("internal/secrets/manager.go", "package secrets\n\nconst Name = \"manager\"\n")
	write("tracked-diff.txt", "before\n")
	write("credentials-tracked.txt", "SOP_TEST_API_KEY="+llm08PathCanary+"\n")

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init", "-q")
	runGit("add", "notes.txt", "internal/secrets/manager.go", "tracked-diff.txt", "credentials-tracked.txt")
	write("tracked-diff.txt", "after token="+llm08DiffCanary+"\n")

	audit := NewAuditLog(0)
	h := New(root, DefaultConfig(), audit)
	ctx := context.Background()

	// B: direct reads of credential paths are denied before content is read.
	for _, path := range []string{".env", "config/.env", "credentials.json", "service.key", ".ssh/id_rsa"} {
		out, err := h.Run(ctx, ToolReadFile, map[string]any{"path": path})
		if !errors.Is(err, ErrSensitivePath) {
			t.Fatalf("read_file %q err = %v, want ErrSensitivePath", path, err)
		}
		if out != "" {
			t.Fatalf("read_file %q returned content despite the denial: %q", path, out)
		}
	}

	// Positive controls: template files and ordinary source remain readable. The
	// generic secrets source directory is deliberately outside the narrow profile.
	for _, tc := range []struct{ path, want string }{
		{".env.example", "replace-me"},
		{".env.sample", "replace-me"},
		{"notes.txt", llm08Ordinary},
		{"internal/secrets/manager.go", "package secrets"},
	} {
		out, err := h.Run(ctx, ToolReadFile, map[string]any{"path": tc.path})
		if err != nil {
			t.Fatalf("read_file %q: %v", tc.path, err)
		}
		if !strings.Contains(out, tc.want) {
			t.Fatalf("read_file %q = %q, want %q", tc.path, out, tc.want)
		}
	}

	// Symlink bypass: resolveArg follows the link, so the real .env path is denied.
	if err := os.Symlink(".env", filepath.Join(root, "link.txt")); err == nil {
		if _, err := h.Run(ctx, ToolReadFile, map[string]any{"path": "link.txt"}); !errors.Is(err, ErrSensitivePath) {
			t.Fatalf("read_file through a .env symlink err = %v, want ErrSensitivePath", err)
		}
	} else {
		t.Logf("symlink test skipped: %v", err)
	}

	// Symlink bypass in the other direction: a denied requested name that resolves
	// to an ordinary target. The requested path and the resolved target are both
	// classified, so symlink resolution cannot erase the denied filename.
	write("config/local-values", llm08Ordinary+"\n")
	if err := os.MkdirAll(filepath.Join(root, "bypass"), 0o755); err != nil {
		t.Fatalf("mkdir bypass: %v", err)
	}
	if err := os.Symlink("../config/local-values", filepath.Join(root, "bypass", ".env")); err == nil {
		if _, err := h.Run(ctx, ToolReadFile, map[string]any{"path": "bypass/.env"}); !errors.Is(err, ErrSensitivePath) {
			t.Fatalf("read_file through a denied-name symlink err = %v, want ErrSensitivePath", err)
		}
		if _, err := h.Run(ctx, ToolCopyFile, map[string]any{"source": "bypass/.env", "dest": "bypass-copied.txt"}); !errors.Is(err, ErrSensitivePath) {
			t.Fatalf("copy_file through a denied-name symlink err = %v, want ErrSensitivePath", err)
		}
		if _, err := h.Run(ctx, ToolSearchFiles, map[string]any{"path": "bypass/.env", "pattern": llm08Ordinary}); !errors.Is(err, ErrSensitivePath) {
			t.Fatalf("search_files through a denied-name symlink err = %v, want ErrSensitivePath", err)
		}
	} else {
		t.Logf("symlink target test skipped: %v", err)
	}
	if got, gerr := h.Run(ctx, ToolReadFile, map[string]any{"path": "config/local-values"}); gerr != nil || !strings.Contains(got, llm08Ordinary) {
		t.Fatalf("ordinary target read = %q, err=%v", got, gerr)
	}

	// B: copy source is denied; no ordinary-name copy is created.
	if _, err := h.Run(ctx, ToolCopyFile, map[string]any{"source": ".env", "dest": "copied.txt"}); !errors.Is(err, ErrSensitivePath) {
		t.Fatalf("copy_file .env err = %v, want ErrSensitivePath", err)
	}
	if _, err := os.Stat(filepath.Join(root, "copied.txt")); !os.IsNotExist(err) {
		t.Fatalf("copy_file created copied.txt despite the credential-source denial")
	}

	// B: a search rooted at a credential file is denied.
	if _, err := h.Run(ctx, ToolSearchFiles, map[string]any{"path": ".env", "pattern": "SOP_TEST_API_KEY"}); !errors.Is(err, ErrSensitivePath) {
		t.Fatalf("search_files .env err = %v, want ErrSensitivePath", err)
	}

	// B: a repository-wide search skips credential paths but keeps ordinary source.
	out, err := h.Run(ctx, ToolSearchFiles, map[string]any{"pattern": "SOP_TEST_API_KEY"})
	if err != nil {
		t.Fatalf("search_files credential pattern: %v", err)
	}
	if strings.Contains(out, llm08PathCanary) {
		t.Fatalf("search_files leaked a credential-path canary:\n%s", out)
	}
	out, err = h.Run(ctx, ToolSearchFiles, map[string]any{"pattern": llm08Ordinary})
	if err != nil || !strings.Contains(out, llm08Ordinary) {
		t.Fatalf("search_files ordinary control = %q, err=%v", out, err)
	}

	// A follows B at the model boundary. At this harness boundary, git_diff and a
	// SAFE git show still return raw bytes; ollamaagent.toolResultMessage redacts
	// them before the model sees the result.
	out, err = h.Run(ctx, ToolGitDiff, nil)
	if err != nil || !strings.Contains(out, llm08DiffCanary) {
		t.Fatalf("git_diff raw boundary = %q, err=%v; want the raw diff before model redaction", out, err)
	}
	out, err = h.Run(ctx, ToolRunCommand, map[string]any{"command": "git show :credentials-tracked.txt"})
	if err != nil || !strings.Contains(out, llm08PathCanary) {
		t.Fatalf("run_command raw boundary = %q, err=%v; want the raw output before model redaction", out, err)
	}
	// cat is REQUIRES_APPROVAL, so the command policy refuses it before execution.
	if out, err := h.Run(ctx, ToolRunCommand, map[string]any{"command": "cat .env"}); err == nil {
		t.Fatalf("cat .env was allowed: %q", out)
	}

	// The harness audit records the call, not result content.
	for _, rec := range audit.Records() {
		if strings.Contains(rec.Request, llm08PathCanary) || strings.Contains(rec.Detail, llm08PathCanary) ||
			strings.Contains(rec.Request, llm08DiffCanary) || strings.Contains(rec.Detail, llm08DiffCanary) {
			t.Fatalf("harness audit persisted result content: %+v", rec)
		}
	}
}
