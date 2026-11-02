package archtest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestACPImplementationNoteRecordsDecisions is the deterministic half of
// ACP-001's acceptance. ACP-001's deliverable is a design note, so no behavioral
// test can prove its content; this guard keeps the note present and keeps every
// decision area the plan requires recorded in it. It reads the note only and
// touches no other file.
var acpNoteSections = []string{
	"Repository inspection",
	"Boundary inventory",
	"Smallest shared service API",
	"Protocol, library, and Zed selection",
	"Lifecycle semantics",
	"Plan loading and preservation",
	"No-speculation statement",
}

func TestACPImplementationNoteRecordsDecisions(t *testing.T) {
	root := moduleRoot(t)
	const note = "docs/plans/ACP-001-IMPLEMENTATION-NOTE.md"

	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(note)))
	if err != nil {
		t.Fatalf("ACP-001 deliverable %s is unreadable: %v", note, err)
	}
	text := string(data)
	if strings.TrimSpace(text) == "" {
		t.Fatalf("ACP-001 deliverable %s is empty", note)
	}
	for _, section := range acpNoteSections {
		if !strings.Contains(text, section) {
			t.Errorf("%s no longer records the %q decision area", note, section)
		}
	}
	// The note must point at the plan it was written for, so a reader can trace
	// the decisions back to the stages that own them.
	if !strings.Contains(text, "PLAN-SOP-ACP-ZED.md") {
		t.Errorf("%s does not name the plan it belongs to", note)
	}
	// A revision-pinned assessment is the note's core evidence claim.
	if !regexp.MustCompile(`(?i)(source revision|revision:)`).MatchString(text) {
		t.Errorf("%s no longer records the pinned source revision", note)
	}
}
