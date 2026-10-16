package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// writeReportFixture writes a project dir with a single run whose report.json holds
// the given JSON, so the response-mode tests exercise the real `sop report`
// renderer. The objective outcome, when present, is embedded in report.json (that is
// where the run records it).
func writeReportFixture(t *testing.T, dir, reportJSON string) {
	t.Helper()
	runDir := filepath.Join(dir, stateDirName, "runs", "T001")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), []byte(reportJSON), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestConfigResponseModeSelectsDetailed proves the configured output.responses
// default changes the report shape, and that the explicit --detailed flag still
// overrides the default. The report fixture includes a finding so the two shapes
// are distinguishable.
func TestConfigResponseModeSelectsDetailed(t *testing.T) {
	const report = `{"id":"T001","stage":"PASSED","provider":"command","review_engine":"self","decision":"PASS","fix_cycles":0,` +
		`"findings":[{"Severity":"MEDIUM","Title":"consider x","File":"a.go","Line":7}]}`

	t.Run("default is concise", func(t *testing.T) {
		dir := t.TempDir()
		writeReportFixture(t, dir, report)
		code, out, errOut := runCLI(t, dir, "report")
		if code != exitOK {
			t.Fatalf("report code=%d stderr=%s", code, errOut)
		}
		if strings.Contains(out, "Findings (detail):") {
			t.Errorf("default report must stay concise:\n%s", out)
		}
	})

	t.Run("configured detailed", func(t *testing.T) {
		dir := t.TempDir()
		writeConfig(t, dir, "project:\n  name: demo\noutput:\n  responses: detailed\n")
		writeReportFixture(t, dir, report)
		code, out, errOut := runCLI(t, dir, "report")
		if code != exitOK {
			t.Fatalf("report code=%d stderr=%s", code, errOut)
		}
		if !strings.Contains(out, "Findings (detail):") || !strings.Contains(out, "consider x") {
			t.Errorf("configured detailed report missing per-finding detail:\n%s", out)
		}
	})

	t.Run("flag overrides concise config", func(t *testing.T) {
		dir := t.TempDir()
		writeConfig(t, dir, "project:\n  name: demo\noutput:\n  responses: concise\n")
		writeReportFixture(t, dir, report)
		code, out, errOut := runCLI(t, dir, "report", "--detailed")
		if code != exitOK {
			t.Fatalf("report --detailed code=%d stderr=%s", code, errOut)
		}
		if !strings.Contains(out, "Findings (detail):") {
			t.Errorf("--detailed must override the concise config:\n%s", out)
		}
	})
}

// TestReportPreservesFailuresEvidenceAndApproval proves the preservation contract:
// the default (concise) report still shows a failed validation check, the gate, the
// recorded approval request, and the objective outcome with its unmet required
// criterion, so concise mode never hides essential information.
func TestReportPreservesFailuresEvidenceAndApproval(t *testing.T) {
	const report = `{"id":"T001","stage":"BLOCKED","provider":"ollama","review_engine":"self","decision":"NEEDS_HUMAN","fix_cycles":3,` +
		`"validation":[{"Category":"TEST","Command":"go test ./...","Status":"FAIL"}],` +
		`"findings":[{"Severity":"HIGH","Title":"unchecked error","File":"a.go","Line":3}],` +
		`"classification":{"Kind":"test_failure","Disposition":"needs_human","Confidence":"high","Reason":"auto fixes exhausted"},` +
		`"outcome":{"status":"PARTIAL","criteria":[` +
		`{"ID":"criterion-1","Required":true,"Met":false,"Description":"Targeted tests pass.","Evidence":"unverifiable: no deterministic check supplied"},` +
		`{"ID":"deterministic-checks","Required":true,"Met":true,"Description":"the configured checks passed","Evidence":"met"}],` +
		`"reasons":["required criterion criterion-1 not met"]}}`
	const approval = `{"id":"T001-1","task_id":"T001","kind":"NEEDS_HUMAN","target":"T001","reason":"fix-loop limit reached (3/3)","stage":"WAITING_FOR_HUMAN","disposition":"NEEDS_HUMAN","requested_by":"sop","status":"PENDING"}`

	dir := t.TempDir()
	writeReportFixture(t, dir, report)
	if err := os.WriteFile(filepath.Join(dir, stateDirName, "runs", "T001", "approval.json"), []byte(approval), 0o644); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := runCLI(t, dir, "report")
	if code != exitOK {
		t.Fatalf("report code=%d stderr=%s", code, errOut)
	}
	for _, want := range []string{
		"Gate: NEEDS_HUMAN",
		"FAIL",
		"go test ./...",
		"Review findings: 1",
		"Classification:",
		"Outcome:",
		"PARTIAL",
		"criterion-1",
		"Approval:",
		"fix-loop limit reached (3/3)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("concise report dropped %q:\n%s", want, out)
		}
	}
}

