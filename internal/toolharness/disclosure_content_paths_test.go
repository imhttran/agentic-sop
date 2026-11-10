package toolharness

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	llm08ContentCanary   = "sk-llm08-content-path-canary-not-a-real-secret" //gitleaks:allow: synthetic test canary, not a credential
	llm08OrdinaryMarker  = "ordinary-source-marker"
	llm08EnvAssignment   = "SOP_TEST_API_KEY=" + llm08ContentCanary
	llm08StagedCredFile  = "tracked-credentials.txt"
	llm08WorkingCredFile = "tracked-diff.txt"
)

// TestDisclosureContentPathsObservation establishes the current, pre-policy
// behavior for every supported harness path that can return repository content to
// the model. It is a baseline observation, not a protection test: on this revision
// the synthetic canary is expected to be visible through direct reads, search,
// copy-then-read, git diff, and a SAFE command's stdout. The test asserts only the
// positive controls and the audit separation, so the observed visibility values are
// logged for the policy proposal and a later implementation must update this
// baseline deliberately.
func TestDisclosureContentPathsObservation(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write(".env", llm08EnvAssignment+"\n")
	write("notes.txt", llm08OrdinaryMarker+" ordinary repository content\n")
	write(llm08StagedCredFile, llm08EnvAssignment+"\n")
	write(llm08WorkingCredFile, "before\n")

	// A real repository makes the git-backed paths executable. The canary is
	// staged, not committed, so no credential reaches repository history.
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init", "-q")
	runGit("add", "notes.txt", llm08StagedCredFile, llm08WorkingCredFile)
	// Change the working tree after staging so `git diff` carries the canary.
	write(llm08WorkingCredFile, "after\n"+llm08EnvAssignment+"\n")

	audit := NewAuditLog(0)
	h := New(root, DefaultConfig(), audit)
	ctx := context.Background()
	observed := map[string]bool{}
	run := func(label, tool string, args map[string]any) string {
		t.Helper()
		out, err := h.Run(ctx, tool, args)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		observed[label] = strings.Contains(out, llm08ContentCanary)
		t.Logf("llm08 content path %-32s canary_visible=%v", label, observed[label])
		return out
	}

	// Positive controls: the same real paths return ordinary content.
	if out := run("read_file ordinary (control)", ToolReadFile, map[string]any{"path": "notes.txt"}); !strings.Contains(out, llm08OrdinaryMarker) {
		t.Fatalf("read_file did not return the ordinary source marker: %q", out)
	}
	if out := run("search_files ordinary (control)", ToolSearchFiles, map[string]any{"pattern": llm08OrdinaryMarker}); !strings.Contains(out, llm08OrdinaryMarker) {
		t.Fatalf("search_files did not return the ordinary source marker: %q", out)
	}

	// Content paths that can expose the canary. Use a non-secret search pattern and
	// a path-only git command so the harness audit never contains the canary value.
	run("read_file .env", ToolReadFile, map[string]any{"path": ".env"})
	run("search_files credential", ToolSearchFiles, map[string]any{"pattern": "SOP_TEST_API_KEY"})
	if _, err := h.Run(ctx, ToolCopyFile, map[string]any{"source": ".env", "dest": "copied-notes.txt"}); err != nil {
		t.Fatalf("copy_file .env: %v", err)
	}
	run("copy_file + read_file", ToolReadFile, map[string]any{"path": "copied-notes.txt"})
	run("git_diff", ToolGitDiff, nil)
	run("run_command git show", ToolRunCommand, map[string]any{"command": "git show :" + llm08StagedCredFile})

	// Baseline assertion: these paths are expected to expose the canary on the
	// un-policied revision. When an approved policy changes one of them, update
	// the assertion and the proposal in the same change rather than silently
	// keeping a stale GAP.
	for _, label := range []string{
		"read_file .env",
		"search_files credential",
		"copy_file + read_file",
		"git_diff",
		"run_command git show",
	} {
		if !observed[label] {
			t.Errorf("baseline changed: %s no longer exposes the canary; update the policy proposal and this assertion", label)
		}
	}
	for _, label := range []string{"read_file ordinary (control)", "search_files ordinary (control)"} {
		if observed[label] {
			t.Errorf("ordinary positive control %s unexpectedly contains the canary", label)
		}
	}

	// The harness audit records the call, not result content. A canary in the
	// audit request would be a separate persistence path; the request strings above
	// deliberately avoid the value.
	for _, rec := range audit.Records() {
		if strings.Contains(rec.Request, llm08ContentCanary) || strings.Contains(rec.Detail, llm08ContentCanary) {
			t.Fatalf("harness audit persisted result content: %+v", rec)
		}
	}
}
