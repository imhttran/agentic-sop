package jev

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// adapterCompatReportPath is the JEV-206 deliverable, relative to the repository
// root.
const adapterCompatReportPath = "docs/reports/JEV-WAVE2-ADAPTER-COMPAT.md"

// TestJEVAdapterCompatReportHasEvidence is the registered JEV-206 acceptance
// check. JEV-206 validates the decision-adapter contract across two repositories;
// the report is the evidence. A missing or heading-only report cannot pass: the
// check requires both repository revisions, the exact contract-test commands,
// and, for each of Julia-1 and Nimble, a command, an exit status, and a concrete
// result.
func TestJEVAdapterCompatReportHasEvidence(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(adapterCompatReportPath)))
	if err != nil {
		t.Fatalf("JEV-206 evidence report missing or unreadable: %v", err)
	}
	report := string(data)

	// Both repository revisions must be recorded with a full 40-hex SHA.
	sha := `[0-9a-f]{40}`
	for _, want := range []string{
		"agentic-sop revision:\\s*`(" + sha + ")`",
		"sop-decision-adapters revision:\\s*`(" + sha + ")`",
	} {
		if !regexp.MustCompile(want).MatchString(report) {
			t.Errorf("report does not record a full 40-hex revision matching %q", want)
		}
	}

	// The exact contract-test commands must be recorded verbatim.
	for _, cmd := range []string{
		"go test -count=1 ./internal/providers/julia/",
		"go test -count=1 ./internal/providers/nimble/",
	} {
		if !strings.Contains(report, cmd) {
			t.Errorf("report does not record the exact command %q", cmd)
		}
	}

	// Each adapter has its own section: a concrete command, an exit status, and a
	// non-trivial result. This is what makes a heading-only report insufficient.
	for _, adapter := range []string{"Julia-1", "Nimble"} {
		section := sectionBody(report, adapter+" verification")
		if section == "" {
			t.Errorf("report has no %q section", adapter+" verification")
			continue
		}
		if !regexp.MustCompile("`go test -count=1 [^`]+`").MatchString(section) {
			t.Errorf("%s section records no concrete test command", adapter)
		}
		if !regexp.MustCompile(`Exit status:\s*0`).MatchString(section) {
			t.Errorf("%s section does not record exit status 0", adapter)
		}
		if !hasConcreteResult(section) {
			t.Errorf("%s section has no concrete result (heading-only is insufficient)", adapter)
		}
	}
}

// TestJEVAdapterCompatReportHasNoCredentials keeps the recorded evidence free of
// credential material.
func TestJEVAdapterCompatReportHasNoCredentials(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(adapterCompatReportPath)))
	if err != nil {
		t.Fatalf("JEV-206 evidence report missing or unreadable: %v", err)
	}
	lower := strings.ToLower(string(data))
	for _, bad := range []string{"api_key", "apikey", "authorization:", "bearer ", "password", "secret", "token=", "-----begin"} {
		if strings.Contains(lower, bad) {
			t.Errorf("report appears to contain credential material: %q", bad)
		}
	}
}

// sectionBody returns the body of the heading line that contains title, up to the
// next heading line, regardless of heading level.
func sectionBody(report, title string) string {
	lines := strings.Split(report, "\n")
	start := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") && strings.Contains(trimmed, title) {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}
	var b strings.Builder
	for _, line := range lines[start+1:] {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			break
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// hasConcreteResult requires a "Result:" label followed by substantive content,
// so a report that merely contains headings and labels cannot satisfy the check.
func hasConcreteResult(section string) bool {
	for _, line := range strings.Split(section, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- Result:") && !strings.HasPrefix(trimmed, "Result:") {
			continue
		}
		body := strings.TrimSpace(strings.TrimPrefix(trimmed, "- Result:"))
		body = strings.TrimSpace(strings.TrimPrefix(body, "Result:"))
		if len([]rune(body)) >= 20 {
			return true
		}
	}
	return false
}

// repoRoot walks up from the working directory to the directory containing go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate repository root (go.mod) from %s", dir)
		}
		dir = parent
	}
}
