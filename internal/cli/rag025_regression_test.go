package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/outcome"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// These are the SOP-HARD-05 regression tests for the RAG-025 scenarios. Each
// reproduces a failure observed while running the RAG-025 plan and pins the
// post-hardening behavior. They are deterministic and need no external infra.

// TestDeliverablePathsHARD03MalformedToken is the exact HARD-03 case: a
// `### Deliverables` bullet that lists two backtick-quoted paths and trailing prose.
// Pre-hardening, deliverablePath took only the first whitespace field, trimmed the
// enclosing backticks, and left the trailing comma — yielding the bogus token
// "internal/ollamaagent/already_satisfied.go`," that "was not present" even though the
// file existed (LOCAL_DONE with PARTIAL). The parser must return both real paths and
// never a token containing a backtick or comma.
func TestDeliverablePathsHARD03MalformedToken(t *testing.T) {
	bullet := "`internal/ollamaagent/already_satisfied.go`, `internal/ollamaagent/orchestrate.go` — index/normalized criterion matching; accept independently verified existing work with **zero** repository mutations; explicit **approver waiver** for an environment-unavailable validation command (decision 4)."

	got := deliverablePaths(bullet)
	if len(got) != 2 || got[0] != "internal/ollamaagent/already_satisfied.go" || got[1] != "internal/ollamaagent/orchestrate.go" {
		t.Fatalf("deliverablePaths = %v, want both backtick-quoted paths", got)
	}
	for _, p := range got {
		if strings.ContainsAny(p, "`,\"' ") {
			t.Fatalf("parsed deliverable %q carries a stray backtick/comma/space (the HARD-03 malformed token)", p)
		}
	}
}

// TestDeliverablePathsRejectsCommandsAndProse pins that a command with a path-like
// token (a backtick-quoted `go test ./...`), a bare command, and prose never become
// deliverables.
func TestDeliverablePathsRejectsCommandsAndProse(t *testing.T) {
	cases := map[string]int{
		"Deterministic fixtures/tests, each mapping to an observed RAG-025 scenario": 0,
		"A `PARTIAL` from unverifiable criteria does not gate":                       0,
		"runs in the standard `go test ./...` suite":                                 0,
		"`internal/cli/` — read-only recovery; `docs/reference/`":                    2,
		"`a/x.go`, `a/y.go` — prose":                                                 2,
		"`internal/planner/markdown.go:164-166`":                                     1,
	}
	for in, want := range cases {
		if got := deliverablePaths(in); len(got) != want {
			t.Errorf("deliverablePaths(%q) = %v, want %d paths", in, got, want)
		}
	}
}

// TestRAG025EDeliverableAbsentPartialsAndGates reproduces RAG-025E: a required
// deliverable is absent while the gate PASSes. The verdict is PARTIAL, and with the
// gate ON there is no LOCAL_DONE.
func TestRAG025EDeliverableAbsentPartialsAndGates(t *testing.T) {
	dir := t.TempDir()
	spec := &taskfile.Spec{ID: "T1", Deliverables: []string{"docs/operations/platform-support.md"}}
	res := deriveOutcome(spec, dir, quality.Result{Decision: quality.Pass})
	if res == nil || res.Status != outcome.Partial {
		t.Fatalf("absent required deliverable: status=%v, want PARTIAL", res)
	}
	applyLifecycleGate(config.Config{Lifecycle: config.Lifecycle{VerdictGate: true}}, spec, res)
	if res.Lifecycle != outcome.NeedsHuman {
		t.Fatalf("gate ON with absent required deliverable: lifecycle=%q, want NEEDS_HUMAN", res.Lifecycle)
	}
	if !res.WaiverDenied {
		t.Fatal("the denial must be observable")
	}
}

// TestRAG025AProseCriteriaAreInformational pins that when a stage declares a
// deterministic check, its free-text criteria are recorded informational
// (non-required) rather than gating — the RAG-025A fix.
func TestRAG025AProseCriteriaAreInformational(t *testing.T) {
	dir := t.TempDir()
	spec := &taskfile.Spec{ID: "T1", AcceptanceChecks: []string{"go vet ./..."}, AcceptanceCriteria: []string{"prose criterion"}}
	res := deriveOutcome(spec, dir, quality.Result{Decision: quality.Pass})
	var found bool
	for _, cr := range res.Criteria {
		if cr.Description == "prose criterion" {
			found = true
			if cr.Required {
				t.Fatalf("prose criterion must be informational, got required: %+v", cr)
			}
		}
	}
	if !found {
		t.Fatalf("prose criterion was not recorded: %+v", res.Criteria)
	}
}

