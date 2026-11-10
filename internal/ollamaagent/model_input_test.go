package ollamaagent

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

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
