package skill

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestContinueSkillContract proves the shipped /sop-continue skill states the
// reconcile-before-continue contract: the workflow phases, every classification,
// the stop boundaries, the diagnostic sinks, the commit/push policy, bounded
// continuation, and the safety rules an operator relies on.
func TestContinueSkillContract(t *testing.T) {
	body := normalizeSpaces(read(t, filepath.Join(skillsRoot, "sop-continue", "SKILL.md")))
	for _, want := range []string{
		"disable-model-invocation: true",
		"## PREFLIGHT", "## RECONCILE", "## VALIDATE CONTINUATION", "## CONTINUE",
		"## OBSERVE", "## STOP BOUNDARIES", "## REPORT",
		"RECONCILE_CLEAN", "RECONCILE_CHANGED", "RECONCILE_SEMANTIC_CHANGE",
		"RECONCILE_FAILED", "RECONCILE_NONDETERMINISTIC",
		"CONTINUE_SAFE", "RUN_AGAIN", "HUMAN_REVIEW_REQUIRED", "APPROVAL_REQUIRED",
		"BLOCKED", "PLAN_COMPLETE",
		"sop continue --check", "sop run", "sop status", "sop approvals --json",
		"--list-changed",
		"SOP_OLLAMA_TRACE_LOG", "SOP_TOOL_AUDIT_LOG",
		"NO COMMIT, NO PUSH", "reconcile before continue",
		"bounded", "DENIED", "REQUIRES_APPROVAL", "state database",
		"Preserve user-owned changes",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("continue contract missing %q", want)
		}
	}
}

// TestContinueSkillCommandsAreSOPOnly proves every documented command drives the
// `sop` CLI and introduces no independent lifecycle action: the only mutating call
// is `sop run`, and reconciliation is read-only (`--list-changed`).
func TestContinueSkillCommandsAreSOPOnly(t *testing.T) {
	body := read(t, filepath.Join(skillsRoot, "sop-continue", "SKILL.md"))
	allowed := map[string]bool{
		"status": true, "approvals": true, "continue": true, "run": true,
		"reconcile": true, "task": true, "report": true,
	}
	for _, line := range bashCommandLines(body) {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "sop" {
			t.Errorf("non-SOP command: %q", line)
			continue
		}
		switch fields[1] {
		case "reconcile":
			// reconcile is either the read-only preview (--list-changed) or the
			// deterministic apply; it must never carry --accept-changed, which is a human
			// decision the skill must not make.
			if strings.Contains(line, "--accept-changed") {
				t.Errorf("continue skill must not pass --accept-changed: %q", line)
			}
		case "continue":
			if !strings.Contains(line, "--check") {
				t.Errorf("continue example must be the read-only check: %q", line)
			}
		default:
			if !allowed[fields[1]] {
				t.Errorf("continue skill introduces an independent lifecycle action: %q", line)
			}
		}
	}
	for _, forbidden := range append([]string{"--yes", "--force", "--disposition"}, ModelLiterals...) {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Errorf("continue skill contains %q", forbidden)
		}
	}
}

// TestContinueSkillNeverMutatesState proves the skill forbids the unacceptable
// recovery mechanisms: no direct state editing, no history rewriting, no forced
// retries, and no approval or tool-policy bypass.
func TestContinueSkillNeverMutatesState(t *testing.T) {
	body := normalizeSpaces(read(t, filepath.Join(skillsRoot, "sop-continue", "SKILL.md")))
	for _, want := range []string{
		"never edits the state database",
		"force retries",
		"reset",
		"stash",
		"bypass tool policy",
		"bypass approvals",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("continue skill must forbid %q", want)
		}
	}
}

// normalizeSpaces collapses runs of whitespace to single spaces, so a prose
// assertion does not depend on the skill's line wrapping.
func normalizeSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