// TestRAG025AInformationalOnlyPartialDoesNotGate pins the gate behavior: a PARTIAL
// whose only unmet criteria are informational keeps LOCAL_DONE with the gate ON.
func TestRAG025AInformationalOnlyPartialDoesNotGate(t *testing.T) {
	res := &outcomeArtifact{
		Status:   outcome.Partial,
		Criteria: []outcome.CriterionResult{{ID: "criterion-1", Required: false, Met: false, Evidence: "unverifiable: no deterministic check supplied"}},
	}
	applyLifecycleGate(config.Config{Lifecycle: config.Lifecycle{VerdictGate: true}}, nil, res)
	if res.Lifecycle != outcome.LocalDone || res.WaiverDenied {
		t.Fatalf("informational-only PARTIAL: lifecycle=%q denied=%v, want LOCAL_DONE/false", res.Lifecycle, res.WaiverDenied)
	}
}

// TestRAG025CUnavailableValidationRequiresWaiver reproduces RAG-025C: a required
// check that cannot pass is never a silent pass. With the gate ON it resolves to
// NEEDS_HUMAN, and only a valid designated-approver waiver resolves it to LOCAL_DONE.
func TestRAG025CUnavailableValidationRequiresWaiver(t *testing.T) {
	dir := t.TempDir() // no Go files: `go build ./...` cannot succeed here
	spec := &taskfile.Spec{ID: "T1", AcceptanceChecks: []string{"go build ./..."}}
	res := deriveOutcome(spec, dir, quality.Result{Decision: quality.Pass})
	if res.Status != outcome.Partial {
		t.Fatalf("unavailable required check: status=%v, want PARTIAL", res.Status)
	}

	gate := config.Config{Lifecycle: config.Lifecycle{VerdictGate: true}}

	// No waiver: never a silent pass.
	noWaiver := &outcomeArtifact{Status: res.Status, Criteria: res.Criteria}
	applyLifecycleGateWithWaiver(gate, spec, noWaiver, nil)
	if noWaiver.Lifecycle != outcome.NeedsHuman {
		t.Fatalf("no waiver: lifecycle=%q, want NEEDS_HUMAN", noWaiver.Lifecycle)
	}

	// An explicit, designated-approver waiver naming the unmet check resolves it.
	w := &outcome.Waiver{Approver: "operator", Criteria: []string{"check-1"}, Reason: "environment unavailable", RecordedAt: "2026-10-12T00:00:00Z"}
	withWaiver := &outcomeArtifact{Status: res.Status, Criteria: res.Criteria}
	applyLifecycleGateWithWaiver(gate, spec, withWaiver, w)
	if withWaiver.Lifecycle != outcome.LocalDone || withWaiver.WaiverDenied {
		t.Fatalf("with waiver: lifecycle=%q denied=%v, want LOCAL_DONE/false", withWaiver.Lifecycle, withWaiver.WaiverDenied)
	}
}

// TestRAG025AuditHistoryPreserved pins that recording a waiver appends audit evidence
// without rewriting the recorded verdict: outcome.json is byte-identical and
// waivers.jsonl is the only new artifact under the run directory.
func TestRAG025AuditHistoryPreserved(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeDesignatedApprovers(t, dir, "operator")
	writeOutcomeArtifactFor(t, dir, "T1", waivablePartial())

	outcomePath := filepath.Join(dir, stateDirName, "runs", "T1", "outcome.json")
	before, err := os.ReadFile(outcomePath)
	if err != nil {
		t.Fatal(err)
	}

	if code, _, se := runCLI(t, dir, "outcome", "T1", "--waive", "deliverable-1", "--reason", "r", "--by", "operator"); code != exitOK {
		t.Fatalf("record waiver: %s", se)
	}

	after, err := os.ReadFile(outcomePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("recording a waiver rewrote the recorded verdict; history must be preserved")
	}
	data, err := os.ReadFile(waiverAuditPath(dir, "T1"))
	if err != nil {
		t.Fatalf("waiver audit missing: %v", err)
	}
	var rec outcome.WaiverRecord
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &rec); err != nil {
		t.Fatalf("waiver record did not parse: %v", err)
	}
	if rec.Task != "T1" || rec.Approver != "operator" {
		t.Fatalf("waiver record not persisted: %+v", rec)
	}
}
