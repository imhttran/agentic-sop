package runmetrics

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func oneOfEach() map[CountKind]int {
	delta := map[CountKind]int{}
	for i, k := range CountKinds {
		delta[k] = i + 1 // distinct per kind, so a fold would show
	}
	return delta
}

func TestCountsSeparatedByKind(t *testing.T) {
	dir := t.TempDir()
	if _, err := RecordReport(dir, "r1", oneOfEach()); err != nil {
		t.Fatal(err)
	}
	got := LoadCounts(dir)
	for i, k := range CountKinds {
		if c := got[k]; !c.Known || c.Value != i+1 {
			t.Fatalf("%s = %+v, want %d", k, c, i+1)
		}
	}
}

func TestCountsSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	if _, err := RecordReport(dir, "r1", oneOfEach()); err != nil {
		t.Fatal(err)
	}
	before := LoadCounts(dir)
	after := LoadCounts(dir) // a fresh load reads only from disk
	for _, k := range CountKinds {
		if before[k] != after[k] {
			t.Fatalf("%s changed across reload: %+v -> %+v", k, before[k], after[k])
		}
	}
}

func TestDuplicateReportNotDoubleCounted(t *testing.T) {
	dir := t.TempDir()
	totals := map[CountKind]int{CountAttempts: 1, CountModelCalls: 2}
	if applied, err := RecordReport(dir, "r1", totals); err != nil || !applied {
		t.Fatalf("first apply = %v, %v", applied, err)
	}
	if applied, err := RecordReport(dir, "r1", totals); err != nil || applied {
		t.Fatalf("repeat apply = %v, %v, want not applied", applied, err)
	}
	got := LoadCounts(dir)
	if got[CountAttempts].Value != 1 || got[CountModelCalls].Value != 2 {
		t.Fatalf("counts = %+v, want attempts 1, model calls 2", got)
	}
	// A later report carries absolute totals; it must not be added to the previous
	// total (which would make attempts 3).
	if _, err := RecordReport(dir, "r2", map[CountKind]int{CountAttempts: 2, CountModelCalls: 2}); err != nil {
		t.Fatal(err)
	}
	if got := LoadCounts(dir); got[CountAttempts].Value != 2 {
		t.Fatalf("attempts after a later total = %d, want 2", got[CountAttempts].Value)
	}
}

// TestReportedTotalDoesNotDoubleCountSeed proves an absolute report does not add
// itself to the seed read from the existing source artifact (the historical
// double-counting defect).
func TestReportedTotalDoesNotDoubleCountSeed(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, attemptsDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, attemptsDirName, "001.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordReport(dir, "r1", map[CountKind]int{CountAttempts: 1}); err != nil {
		t.Fatal(err)
	}
	if got := LoadCounts(dir)[CountAttempts]; !got.Known || got.Value != 1 {
		t.Fatalf("attempts = %+v, want the reported total 1 (not seed+report)", got)
	}
}

func TestMissingHistoryIsUnknown(t *testing.T) {
	dir := t.TempDir()
	for _, k := range CountKinds {
		if c := LoadCounts(dir)[k]; c.Known || c.String() != UNKNOWN {
			t.Fatalf("%s with no history = %s, want UNKNOWN", k, c)
		}
	}
	// A recorded zero in a source artifact is a known zero, not UNKNOWN.
	if err := os.WriteFile(filepath.Join(dir, continuationsFileName), []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if c := LoadCounts(dir)[CountContinuations]; !c.Known || c.String() != "0" {
		t.Fatalf("continuations with recorded 0 = %s, want 0", c)
	}
	if c := LoadCounts(dir)[CountInvocations]; c.String() != UNKNOWN {
		t.Fatalf("invocations = %s, want UNKNOWN", c)
	}
}

func TestCandidateBinaryProvenance(t *testing.T) {
	bin, err := os.Executable() // the test binary is a test-built Go binary
	if err != nil {
		t.Fatal(err)
	}
	p, err := BinaryProvenance(bin)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if p.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("checksum = %s, want independent SHA-256", p.SHA256)
	}
	if p.Revision == "" || p.Modified == "" || p.Module == "" {
		t.Fatalf("identity fields must be a value or UNKNOWN, got %+v", p)
	}

	if _, err := BinaryProvenance(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing binary returned no error")
	}

	// The lifecycle records provenance once and preserves it on a repeat.
	dir := t.TempDir()
	recorded, stored, err := RecordBinaryProvenance(dir, bin)
	if err != nil || stored {
		t.Fatalf("record provenance = %+v stored=%v err=%v", recorded, stored, err)
	}
	again, stored, err := RecordBinaryProvenance(dir, bin)
	if err != nil || !stored || again.SHA256 != recorded.SHA256 {
		t.Fatalf("repeat provenance = %+v stored=%v err=%v", again, stored, err)
	}
	if got, ok := LoadBinaryProvenance(dir); !ok || got.SHA256 != recorded.SHA256 {
		t.Fatalf("loaded provenance = %+v ok=%v", got, ok)
	}
}

// TestApprovalsCountDecisionsNotRequests proves the approvals count is the number
// of approved decisions, not the number of recorded requests.
func TestApprovalsCountDecisionsNotRequests(t *testing.T) {
	dir := t.TempDir()
	history := `[{"id":"a","status":"PENDING"},{"id":"b","status":"APPROVED"},{"id":"c","status":"DECLINED"},{"id":"d","status":"APPROVED"}]`
	if err := os.WriteFile(filepath.Join(dir, approvalHistoryFileName), []byte(history), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadCounts(dir)[CountApprovals]; !got.Known || got.Value != 2 {
		t.Fatalf("approvals = %+v, want 2 approved decisions", got)
	}
}
