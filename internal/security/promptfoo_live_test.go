//go:build promptfoo_live

package security

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
)

// TestPromptfooPinnedBinaryVersion proves the adapter invokes the pinned binary
// through its real executor. It is fast and does not call a model.
func TestPromptfooPinnedBinaryVersion(t *testing.T) {
	t.Setenv("PATH", "/opt/homebrew/opt/node@22/bin:/opt/homebrew/bin:"+os.Getenv("PATH"))
	if _, err := exec.LookPath("promptfoo"); err != nil {
		t.Fatalf("pinned promptfoo is not installed: %v", err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ok, note, _ := promptfooVersionCheck(context.Background(), execRunner{}, root, "0.118.0", 1<<20)
	if !ok {
		t.Fatalf("pinned binary check failed: %s", note)
	}
}

// TestPromptfooLiveInvocation runs the pinned Promptfoo binary through the SOP
// adapter against a local Ollama model. It is compiled only with the
// promptfoo_live build tag so ordinary tests stay deterministic.
func TestPromptfooLiveInvocation(t *testing.T) {
	if os.Getenv("OLLAMA_BASE_URL") == "" {
		t.Setenv("OLLAMA_BASE_URL", "http://127.0.0.1:11434")
	}
	t.Setenv("PATH", "/opt/homebrew/opt/node@22/bin:/opt/homebrew/bin:"+os.Getenv("PATH"))
	if _, err := exec.LookPath("promptfoo"); err != nil {
		t.Fatalf("pinned promptfoo is not installed: %v", err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join("evals", "promptfoo", "promptfooconfig.yaml")
	if _, err := os.Stat(filepath.Join(root, configPath)); err != nil {
		t.Skipf("live config is unavailable: %v", err)
	}
	enabled := true
	eval := config.SecurityEvalConfig{
		Enabled:               &enabled,
		Profile:               config.SecurityEvalProfileLocal,
		Target:                "qwen3.5:4b",
		PromptfooVersion:      "0.118.0",
		AttackProvider:        "local",
		GradingProvider:       "local",
		TargetProvider:        "local",
		ConfigPath:            configPath,
		RequestLimit:          1,
		Concurrency:           1,
		ProcessTimeoutSeconds: 600,
		DataHandling:          config.SecurityEvalDataLocalOnly,
	}
	adapter, err := buildPromptfooAdapter(eval, BuildOptions{
		Executor:       execRunner{},
		WorkspaceRoot:  root,
		Timeout:        10 * time.Minute,
		MaxOutputBytes: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	a, ok := adapter.(*promptfooAdapter)
	if !ok {
		t.Fatalf("adapter type = %T", adapter)
	}
	out := t.TempDir()
	res, err := a.Scan(context.Background(), Request{
		WorkspaceRoot: root,
		RunID:         "promptfoo-live",
		OutputPath:    out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusCompleted {
		t.Fatalf("live status = %s, diagnostics = %v", res.Status, res.Diagnostics)
	}
	if res.Coverage != CoverageComplete {
		t.Fatalf("live coverage = %s", res.Coverage)
	}
	rep := readEvalReport(t, out)
	if len(rep.Cases) != 1 || rep.Cases[0].Status != EvalCasePassed {
		t.Fatalf("live report = %+v", rep)
	}
	if rep.PromptfooVersion != "0.118.0" || rep.TargetModel == EvalUnavailable {
		t.Fatalf("live identity = %+v", rep)
	}
	if rep.ConfigHash == "" || rep.ConfigHash == EvalUnavailable {
		t.Fatalf("live config hash = %q", rep.ConfigHash)
	}
}
