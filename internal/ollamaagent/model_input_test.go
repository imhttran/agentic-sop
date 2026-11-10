package ollamaagent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/redact"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// Task and caller input reach the model through userPrompt, so they pass the
// same model-input redaction as tool results; ordinary text is unchanged.
func TestUserPromptRedactsTaskAndInput(t *testing.T) {
	const taskCanary = "sk-llm02-task-canary-not-a-real-secret"
	const inputCanary = "sk-llm02-input-canary-not-a-real-secret"
	got := userPrompt(agent.Request{Task: "fix login, key " + taskCanary, Input: "context " + inputCanary})
	for _, c := range []string{taskCanary, inputCanary} {
		if strings.Contains(got, c) {
			t.Fatalf("prompt leaked %q:\n%s", c, got)
		}
	}
	if !strings.Contains(got, "fix login") || !strings.Contains(got, "[redacted]") {
		t.Fatalf("prompt lost ordinary text or redaction marker:\n%s", got)
	}
}

const (
	llm08EnvCanary     = "sk-llm08-env-canary-not-a-real-secret"
	llm08DiffCanary    = "sk-llm08-diff-canary-not-a-real-secret"
	llm08CommandCanary = "sk-llm08-command-canary-not-a-real-secret"
	llm08ErrorCanary   = "sk-llm08-error-canary-not-a-real-secret"
)

// TestModelInputToolBoundaries is the end-to-end enforcement test for the
// approved A+B policy. It distinguishes path denial (read_file, search_files,
// copy_file source) from model-bound redaction (git_diff, run_command output),
// includes an ordinary-source positive control, and exercises a denial error
// whose detail contains a synthetic credential.
func TestModelInputToolBoundaries(t *testing.T) {
	cases := []struct {
		name         string
		toolCall     string
		forbidden    string
		wantContains string
	}{
		{
			name:         "read_file credential path denied",
			toolCall:     `{"tool":"read_file","args":{"path":".env"}}`,
			forbidden:    llm08EnvCanary,
			wantContains: "refusing to expose a credential-bearing path",
		},
		{
			name:         "search_files credential path denied",
			toolCall:     `{"tool":"search_files","args":{"path":".env","pattern":"SOP_TEST_API_KEY"}}`,
			forbidden:    llm08EnvCanary,
			wantContains: "refusing to expose a credential-bearing path",
		},
		{
			name:         "copy_file credential source denied",
			toolCall:     `{"tool":"copy_file","args":{"source":".env","dest":"copied.txt"}}`,
			forbidden:    llm08EnvCanary,
			wantContains: "refusing to expose a credential-bearing path",
		},
		{
			name:         "git_diff result redacted",
			toolCall:     `{"tool":"git_diff","args":{}}`,
			forbidden:    llm08DiffCanary,
			wantContains: "[redacted]",
		},
		{
			name:         "run_command output redacted",
			toolCall:     `{"tool":"run_command","args":{"command":"git show :credentials-tracked.txt"}}`,
			forbidden:    llm08CommandCanary,
			wantContains: "[redacted]",
		},
		{
			name:         "run_command denial error redacted",
			toolCall:     `{"tool":"run_command","args":{"command":"cat ` + llm08ErrorCanary + `"}}`,
			forbidden:    llm08ErrorCanary,
			wantContains: "[redacted]",
		},
		{
			name:         "run_command ordinary SAFE output remains usable",
			toolCall:     `{"tool":"run_command","args":{"command":"git show :notes.txt"}}`,
			wantContains: "ordinary-source-marker",
		},
		{
			name:         "ordinary source remains usable",
			toolCall:     `{"tool":"read_file","args":{"path":"notes.txt"}}`,
			wantContains: "ordinary-source-marker",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			gitInit(t, dir)
			writeFile(t, dir, ".env", "SOP_TEST_API_KEY="+llm08EnvCanary+"\n")
			writeFile(t, dir, ".env.example", "SOP_TEST_API_KEY=replace-me\n")
			writeFile(t, dir, "notes.txt", "ordinary-source-marker ordinary repository content\n")
			writeFile(t, dir, "tracked-diff.txt", "before\n")
			writeFile(t, dir, "credentials-tracked.txt", "SOP_TEST_API_KEY="+llm08CommandCanary+"\n")
			runGitForTest(t, dir, "add", "notes.txt", "tracked-diff.txt", "credentials-tracked.txt")
			writeFile(t, dir, "tracked-diff.txt", "after token="+llm08DiffCanary+"\n")

			fake, srv := newFakeOllama(t,
				tc.toolCall,
				`{"status":"completed","summary":"done","changes_expected":false}`,
			)
			if _, err := New(testConfig(srv.URL), dir).Execute(context.Background(), implementRequest()); err != nil {
				t.Fatalf("Execute failed: %v", err)
			}
			if fake.count() < 2 {
				t.Fatalf("chat calls = %d, want at least 2", fake.count())
			}
			got := lastToolResult(t, fake.request(1))
			if tc.forbidden != "" && strings.Contains(got, tc.forbidden) {
				t.Fatalf("model-visible tool result leaked %q:\n%s", tc.forbidden, got)
			}
			if !strings.Contains(got, tc.wantContains) {
				t.Fatalf("tool result missing %q:\n%s", tc.wantContains, got)
			}
			if tc.name == "copy_file credential source denied" {
				if _, err := os.Stat(dir + "/copied.txt"); !os.IsNotExist(err) {
					t.Fatalf("copy_file created a copy despite the credential-source denial")
				}
			}
		})
	}
}

