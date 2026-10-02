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

// Models the trusted native harness boundary; raw model JSON is tested separately.
type satisfiedAgent struct {
	outcomeAgent
	verified bool
	requests []agent.Request
}

func (a *satisfiedAgent) Generate(ctx context.Context, req agent.Request) (agent.Response, error) {
	a.requests = append(a.requests, req)
	response, err := a.outcomeAgent.Generate(ctx, req)
	if req.Capability == agent.Implement || req.Capability == agent.Fix {
		response.VerifiedAlreadySatisfied = a.verified
		response.Content = `{"status":"completed","completion":"ALREADY_SATISFIED","changes_expected":false}`
	}
	return response, err
}

func TestRunVerifiedAlreadySatisfied(t *testing.T) {
	for _, tc := range []struct {
		name       string
		verified   bool
		validation string
		wantPass   bool
	}{
		{"verified with green validation", true, "true", true},
		{"model claim only", false, "true", false},
		{"independent validation fails", true, "false", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "TASK.md", runTaskFile+"\n## Acceptance Criteria\n\n- existing implementation\n")
			writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  test:\n    - \""+tc.validation+"\"\nquality:\n  max_fix_cycles: 1\n")
			a := &satisfiedAgent{verified: tc.verified, outcomeAgent: outcomeAgent{outcome: &agent.Outcome{
				Status: agent.OutcomeCompleted, Completion: agent.AlreadySatisfied, ChangesExpected: false,
				Evidence: &agent.CompletionEvidence{Acceptance: []agent.AcceptanceEvidence{{Criterion: "existing implementation", Paths: []string{"existing.go"}}}, ValidationCommands: []string{tc.validation}},
			}}}
			code, stdout, stderr := runInjectedCLI(t, dir, "", a, "run", "--task", "TASK.md")
			if (code == exitOK) != tc.wantPass {
				t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
			}
			if tc.wantPass && !strings.Contains(stdout, "PASS") {
				t.Fatalf("stdout=%s", stdout)
			}
			var implementRequest *agent.Request
			for i := range a.requests {
				if a.requests[i].Capability == agent.Implement {
					implementRequest = &a.requests[i]
				}
			}
			if implementRequest == nil || len(implementRequest.AcceptanceCriteria) == 0 || len(implementRequest.ValidationCommands) != 1 || implementRequest.ValidationCommands[0] != tc.validation {
				t.Fatalf("contract not passed: %+v", implementRequest)
			}
			if tc.wantPass {
				reports, err := filepath.Glob(filepath.Join(dir, stateDirName, "runs", "*", "report.md"))
				if err != nil {
					t.Fatal(err)
				}
				if len(reports) != 1 {
					t.Fatalf("reports=%v", reports)
				}
				report, err := os.ReadFile(reports[0])
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(filepath.Join(filepath.Dir(reports[0]), "report.json"))
				if err != nil {
					t.Fatal(err)
				}
				var machine runReportDoc
				if err := json.Unmarshal(data, &machine); err != nil {
					t.Fatal(err)
				}
				if machine.Completion != agent.AlreadySatisfied || machine.RepositoryMutations == nil || *machine.RepositoryMutations != 0 || machine.AlreadySatisfied == nil {
					t.Fatalf("report=%s", data)
				}
				for _, want := range []string{"Completion: ALREADY_SATISFIED", "Repository mutations: 0", "existing.go"} {
					if !strings.Contains(string(report), want) {
						t.Errorf("report missing %q: %s", want, report)
					}
				}
			}
		})
	}
}
