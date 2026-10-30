package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/github"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/model"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

const stageModelsConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n" +
	"models:\n  default_class: small\n" +
	"  small:\n    provider: ollama\n    name: small-model\n    locality: cloud\n" +
	"  large:\n    provider: ollama\n    name: large-model\n    locality: cloud\n"

// TestRunPinnedStagesUseTheirClass proves a pinned plan/review class runs on that
// class's model while implementation stays on the run-level class, and that the
// model-selection artifact records which model every stage ran on.
func TestRunPinnedStagesUseTheirClass(t *testing.T) {
	clearModelEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, stageModelsConfig+"  stages:\n    review: large\n")
	t.Setenv(model.StageEnvKey(model.StagePlan), "large")

	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}
	built := map[string]int{}
	var out, errOut bytes.Buffer
	d := deps{
		getwd: func() (string, error) { return dir, nil },
		newAgent: func(_, _, name, _ string) (agent.Agent, error) {
			built[name]++
			return a, nil
		},
		readDiff:  func(context.Context, string) (string, error) { return "diff --git a/x b/x\n", nil },
		commit:    func(context.Context, string, string) error { return nil },
		newGitHub: func(string) github.Client { return &fakeGitHub{} },
	}
	if code := run([]string{"run", "--task", "TASK.md"}, &out, &errOut, d); code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, &out, &errOut)
	}
	// Run-level agent (small) plus one pinned plan agent and one pinned review agent (large).
	if built["small-model"] < 1 || built["large-model"] != 2 {
		t.Fatalf("agents built = %v, want small for implement and large for plan and review", built)
	}

	var art runpkg.ModelSelectionArtifact
	readRunJSON(t, filepath.Join(dir, stateDirName, "runs", "T001", "model-selection.json"), &art)
	want := map[string]runpkg.StageModel{
		"plan":   {Class: "large", Provider: "ollama", Model: "large-model", Source: "stage"},
		"review": {Class: "large", Provider: "ollama", Model: "large-model", Source: "stage"},
	}
	for st, w := range want {
		if art.Stages[st] != w {
			t.Errorf("stages[%s] = %+v, want %+v", st, art.Stages[st], w)
		}
	}
	if _, ok := art.Stages["jev"]; ok {
		t.Errorf("jev recorded although no JEV gate is enabled: %+v", art.Stages)
	}
}

// TestApplyStageModels covers the startup resolution: an unpinned stage is left
// out, a pinned JEV class swaps the analyzer model, and a JEV class on a provider
// JEV cannot run on is rejected.
func TestApplyStageModels(t *testing.T) {
	clearModelEnv(t)
	cfg := config.Default()
	cfg.Models = model.Route{
		Small:  model.ClassConfig{Provider: "ollama", Name: "small-model", Locality: model.LocalityCloud},
		Medium: model.ClassConfig{Provider: "llamacpp", Name: "medium-model", Locality: model.LocalityLocal},
		Stages: model.StageClasses{JEV: model.ClassSmall},
	}
	var analyzed string
	d := deps{newJEVAnalyzerForModel: func(name string) (jev.Analyzer, error) { analyzed = name; return nil, nil }}
	if err := applyStageModels(context.Background(), &d, cfg); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.stageModels[model.StagePlan]; ok {
		t.Fatal("unpinned plan stage must be left out")
	}
	if _, err := d.newJEVAnalyzer(cfg); err != nil || analyzed != "small-model" {
		t.Fatalf("JEV analyzer model = %q, %v; want small-model", analyzed, err)
	}

	cfg.Models.Stages.JEV = model.ClassMedium
	err := applyStageModels(context.Background(), &deps{}, cfg)
	if err == nil || !strings.Contains(err.Error(), "JEV runs only on provider") {
		t.Fatalf("err = %v, want a JEV provider rejection", err)
	}
}