// lastToolResult returns the last model-visible tool-result message from req.
func lastToolResult(t *testing.T, req ollamaChatRequest) string {
	t.Helper()
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if strings.HasPrefix(req.Messages[i].Content, `Tool "`) {
			return req.Messages[i].Content
		}
	}
	t.Fatalf("no tool-result message in request:\n%s", messageText(req))
	return ""
}

func runGitForTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// TestModelInputToolBoundariesResidualLimits pins the documented limits of the
// approved A+B policy rather than claiming broader protection. It is an
// observation test: an unknown-format value in an ordinary file, diff, or SAFE
// command output still reaches the model, while a pre-existing hardlink with an
// ordinary name is now denied by the Option C Phase 1 identity policy.
// Recognised credential shapes in the ordinary paths are still redacted.
func TestModelInputToolBoundariesResidualLimits(t *testing.T) {
	const (
		opaqueFile    = "opaque-live-value-42"
		opaqueEnv     = "opaque-env-value-43"
		diffCanary    = "sk-llm02-limit-diff-not-a-real-secret"
		commandCanary = "sk-llm02-limit-command-not-a-real-secret"
	)
	dir := t.TempDir()
	gitInit(t, dir)
	writeFile(t, dir, ".env", "SOP_TEST_API_KEY="+llm08EnvCanary+"\nSOP_OPAQUE_SECRET="+opaqueEnv+"\n")
	writeFile(t, dir, "opaque.txt", "opaque_secret="+opaqueFile+"\n")
	writeFile(t, dir, "credentials-tracked.txt", "SOP_TEST_API_KEY="+commandCanary+"\n")
	writeFile(t, dir, "tracked-diff.txt", "before\n")
	writeFile(t, dir, "opaque-tracked.txt", "opaque_secret="+opaqueFile+"\n")
	runGitForTest(t, dir, "add", "credentials-tracked.txt", "tracked-diff.txt", "opaque-tracked.txt")
	writeFile(t, dir, "tracked-diff.txt", "after token="+diffCanary+"\n")
	writeFile(t, dir, "opaque-tracked.txt", "opaque_secret="+opaqueFile+"-worktree\n")
	if err := os.Link(filepath.Join(dir, ".env"), filepath.Join(dir, "hardlink.txt")); err != nil {
		t.Fatalf("create hardlink: %v", err)
	}

	h := toolharness.New(dir, toolharness.DefaultConfig(), toolharness.NewAuditLog(0))
	ctx := context.Background()

	// Documented limit: an ordinary, non-denied file with an unknown-format
	// secret is model-visible.
	out, err := h.Run(ctx, toolharness.ToolReadFile, map[string]any{"path": "opaque.txt"})
	if err != nil {
		t.Fatalf("read_file opaque.txt: %v", err)
	}
	if msg := toolResultMessage(toolharness.ToolReadFile, out, err); !strings.Contains(msg, opaqueFile) {
		t.Fatalf("the documented unknown-format gap is no longer observed; update the policy evidence: %q", msg)
	}

	// The Option C Phase 1 identity policy denies a pre-existing hardlink with
	// an ordinary name. This is a path-level refusal, not C1 redaction.
	out, err = h.Run(ctx, toolharness.ToolReadFile, map[string]any{"path": "hardlink.txt"})
	if !errors.Is(err, toolharness.ErrSensitivePath) {
		t.Fatalf("read_file hardlink.txt: want identity denial, got err=%v out=%q", err, out)
	}
	if out != "" {
		t.Fatalf("read_file hardlink.txt returned content despite the identity denial: %q", out)
	}
	msg := toolResultMessage(toolharness.ToolReadFile, out, err)

	// Documented limit: diff content is covered by shape redaction only.
	out, err = h.Run(ctx, toolharness.ToolGitDiff, nil)
	if err != nil {
		t.Fatalf("git_diff: %v", err)
	}
	msg = toolResultMessage(toolharness.ToolGitDiff, out, err)
	if strings.Contains(msg, diffCanary) {
		t.Fatalf("recognised diff credential shape was not redacted: %q", msg)
	}
	if !strings.Contains(msg, opaqueFile) {
		t.Fatalf("the documented diff/unknown-format gap is no longer observed; update the policy evidence: %q", msg)
	}

	// Documented limit: SAFE command output is covered by shape redaction only.
	out, err = h.Run(ctx, toolharness.ToolRunCommand, map[string]any{"command": "git show :opaque-tracked.txt"})
	if err != nil {
		t.Fatalf("run_command opaque: %v", err)
	}
	msg = toolResultMessage(toolharness.ToolRunCommand, out, err)
	if !strings.Contains(msg, opaqueFile) {
		t.Fatalf("the documented command/unknown-format gap is no longer observed; update the policy evidence: %q", msg)
	}

	out, err = h.Run(ctx, toolharness.ToolRunCommand, map[string]any{"command": "git show :credentials-tracked.txt"})
	if err != nil {
		t.Fatalf("run_command command canary: %v", err)
	}
	if msg := toolResultMessage(toolharness.ToolRunCommand, out, err); strings.Contains(msg, commandCanary) {
		t.Fatalf("recognised command credential shape was not redacted: %q", msg)
	}
}

