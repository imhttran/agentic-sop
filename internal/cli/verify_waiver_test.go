package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/outcome"
)

// These are the focused SOP-HARD-04 recovery tests. They exercise the read-only
// `sop verify` and the designated-approver `sop outcome --waive` surface directly
// (no SQLite, no model), so they run in milliseconds and are the package-local half
// of HARD-04's acceptance checks.

func designatedConfig(approvers ...string) string {
	var b strings.Builder
	b.WriteString("version: 1\nproject:\n  name: t\nagent:\n  harness: tool\n  provider: ollama\n  model: m\nreview:\n  engine: self\nhuman:\n  designated_approvers:\n")
	for _, a := range approvers {
		b.WriteString("    - " + a + "\n")
	}
	return b.String()
}

func writeDesignatedApprovers(t *testing.T, dir string, approvers ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, stateDirName), 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	writeFile(t, dir, filepath.Join(stateDirName, "config.yaml"), designatedConfig(approvers...))
}

func writeOutcomeArtifactFor(t *testing.T, dir, taskID string, res *outcomeArtifact) {
	t.Helper()
	runDir := filepath.Join(dir, stateDirName, "runs", taskID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal outcome: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "outcome.json"), data, 0o644); err != nil {
		t.Fatalf("write outcome: %v", err)
	}
}

func readWaiverFile(t *testing.T, dir, taskID string) []outcome.WaiverRecord {
	t.Helper()
	recs, err := readWaiverRecords(dir, taskID)
	if err != nil {
		t.Fatalf("read waivers: %v", err)
	}
	return recs
}

func assertNoWaiverFile(t *testing.T, dir, taskID string) {
	t.Helper()
	if _, err := os.Stat(waiverAuditPath(dir, taskID)); err == nil {
		t.Fatalf("waiver audit was written for %s when it should not have been", taskID)
	}
}

func waivablePartial() *outcomeArtifact {
	return &outcomeArtifact{
		Status: outcome.Partial,
		Criteria: []outcome.CriterionResult{
			{ID: "deterministic-checks", Required: true, Met: true, Evidence: "met"},
			{ID: "deliverable-1", Required: true, Met: false, Description: "docs/operations/platform-support.md", Evidence: `deliverable "docs/operations/platform-support.md" not present`},
			{ID: "criterion-1", Required: false, Met: false, Description: "prose", Evidence: "unverifiable: no deterministic check supplied"},
		},
		Reasons: []string{"required criterion deliverable-1 not met"},
	}
}

// TestRunVerifyReadOnlyPrintsCriteria pins that `sop verify` prints the per-criterion
// verdict and performs no state transition (state.db is byte-identical).
func TestRunVerifyReadOnlyPrintsCriteria(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeOutcomeArtifactFor(t, dir, "T1", waivablePartial())
	before := readStateBytes(t, dir)

	code, stdout, stderr := runCLI(t, dir, "verify", "T1")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{"status: PARTIAL", "deliverable-1", "not met", "informational"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("verify output missing %q:\n%s", want, stdout)
		}
	}
	if !bytes.Equal(before, readStateBytes(t, dir)) {
		t.Fatal("`sop verify` mutated state.db; it must be strictly read-only")
	}
}

// TestRunVerifyUsageAndNoOutcome covers the usage error and the no-outcome case.
func TestRunVerifyUsageAndNoOutcome(t *testing.T) {
	if code, _, _ := runCLI(t, t.TempDir(), "verify"); code != exitUsage {
		t.Fatalf("verify with no args: code=%d, want %d", code, exitUsage)
	}
	dir := t.TempDir()
	initProject(t, dir)
	code, stdout, _ := runCLI(t, dir, "verify", "MISSING")
	if code != exitOK {
		t.Fatalf("verify missing outcome: code=%d, want %d", code, exitOK)
	}
	if !strings.Contains(stdout, "no recorded outcome") {
		t.Errorf("verify missing outcome should say so: %q", stdout)
	}
}

// TestRecordWaiverRejectsNonDesignatedApprover pins that only a designated approver
// can waive: a non-designated identity is rejected and nothing is written.
func TestRecordWaiverRejectsNonDesignatedApprover(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeDesignatedApprovers(t, dir, "operator")
	writeOutcomeArtifactFor(t, dir, "T1", waivablePartial())

	code, _, stderr := runCLI(t, dir, "outcome", "T1", "--waive", "deliverable-1", "--reason", "r", "--by", "mallory")
	if code == exitOK {
		t.Fatal("a non-designated approver was allowed to waive")
	}
	if !strings.Contains(stderr, "not a designated approver") {
		t.Errorf("stderr should name the reason: %q", stderr)
	}
	assertNoWaiverFile(t, dir, "T1")
}

// TestRecordWaiverRejectsMissingReasonAndNonUnmetCriterion pins that a missing
// reason and a criterion that is not an unmet required criterion are both rejected
// without writing.
func TestRecordWaiverRejectsMissingReasonAndNonUnmetCriterion(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeDesignatedApprovers(t, dir, "operator")
	writeOutcomeArtifactFor(t, dir, "T1", waivablePartial())

	if code, _, _ := runCLI(t, dir, "outcome", "T1", "--waive", "deliverable-1", "--by", "operator"); code == exitOK {
		t.Fatal("a waiver without a reason was accepted")
	}
	if code, _, _ := runCLI(t, dir, "outcome", "T1", "--waive", "deterministic-checks", "--reason", "r", "--by", "operator"); code == exitOK {
		t.Fatal("a met criterion was waived")
	}
	if code, _, _ := runCLI(t, dir, "outcome", "T1", "--waive", "criterion-1", "--reason", "r", "--by", "operator"); code == exitOK {
		t.Fatal("an informational criterion was waived")
	}
	assertNoWaiverFile(t, dir, "T1")
}

