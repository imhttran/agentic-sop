package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/planner"
)

func historicalCLIFixture(t *testing.T) (dir, archiveRoot, evidenceHash string) {
	t.Helper()
	dir = t.TempDir()
	archiveRoot = filepath.Join(dir, ".agent-sdlc", "archive", "worktree-evidence", "test-plan")
	archiveDir := filepath.Join(archiveRoot, "archive")
	for _, p := range []string{
		filepath.Join(dir, ".agent-sdlc"),
		filepath.Join(dir, "docs", "plans"),
		filepath.Join(dir, "docs", "reports"),
		archiveDir,
	} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, ".gitignore"), ".agent-sdlc/\n")
	write(filepath.Join(dir, ".agent-sdlc", "config.yaml"), "project:\n  name: \"test\"\nhuman:\n  designated_approvers:\n    - \"Approver\"\n")
	write(filepath.Join(dir, "docs", "plans", "PLAN-TEST.md"), "# Test plan\n")
	evidence := "# Evidence\n\n## Reviewed section\nordinary evidence\n"
	write(filepath.Join(dir, "docs", "reports", "evidence.md"), evidence)
	sum := sha256.Sum256([]byte(evidence))
	evidenceHash = hex.EncodeToString(sum[:])

	plan := planner.Plan{Stages: []planner.Stage{
		{
			ID:                 "STAGE-A",
			AcceptanceCriteria: []string{"manual criterion"},
			AcceptanceMapping:  []planner.AcceptanceMapping{{Criterion: 1, Manual: "docs/reports/evidence.md §Reviewed section"}},
		},
		{
			ID:                 "STAGE-B",
			AcceptanceCriteria: []string{"check criterion"},
			AcceptanceChecks:   []string{"git rev-parse HEAD"},
			AcceptanceMapping:  []planner.AcceptanceMapping{{Criterion: 1, Check: 1}},
		},
	}}
	if err := writeJSONFile(filepath.Join(archiveDir, "plan.json"), plan); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(filepath.Join(archiveDir, "tasks.json"), []domain.Task{
		{ID: "STAGE-A", AcceptanceCriteria: "manual criterion"},
		{ID: "STAGE-B", AcceptanceCriteria: "check criterion"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(filepath.Join(archiveDir, "lifecycle.json"), map[string]string{
		"plan_id": "plan-test", "source": "docs/plans/PLAN-TEST.md", "disposition": "COMPLETE", "recorded_at": "2026-11-10T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test")
	runGit("add", ".")
	runGit("commit", "-m", "fixture")
	return dir, archiveRoot, evidenceHash
}

func TestHistoricalVerifyAndAcceptCLI(t *testing.T) {
	dir, archiveRoot, hash := historicalCLIFixture(t)
	base := []string{"--plan", "docs/plans/PLAN-TEST.md", "--archive", archiveRoot}

	code, out, errOut := runCLI(t, dir, append([]string{"verify"}, base...)...)
	if code != 0 {
		t.Fatalf("verify code=%d stderr=%s", code, errOut)
	}
	for _, want := range []string{"mapped: 2 (1 manual, 1 check)", "accepted: 0/2", "decision-eligible: 1", "check unverified: 1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("verify output missing %q:\n%s", want, out)
		}
	}

	code, out, errOut = runCLI(t, dir, append([]string{"outcome"}, append(base,
		"--accept", "STAGE-A.criterion-1",
		"--evidence", "reviewed evidence",
		"--evidence-ref", "path=docs/reports/evidence.md,sha256="+hash+",section=Reviewed section",
		"--by", "Approver")...)...)
	if code != 0 {
		t.Fatalf("accept code=%d stderr=%s", code, errOut)
	}
	if !strings.Contains(out, "recorded historical-plan accept decision") {
		t.Fatalf("accept output:\n%s", out)
	}

	code, out, errOut = runCLI(t, dir, append([]string{"outcome"}, append(base, "--run-check", "STAGE-B.criterion-1", "--by", "Operator")...)...)
	if code != 0 {
		t.Fatalf("run-check code=%d stderr=%s", code, errOut)
	}
	if !strings.Contains(out, "recorded historical check") {
		t.Fatalf("run-check output:\n%s", out)
	}

	code, out, errOut = runCLI(t, dir, append([]string{"verify"}, base...)...)
	if code != 0 {
		t.Fatalf("verify after code=%d stderr=%s", code, errOut)
	}
	for _, want := range []string{"manually accepted: 1", "check passed: 1", "accepted: 2/2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("verify-after output missing %q:\n%s", want, out)
		}
	}
}

func TestHistoricalCheckRejectsManualAcceptance(t *testing.T) {
	dir, archiveRoot, hash := historicalCLIFixture(t)
	code, _, errOut := runCLI(t, dir,
		"outcome", "--plan", "docs/plans/PLAN-TEST.md", "--archive", archiveRoot,
		"--accept", "STAGE-B.criterion-1",
		"--evidence", "e",
		"--evidence-ref", "path=docs/reports/evidence.md,sha256="+hash,
		"--by", "Approver")
	if code == 0 || !strings.Contains(errOut, "check-mapped") {
		t.Fatalf("code=%d stderr=%s, want check-mapped rejection", code, errOut)
	}
}

func TestHistoricalCheckRevisionAssertion(t *testing.T) {
	dir, archiveRoot, _ := historicalCLIFixture(t)
	code, _, errOut := runCLI(t, dir,
		"outcome", "--plan", "docs/plans/PLAN-TEST.md", "--archive", archiveRoot,
		"--run-check", "STAGE-B.criterion-1", "--tested-revision", "0000000000000000000000000000000000000000")
	if code == 0 || !strings.Contains(errOut, "assertion failed") {
		t.Fatalf("code=%d stderr=%s, want revision assertion failure", code, errOut)
	}
}

func TestHistoricalNonDesignatedApproverRejected(t *testing.T) {
	dir, archiveRoot, hash := historicalCLIFixture(t)
	code, _, errOut := runCLI(t, dir,
		"outcome", "--plan", "docs/plans/PLAN-TEST.md", "--archive", archiveRoot,
		"--accept", "STAGE-A.criterion-1", "--evidence", "e",
		"--evidence-ref", "path=docs/reports/evidence.md,sha256="+hash,
		"--by", "NotApprover")
	if code == 0 || !strings.Contains(errOut, "not a designated approver") {
		t.Fatalf("code=%d stderr=%s, want designated-approver rejection", code, errOut)
	}
}

func TestHistoricalRejectCLI(t *testing.T) {
	dir, archiveRoot, hash := historicalCLIFixture(t)
	base := []string{"--plan", "docs/plans/PLAN-TEST.md", "--archive", archiveRoot}
	code, out, errOut := runCLI(t, dir, append([]string{"outcome"}, append(base,
		"--reject", "STAGE-A.criterion-1",
		"--evidence", "reviewed and declined",
		"--evidence-ref", "path=docs/reports/evidence.md,sha256="+hash+",section=Reviewed section",
		"--by", "Approver")...)...)
	if code != 0 {
		t.Fatalf("reject code=%d stderr=%s", code, errOut)
	}
	if !strings.Contains(out, "recorded historical-plan reject decision") {
		t.Fatalf("reject output:\n%s", out)
	}
	code, out, errOut = runCLI(t, dir, append([]string{"verify"}, base...)...)
	if code != 0 {
		t.Fatalf("verify code=%d stderr=%s", code, errOut)
	}
	if !strings.Contains(out, "rejected: 1") || !strings.Contains(out, "accepted: 0/2") {
		t.Fatalf("verify output missing rejection:\n%s", out)
	}
}
