package sopclient

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An artifact that cannot be reached is not an absent one: ReportRef records the
// failure instead of presenting a missing link as "no report".
//
// A file where the run directory belongs makes os.Stat fail with ENOTDIR, an I/O
// error that is not fs.ErrNotExist.
func TestReportRefReportsStatFailure(t *testing.T) {
	root := newProject(t)
	runs := filepath.Join(root, ".agent-sdlc", "runs")
	if err := os.WriteFile(filepath.Join(runs, "t1"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ref := st.ReportRef("t1")
	if ref.Present {
		t.Fatalf("ReportRef = %+v, want Present=false for an unreachable artifact", ref)
	}
	if ref.Error == "" {
		t.Fatal("ReportRef.Error = \"\", want the stat failure recorded")
	}
	if !strings.Contains(ref.Error, reportArtifact) {
		t.Fatalf("ReportRef.Error = %q, want it to name %s", ref.Error, reportArtifact)
	}
}

// Absence stays absence: no artifact, no error.
func TestReportRefAbsentIsNotAnError(t *testing.T) {
	root := newProject(t)
	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ref := st.ReportRef("t1")
	if ref.Present || ref.Error != "" || ref.Path != "" {
		t.Fatalf("ReportRef = %+v, want a plain absence", ref)
	}
}

// A present artifact is still reported as present, with no error.
func TestReportRefPresent(t *testing.T) {
	root := newProject(t)
	writeArtifact(t, root, "t1", reportArtifact, `{"id":"t1"}`)

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ref := st.ReportRef("t1")
	if !ref.Present || ref.Error != "" || ref.Path == "" {
		t.Fatalf("ReportRef = %+v, want Present with no error", ref)
	}
}

// A config that exists but cannot be read is reported on the summary, so the id
// standing in for the name is explained rather than silent.
//
// A directory where config.yaml belongs makes the read fail with EISDIR.
func TestSummaryReportsUnreadableConfig(t *testing.T) {
	root := newProject(t)
	cfg := filepath.Join(root, ".agent-sdlc", "config.yaml")
	if err := os.Remove(cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cfg, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	sum, err := st.Summary(context.Background(), ChangedTasks{}, ApprovalsListing{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if sum.ConfigError == "" {
		t.Fatal("ConfigError = \"\", want the unreadable config reported")
	}
	if !strings.Contains(sum.ConfigError, "config.yaml") {
		t.Fatalf("ConfigError = %q, want it to name config.yaml", sum.ConfigError)
	}
	if sum.Name != sum.ID {
		t.Fatalf("Name = %q, want the id %q as the documented fallback", sum.Name, sum.ID)
	}
}

// A missing config is a legitimate absence: the id fallback is not an error.
func TestSummaryMissingConfigIsNotAnError(t *testing.T) {
	root := newProject(t)
	if err := os.Remove(filepath.Join(root, ".agent-sdlc", "config.yaml")); err != nil {
		t.Fatal(err)
	}

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	sum, err := st.Summary(context.Background(), ChangedTasks{}, ApprovalsListing{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if sum.ConfigError != "" {
		t.Fatalf("ConfigError = %q, want no error for a missing config", sum.ConfigError)
	}
}

// A readable config still supplies the declared name, with no error.
func TestSummaryReadsConfig(t *testing.T) {
	root := newProject(t)
	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	sum, err := st.Summary(context.Background(), ChangedTasks{}, ApprovalsListing{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if sum.ConfigError != "" {
		t.Fatalf("ConfigError = %q, want no error for a readable config", sum.ConfigError)
	}
	if sum.Name != "demo" {
		t.Fatalf("Name = %q, want the config's declared name", sum.Name)
	}
}
