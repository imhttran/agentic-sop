package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/jev"
)

func wellFormedArtifact() EarlyArtifact {
	return EarlyArtifact{
		Version:    EarlyArtifactVersion,
		Checkpoint: CheckpointTaskTriage,
		Task:       "P3-004",
		Evidence: jev.Evidence{
			Version:  jev.EvidenceVersion,
			Purpose:  jev.PurposeQuality,
			Severity: jev.SeverityInfo,
			Status:   jev.EvidencePass,
			Summary:  "no concerns found",
		},
		Provider:       "openjev",
		Timestamp:      "2024-01-01T00:00:00Z",
		PolicyDecision: "CONTINUE",
		PolicyReason:   "no concerns",
	}
}

// TestEarlyArtifactValidateAcceptsWellFormed asserts the contract accepts a
// well-formed value (P3-004-S1).
func TestEarlyArtifactValidateAcceptsWellFormed(t *testing.T) {
	if err := wellFormedArtifact().Validate(); err != nil {
		t.Fatalf("Validate failed on well-formed artifact: %v", err)
	}
}

// TestEarlyArtifactValidateFailsClosedOnUnknownCheckpoint asserts an unknown
// checkpoint fails closed rather than defaulting (P3-004-S1).
func TestEarlyArtifactValidateFailsClosedOnUnknownCheckpoint(t *testing.T) {
	a := wellFormedArtifact()
	a.Checkpoint = Checkpoint("unknown")
	if err := a.Validate(); err == nil {
		t.Fatal("Validate accepted unknown checkpoint")
	}
}

// TestEarlyArtifactValidateFailsClosedOnUnknownVersion asserts an unknown schema
// version fails closed (P3-004-S1).
func TestEarlyArtifactValidateFailsClosedOnUnknownVersion(t *testing.T) {
	a := wellFormedArtifact()
	a.Version = 999
	if err := a.Validate(); err == nil {
		t.Fatal("Validate accepted unknown version")
	}
}

// TestEarlyArtifactValidateFailsClosedOnInvalidEvidence asserts the reused
// jev.Evidence contract fails closed inside the artifact (P3-004-S1).
func TestEarlyArtifactValidateFailsClosedOnInvalidEvidence(t *testing.T) {
	a := wellFormedArtifact()
	a.Evidence.Purpose = jev.Purpose("UNKNOWN_PURPOSE")
	if err := a.Validate(); err == nil {
		t.Fatal("Validate accepted invalid evidence")
	}
}

// TestEarlyArtifactValidateRejectsProviderFailureWithEvidence asserts that a
// provider failure carrying a fabricated evidence payload is rejected, so a
// provider failure can never persist a value a later consumer would read as a
// PASS (fail-closed contract; P3-004-S1).
func TestEarlyArtifactValidateRejectsProviderFailureWithEvidence(t *testing.T) {
	a := wellFormedArtifact()
	a.ProviderFailed = true // keeps the PASS evidence, which must be rejected

	if err := a.Validate(); err == nil {
		t.Fatal("Validate accepted a provider failure carrying a PASS evidence payload")
	}
	if err := (&Run{}).WriteEarlyJEVArtifact(a); err == nil {
		t.Fatal("WriteEarlyJEVArtifact persisted a contradictory provider-failure artifact")
	}
}

// TestEarlyArtifactValidateAcceptsProviderFailureWithoutEvidence asserts a
// provider failure with no evidence payload is accepted (it is the only valid
// provider-failure shape).
func TestEarlyArtifactValidateAcceptsProviderFailureWithoutEvidence(t *testing.T) {
	a := EarlyArtifact{
		Version:        EarlyArtifactVersion,
		Checkpoint:     CheckpointPreExecution,
		Task:           "P3-004",
		Provider:       "openjev",
		Timestamp:      "2024-01-01T00:00:00Z",
		ProviderFailed: true,
		PolicyDecision: "CONTINUE",
		PolicyReason:   "provider unavailable",
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate rejected a provider failure with no evidence: %v", err)
	}
}