// TestModelInputWithPolicyRedactsUnknownFormats is the C1 integration test. It
// exercises the real tool harness and the actual model-input rendering functions
// for an ordinary file, a diff, and SAFE command output, plus an exact inventory
// value and ordinary positive controls. The default functions are asserted to
// retain the pre-C1 behavior.
func TestModelInputWithPolicyRedactsUnknownFormats(t *testing.T) {
	const (
		opaque   = "opaque-live-value-42"
		bare     = "bare-opaque-value-88"
		ordinary = "ordinary-source-marker"
	)
	dir := t.TempDir()
	gitInit(t, dir)
	writeFile(t, dir, "opaque.txt", "opaque_secret="+opaque+"\n")
	writeFile(t, dir, "suffixed.txt", "password1="+opaque+"\n")
	writeFile(t, dir, "bare.txt", "the value is "+bare+"\n")
	writeFile(t, dir, "notes.txt", ordinary+" ordinary repository content\n")
	writeFile(t, dir, ".env.example", "SOP_TEST_API_KEY=replace-me\n")
	writeFile(t, dir, "opaque-tracked.txt", "opaque_secret="+opaque+"\n")
	writeFile(t, dir, "tracked-diff.txt", "before\n")
	runGitForTest(t, dir, "add", "opaque-tracked.txt", "tracked-diff.txt", "notes.txt", ".env.example")
	writeFile(t, dir, "opaque-tracked.txt", "opaque_secret="+opaque+"-worktree\n")
	writeFile(t, dir, "tracked-diff.txt", "after token="+opaque+"\n")

	h := toolharness.New(dir, toolharness.DefaultConfig(), toolharness.NewAuditLog(0))
	ctx := context.Background()
	policy := redact.Policy{Enabled: true, Values: []string{bare}}

	render := func(tool string, args map[string]any) string {
		t.Helper()
		out, err := h.Run(ctx, tool, args)
		if err != nil {
			t.Fatalf("%s: %v", tool, err)
		}
		return toolResultMessageWithPolicy(tool, out, err, policy)
	}

	for _, tc := range []struct {
		name string
		tool string
		args map[string]any
	}{
		{"ordinary file", toolharness.ToolReadFile, map[string]any{"path": "opaque.txt"}},
		{"suffixed key", toolharness.ToolReadFile, map[string]any{"path": "suffixed.txt"}},
		{"git diff", toolharness.ToolGitDiff, nil},
		{"SAFE command", toolharness.ToolRunCommand, map[string]any{"command": "git show :opaque-tracked.txt"}},
		{"inventory value", toolharness.ToolReadFile, map[string]any{"path": "bare.txt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg := render(tc.tool, tc.args)
			for _, forbidden := range []string{opaque, bare} {
				if strings.Contains(msg, forbidden) {
					t.Fatalf("model-bound content leaked %q:\n%s", forbidden, msg)
				}
			}
			if !strings.Contains(msg, "[redacted]") {
				t.Fatalf("model-bound content has no redaction marker:\n%s", msg)
			}
		})
	}

	// Positive controls: ordinary content and the documented template survive.
	if msg := render(toolharness.ToolReadFile, map[string]any{"path": "notes.txt"}); !strings.Contains(msg, ordinary) {
		t.Fatalf("ordinary content was redacted:\n%s", msg)
	}
	if msg := render(toolharness.ToolReadFile, map[string]any{"path": ".env.example"}); !strings.Contains(msg, "replace-me") {
		t.Fatalf("template content was redacted:\n%s", msg)
	}

	// Default behavior is unchanged: the same raw content still reaches the
	// default model-input renderer when C1 is disabled.
	out, err := h.Run(ctx, toolharness.ToolReadFile, map[string]any{"path": "opaque.txt"})
	if err != nil {
		t.Fatalf("read opaque.txt: %v", err)
	}
	if msg := toolResultMessage(toolharness.ToolReadFile, out, err); !strings.Contains(msg, opaque) {
		t.Fatalf("default behavior changed; expected the pre-C1 unknown-format value:\n%s", msg)
	}
}

