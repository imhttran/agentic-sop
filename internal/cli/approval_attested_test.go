package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// attestedFixture seeds a legacy PENDING request at an acceptance boundary whose own
// evidence names a requirement the current definition no longer contains, with a
// legacy reconciliation record naming the task.
func attestedFixture(t *testing.T) invalidationFixture {
	t.Helper()
	f := newInvalidationFixture(t, invalidationFixtureOptions{
		taskStatus:       domain.READY,
		legacyReconciled: []string{"S001"},
	})
	var head domain.ApprovalRequest
	readApprovalHead(t, f.runDir, &head)
	head.Stage = "WAITING_FOR_HUMAN"
	head.Reason = "required criterion deliverable-2 not met: deliverable \"domain.ApprovalRequest\" not present"
	head.Evidence = head.Reason
	writeJSONDoc(t, filepath.Join(f.runDir, "approval.json"), head)
	return f
}

func TestCLIInvalidateAttestedSucceeds(t *testing.T) {
	f := attestedFixture(t)
	var out, errb bytes.Buffer
	code := runApprovalInvalidateAttested([]string{
		"S001",
		"--reason", "the cited deliverable was removed by the reconciled plan",
		"--by", "operator",
		"--expected-request", "S001-1",
		"--obsolete-requirement", "domain.ApprovalRequest",
		"--replacement-definition", "internal/notify/event.go",
		"--expect-definition-digest", f.digest,
	}, &out, &errb, invalidationDeps(f.dir))
	if code != exitOK {
		t.Fatalf("exit = %d; stderr=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "invalidated (attested) S001") {
		t.Errorf("output = %q", out.String())
	}
	if !strings.Contains(out.String(), "attested (operator statement") {
		t.Errorf("output does not mark the invalidation attested: %q", out.String())
	}

	var head domain.ApprovalRequest
	readApprovalHead(t, f.runDir, &head)
	if head.Status != domain.ApprovalInvalidated || head.Decision != nil {
		t.Fatalf("head = %s decision=%+v, want INVALIDATED with no decision", head.Status, head.Decision)
	}
	if head.Invalidation == nil || head.Invalidation.Kind != domain.ApprovalInvalidationAttested {
		t.Fatalf("invalidation = %+v", head.Invalidation)
	}
	if head.Invalidation.Source != domain.InvalidationSourceUnknownLegacy {
		t.Errorf("source = %q", head.Invalidation.Source)
	}
	var hist []domain.ApprovalRequest
	data, err := os.ReadFile(filepath.Join(f.runDir, "approval-history.json"))
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if err := json.Unmarshal(data, &hist); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(hist) != 1 || hist[0].ID != "S001-1" {
		t.Fatalf("history = %+v, want the original request preserved", hist)
	}
}

func TestCLIInvalidateAttestedRefusesStaleDigest(t *testing.T) {
	f := attestedFixture(t)
	var out, errb bytes.Buffer
	code := runApprovalInvalidateAttested([]string{
		"S001",
		"--reason", "r",
		"--by", "operator",
		"--expected-request", "S001-1",
		"--obsolete-requirement", "domain.ApprovalRequest",
		"--replacement-definition", "internal/notify/event.go",
		"--expect-definition-digest", "digest-reviewed-earlier",
	}, &out, &errb, invalidationDeps(f.dir))
	if code == exitOK {
		t.Fatal("expected refusal when the definition changed since review")
	}
	if !strings.Contains(errb.String(), "changed since it was reviewed") {
		t.Errorf("stderr = %q", errb.String())
	}
	assertHeadPending(t, f.runDir)
}

func TestCLIInvalidateAttestedUsage(t *testing.T) {
	f := attestedFixture(t)
	var out, errb bytes.Buffer
	if code := runApprovalInvalidateAttested([]string{"S001", "--by", "operator"}, &out, &errb, invalidationDeps(f.dir)); code != exitUsage {
		t.Fatalf("exit = %d, want usage", code)
	}
	assertHeadPending(t, f.runDir)
}
