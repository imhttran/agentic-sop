package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/retrievalboundary"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

func TestMemoryWorkspaceEnableDisablePreservesConfig(t *testing.T) {
	dir := t.TempDir()
	clearAdvisoryEnv(t)
	initProject(t, dir)
	before, err := config.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"enable", "enable", "status", "disable"} {
		code, out, errOut := runCLI(t, dir, "memory", command)
		if code != exitOK {
			t.Fatalf("%s: %d %s %s", command, code, out, errOut)
		}
	}
	after, err := config.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if after.AdvisoryRetrieval.Enabled || before.Project != after.Project || before.Agent != after.Agent {
		t.Fatal("configuration was not preserved")
	}
	if !filepath.IsAbs(after.AdvisoryRetrieval.IndexPath) {
		t.Fatal("missing workspace memory path")
	}
	if _, err := os.Stat(after.AdvisoryRetrieval.OutcomesDir); err != nil {
		t.Fatal(err)
	}
}

// TestMemoryEnablePreparesEnvironmentIndexPath is the reported failure: an environment
// index override pointing at a new directory was never prepared, so enable reported
// success while the following capture could not open the index.
func TestMemoryEnablePreparesEnvironmentIndexPath(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	envRoot := filepath.Join(t.TempDir(), "env-memory")
	indexPath := filepath.Join(envRoot, "index.db")
	t.Setenv(config.EnvAdvisoryRetrievalIndex, indexPath)

	code, out, errOut := runCLI(t, dir, "memory", "enable")
	if code != exitOK {
		t.Fatalf("enable: %d %s %s", code, out, errOut)
	}
	if !strings.Contains(out, config.EnvAdvisoryRetrievalIndex+"="+indexPath) {
		t.Fatalf("enable did not report the applicable environment override: %s", out)
	}
	if _, err := os.Stat(envRoot); err != nil {
		t.Fatalf("enable did not prepare the effective index parent: %v", err)
	}

	cfg, err := config.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	writeRunReport(t, dir, "T1", "PARTIAL")
	if _, err := captureOutcome(dir, *cfg, "T1"); err != nil {
		t.Fatalf("capture after enable: %v", err)
	}
	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("capture did not use the prepared environment index: %v", err)
	}
}

// TestMemoryEnableRejectsInvalidEffectiveConfiguration covers both an environment override
// and a preserved workspace value that Resolved alone accepts but every retrieval rejects.
func TestMemoryEnableRejectsInvalidEffectiveConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name  string
		env   map[string]string
		write string
		want  string
	}{
		{
			name: "environment corpus",
			env:  map[string]string{config.EnvAdvisoryRetrievalCorpora: "bogus"},
			want: "unsupported corpus",
		},
		{
			name: "environment timeout above the retrieval ceiling",
			env:  map[string]string{config.EnvAdvisoryRetrievalTimeout: "60000"},
			want: "time budget out of range",
		},
		{
			name: "preserved workspace corpus",
			write: `project:
  name: x
advisory_retrieval:
  enabled: false
  allowed_corpora:
    - not-a-corpus
`,
			want: "unsupported corpus",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			initProject(t, dir)
			if tc.write != "" {
				writeConfig(t, dir, tc.write)
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			code, out, errOut := runCLI(t, dir, "memory", "enable")
			if code != exitError {
				t.Fatalf("enable accepted an invalid effective configuration: %d %s %s", code, out, errOut)
			}
			if !strings.Contains(errOut, tc.want) {
				t.Fatalf("stderr = %q, want an actionable %q message", errOut, tc.want)
			}
			if strings.Contains(out, "enabled=true") {
				t.Fatalf("enable reported success for an invalid effective configuration: %s", out)
			}
			cfg, err := config.LoadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.AdvisoryRetrieval.Enabled {
				t.Fatal("enable persisted enabled=true for an invalid effective configuration")
			}
		})
	}
}

// TestMemoryEnableReportsEnvironmentPrecedence proves the workspace flag and the effective
// flag stay distinguishable when the environment disables the layer.
func TestMemoryEnableReportsEnvironmentPrecedence(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	t.Setenv(config.EnvAdvisoryRetrievalEnabled, "false")
	code, out, errOut := runCLI(t, dir, "memory", "enable")
	if code != exitOK {
		t.Fatalf("enable: %d %s %s", code, out, errOut)
	}
	if !strings.Contains(out, "Workspace advisory memory enabled=true") || !strings.Contains(out, "Effective advisory memory enabled=false") {
		t.Fatalf("enable did not report workspace and effective state separately: %s", out)
	}
}

// TestMemoryDisableIgnoresUnhealthyStorage proves disable stays usable when the configured
// memory storage cannot be opened.
func TestMemoryDisableIgnoresUnhealthyStorage(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, `project:
  name: x
advisory_retrieval:
  enabled: true
  index_path: /dev/null/index.db
  outcomes_dir: /dev/null/outcomes
  allowed_corpora:
    - outcome
`)
	code, out, errOut := runCLI(t, dir, "memory", "disable")
	if code != exitOK {
		t.Fatalf("disable: %d %s %s", code, out, errOut)
	}
	cfg, err := config.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AdvisoryRetrieval.Enabled {
		t.Fatal("disable did not persist enabled=false")
	}
}

type deadlineMemoryRetriever struct{}

func (deadlineMemoryRetriever) Retrieve(ctx context.Context, q retrievalboundary.ScopedQuery) (retrievalboundary.Result, error) {
	<-ctx.Done()
	return retrievalboundary.Result{}, ctx.Err()
}
func TestMemoryRetrievalTimeoutContinuesWithoutContext(t *testing.T) {
	cfg := advisoryCfg(t, true)
	cfg.AdvisoryRetrieval.TimeoutMS = 10
	started := time.Now()
	items, res := advisoryKnowledgeForAttempt(context.Background(), cfg, specFixture(), deadlineMemoryRetriever{}, "agentic-sop", "")
	if len(items) != 0 || res.ReasonCode != ragReasonAdvisoryTimeout {
		t.Fatalf("%+v", res)
	}
	if time.Since(started) > time.Second {
		t.Fatal("retrieval did not respect deadline")
	}
}
func TestMemoryObservationHistoryAndStatus(t *testing.T) {
	dir := t.TempDir()
	clearAdvisoryEnv(t)
	initProject(t, dir)
	rn, err := runpkg.New(dir, "T1")
	if err != nil {
		t.Fatal(err)
	}
	rec := ragTelemetry{TaskID: "T1", Attempt: 1, AdvisoryStatus: ragStatusSucceed, AdvisoryPhase: "planning", AdvisoryCount: 1, AdvisoryRecords: []string{"source-1"}}
	writeRAGTelemetry(rn, rec)
	rec.Attempt = 2
	rec.AdvisoryStatus = ragStatusFailed
	rec.AdvisoryReason = ragReasonAdvisoryTimeout
	writeRAGTelemetry(rn, rec)
	files, _ := filepath.Glob(filepath.Join(dir, config.DirName, "runs", "T1", "memory-retrieval-*.json"))
	if len(files) != 2 {
		t.Fatalf("history count=%d", len(files))
	}
	b, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var stored ragTelemetry
	if err = json.Unmarshal(b, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.ObservedAt.IsZero() {
		t.Fatal("missing observation timestamp")
	}
	code, out, errOut := runCLI(t, dir, "memory", "status")
	if code != exitOK || !strings.Contains(out, "ADVISORY_TIMEOUT") {
		t.Fatalf("%d %s %s", code, out, errOut)
	}
}
