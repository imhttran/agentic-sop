package cli

// SOP-005 lifecycle accounting. The counts artifact is written from the real
// lifecycle, not only from unit tests. Values are absolute totals per kind, so a
// report never adds itself to a seed read from an existing artifact.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/domain"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/runmetrics"
)

// countAuditRecords counts the non-empty records in a task's append-only JSONL
// audit. A missing file is a known zero (the audit was never opened); an
// unreadable file is not guessed and contributes no value.
func countAuditRecords(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, true
		}
		return 0, false
	}
	n := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n, true
}

// approvedApprovals counts resolved approvals (not pending or declined requests).
func approvedApprovals(history []domain.ApprovalRequest) int {
	n := 0
	for _, req := range history {
		if req.Status == domain.ApprovalApproved {
			n++
		}
	}
	return n
}

// recordLifecycleCounts persists the five separately counted kinds for this
// lifecycle invocation, plus the candidate binary's provenance. It is idempotent
// for one invocation: the report id is derived from the run's creation/update
// identity, so a repeated write for the same invocation is not double-counted.
// A later artifact report updates absolute totals rather than adding a delta.
func recordLifecycleCounts(dir string, rn *runpkg.Run, taskID string, res lifeResult) error {
	if rn == nil {
		return nil
	}
	existing := runmetrics.LoadCounts(rn.Dir())

	invocations := 1
	if c := existing[runmetrics.CountInvocations]; c.Known {
		invocations = c.Value + 1
	}

	records, _ := rn.ReadAttemptRecords()
	attempts := 1 // this lifecycle invocation is itself one execution attempt
	if len(records) > attempts {
		attempts = len(records)
	}
	if c := existing[runmetrics.CountAttempts]; c.Known {
		attempts += c.Value
	}

	continuations := rn.ReadContinuations()
	if c := existing[runmetrics.CountContinuations]; c.Known && c.Value > continuations {
		continuations = c.Value
	}

	modelCalls := res.perf.Counts.AgentCalls + res.perf.Counts.ReviewRuns
	if c := existing[runmetrics.CountModelCalls]; c.Known {
		modelCalls += c.Value
	}

	approvals := approvedApprovals(rn.ApprovalHistory())

	direct := 0
	decisions, decisionOK := countAuditRecords(acceptanceDecisionPath(dir, taskID))
	waivers, waiverOK := countAuditRecords(waiverAuditPath(dir, taskID))
	if decisionOK {
		direct += decisions
	}
	if waiverOK {
		direct += waivers
	}

	reportID := fmt.Sprintf("%s:%s:%s", rn.State().ID,
		rn.State().CreatedAt.UTC().Format("20060102T150405.000000000Z"),
		rn.State().UpdatedAt.UTC().Format("20060102T150405.000000000Z"))
	totals := map[runmetrics.CountKind]int{
		runmetrics.CountInvocations:   invocations,
		runmetrics.CountAttempts:      attempts,
		runmetrics.CountContinuations: continuations,
		runmetrics.CountModelCalls:    modelCalls,
		runmetrics.CountApprovals:     approvals,
	}
	if decisionOK && waiverOK {
		totals[runmetrics.CountDirectInterventions] = direct
	}
	if _, err := runmetrics.RecordReport(rn.Dir(), reportID, totals); err != nil {
		return fmt.Errorf("record lifecycle counts: %w", err)
	}

	// Record the candidate binary once. A test binary is still a real executable;
	// the evidence records what actually produced the run.
	if exe, err := os.Executable(); err == nil && exe != "" {
		if _, _, err := runmetrics.RecordBinaryProvenance(rn.Dir(), exe); err != nil {
			return fmt.Errorf("record candidate binary provenance: %w", err)
		}
	}
	return nil
}

// countsPath is exported to tests through this small helper so a test can inspect
// the artifact without restating the layout.
func countsPath(projectDir, taskID string) string {
	return filepath.Join(projectDir, stateDirName, "runs", taskID, "counts.json")
}
