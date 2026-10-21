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
