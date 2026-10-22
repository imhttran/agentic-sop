package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/memindex"
	"github.com/imhttran/agentic-sop/internal/outcomefile"
	"github.com/imhttran/agentic-sop/internal/retrievalboundary"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// clearAdvisoryEnv isolates a test from the operator's advisory-retrieval overrides.
func clearAdvisoryEnv(t *testing.T) {
	t.Helper()
	for _, name := range advisoryOverrideNames {
		t.Setenv(name, "")
	}
}

func memoryConfig(t *testing.T, dir string) config.Config {
	t.Helper()
	clearAdvisoryEnv(t)
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
				if !strings.Contains(stdout, "decision memory: capture failed") {
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
		if _, err := captureOutcome(dir, cfg, "T1"); err != nil {
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
	if _, err := captureOutcome(dir, cfg, "T2"); err != nil {
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
	if _, err := captureOutcome(dir, cfg, "T3"); err == nil {
		t.Fatal("a misconfigured memory layer must return an error to the best-effort caller")
	}
}

// TestCaptureRetainsValidRecordsWhenOneIsRejected is the reported failure: a malformed
// outcome document was invisible because the rebuild report was discarded. The valid
// documents must stay indexed while the rejected one is reported.
func TestCaptureRetainsValidRecordsWhenOneIsRejected(t *testing.T) {
	dir := t.TempDir()
	cfg := memoryConfig(t, dir)
	writeRunReport(t, dir, "T1", "PARTIAL")
	if _, err := captureOutcome(dir, cfg, "T1"); err != nil {
		t.Fatal(err)
	}
	writeRunReport(t, dir, "T2", "PARTIAL")
	if err := os.WriteFile(filepath.Join(cfg.AdvisoryRetrieval.OutcomesDir, "broken.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := captureOutcome(dir, cfg, "T2")
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if report.Indexed != 2 {
		t.Fatalf("indexed=%d, want the two valid records", report.Indexed)
	}
	if len(report.Rejected) != 1 || !strings.Contains(report.Rejected["broken.json"], "decode") {
		t.Fatalf("rejected=%v, want the malformed document with a reason", report.Rejected)
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
	if st.Records != 2 {
		t.Fatalf("index records=%d, want 2", st.Records)
	}
}

// TestMemoryStatusReportsDegradedCapture proves telemetry and the status command expose the
// rejection instead of reporting an unqualified success.
func TestMemoryStatusReportsDegradedCapture(t *testing.T) {
	dir := t.TempDir()
	clearAdvisoryEnv(t)
	initProject(t, dir)
	if code, out, errOut := runCLI(t, dir, "memory", "enable"); code != exitOK {
		t.Fatalf("enable: %d %s %s", code, out, errOut)
	}
	cfg, err := config.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	writeRunReport(t, dir, "T1", "PARTIAL")
	rn, err := runpkg.New(dir, "T1")
	if err != nil {
		t.Fatal(err)
	}
	report, captureErr := captureOutcome(dir, *cfg, "T1")
	if captureErr != nil {
		t.Fatal(captureErr)
	}
	writeMemoryCapture(rn, *cfg, time.Now(), report, captureErr, io.Discard)

	if err := os.WriteFile(filepath.Join(cfg.AdvisoryRetrieval.OutcomesDir, "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, captureErr = captureOutcome(dir, *cfg, "T1")
	if captureErr != nil {
		t.Fatal(captureErr)
	}
	if len(report.Rejected) != 1 {
		t.Fatalf("rejected=%v, want one malformed document", report.Rejected)
	}
	var stdout strings.Builder
	writeMemoryCapture(rn, *cfg, time.Now(), report, captureErr, &stdout)
	if !strings.Contains(stdout.String(), "capture degraded") {
		t.Fatalf("run output = %q, want a visible degraded capture", stdout.String())
	}

	path := filepath.Join(dir, config.DirName, "runs", "T1", "memory-capture.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var tele memoryCaptureTelemetry
	if err := json.Unmarshal(b, &tele); err != nil {
		t.Fatal(err)
	}
	if tele.Status != memoryCaptureDegraded || tele.Reason != memoryCaptureReasonRejected {
		t.Fatalf("telemetry status=%q reason=%q, want degraded/%s", tele.Status, tele.Reason, memoryCaptureReasonRejected)
	}
	if tele.Rejected != 1 || tele.Indexed != 1 || len(tele.RejectedReasons) != 1 {
		t.Fatalf("telemetry counts = rejected %d indexed %d reasons %d", tele.Rejected, tele.Indexed, len(tele.RejectedReasons))
	}

	code, out, errOut := runCLI(t, dir, "memory", "status")
	if code != exitOK {
		t.Fatalf("status: %d %s %s", code, out, errOut)
	}
	for _, want := range []string{"capture=degraded", "reason=" + memoryCaptureReasonRejected, "rejected=1", "rejected: broken.json"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status output missing %q:\n%s", want, out)
		}
	}
}

// TestMalformedOutcomeDoesNotChangeVerdict keeps the end-to-end acceptance: a rejected
// outcome document is visible, valid records stay indexed, and the task verdict is intact.
func TestMalformedOutcomeDoesNotChangeVerdict(t *testing.T) {
	clearAdvisoryEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	outcomes := filepath.Join(dir, "memory", "outcomes")
	if err := os.MkdirAll(outcomes, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outcomes, "broken.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
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
`, filepath.Join(dir, "memory", "index.db"), outcomes))
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}
	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("a malformed outcome changed the task verdict: %d %s %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "capture degraded") {
		t.Fatalf("stdout = %q, want a visible degraded capture", stdout)
	}
	b, err := os.ReadFile(filepath.Join(dir, config.DirName, "runs", "T001", "memory-capture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tele memoryCaptureTelemetry
	if err := json.Unmarshal(b, &tele); err != nil {
		t.Fatal(err)
	}
	if tele.Status != memoryCaptureDegraded || tele.Rejected != 1 || tele.Indexed != 1 {
		t.Fatalf("telemetry = %+v, want degraded with one indexed and one rejected record", tele)
	}
	ix, err := memindex.Open(filepath.Join(dir, "memory", "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	st, err := ix.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Records != 1 {
		t.Fatalf("index records=%d, want the valid outcome", st.Records)
	}
}