// TestRecordWaiverAppendsAndSurfaces pins the happy path: a designated approver
// records an attributed, reasoned, timestamped waiver as durable append-only audit
// evidence, and `sop verify` surfaces it. state.db is untouched.
func TestRecordWaiverAppendsAndSurfaces(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeDesignatedApprovers(t, dir, "operator")
	writeOutcomeArtifactFor(t, dir, "T1", waivablePartial())
	before := readStateBytes(t, dir)

	code, stdout, stderr := runCLI(t, dir, "outcome", "T1", "--waive", "deliverable-1", "--reason", "legitimate local-done", "--by", "operator")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "recorded waiver") {
		t.Errorf("waiver should be surfaced on record: %q", stdout)
	}

	recs := readWaiverFile(t, dir, "T1")
	if len(recs) != 1 {
		t.Fatalf("want 1 waiver record, got %d", len(recs))
	}
	r := recs[0]
	if r.Task != "T1" || r.CriterionID != "deliverable-1" || r.Approver != "operator" || r.Reason == "" || r.RecordedAt == "" || r.Outcome != outcome.Partial {
		t.Fatalf("waiver record is incomplete: %+v", r)
	}
	if r.Scope == "" {
		t.Error("waiver scope should default to the criterion it covers")
	}

	// Append-only: a second grant extends the audit, never rewrites it.
	if code, _, se := runCLI(t, dir, "outcome", "T1", "--waive", "deliverable-1", "--reason", "again", "--by", "operator"); code != exitOK {
		t.Fatalf("second waiver: code=%d stderr=%s", code, se)
	}
	if got := len(readWaiverFile(t, dir, "T1")); got != 2 {
		t.Fatalf("append-only audit should hold 2 records, got %d", got)
	}

	// Surfaced by `sop verify`.
	_, vout, _ := runCLI(t, dir, "verify", "T1")
	if !strings.Contains(vout, "waivers:") || !strings.Contains(vout, "operator") {
		t.Errorf("verify should surface the recorded waiver: %q", vout)
	}

	if !bytes.Equal(before, readStateBytes(t, dir)) {
		t.Fatal("recording a waiver mutated state.db; the audit is a separate artifact")
	}
}

// TestRecordedWaiverResolvesGatedPartial pins the functional wiring: with the gate
// ON, a recorded designated-approver waiver resolves a waivable PARTIAL to
// LOCAL_DONE, and its absence leaves it at NEEDS_HUMAN (fail-closed).
func TestRecordedWaiverResolvesGatedPartial(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeDesignatedApprovers(t, dir, "operator")
	writeOutcomeArtifactFor(t, dir, "T1", waivablePartial())
	if code, _, se := runCLI(t, dir, "outcome", "T1", "--waive", "deliverable-1", "--reason", "r", "--by", "operator"); code != exitOK {
		t.Fatalf("record waiver: %s", se)
	}

	cfg := config.Config{Lifecycle: config.Lifecycle{VerdictGate: true}}

	// No waiver supplied -> the gated PARTIAL is NEEDS_HUMAN and the denial is visible.
	noWaiver := &outcomeArtifact{Status: outcome.Partial, Criteria: waivablePartial().Criteria}
	applyLifecycleGateWithWaiver(cfg, nil, noWaiver, nil)
	if noWaiver.Lifecycle != outcome.NeedsHuman || !noWaiver.WaiverDenied {
		t.Fatalf("gated PARTIAL without waiver: Lifecycle=%q WaiverDenied=%v", noWaiver.Lifecycle, noWaiver.WaiverDenied)
	}

	// The recorded waiver resolves it to LOCAL_DONE.
	withWaiver := &outcomeArtifact{Status: outcome.Partial, Criteria: waivablePartial().Criteria}
	applyLifecycleGateWithWaiver(cfg, nil, withWaiver, recordedWaiverForTask(dir, "T1"))
	if withWaiver.Lifecycle != outcome.LocalDone || withWaiver.WaiverDenied {
		t.Fatalf("gated PARTIAL with recorded waiver: Lifecycle=%q WaiverDenied=%v, want LOCAL_DONE/false", withWaiver.Lifecycle, withWaiver.WaiverDenied)
	}
}

// TestWaiverRecordingLeavesArchiveUntouched pins that recording a waiver never
// rewrites the plan archive: a sentinel under .agent-sdlc/archive/ is byte-identical.
func TestWaiverRecordingLeavesArchiveUntouched(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeDesignatedApprovers(t, dir, "operator")
	writeOutcomeArtifactFor(t, dir, "T1", waivablePartial())

	archiveDir := filepath.Join(dir, stateDirName, "archive", "plan-x")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatalf("mkdir archive: %v", err)
	}
	sentinel := filepath.Join(archiveDir, "record.json")
	if err := os.WriteFile(sentinel, []byte(`{"frozen":true}`), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	if code, _, se := runCLI(t, dir, "outcome", "T1", "--waive", "deliverable-1", "--reason", "r", "--by", "operator"); code != exitOK {
		t.Fatalf("record waiver: %s", se)
	}

	got, err := os.ReadFile(sentinel)
	if err != nil || string(got) != `{"frozen":true}` {
		t.Fatalf("archive was rewritten: %q err=%v", got, err)
	}
}