// TestWriteEarlyJEVArtifactWritesEachCheckpoint asserts a successful write of
// each checkpoint's artifact and that content round-trips through the contract
// with every required field present (P3-004-S2).
func TestWriteEarlyJEVArtifactWritesEachCheckpoint(t *testing.T) {
	for _, cp := range []Checkpoint{CheckpointTaskTriage, CheckpointPreExecution} {
		dir := t.TempDir()
		r, err := New(dir, "T004")
		if err != nil {
			t.Fatalf("New failed: %v", err)
		}

		a := wellFormedArtifact()
		a.Checkpoint = cp
		if err := r.WriteEarlyJEVArtifact(a); err != nil {
			t.Fatalf("WriteEarlyJEVArtifact(%s) failed: %v", cp, err)
		}

		raw, err := os.ReadFile(filepath.Join(r.Dir(), earlyJEVArtifactFileName))
		if err != nil {
			t.Fatalf("early-jev.json not written: %v", err)
		}
		var got EarlyArtifact
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("early-jev.json invalid: %v", err)
		}
		if got.Checkpoint != cp {
			t.Errorf("checkpoint = %s, want %s", got.Checkpoint, cp)
		}
		if got.Task != "P3-004" || got.Provider != "openjev" || got.Timestamp == "" {
			t.Errorf("missing task/provider/timestamp: %+v", got)
		}
		if got.PolicyDecision != "CONTINUE" || got.PolicyReason != "no concerns" {
			t.Errorf("policy fields not preserved: %+v", got)
		}
		if err := got.Validate(); err != nil {
			t.Errorf("round-tripped artifact invalid: %v", err)
		}
	}
}

// TestWriteEarlyJEVArtifactAppendsHistory asserts prior history lines are
// preserved on repeated writes (P3-004-S2).
func TestWriteEarlyJEVArtifactAppendsHistory(t *testing.T) {
	dir := t.TempDir()
	r, err := New(dir, "T004")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	const n = 3
	for i := 1; i <= n; i++ {
		a := wellFormedArtifact()
		a.PolicyReason = strings.Repeat("x", i)
		if err := r.WriteEarlyJEVArtifact(a); err != nil {
			t.Fatalf("write %d failed: %v", i, err)
		}
	}

	hist, err := os.ReadFile(filepath.Join(r.Dir(), earlyJEVHistoryFileName))
	if err != nil {
		t.Fatalf("early-jev-history.jsonl not written: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(hist)), "\n")
	if len(lines) != n {
		t.Fatalf("history lines = %d, want %d", len(lines), n)
	}
	for i, line := range lines {
		var rec EarlyArtifact
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("history line %d invalid: %v", i, err)
		}
		if rec.PolicyReason != strings.Repeat("x", i+1) {
			t.Errorf("history line %d reason = %q, want %d x", i, rec.PolicyReason, i+1)
		}
	}
}

// TestWriteEarlyJEVArtifactFailureDoesNotPanicOrAlterOtherFiles asserts a write
// failure returns an error without panicking or altering other files
// (P3-004-S2/S3).
func TestWriteEarlyJEVArtifactFailureDoesNotPanicOrAlterOtherFiles(t *testing.T) {
	dir := t.TempDir()
	r, err := New(dir, "T004")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Write an unrelated file we expect to survive untouched.
	other := filepath.Join(r.Dir(), "other.txt")
	if err := os.WriteFile(other, []byte("keep"), 0o644); err != nil {
		t.Fatalf("seed other file: %v", err)
	}

	// Make the run directory unwritable so the write fails.
	if err := os.Chmod(r.Dir(), 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(r.Dir(), 0o755) })

	if err := r.WriteEarlyJEVArtifact(wellFormedArtifact()); err == nil {
		t.Fatal("expected error writing to unwritable run dir")
	}

	// Best-effort: the failure must not alter other files.
	if err := os.Chmod(r.Dir(), 0o755); err != nil {
		t.Fatalf("restore chmod: %v", err)
	}
	got, err := os.ReadFile(other)
	if err != nil || string(got) != "keep" {
		t.Errorf("unrelated file altered: %q err=%v", got, err)
	}
}

