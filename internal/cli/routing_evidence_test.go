package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/internal/adaptiveroute"
	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/runtrace"
)

// seedRoutingEvidence writes n run traces recording an IMPLEMENT run at the given model
// class with the given outcome, so the adaptive-routing evidence harvester has data.
func seedRoutingEvidence(t *testing.T, dir, class string, success bool, n int) {
	t.Helper()
	stage := "FAILED"
	if success {
		stage = "PASSED"
	}
	for i := 0; i < n; i++ {
		runDir := filepath.Join(dir, stateDirName, "runs", fmt.Sprintf("EV%03d", i))
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(runtrace.Trace{
			Execution:   runtrace.Execution{Capability: "implement", ModelClass: class},
			Termination: runtrace.Termination{Stage: stage},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(runDir, runtrace.FileName), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestRoutingEvidenceHarvestedFromTraces proves the harvester reads recorded run traces
// and that the policy escalates a failing baseline from the harvested evidence.
func TestRoutingEvidenceHarvestedFromTraces(t *testing.T) {
	dir := t.TempDir()
	seedRoutingEvidence(t, dir, "small", false, 3)

	ev := routingEvidence(dir)
	if len(ev) != 3 {
		t.Fatalf("evidence = %+v, want 3 outcomes", ev)
	}
	for _, o := range ev {
		if o.Class != model.ClassSmall || o.Capability != agent.Implement || o.Success {
			t.Errorf("outcome = %+v, want small/implement/failure", o)
		}
	}
	d := adaptiveroute.Route(adaptiveroute.Input{Baseline: model.ClassSmall, Capability: agent.Implement, Evidence: ev})
	if d.Class != model.ClassMedium {
		t.Errorf("class = %s, want medium", d.Class)
	}
}

// TestRoutingEvidenceIgnoresUnknownClass proves a trace without a known model class
// yields no evidence.
func TestRoutingEvidenceIgnoresUnknownClass(t *testing.T) {
	dir := t.TempDir()
	seedRoutingEvidence(t, dir, "bogus", false, 1)
	if ev := routingEvidence(dir); len(ev) != 0 {
		t.Errorf("an unknown model class must yield no evidence: %+v", ev)
	}
}

// TestRoutingAdaptiveEscalatesOnFailureEvidence proves the router's small baseline is
// escalated when harvested evidence shows the smallest class failing.
func TestRoutingAdaptiveEscalatesOnFailureEvidence(t *testing.T) {
	clearModelEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)
	seedRoutingEvidence(t, dir, "small", false, 3)

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", routingAgent(), fakeJEV(jev.NewClearFake()), "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	art := readRoutingArtifact(t, dir)
	if art.Class != "medium" {
		t.Fatalf("routing class = %q, want medium (escalated from a failing small baseline)", art.Class)
	}
}
