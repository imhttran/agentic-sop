package skill

import (
	"os/exec"
	"path/filepath"
	"runtime"
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
		"## INDEPENDENT VALIDATION", "## CROSS-AGENT CONTINUATION", "## FAILURE LOOP", "## EVIDENCE",
		"## ROLLBACK", "## PR PREPARATION", "## COMPLETION REPORT", "## REUSE MAP",
		"scripts/planner-tunnel-candidate.sh", "scripts/planner-tunnel-point.sh",
		"scripts/validation-handoff.py", "docs/reference/validation-handoff.schema.json",
		"docs/testing/MCP-CANDIDATE-HANDOFF.md", "docs/guides/MCP-LIVE-VALIDATION-PROVIDERS.md",
		"make test-planner-tunnel", "make test-mcp-portability",
		"PENDING", "PASS", "FAIL",
		"10 rounds", "20 calls", "no retries", "no state changes",
		"mcp_initialization_required", "409", "JSON-RPC",
		"cannot invoke another agent's MCP tools", "local tests", "not independent validation",
		"approval", "rollback", "never merges", "production binary",
		"credentials", "PATH", "different MCP surface", "artifact-only",
		"provider-neutral", "ChatGPT", "Claude", "next_permitted_action",
		"START_CANDIDATE", "validation-handoff.json", "orchestration framework", "sop status",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("mcp-live-validation contract missing %q", want)
		}
	}

	// The skill's shell examples are repository helpers, Go, Make, Bash, and SOP
	// state checks. It must not drive an ad-hoc lifecycle or a process killer.
	allowedPrefixes := []string{
		"sop ", "scripts/", "go ", "make ", "bash ", "shasum ", "sha256sum ", "python3 ",
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

// TestValidationHandoffPortability runs the simulated cross-agent scenarios:
// Claude prepares a candidate, ChatGPT records independent results, and a second
// agent resumes from the evidence. It also proves invalid handoffs are rejected
// without touching the candidate, baseline, or running process. The script needs
// bash, git, ps, and python3; it is skipped on Windows.
func TestValidationHandoffPortability(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the portability fixture uses POSIX processes and shell")
	}
	cmd := exec.Command("bash", filepath.Join("..", "..", "scripts", "checks", "test-validation-handoff-portability.sh"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("portability test failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "all validation-handoff portability tests passed") {
		t.Fatalf("portability test did not report success:\n%s", out)
	}
}

// TestMCPLiveValidationHandoffArtifact proves the helper's JSON handoff records the
// portable, versioned contract without claiming independent validation.
func TestMCPLiveValidationHandoffArtifact(t *testing.T) {
	body := read(t, filepath.Join(skillsRoot, "mcp-live-validation", "SKILL.md"))
	for _, want := range []string{
		"mcp-live-validation/validation-handoff/v1",
		"schema_version", "workflow_id", "next_permitted_action",
		"candidate", "baseline", "process", "logs", "rollback",
		"independent_validation", "local_tests_are_not_validation",
		"validation-results/v1", "results_template",
		"--validation-status", "requires `--results-json FILE`",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("handoff contract missing %q", want)
		}
	}
}
