package skill

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestMCPLiveValidationSkillContract proves the shipped /mcp-live-validation skill
// covers the full issue-to-evidence workflow, reuses the existing candidate helper,
// separates local tests from independent validation, and preserves the safety
// boundaries: no production replacement, no automatic merge, no broad process kill,
// and no credentials in evidence.
func TestMCPLiveValidationSkillContract(t *testing.T) {
	body := normalizeSpaces(read(t, filepath.Join(skillsRoot, "mcp-live-validation", "SKILL.md")))
	for _, want := range []string{
		"disable-model-invocation: true",
		"## DISCOVER", "## DEPENDENCIES", "## PROVISION", "## LOCAL VERIFY", "## IDENTITY",
		"## APPROVAL GATE", "## DEPLOY", "## HANDOFF",
		"## INDEPENDENT VALIDATION", "## FAILURE LOOP", "## EVIDENCE",
		"## ROLLBACK", "## PR PREPARATION", "## COMPLETION REPORT", "## REUSE MAP",
		"scripts/planner-tunnel-candidate.sh", "scripts/planner-tunnel-point.sh",
		"docs/testing/MCP-CANDIDATE-HANDOFF.md", "make test-planner-tunnel",
		"PENDING", "PASS", "FAIL",
		"10 rounds", "20 calls", "no retries", "no state changes",
		"mcp_initialization_required", "409", "JSON-RPC",
		"cannot invoke ChatGPT tools", "local tests", "not independent validation",
		"approval", "rollback", "never merges", "production binary",
		"credentials", "PATH", "different MCP surface",
		"orchestration framework", "sop status",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("mcp-live-validation contract missing %q", want)
		}
	}

	// The skill's shell examples are repository helpers, Go, Make, Bash, and SOP
	// state checks. It must not drive an ad-hoc lifecycle or a process killer.
	allowedPrefixes := []string{
		"sop ", "scripts/", "go ", "make ", "bash ", "shasum ", "sha256sum ",
	}
	for _, line := range bashCommandLines(read(t, filepath.Join(skillsRoot, "mcp-live-validation", "SKILL.md"))) {
		ok := false
		for _, prefix := range allowedPrefixes {
			if strings.HasPrefix(line, prefix) {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("unexpected mcp-live-validation shell example: %q", line)
		}
	}

	for _, forbidden := range append([]string{
		"git commit", "git push", "git add", "sed -i", "rm -rf",
		"pkill", "killall", "fail_on", "max_fix_cycles",
	}, ModelLiterals...) {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Errorf("mcp-live-validation skill contains %q", forbidden)
		}
	}
}

// TestMCPLiveValidationHandoffArtifact proves the helper's JSON handoff records the
// minimum machine-readable contract without claiming independent validation.
func TestMCPLiveValidationHandoffArtifact(t *testing.T) {
	body := read(t, filepath.Join(skillsRoot, "mcp-live-validation", "SKILL.md"))
	for _, want := range []string{
		"mcp-live-validation/handoff/v1",
		"candidate", "baseline", "process", "logs", "rollback",
		"independent_validation", "local_tests_are_not_validation",
		"--validation-status PASS", "requires `--results FILE`",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("handoff contract missing %q", want)
		}
	}
}
