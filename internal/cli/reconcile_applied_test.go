package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
)

// TestPendingDefinitionDigestsReadsOnlyPendingStampedRequests proves the CLI only
// supplies a prior digest for a PENDING, stamped request, so reconcile cannot treat a
// resolved or unstamped request as an already-applied change.
func TestPendingDefinitionDigestsReadsOnlyPendingStampedRequests(t *testing.T) {
	dir := t.TempDir()
	writeApproval := func(taskID string, req domain.ApprovalRequest) {
		t.Helper()
		runDir := filepath.Join(dir, config.DirName, "runs", taskID)
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(runDir, "approval.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeApproval("PENDING-1", domain.ApprovalRequest{
		ID: "req-1", TaskID: "PENDING-1", Status: domain.ApprovalPending, DefinitionDigest: "digest-1",
	})
	writeApproval("DECLINED-1", domain.ApprovalRequest{
		ID: "req-2", TaskID: "DECLINED-1", Status: domain.ApprovalDeclined, DefinitionDigest: "digest-2",
	})
	writeApproval("EMPTY-1", domain.ApprovalRequest{
		ID: "req-3", TaskID: "EMPTY-1", Status: domain.ApprovalPending,
	})

	got := pendingDefinitionDigests(dir, []string{"PENDING-1", "DECLINED-1", "EMPTY-1", "MISSING-1"})
	if len(got) != 1 || got["PENDING-1"] != "digest-1" {
		t.Fatalf("pendingDefinitionDigests = %v, want only the PENDING stamped request", got)
	}
}
