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

// These regressions prove the prompt-driven implementation path records an
// evidence-backed outcome verdict: successful verification (a satisfied declared
// deliverable), missing evidence (no declared requirement), and failed acceptance
// (a declared deliverable that is absent). A model PASS never yields VERIFIED on
// its own.

type promptDeliverableAgent struct {
	dir   string
	write string // repository-relative path to create on IMPLEMENT ("" creates nothing)
}

func (a *promptDeliverableAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Implement:
		if a.write != "" {
			if err := writeFixtureFile(filepath.Join(a.dir, filepath.FromSlash(a.write)), "done\n"); err != nil {
				return agent.Response{}, err
			}
		}
		return agent.Response{Content: "implemented", Outcome: &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "implemented", ChangesExpected: true}}, nil
	case agent.Review:
		return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

func promptOutcomeStatus(t *testing.T, dir string) string {
	t.Helper()
	base := filepath.Join(dir, stateDirName, "runs", "prompts")
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatalf("read prompts dir: %v", err)
	}
	found := ""
	for _, e := range entries {
		if e.IsDir() {
			found = e.Name()
		}
	}
	if found == "" {
		t.Fatal("no prompt run dir")
	}
	b, err := os.ReadFile(filepath.Join(base, found, "outcome.json"))
	if err != nil {
		t.Fatalf("read outcome.json: %v", err)
	}
	var doc struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal outcome.json: %v", err)
	}
	return doc.Status
}

func TestPromptOutcomeVerifiedWithSatisfiedDeliverable(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &promptDeliverableAgent{dir: dir, write: "out.md"}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/out.md b/out.md\n", a,
		"prompt", "--capability", "implement", "--deliverable", "out.md", "write the report")
	if code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "outcome: VERIFIED") {
		t.Errorf("stdout missing verified outcome: %q", stdout)
	}
	if got := promptOutcomeStatus(t, dir); got != "VERIFIED" {
		t.Errorf("outcome.json status = %q, want VERIFIED", got)
	}
}

func TestPromptOutcomeMissingEvidenceIsPartial(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &promptDeliverableAgent{dir: dir, write: "x.go"}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x.go b/x.go\n", a,
		"prompt", "--capability", "implement", "do something")
	if code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "outcome: PARTIAL") {
		t.Errorf("stdout missing partial outcome: %q", stdout)
	}
	if got := promptOutcomeStatus(t, dir); got != "PARTIAL" {
		t.Errorf("outcome.json status = %q, want PARTIAL", got)
	}
}

func TestPromptOutcomeFailedAcceptanceIsPartial(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	// The declared deliverable is never created, so acceptance is unmet even though
	// the run may PASS.
	a := &promptDeliverableAgent{dir: dir, write: ""}

	code, _, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a,
		"prompt", "--capability", "implement", "--deliverable", "missing.md", "write the report")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if got := promptOutcomeStatus(t, dir); got != "PARTIAL" {
		t.Errorf("outcome.json status = %q, want PARTIAL", got)
	}
}
