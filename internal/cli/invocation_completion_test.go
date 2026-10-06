package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

type invocationAgent struct {
	outcomeAgent
	mutate func()
	fix    func()
}

func (a invocationAgent) Generate(ctx context.Context, req agent.Request) (agent.Response, error) {
	if req.Capability == agent.Implement && a.mutate != nil {
		a.mutate()
	}
	if req.Capability == agent.Fix && a.fix != nil {
		a.fix()
	}
	return a.outcomeAgent.Generate(ctx, req)
}

// Use the production repository observer, not an injected post-implementation
// diff: inherited dirty work must neither prove mutation nor become task evidence.
func TestRunInvocationCompletionEvidence(t *testing.T) {
	for _, tc := range []struct {
		name            string
		changesExpected bool
		validation      string
		mutate          string
		wantPass        bool
	}{
		{"required mutation absent in dirty tree", true, "true", "", false},
		{"same-content write is not a mutation", true, "true", "noop", false},
		{"validated legacy no-change completion", false, "true", "", true},
		{"no-change validation fails", false, "false", "", false},
		{"no-change without validation evidence", false, "", "", false},
		{"actual repository mutation", true, "true", "source", true},
		{"mutation of pre-existing dirty file", true, "true", "tracked", true},
		{"deletion of pre-existing dirty file", true, "true", "delete", true},
		{"fix produces the required mutation", false, "test -f result.txt", "fix", true},
		{"SOP-owned mutation only", true, "true", "state", false},
		{"excluded report mutation only", true, "true", "report", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := newDirtyRepo(t)
			before, err := os.ReadFile(filepath.Join(dir, "tracked.txt"))
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, dir, "TASK.md", runTaskFile)
			cfg := "project:\n  name: x\nquality:\n  max_fix_cycles: 1\n"
			if tc.validation != "" {
				cfg += "validation:\n  build:\n    - " + tc.validation + "\n"
			}
			writeConfig(t, dir, cfg)
			a := invocationAgent{outcomeAgent: outcomeAgent{outcome: &agent.Outcome{
				Status: agent.OutcomeCompleted, ChangesExpected: tc.changesExpected,
			}}, mutate: func() {
				switch tc.mutate {
				case "source":
					writeFile(t, dir, "result.txt", "implemented\n")
				case "tracked":
					writeFile(t, dir, "tracked.txt", "implemented\n")
				case "noop":
					writeFile(t, dir, "tracked.txt", "user edit\n")
				case "delete":
					if err := os.Remove(filepath.Join(dir, "tracked.txt")); err != nil {
						t.Fatal(err)
					}
				case "state":
					writeFile(t, dir, ".agent-sdlc/diagnostic.txt", "runtime evidence\n")
				case "report":
					writeRepoFile(t, dir, "docs/reports/generated.md", "SOP report\n")
				}
			}}
			if tc.mutate == "fix" {
				a.fix = func() { writeFile(t, dir, "result.txt", "fixed\n") }
			}
			d := defaultDeps()
			d.getwd = func() (string, error) { return dir, nil }
			d.newAgent = func(string, string, string) (agent.Agent, error) { return a, nil }
			var out, errOut bytes.Buffer
			code := run([]string{"run", "--task", "TASK.md"}, &out, &errOut, d)
			if (code == exitOK) != tc.wantPass {
				t.Fatalf("code=%d stdout=%s stderr=%s", code, &out, &errOut)
			}
			after, err := os.ReadFile(filepath.Join(dir, "tracked.txt"))
			if tc.mutate != "tracked" && tc.mutate != "delete" && (err != nil || !bytes.Equal(before, after)) {
				t.Fatalf("unrelated user work changed: %q, %v", after, err)
			}
			if data, err := os.ReadFile(filepath.Join(dir, "untracked.txt")); err != nil || string(data) != "scratch\n" {
				t.Fatalf("unrelated untracked work changed: %q, %v", data, err)
			}
			data, err := os.ReadFile(filepath.Join(dir, stateDirName, "runs", "T001", "changed-files.json"))
			if tc.wantPass && tc.mutate != "" {
				want := "result.txt"
				if tc.mutate == "tracked" || tc.mutate == "delete" {
					want = "tracked.txt"
				}
				var paths []string
				if err != nil || json.Unmarshal(data, &paths) != nil || !sameStrings(paths, []string{want}) {
					t.Fatalf("task attribution = %s, %v; want only %s", data, err, want)
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("no invocation mutation, but task evidence was recorded: %s, %v", data, err)
			}
			if !tc.wantPass && tc.validation != "false" {
				want := "IMPLEMENT_NO_CHANGES"
				if tc.changesExpected {
					want = "NO_CHANGES_PRODUCED"
				}
				if !strings.Contains(out.String(), want) {
					t.Fatalf("missing bounded no-change classification %s: %s", want, &out)
				}
			}
		})
	}
}

func TestRunInvocationMutationObservationFailsClosed(t *testing.T) {
	dir := newDirtyRepo(t)
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - true\n")
	a := invocationAgent{outcomeAgent: outcomeAgent{outcome: &agent.Outcome{
		Status: agent.OutcomeCompleted, ChangesExpected: true,
	}}, mutate: func() { writeFile(t, dir, "result.txt", "implemented\n") }}
	d := defaultDeps()
	d.getwd = func() (string, error) { return dir, nil }
	d.newAgent = func(string, string, string) (agent.Agent, error) { return a, nil }
	calls := 0
	d.snapshotRepository = func(ctx context.Context, dir string) (map[string]string, error) {
		calls++
		if calls == 1 {
			return snapshotRepository(ctx, dir)
		}
		return nil, errors.New("observation unavailable")
	}
	var out, errOut bytes.Buffer
	code := run([]string{"run", "--task", "TASK.md"}, &out, &errOut, d)
	if code != exitError || strings.Contains(out.String(), "PASS") || !strings.Contains(errOut.String(), "implement mutation observation: observation unavailable") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, &out, &errOut)
	}
	// A dirty diff (including the real edit) cannot substitute for an unavailable
	// invocation observer or manufacture task-scoped implementation evidence.
	if data, err := os.ReadFile(filepath.Join(dir, stateDirName, "runs", "T001", "changed-files.json")); !os.IsNotExist(err) {
		t.Fatalf("unverified mutation was attributed: %s, %v", data, err)
	}
}
