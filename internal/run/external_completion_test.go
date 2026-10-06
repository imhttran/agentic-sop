package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestExternalCompletionValidateFailsClosed proves a malformed provenance record is
// rejected, so a bad artifact is never written.
func TestExternalCompletionValidateFailsClosed(t *testing.T) {
	valid := ExternalCompletion{
		Version: ExternalCompletionVersion, TaskID: "T001", CompletionSource: CompletionSourceExternal,
		RepositoryHead: "abc", ImplementationCommit: "abc", Verification: "PASS",
		RecordedBy: RecordedByOperatorAction, RecordedAt: time.Unix(0, 0).UTC(),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid record rejected: %v", err)
	}
	for name, mut := range map[string]func(c *ExternalCompletion){
		"unknown version":   func(c *ExternalCompletion) { c.Version = 99 },
		"missing task":      func(c *ExternalCompletion) { c.TaskID = "" },
		"bad source":        func(c *ExternalCompletion) { c.CompletionSource = "model" },
		"missing commit":    func(c *ExternalCompletion) { c.ImplementationCommit = "" },
		"failed validation": func(c *ExternalCompletion) { c.Verification = "FAIL" },
	} {
		c := valid
		mut(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: want an error, got none", name)
		}
	}
}

// TestWriteExternalCompletionPersists proves the record round-trips through the run
// artifact directory.
func TestWriteExternalCompletionPersists(t *testing.T) {
	dir := t.TempDir()
	r, err := New(dir, "T001")
	if err != nil {
		t.Fatalf("run.New: %v", err)
	}
	rec := ExternalCompletion{
		TaskID: "T001", CompletionSource: CompletionSourceExternal, RepositoryHead: "abc",
		ImplementationCommit: "abc", Verification: "PASS", RecordedBy: RecordedByOperatorAction,
		RecordedAt: time.Unix(0, 0).UTC(),
	}
	if err := r.WriteExternalCompletion(rec); err != nil {
		t.Fatalf("WriteExternalCompletion: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(r.Dir(), ExternalCompletionFile))
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	for _, want := range []string{`"completion_source": "external"`, `"verification": "PASS"`, `"recorded_by": "explicit_operator_action"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("artifact missing %s: %s", want, data)
		}
	}
}
