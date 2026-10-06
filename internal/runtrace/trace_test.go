package runtrace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildAndWriteIsVersionedAndValid(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1000, 0).UTC()
	tr := Build(Inputs{
		RunID:       "T001",
		TaskID:      "T001",
		StartedAt:   now,
		CompletedAt: now.Add(2 * time.Second),
		Execution: Execution{
			Capability: "implement", ModelClass: "medium", Provider: "ollama",
			Model: "m", Locality: "cloud", ExecutionSource: "primary",
		},
		RepositoryMutations: 1,
		ChangedFiles:        []string{"internal/x.go"},
		Iterations:          []Iteration{{Sequence: 1, Phase: "CHANGE", Timestamp: now, RepositoryMutation: true, ChangedFiles: []string{"internal/x.go"}}},
		Verification:        []Verification{{Command: "go test ./...", Status: "PASS", ExitCode: 0, DurationMS: 5}},
		Termination:         Termination{Stage: "PASSED"},
	})
	if err := Write(dir, tr); err != nil {
		t.Fatalf("Write: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatalf("read %s: %v", FileName, err)
	}
	var got Trace
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("trace.json is not valid JSON: %v", err)
	}
	if got.SchemaVersion != SchemaVersion {
		t.Errorf("schema_version = %d, want %d", got.SchemaVersion, SchemaVersion)
	}
	for _, k := range []string{`"schema_version"`, `"run_id"`, `"started_at"`, `"completed_at"`, `"execution"`, `"iterations"`, `"verification"`, `"termination"`} {
		if !strings.Contains(string(data), k) {
			t.Errorf("trace.json missing stable field %s", k)
		}
	}
}

func TestWriteRejectsUnknownVersionAndBlankDir(t *testing.T) {
	if err := Write(t.TempDir(), Trace{SchemaVersion: 99}); err == nil {
		t.Errorf("an unknown schema version must be rejected")
	}
	if err := Write("", Trace{SchemaVersion: SchemaVersion}); err == nil {
		t.Errorf("a blank run directory must be rejected")
	}
}

func TestOneLineCollapsesAndCaps(t *testing.T) {
	if got := OneLine("  a\nb  "); got != "a" {
		t.Errorf("OneLine = %q, want %q", got, "a")
	}
	long := strings.Repeat("x", maxText+50)
	if got := OneLine(long); len(got) != maxText {
		t.Errorf("OneLine length = %d, want capped at %d", len(got), maxText)
	}
}

// TestBuildPersistsReplans proves the bounded strategy changes are recorded
// observationally and round-trip through the versioned artifact, and that none are
// fabricated when no replan occurred.
func TestBuildPersistsReplans(t *testing.T) {
	dir := t.TempDir()
	tr := Build(Inputs{
		RunID:       "T001",
		StartedAt:   time.Unix(1000, 0).UTC(),
		CompletedAt: time.Unix(1002, 0).UTC(),
		Replans:     []ReplanRecord{{Sequence: 1, Reason: "review failure", FromAttempt: 1, ToAttempt: 2}},
	})
	if err := Write(dir, tr); err != nil {
		t.Fatalf("Write: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatalf("read %s: %v", FileName, err)
	}
	var got Trace
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("trace.json is not valid JSON: %v", err)
	}
	if len(got.Replans) != 1 || got.Replans[0] != tr.Replans[0] {
		t.Errorf("replans = %+v, want %+v", got.Replans, tr.Replans)
	}

	// With no replans the field is omitted rather than fabricated as an empty list.
	empty := Build(Inputs{RunID: "T002"})
	if empty.Replans != nil {
		t.Errorf("replans = %+v, want nil when none occurred", empty.Replans)
	}
	edir := t.TempDir()
	if err := Write(edir, empty); err != nil {
		t.Fatalf("Write: %v", err)
	}
	eData, err := os.ReadFile(filepath.Join(edir, FileName))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(eData), `"replans"`) {
		t.Errorf("trace.json must omit replans when none occurred: %s", eData)
	}
}
