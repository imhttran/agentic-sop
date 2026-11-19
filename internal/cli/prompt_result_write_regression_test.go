package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// resultWriteFailingAgent succeeds at generation, then makes the run's
// result.md path a directory so only the result.md write fails. Everything
// before it (prompt.md) and after it (metadata.json) can still be written.
type resultWriteFailingAgent struct {
	dir     string
	content string
}

func (a *resultWriteFailingAgent) Generate(_ context.Context, _ agent.Request) (agent.Response, error) {
	runDirs, _ := filepath.Glob(filepath.Join(a.dir, stateDirName, "runs", promptsDirName, "*"))
	for _, runDir := range runDirs {
		_ = os.MkdirAll(filepath.Join(runDir, "result.md"), 0o700)
	}
	return agent.Response{Content: a.content}, nil
}

func (a *resultWriteFailingAgent) Capabilities() agent.Capabilities {
	return agent.NewCapabilities(agent.Plan, agent.DesignTests, agent.DiagnoseFailure, agent.Review)
}

// TestPromptResultWriteFailureIsSurfaced is the deterministic regression check:
// the fake provider SUCCEEDS, only the result.md write fails, and the CLI must
// surface the error, exit nonzero, and never claim a saved result.
func TestPromptResultWriteFailureIsSurfaced(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	a := &resultWriteFailingAgent{dir: dir, content: "the generated result"}

	code, stdout, stderr := runCLIWithAgent(t, dir, a, "prompt", "--json", "plan this")

	if code == exitOK {
		t.Fatalf("a failed result.md write must not exit 0; stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(strings.ToLower(stderr), "result.md") {
		t.Errorf("stderr must name result.md: %q", stderr)
	}
	if strings.Contains(stdout, `"status": "completed"`) {
		t.Errorf("stdout must not report a completed prompt: %s", stdout)
	}
	if strings.Contains(stdout, `"result_path"`) && !strings.Contains(stdout, `"result_path": ""`) {
		t.Errorf("stdout must not report a saved result_path: %s", stdout)
	}

	dirs := promptRunDirs(t, dir)
	if len(dirs) != 1 {
		t.Fatalf("want one prompt run dir, got %v", dirs)
	}
	data, err := os.ReadFile(filepath.Join(dirs[0], "metadata.json"))
	if err != nil {
		// No metadata is acceptable: the run must not claim success.
		return
	}
	var md promptResultDoc
	if err := json.Unmarshal(data, &md); err != nil {
		t.Fatalf("metadata.json is not JSON: %v", err)
	}
	if md.Status == "completed" {
		t.Errorf("metadata must not report completed status: %+v", md)
	}
	if md.ResultPath != "" {
		t.Errorf("metadata must not report a result_path: %q", md.ResultPath)
	}
}

// TestPromptResultWriteSuccessReturnsReadableResultPath is the control: the
// same path must still report a saved result and a readable result file.
func TestPromptResultWriteSuccessReturnsReadableResultPath(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	a := &fakeCapabilityAgent{plan: "the generated result"}

	code, stdout, stderr := runCLIWithAgent(t, dir, a, "prompt", "--json", "plan this")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	var doc promptResultDoc
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &doc); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if doc.Status != "completed" {
		t.Errorf("status = %q, want completed", doc.Status)
	}
	if doc.ResultPath == "" {
		t.Fatal("a successful write must report result_path")
	}
	saved, err := os.ReadFile(filepath.Join(dir, doc.ResultPath))
	if err != nil {
		t.Fatalf("result_path is not readable: %v", err)
	}
	if string(saved) != doc.Result {
		t.Errorf("saved result = %q, want %q", string(saved), doc.Result)
	}
}