// TestUserPromptWithPolicyRedactsUnknownFormats proves the task and caller input
// use the same C1 policy. The default userPrompt retains the pre-C1 behavior.
func TestUserPromptWithPolicyRedactsUnknownFormats(t *testing.T) {
	const canary = "opaque-live-value-42"
	req := agent.Request{
		Task:  "fix opaque_secret=" + canary,
		Input: "context auth_token=" + canary,
	}
	policy := redact.Policy{Enabled: true}
	got := userPromptWithPolicy(req, policy)
	if strings.Contains(got, canary) {
		t.Fatalf("user prompt leaked the unknown-format value:\n%s", got)
	}
	if !strings.Contains(got, "[redacted]") || !strings.Contains(got, "fix") {
		t.Fatalf("user prompt lost the marker or ordinary text:\n%s", got)
	}
	if !strings.Contains(userPrompt(req), canary) {
		t.Fatalf("default userPrompt behavior changed; expected the pre-C1 value")
	}
}

// TestHarnessUsesC1RedactionPolicy proves the loaded policy is actually wired
// through the real agent loop: the first user prompt and the tool-result message
// both use it. This complements the direct render-function tests.
func TestHarnessUsesC1RedactionPolicy(t *testing.T) {
	const canary = "opaque-live-value-42"
	dir := t.TempDir()
	gitInit(t, dir)
	writeFile(t, dir, "opaque.txt", "opaque_secret="+canary+"\n")

	fake, srv := newFakeOllama(t,
		`{"tool":"read_file","args":{"path":"opaque.txt"}}`,
		`{"status":"completed","summary":"done","changes_expected":false}`,
	)
	cfg := testConfig(srv.URL)
	cfg.Redaction = redact.Policy{Enabled: true}
	req := implementRequest()
	req.Input = "context auth_token=" + canary
	if _, err := New(cfg, dir).Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if fake.count() < 2 {
		t.Fatalf("chat calls = %d, want at least 2", fake.count())
	}
	if prompt := messageText(fake.request(0)); strings.Contains(prompt, canary) {
		t.Fatalf("first user prompt leaked the unknown-format value:\n%s", prompt)
	}
	got := lastToolResult(t, fake.request(1))
	if strings.Contains(got, canary) {
		t.Fatalf("tool result leaked the unknown-format value:\n%s", got)
	}
	if !strings.Contains(got, "[redacted]") {
		t.Fatalf("tool result has no redaction marker:\n%s", got)
	}
}
