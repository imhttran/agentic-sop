package skill

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestBrowserTestSkillContract proves the shipped /sop-browser-test skill runs and
// maintains a project's existing Playwright suite: it discovers the project's own
// configuration and filters, runs the documented command, refuses to report an unrun
// suite as a pass, verifies backend state for state-changing flows, updates tests on
// --update, and fails closed when the project has no suite or a dependency is missing.
func TestBrowserTestSkillContract(t *testing.T) {
	body := normalizeSpaces(read(t, filepath.Join(skillsRoot, "sop-browser-test", "SKILL.md")))
	for _, want := range []string{
		"Playwright",
		"--mobile", "--approvals", "--update",
		"playwright.config",
		"npm run test:e2e",
		"sop approvals --json",
		"APPROVED", "DECLINED",
		"skipped",
		"trace",
		"no Playwright suite",
		"CLEANUP",
		"fail closed",
		"PATH",
		"assume nothing",
		"not a pass",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("browser-test contract missing %q", want)
		}
	}
}

// TestBrowserTestSkillDrivesProjectSuite proves the skill's shell examples run the
// project's own Playwright runner — it is the one command whose shell examples are the
// target's test suite, not the SOP CLI — while still calling `sop` for the SOP state it
// must verify.
func TestBrowserTestSkillDrivesProjectSuite(t *testing.T) {
	body := read(t, filepath.Join(skillsRoot, "sop-browser-test", "SKILL.md"))
	var runner, sopLines int
	for _, line := range bashCommandLines(body) {
		switch {
		case strings.HasPrefix(line, "sop "):
			sopLines++
		case strings.HasPrefix(line, "npm "), strings.HasPrefix(line, "npx "),
			strings.HasPrefix(line, "pnpm "), strings.HasPrefix(line, "yarn "),
			strings.HasPrefix(line, "make "):
			runner++
		default:
			t.Errorf("unexpected shell example in browser-test skill: %q", line)
		}
	}
	if runner == 0 {
		t.Error("browser-test must show the project's documented test command")
	}
	if sopLines == 0 {
		t.Error("browser-test must show the `sop` calls it verifies against")
	}
	for _, forbidden := range append([]string{
		"puppeteer", "webdriver", "git commit", "git push", "git add",
	}, ModelLiterals...) {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Errorf("browser-test skill contains %q", forbidden)
		}
	}
}