// TestWriteEarlyJEVArtifactProviderFailureIsDistinct asserts a provider-failure
// scenario persists ProviderFailed=true while never being recorded as a finding
// or PASS (P3-004-S3).
func TestWriteEarlyJEVArtifactProviderFailureIsDistinct(t *testing.T) {
	dir := t.TempDir()
	r, err := New(dir, "T004")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	fake := jev.NewProviderFailureFake()
	if _, err := fake.Analyze(nil, jev.Request{}); err == nil {
		t.Fatal("provider-failure fake did not fail")
	}

	a := EarlyArtifact{
		Version:        EarlyArtifactVersion,
		Checkpoint:     CheckpointPreExecution,
		Task:           "P3-004",
		Provider:       "openjev",
		Timestamp:      "2024-01-01T00:00:00Z",
		ProviderFailed: true,
		PolicyDecision: "CONTINUE",
		PolicyReason:   "provider unavailable",
	}
	if err := r.WriteEarlyJEVArtifact(a); err != nil {
		t.Fatalf("WriteEarlyJEVArtifact failed: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(r.Dir(), earlyJEVArtifactFileName))
	if err != nil {
		t.Fatalf("early-jev.json not written: %v", err)
	}
	var got EarlyArtifact
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("early-jev.json invalid: %v", err)
	}
	if !got.ProviderFailed {
		t.Error("ProviderFailed = false, want true")
	}
	if got.Evidence.Status == jev.EvidencePass {
		t.Error("provider failure recorded as PASS")
	}
}

// TestWriteEarlyJEVArtifactDoesNotTouchPersistenceAuthority asserts the early
// write path does not create, modify, migrate, or delete state.json/state.db and
// performs no state transition (P3-004-S3).
func TestWriteEarlyJEVArtifactDoesNotTouchPersistenceAuthority(t *testing.T) {
	dir := t.TempDir()
	r, err := New(dir, "T004")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := r.SetStage(Validating); err != nil {
		t.Fatalf("SetStage failed: %v", err)
	}

	statePath := filepath.Join(r.Dir(), stateFileName)
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state.json: %v", err)
	}

	if err := r.WriteEarlyJEVArtifact(wellFormedArtifact()); err != nil {
		t.Fatalf("WriteEarlyJEVArtifact failed: %v", err)
	}

	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("re-read state.json: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("state.json changed: before=%q after=%q", before, after)
	}
	if _, err := os.Stat(filepath.Join(r.Dir(), "state.db")); !os.IsNotExist(err) {
		t.Errorf("unexpected state.db presence: %v", err)
	}
	if r.State().Stage != Validating {
		t.Errorf("stage = %s, want VALIDATING", r.State().Stage)
	}
}

// TestEarlyAndQualityArtifactsAreSeparate asserts early artifacts never
// overwrite the quality-seam artifacts and vice versa (P3-004-S2).
func TestEarlyAndQualityArtifactsAreSeparate(t *testing.T) {
	dir := t.TempDir()
	r, err := New(dir, "T004")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if err := r.WriteJEVArtifact(map[string]any{"status": "PASS"}); err != nil {
		t.Fatalf("WriteJEVArtifact failed: %v", err)
	}
	if err := r.WriteEarlyJEVArtifact(wellFormedArtifact()); err != nil {
		t.Fatalf("WriteEarlyJEVArtifact failed: %v", err)
	}

	quality, err := os.ReadFile(filepath.Join(r.Dir(), jevArtifactFileName))
	if err != nil {
		t.Fatalf("quality artifact missing: %v", err)
	}
	if !strings.Contains(string(quality), "\"PASS\"") {
		t.Errorf("quality artifact overwritten: %s", quality)
	}

	early, err := os.ReadFile(filepath.Join(r.Dir(), earlyJEVArtifactFileName))
	if err != nil {
		t.Fatalf("early artifact missing: %v", err)
	}
	if !strings.Contains(string(early), "\"task_triage\"") {
		t.Errorf("early artifact content unexpected: %s", early)
	}
}
