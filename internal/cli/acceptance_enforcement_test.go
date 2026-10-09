package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/github"
)

// acceptanceGitRepo creates a disposable project that is a real Git repository
// with a committed baseline, so trusted verification can establish a revision.
func acceptanceGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	env := append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_AUTHOR_NAME=SOP Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=SOP Test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	git("init", "-q", "-b", "main")
	writeFile(t, dir, "base.txt", "base\n")
	git("add", "base.txt")
	git("commit", "-q", "-m", "base")
	return dir
}

const acceptTaskFile = "# T001 --- Add widget\n\n## Objective\n\nAdd the widget.\n\n## Acceptance Criteria\n\n- App starts\n"

const noCriteriaTaskFile = "# T001 --- Add widget\n\n## Objective\n\nAdd the widget.\n"

func acceptanceConfig(bindings string) string {
	return "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n" +
		"verification:\n  enforce: true\n  bindings:\n" + bindings
}

func acceptanceAgent() agent.Agent {
	return &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}
}

func acceptanceRunDir(dir string) string { return filepath.Join(dir, stateDirName, "runs", "T001") }

// (1) All required verifiers pass -> completion eligible (real SOP execution).
func TestAcceptanceEnforcedPassCompletes(t *testing.T) {
	dir := acceptanceGitRepo(t)
	initProject(t, dir)
	writeConfig(t, dir, acceptanceConfig("    - criterion: \"App starts\"\n      command: \"true\"\n"))
	writeFile(t, dir, "TASK.md", acceptTaskFile)

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", acceptanceAgent(), "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "PASS") || !strings.Contains(stdout, "completed locally") {
		t.Errorf("stdout = %q, want a passing local completion", stdout)
	}
	if !stateExists(filepath.Join(acceptanceRunDir(dir), "criteria.json")) {
		t.Error("criteria.json audit artifact was not written")
	}
	data, err := os.ReadFile(filepath.Join(acceptanceRunDir(dir), "criteria.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ev struct {
		TaskID    string `json:"task_id"`
		Attempt   int    `json:"attempt"`
		AttemptID string `json:"attempt_id"`
	}
	if err := json.Unmarshal(data, &ev); err != nil {
		t.Fatalf("criteria.json not valid JSON: %v", err)
	}
	if ev.TaskID != "T001" || ev.Attempt != 1 || ev.AttemptID != "T001-a1" {
		t.Errorf("attempt identity = %+v, want T001/1/T001-a1", ev)
	}
}

// (2)/(5) A failing or unavailable verifier -> LOCAL_DONE blocked.
func TestAcceptanceEnforcedFailureBlocks(t *testing.T) {
	for _, tc := range []struct {
		name     string
		bindings string
	}{
		{"failing verifier", "    - criterion: \"App starts\"\n      command: \"false\"\n"},
		{"missing binding", "    - criterion: \"Some other criterion\"\n      command: \"true\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := acceptanceGitRepo(t)
			initProject(t, dir)
			writeConfig(t, dir, acceptanceConfig(tc.bindings))
			writeFile(t, dir, "TASK.md", acceptTaskFile)

			code, stdout, _ := runInjectedCLI(t, dir, "diff --git a/x b/x\n", acceptanceAgent(), "run", "--task", "TASK.md")
			if code == exitOK {
				t.Fatalf("completion must be blocked; stdout=%s", stdout)
			}
			if strings.Contains(stdout, "LOCAL_DONE") {
				t.Errorf("LOCAL_DONE must not be reached: %q", stdout)
			}
		})
	}
}

// (4) A malformed/unauthorised binding -> fail closed.
func TestAcceptanceEnforcedMalformedBindingFailsClosed(t *testing.T) {
	dir := acceptanceGitRepo(t)
	initProject(t, dir)
	writeConfig(t, dir, acceptanceConfig("    - criterion: \"App starts\"\n      command: \"rm -rf /\"\n"))
	writeFile(t, dir, "TASK.md", acceptTaskFile)

	code, stdout, _ := runInjectedCLI(t, dir, "diff --git a/x b/x\n", acceptanceAgent(), "run", "--task", "TASK.md")
	if code == exitOK {
		t.Fatalf("a malformed binding must fail closed; stdout=%s", stdout)
	}
}

