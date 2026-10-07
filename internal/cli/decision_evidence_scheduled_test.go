package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/decision"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/planner"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

// TestScheduledTaskDecisionEvidenceEscalates closes the scheduler-path coverage gap
// in SEAM-006/007: provider-added attention must be enforced by SOP's own scheduler
// machinery. An adverse external provider raises a governed continuation to a human
// boundary, and SOP — not the provider — records the approval request and requeues
// the task. With no provider, the same task continues automatically, unchanged.
func TestScheduledTaskDecisionEvidenceEscalates(t *testing.T) {
	const cfgEnabled = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nautonomy:\n  level: high\ndecision:\n  enabled: true\n  provider: command\n  command:\n    - placeholder\n"
	const cfgDisabled = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nautonomy:\n  level: high\n"

	for _, tc := range []struct {
		name      string
		config    string
		mode      string
		off       bool
		wantHuman bool
	}{
		{"no provider continues", cfgDisabled, "", true, false},
		{"benign evidence continues", cfgEnabled, "low", false, false},
		{"adverse evidence escalates", cfgEnabled, "high", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := newDirtyRepo(t)
			initProject(t, dir)
			writeConfig(t, dir, tc.config)

			plan := planner.Plan{Project: "x", Summary: "S", Stages: []planner.Stage{{
				ID: "T001", Title: "Baseline", Objective: "Do it.",
				AcceptanceCriteria: []string{"done"},
			}}}
			data, err := json.Marshal(plan)
			if err != nil {
				t.Fatal(err)
			}
			writeRepoFile(t, dir, stateDirName+"/plan.json", string(data))

			task := &domain.Task{ID: "T001", Title: "Baseline", Objective: "Do it.", AcceptanceCriteria: "done", Status: domain.READY, MaxAttempts: 3}
			st, err := store.Open(statePath(dir))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			if err := st.Save(task); err != nil {
				t.Fatal(err)
			}

			d := defaultDeps()
			d.getwd = func() (string, error) { return dir, nil }
			a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: budgetExhaustedReason}}
			d.newAgent = func(string, string, string) (agent.Agent, error) { return a, nil }
			if !tc.off {
				p := crossModuleAdapter(t, "fake-external", tc.mode)
				d.newDecisionProvider = func(config.Config) (decision.Provider, error) { return p, nil }
			}

			cfg, err := loadConfigOrDefault(dir)
			if err != nil {
				t.Fatal(err)
			}

			var out, errOut bytes.Buffer
			code := runScheduledTask(context.Background(), dir, cfg, a, d, st, task, newRunSession(), &out, &errOut)
			if code != exitError {
				t.Fatalf("code=%d, want a non-pass result; stdout=%s stderr=%s", code, &out, &errOut)
			}

			rn, err := runpkg.New(dir, "T001")
			if err != nil {
				t.Fatal(err)
			}
			_, hasApproval := rn.Approval()
			if hasApproval != tc.wantHuman {
				t.Fatalf("approval recorded = %v, want %v; stdout=%s", hasApproval, tc.wantHuman, &out)
			}
			gotAuto := strings.Contains(out.String(), "decision=AUTO_CONTINUE")
			if gotAuto == tc.wantHuman {
				t.Fatalf("decision=AUTO_CONTINUE = %v, want %v (wantHuman=%v); stdout=%s", gotAuto, !tc.wantHuman, tc.wantHuman, &out)
			}
			if tc.wantHuman && !strings.Contains(out.String(), "decision=HUMAN_APPROVAL_REQUIRED") {
				t.Fatalf("adverse evidence must record a human-approval decision; stdout=%s", &out)
			}
		})
	}
}
