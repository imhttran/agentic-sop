package skill

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEndToEndSkillContract(t *testing.T) {
	body := read(t, filepath.Join(skillsRoot, "sop-end-to-end", "SKILL.md"))
	for _, want := range []string{
		"disable-model-invocation: true",
		"## DISCOVER", "## PREFLIGHT", "## DELEGATE", "## OBSERVE",
		"## HUMAN BOUNDARY", "## RESUME/RECOVER", "## REPORT",
		"sop run", "sop status", "sop approvals --json", "--list-changed",
		"SOP state is authoritative", "one execution invocation",
		"APPROVAL_REQUIRED", "NEEDS_HUMAN", "BLOCKED", "FAIL", "CONTINUE",
		"IMPLEMENT_NO_CHANGES", "IMPLEMENT_NO_PROGRESS", "ALREADY_SATISFIED",
		"SMALL/MEDIUM/LARGE", "NOT RUN", "UNAVAILABLE",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("end-to-end contract missing %q", want)
		}
	}

	// Execution has one delegation; other documented commands only observe SOP.
	for _, line := range bashCommandLines(body) {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "sop" {
			t.Errorf("non-SOP command: %q", line)
			continue
		}
		switch fields[1] {
		case "run", "status", "task", "report", "approvals":
		case "reconcile":
			if !strings.Contains(line, "--list-changed") {
				t.Errorf("reconciliation example must only inspect changes: %q", line)
			}
		default:
			t.Errorf("end-to-end skill introduces an independent lifecycle action: %q", line)
		}
	}
	for _, forbidden := range append([]string{"--force", "--yes", "--accept-changed", "sleep ", "while ", "--model-class", "sop run --json"}, ModelLiterals...) {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Errorf("end-to-end skill contains %q", forbidden)
		}
	}
}

func TestSOPEndToEndAliasesDelegateToRunSkill(t *testing.T) {
	body := read(t, filepath.Join(skillDir, "SKILL.md"))
	for _, want := range []string{"/sop end-end", "/sop end-to-end", "/sop-end-to-end", "sop run"} {
		if !strings.Contains(body, want) {
			t.Errorf("general entry point does not route end-to-end intent: missing %q", want)
		}
	}
}
