package security

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
)

func TestBuildAdaptersIncludesConfiguredEvaluation(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "promptfooconfig.yaml")
	if err := os.WriteFile(configPath, []byte("prompts: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	datasetPath := filepath.Join(root, "dataset.json")
	if err := os.WriteFile(datasetPath, []byte(`{"cases":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	enabled := true
	eval := config.SecurityEvalConfig{
		Enabled:               &enabled,
		Profile:               config.SecurityEvalProfileLocal,
		Target:                "local-model",
		PromptfooVersion:      "0.118.0",
		AttackProvider:        "local-attack",
		GradingProvider:       "local-grading",
		TargetProvider:        "local-target",
		ConfigPath:            "promptfooconfig.yaml",
		DatasetPath:           "dataset.json",
		RequestLimit:          3,
		Concurrency:           2,
		ProcessTimeoutSeconds: 42,
		DataHandling:          config.SecurityEvalDataLocalOnly,
		SeverityMode:          config.SecurityEvalSeverityNone,
	}
	adapters, err := BuildAdapters(nil, BuildOptions{
		WorkspaceRoot: root,
		Evaluation:    &eval,
		Timeout:       time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(adapters) != 1 {
		t.Fatalf("adapters = %d, want the promptfoo adapter", len(adapters))
	}
	descriptor := adapters[0].Descriptor()
	if descriptor.AdapterID != "promptfoo" || len(descriptor.Categories) != 1 || descriptor.Categories[0] != CategoryEvaluation {
		t.Fatalf("descriptor = %+v", descriptor)
	}
	adapter, ok := adapters[0].(*promptfooAdapter)
	if !ok {
		t.Fatalf("adapter type = %T", adapters[0])
	}
	if adapter.configHash == "" || adapter.configHash == EvalUnavailable {
		t.Fatalf("config hash = %q, want the file digest", adapter.configHash)
	}
	if adapter.datasetHash == "" || adapter.datasetHash == EvalUnavailable {
		t.Fatalf("dataset hash = %q, want the file digest", adapter.datasetHash)
	}
	if adapter.requestLimit != 3 || adapter.concurrency != 2 {
		t.Fatalf("limits = %d/%d", adapter.requestLimit, adapter.concurrency)
	}
	if adapter.timeout != 42*time.Second {
		t.Fatalf("timeout = %s, want the evaluation process timeout", adapter.timeout)
	}
}

func TestBuildAdaptersOmitsDisabledEvaluation(t *testing.T) {
	disabled := false
	eval := config.SecurityEvalConfig{Enabled: &disabled, Target: "x", ConfigPath: "c.yaml"}
	adapters, err := BuildAdapters(nil, BuildOptions{WorkspaceRoot: t.TempDir(), Evaluation: &eval})
	if err != nil {
		t.Fatal(err)
	}
	if len(adapters) != 0 {
		t.Fatalf("adapters = %d, want none", len(adapters))
	}
}
