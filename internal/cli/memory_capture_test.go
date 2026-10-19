package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/memindex"
	"github.com/imhttran/agentic-sop/internal/outcomefile"
	"github.com/imhttran/agentic-sop/internal/retrievalboundary"
)

func memoryConfig(t *testing.T, dir string) config.Config {
	t.Helper()
	for _, k := range []string{
		config.EnvAdvisoryRetrievalEnabled,
		config.EnvAdvisoryRetrievalIndex,
		config.EnvAdvisoryRetrievalOutcomes,
		config.EnvAdvisoryRetrievalCorpora,
	} {
		t.Setenv(k, "")
	}
	cfg := config.Config{Project: config.Project{Name: "agentic-sop"}}
	cfg.AdvisoryRetrieval = config.AdvisoryRetrievalConfig{
		Enabled:        true,
		IndexPath:      filepath.Join(dir, "memory", "index.db"),
		OutcomesDir:    filepath.Join(dir, "memory", "outcomes"),
		AllowedCorpora: []string{"outcome", "failure"},
		TimeoutMS:      1000,
		MaxResults:     5,
		TokenBudget:    1000,
	}
	return cfg
}

func writeRunReport(t *testing.T, dir, id, outcomeStatus string) {
	t.Helper()
	runDir := filepath.Join(dir, config.DirName, "runs", id)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	doc := map[string]any{
		"id": id, "stage": "PASSED", "decision": "PASS",
		"reasons": []string{"reason"},
		"outcome": map[string]any{
			"status": outcomeStatus, "reasons": []string{"reason"},
			"lifecycle": "LOCAL_DONE", "criteria": []any{},
		},
		"generated_at": "2026-10-19T12:00:00Z",
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "state.json"), []byte(`{"id":"`+id+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunCompletionCapturesDecisionMemory(t *testing.T) {
	for _, k := range []string{
		config.EnvAdvisoryRetrievalEnabled, config.EnvAdvisoryRetrievalIndex,
		config.EnvAdvisoryRetrievalOutcomes, config.EnvAdvisoryRetrievalCorpora,
	} {
		t.Setenv(k, "")
	}
	for _, tc := range []struct {
		name      string
		indexPath string
		wantPass  bool
		wantFiles int
	}{
		{"enabled", filepath.Join("memory", "index.db"), true, 1},
		// A memory failure is best-effort and must not change the task verdict.
		{"capture failure", "/dev/null/index.db", true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "TASK.md", runTaskFile)
			indexPath := tc.indexPath
			if !filepath.IsAbs(indexPath) {
				indexPath = filepath.Join(dir, indexPath)
			}
			writeConfig(t, dir, fmt.Sprintf(`project:
  name: x
validation:
  build:
    - "true"
  test:
    - "true"
advisory_retrieval:
  enabled: true
  index_path: %q
  outcomes_dir: %q
  allowed_corpora:
    - outcome
    - failure
`, indexPath, filepath.Join(dir, "memory", "outcomes")))
			a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}
			code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run", "--task", "TASK.md")
			if (code == exitOK) != tc.wantPass {
				t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
			}
			files, _ := filepath.Glob(filepath.Join(dir, "memory", "outcomes", "*.json"))
			if len(files) != tc.wantFiles {
				t.Fatalf("outcome files = %d, want %d (stdout=%s)", len(files), tc.wantFiles, stdout)
			}
			if tc.wantFiles == 0 {
				return
			}
			rec, err := outcomefile.Read(files[0])
			if err != nil {
				t.Fatal(err)
			}
			if rec.Repository != "x" || rec.TaskID != "T001" {
				t.Fatalf("record repo=%q task=%q", rec.Repository, rec.TaskID)
			}
			if tc.indexPath == "/dev/null/index.db" {
				if !strings.Contains(stdout, "decision memory: capture skipped") {
					t.Fatalf("a capture failure was not reported as best-effort: %s", stdout)
				}
				return
			}
			ix, err := memindex.Open(indexPath)
			if err != nil {
				t.Fatal(err)
			}
			defer ix.Close()
			st, err := ix.Status(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if st.Records != 1 {
				t.Fatalf("index records = %d, want 1 after automatic capture", st.Records)
			}
		})
	}
}

func TestCaptureOutcomeIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	cfg := memoryConfig(t, dir)
	writeRunReport(t, dir, "T1", "PARTIAL")

	for i := 0; i < 2; i++ {
		if err := captureOutcome(dir, cfg, "T1"); err != nil {
			t.Fatalf("capture %d: %v", i, err)
		}
	}
	files, err := filepath.Glob(filepath.Join(cfg.AdvisoryRetrieval.OutcomesDir, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("outcome files = %d, %v; repeated capture must be idempotent", len(files), err)
	}
	rec, err := outcomefile.Read(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if rec.Repository != "agentic-sop" || rec.Status != outcomefile.StatusPartial {
		t.Fatalf("record = repo %q status %q, want the scoped repository and preserved status", rec.Repository, rec.Status)
	}
	ix, err := memindex.Open(cfg.AdvisoryRetrieval.IndexPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	st, err := ix.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Records != 1 {
		t.Fatalf("index records = %d, want 1", st.Records)
	}
}

func TestCaptureFailedOutcomeIsFailureHistory(t *testing.T) {
	dir := t.TempDir()
	cfg := memoryConfig(t, dir)
	writeRunReport(t, dir, "T2", "FAILED")
	if err := captureOutcome(dir, cfg, "T2"); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(cfg.AdvisoryRetrieval.OutcomesDir, "*.json"))
	if len(files) != 1 {
		t.Fatalf("outcome files = %d", len(files))
	}
	rec, err := outcomefile.Read(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if rec.Corpus != retrievalboundary.CorpusFailure {
		t.Fatalf("corpus = %q, want failure history", rec.Corpus)
	}
}

func TestCaptureIsBestEffortOnMemoryFailure(t *testing.T) {
	dir := t.TempDir()
	cfg := memoryConfig(t, dir)
	writeRunReport(t, dir, "T3", "PARTIAL")
	// An unusable index path must return an error to the caller (which only reports it)
	// and must not panic or alter the run artifacts.
	cfg.AdvisoryRetrieval.IndexPath = ""
	if err := captureOutcome(dir, cfg, "T3"); err == nil {
		t.Fatal("a misconfigured memory layer must return an error to the best-effort caller")
	}
}