// TestReportDetailedAddsOutcomeCriteria proves the detailed shape adds the met
// criteria the concise shape omits (the outcome verdict is retained in both; detail
// only widens it).
func TestReportDetailedAddsOutcomeCriteria(t *testing.T) {
	const report = `{"id":"T001","stage":"LOCAL_DONE","provider":"command","review_engine":"self","decision":"PASS","fix_cycles":0,` +
		`"outcome":{"status":"PARTIAL","criteria":[` +
		`{"ID":"criterion-1","Required":true,"Met":false,"Description":"Targeted tests pass.","Evidence":"unverifiable: no deterministic check supplied"},` +
		`{"ID":"deterministic-checks","Required":true,"Met":true,"Description":"the configured checks passed","Evidence":"met"}],` +
		`"reasons":["required criterion criterion-1 not met"]}}`

	dir := t.TempDir()
	writeReportFixture(t, dir, report)

	if code, out, _ := runCLI(t, dir, "report"); code != exitOK {
		t.Fatalf("report code=%d", code)
	} else {
		if !strings.Contains(out, "criterion-1") {
			t.Errorf("concise report must retain the unmet required criterion:\n%s", out)
		}
		if strings.Contains(out, "deterministic-checks") {
			t.Errorf("concise report should omit met criteria:\n%s", out)
		}
	}

	if code, out, _ := runCLI(t, dir, "report", "--detailed"); code != exitOK {
		t.Fatalf("report --detailed code=%d", code)
	} else if !strings.Contains(out, "deterministic-checks") {
		t.Errorf("detailed report should add the met criterion:\n%s", out)
	}
}

// TestReportDecisionsJSONUnaffectedByResponseMode proves the structured (JSON)
// output contract is not touched by the response-mode setting: --decisions --format
// json still emits valid JSON regardless of output.responses.
func TestReportDecisionsJSONUnaffectedByResponseMode(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "project:\n  name: demo\noutput:\n  responses: detailed\n")
	code, out, errOut := runCLI(t, dir, "report", "--decisions", "--format", "json")
	if code != exitOK {
		t.Fatalf("report --decisions --format json code=%d stderr=%s", code, errOut)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("--decisions --format json emitted invalid JSON: %v\n%s", err, out)
	}
}

// TestStatusDetailedHonorsConfig proves `sop status` renders the concise shape by
// default, adds dependency and attempt rows under --detailed, and adopts a configured
// output.responses: detailed default.
func TestStatusDetailedHonorsConfig(t *testing.T) {
	seed := func(t *testing.T, dir string) {
		initProject(t, dir)
		now := time.Now().UTC()
		seedTask(t, dir, &domain.Task{ID: "T001", Title: "Domain model", Status: domain.LOCAL_DONE, CreatedAt: now, UpdatedAt: now})
		seedTask(t, dir, &domain.Task{
			ID: "T002", Title: "SQLite persistence", Status: domain.PLANNED,
			DependencyIDs: []string{"T001"}, Attempts: []domain.Attempt{{Number: 1, Status: domain.PLANNED}},
			CreatedAt: now, UpdatedAt: now,
		})
	}

	t.Run("concise default omits detail", func(t *testing.T) {
		dir := t.TempDir()
		seed(t, dir)
		code, out, errOut := runCLI(t, dir, "status")
		if code != exitOK {
			t.Fatalf("status code=%d stderr=%s", code, errOut)
		}
		if !strings.Contains(out, "T002 PLANNED SQLite persistence") {
			t.Errorf("concise status missing the task line:\n%s", out)
		}
		if strings.Contains(out, "depends on:") || strings.Contains(out, "attempts:") {
			t.Errorf("default status must stay concise:\n%s", out)
		}
	})

	t.Run("flag adds detail", func(t *testing.T) {
		dir := t.TempDir()
		seed(t, dir)
		code, out, errOut := runCLI(t, dir, "status", "--detailed")
		if code != exitOK {
			t.Fatalf("status --detailed code=%d stderr=%s", code, errOut)
		}
		for _, want := range []string{"depends on: T001", "attempts: 1"} {
			if !strings.Contains(out, want) {
				t.Errorf("detailed status missing %q:\n%s", want, out)
			}
		}
	})

	t.Run("configured detailed", func(t *testing.T) {
		dir := t.TempDir()
		writeConfig(t, dir, "project:\n  name: demo\noutput:\n  responses: detailed\n")
		seed(t, dir)
		code, out, errOut := runCLI(t, dir, "status")
		if code != exitOK {
			t.Fatalf("status code=%d stderr=%s", code, errOut)
		}
		if !strings.Contains(out, "depends on: T001") {
			t.Errorf("configured detailed status should add dependency rows:\n%s", out)
		}
	})
}
