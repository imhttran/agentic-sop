package store

import (
	"errors"
	"reflect"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/handoff"
)

func openStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func sampleRecord(taskID string) handoff.Record {
	return handoff.Record{
		TaskID: taskID,
		Capsule: handoff.Capsule{
			TaskID:       taskID,
			Result:       domain.DONE,
			Summary:      "did the thing",
			Changes:      []string{"added x"},
			Decisions:    []string{"only FAIL is RED"},
			Files:        []string{"internal/x/x.go"},
			Verification: []handoff.VerificationSummary{{Check: "unit", Status: "PASS"}},
			CarryForward: []string{"cleanup later"},
		},
		Status:    handoff.StatusCompressed,
		Content:   "compressed body",
		CreatedAt: fixedTime(),
	}
}

func TestSaveAndGetHandoff(t *testing.T) {
	s := openStore(t)
	saveDependency(t, s, "T010")

	record := sampleRecord("T010")
	record.CompressionError = "boom"
	record.References = []handoff.Reference{{Kind: handoff.ArtifactCILog, Locator: "run/123"}}
	if err := s.SaveHandoff(record); err != nil {
		t.Fatalf("SaveHandoff failed: %v", err)
	}

	got, err := s.GetHandoff("T010")
	if err != nil {
		t.Fatalf("GetHandoff failed: %v", err)
	}
	if !reflect.DeepEqual(got, record) {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", got, record)
	}
}

func TestGetHandoffNotFound(t *testing.T) {
	s := openStore(t)
	if _, err := s.GetHandoff("missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestSaveHandoffIsIdempotentUpsert(t *testing.T) {
	s := openStore(t)
	saveDependency(t, s, "T010")

	if err := s.SaveHandoff(sampleRecord("T010")); err != nil {
		t.Fatalf("first SaveHandoff failed: %v", err)
	}
	updated := sampleRecord("T010")
	updated.Status = handoff.StatusFailed
	updated.Content = ""
	if err := s.SaveHandoff(updated); err != nil {
		t.Fatalf("second SaveHandoff failed: %v", err)
	}

	records, err := s.Handoffs()
	if err != nil {
		t.Fatalf("Handoffs failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d handoffs, want 1 (upsert, not duplicate)", len(records))
	}
	if records[0].Status != handoff.StatusFailed {
		t.Errorf("status = %s, want FAILED (updated in place)", records[0].Status)
	}
}

func TestHandoffsSurviveReopen(t *testing.T) {
	dbPath := testDB(t)
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	saveDependency(t, s, "T010")
	if err := s.SaveHandoff(sampleRecord("T010")); err != nil {
		t.Fatalf("SaveHandoff failed: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	reopened, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	defer reopened.Close()

	got, err := reopened.GetHandoff("T010")
	if err != nil {
		t.Fatalf("GetHandoff after reopen failed: %v", err)
	}
	if got.Capsule.TaskID != "T010" || got.Status != handoff.StatusCompressed {
		t.Errorf("handoff did not survive reopen: %+v", got)
	}
}
