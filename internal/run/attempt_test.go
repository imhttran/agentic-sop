package run

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestRun(t *testing.T) *Run {
	t.Helper()
	r, err := New(t.TempDir(), "T001")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

func TestWriteAttemptRecordRoundTrip(t *testing.T) {
	r := newTestRun(t)
	rec := AttemptRecord{
		Version:      AttemptRecordVersion,
		Attempt:      2,
		Class:        "medium",
		Provider:     "ollama",
		Model:        "glm-5.3-flash:cloud",
		Locality:     "cloud",
		Reason:       "implementation validation failed",
		Result:       AttemptFailed,
		FailureStage: "test",
		Action:       "escalate",
		Timestamp:    "2026-01-02T03:04:05Z",
	}
	if err := r.WriteAttemptRecord(rec); err != nil {
		t.Fatalf("WriteAttemptRecord: %v", err)
	}
	got, err := r.ReadAttemptRecords()
	if err != nil {
		t.Fatalf("ReadAttemptRecords: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("records = %d, want 1", len(got))
	}
	if got[0] != rec {
		t.Errorf("record = %+v, want %+v", got[0], rec)
	}
	if _, err := os.Stat(filepath.Join(r.Dir(), "attempts", "002.json")); err != nil {
		t.Errorf("attempt file not numbered by attempt: %v", err)
	}
}

func TestAttemptRecordsReadInOrder(t *testing.T) {
	r := newTestRun(t)
	for _, n := range []int{3, 1, 2} {
		if err := r.WriteAttemptRecord(AttemptRecord{
			Version: AttemptRecordVersion, Attempt: n, Result: AttemptPassed,
		}); err != nil {
			t.Fatalf("write attempt %d: %v", n, err)
		}
	}
	got, _ := r.ReadAttemptRecords()
	if len(got) != 3 {
		t.Fatalf("records = %d, want 3", len(got))
	}
	for i, want := range []int{1, 2, 3} {
		if got[i].Attempt != want {
			t.Errorf("record %d = attempt %d, want %d", i, got[i].Attempt, want)
		}
	}
}

func TestWriteAttemptRecordFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		rec  AttemptRecord
	}{
		{"unknown version", AttemptRecord{Version: 99, Attempt: 1, Result: AttemptPassed}},
		{"unknown result", AttemptRecord{Version: AttemptRecordVersion, Attempt: 1, Result: "maybe"}},
		{"zero attempt", AttemptRecord{Version: AttemptRecordVersion, Attempt: 0, Result: AttemptPassed}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestRun(t)
			if err := r.WriteAttemptRecord(tc.rec); err == nil {
				t.Fatal("expected an error")
			}
			if _, err := os.Stat(filepath.Join(r.Dir(), "attempts")); err == nil {
				t.Error("no attempt file may be written on a contract violation")
			}
		})
	}
}

func TestReadAttemptRecordsMissingOrMalformed(t *testing.T) {
	r := newTestRun(t)
	if got, err := r.ReadAttemptRecords(); err != nil || len(got) != 0 {
		t.Fatalf("missing directory = %v, %v; want empty, nil", got, err)
	}
	dir := filepath.Join(r.Dir(), "attempts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "001.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A malformed record is skipped rather than failing the read.
	if got, err := r.ReadAttemptRecords(); err != nil || len(got) != 0 {
		t.Fatalf("malformed record = %v, %v; want empty, nil", got, err)
	}
	// ReadAttemptRecordsAt is the directory-based form used by display surfaces.
	if got := ReadAttemptRecordsAt(r.Dir()); len(got) != 0 {
		t.Fatalf("ReadAttemptRecordsAt = %v, want empty", got)
	}
}