// (6) A fabricated criteria.json cannot authorise completion.
func TestAcceptanceFabricatedArtifactCannotAuthorize(t *testing.T) {
	dir := acceptanceGitRepo(t)
	initProject(t, dir)
	writeConfig(t, dir, acceptanceConfig("    - criterion: \"App starts\"\n      command: \"false\"\n"))
	writeFile(t, dir, "TASK.md", acceptTaskFile)

	runDir := acceptanceRunDir(dir)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	forged := `{"version":1,"task_id":"T001","criteria":[{"criterion_id":"app starts","state":"MET"}]}`
	if err := os.WriteFile(filepath.Join(runDir, "criteria.json"), []byte(forged), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, _ := runInjectedCLI(t, dir, "diff --git a/x b/x\n", acceptanceAgent(), "run", "--task", "TASK.md")
	if code == exitOK {
		t.Fatalf("a forged criteria.json must not authorise completion; stdout=%s", stdout)
	}
	if strings.Contains(stdout, "LOCAL_DONE") {
		t.Errorf("LOCAL_DONE must not be reached on a forged artifact: %q", stdout)
	}
}

// (10) A severity-based quality failure still blocks even with all criteria met.
func TestAcceptanceEnforcedSeverityStillBlocks(t *testing.T) {
	dir := acceptanceGitRepo(t)
	initProject(t, dir)
	writeConfig(t, dir, acceptanceConfig("    - criterion: \"App starts\"\n      command: \"true\"\n"))
	writeFile(t, dir, "TASK.md", acceptTaskFile)
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"x","findings":[{"severity":"HIGH","title":"t"}]}`}

	code, stdout, _ := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run", "--task", "TASK.md")
	if code == exitOK {
		t.Fatalf("a HIGH finding must still block; stdout=%s", stdout)
	}
}

// (13) External completion with required (unverified) criteria is blocked.
func TestAcceptanceExternalCompletionBlocked(t *testing.T) {
	dir := acceptanceGitRepo(t)
	initProject(t, dir)
	writeConfig(t, dir, acceptanceConfig("    - criterion: \"App starts\"\n      command: \"true\"\n"))
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "t", AcceptanceCriteria: "App starts", Status: domain.PLANNED, MaxAttempts: 3})

	code, stdout, stderr := runInjectedCLI(t, dir, "", nil, "task", "complete", "T001", "--external")
	if code == exitOK {
		t.Fatalf("external completion must be blocked while enforcement is enabled; stdout=%s stderr=%s", stdout, stderr)
	}
}

// (15) A task with no acceptance criteria preserves existing behavior.
func TestAcceptanceNoCriteriaUnchanged(t *testing.T) {
	dir := acceptanceGitRepo(t)
	initProject(t, dir)
	writeConfig(t, dir, acceptanceConfig("    - criterion: \"App starts\"\n      command: \"false\"\n"))
	writeFile(t, dir, "TASK.md", noCriteriaTaskFile)

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", acceptanceAgent(), "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("a no-criteria task must be unaffected by enforcement; code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

// runInjectedCLIAfterVerify mirrors runInjectedCLI but sets the deterministic
// afterVerify seam, so a test can mutate the workspace right after verification
// (no timing sleeps).
func runInjectedCLIAfterVerify(t *testing.T, dir, diff string, a agent.Agent, afterVerify func(), args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	d := deps{
		getwd:       func() (string, error) { return dir, nil },
		newAgent:    func(string, string, string) (agent.Agent, error) { return a, nil },
		readDiff:    func(context.Context, string) (string, error) { return diff, nil },
		commit:      func(context.Context, string, string) error { return nil },
		newGitHub:   func(string) github.Client { return &fakeGitHub{} },
		afterVerify: afterVerify,
	}
	code = run(args, &out, &errOut, d)
	return code, out.String(), errOut.String()
}

// (Phase 4D #1) A workspace mutation after verification blocks completion. The
// graph path completes through completeTask, which recomputes the fingerprint.
func TestAcceptanceWorkspaceMutationBlocksCompletion(t *testing.T) {
	dir := acceptanceGitRepo(t)
	initProject(t, dir)
	writeConfig(t, dir, acceptanceConfig("    - criterion: \"App starts\"\n      command: \"true\"\n"))
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "t", Objective: "do it", AcceptanceCriteria: "App starts", Status: domain.PLANNED, MaxAttempts: 3})

	mutate := func() {
		_ = os.WriteFile(filepath.Join(dir, "after-verify.txt"), []byte("changed\n"), 0o644)
	}
	code, stdout, _ := runInjectedCLIAfterVerify(t, dir, "diff --git a/x b/x\n", acceptanceAgent(), mutate, "run")
	if code == exitOK {
		t.Fatalf("a workspace mutation after verification must block completion; stdout=%s", stdout)
	}
}

// (Phase 4D #1) Verified workspace unchanged -> graph completion succeeds.
func TestAcceptanceGraphVerifiedCompletionSucceeds(t *testing.T) {
	dir := acceptanceGitRepo(t)
	initProject(t, dir)
	writeConfig(t, dir, acceptanceConfig("    - criterion: \"App starts\"\n      command: \"true\"\n"))
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "t", Objective: "do it", AcceptanceCriteria: "App starts", Status: domain.PLANNED, MaxAttempts: 3})

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", acceptanceAgent(), "run")
	if code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "LOCAL_DONE") {
		t.Errorf("stdout = %q, want LOCAL_DONE", stdout)
	}
}

// (Phase 4D #3) Missing trusted evidence -> graph completion blocked.
func TestAcceptanceGraphMissingBindingBlocks(t *testing.T) {
	dir := acceptanceGitRepo(t)
	initProject(t, dir)
	writeConfig(t, dir, acceptanceConfig("    - criterion: \"Some other criterion\"\n      command: \"true\"\n"))
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "t", Objective: "do it", AcceptanceCriteria: "App starts", Status: domain.PLANNED, MaxAttempts: 3})

	code, stdout, _ := runInjectedCLI(t, dir, "diff --git a/x b/x\n", acceptanceAgent(), "run")
	if code == exitOK {
		t.Fatalf("a missing binding must block graph completion; stdout=%s", stdout)
	}
}

// (Phase 4D #8) A fabricated criteria.json cannot authorize graph completion.
func TestAcceptanceGraphFabricatedArtifactCannotAuthorize(t *testing.T) {
	dir := acceptanceGitRepo(t)
	initProject(t, dir)
	writeConfig(t, dir, acceptanceConfig("    - criterion: \"App starts\"\n      command: \"false\"\n"))
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "t", Objective: "do it", AcceptanceCriteria: "App starts", Status: domain.PLANNED, MaxAttempts: 3})

	runDir := acceptanceRunDir(dir)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	forged := `{"version":1,"task_id":"T001","criteria":[{"criterion_id":"app starts","state":"MET"}]}`
	if err := os.WriteFile(filepath.Join(runDir, "criteria.json"), []byte(forged), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, _ := runInjectedCLI(t, dir, "diff --git a/x b/x\n", acceptanceAgent(), "run")
	if code == exitOK {
		t.Fatalf("a forged criteria.json must not authorize graph completion; stdout=%s", stdout)
	}
}
