package skill

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestBrowserTestSkillContract proves the shipped /sop-browser-test skill detects UI
// changes, analyzes browser-test coverage, and runs or maintains a project's existing
// Playwright suite: it discovers the project's own configuration and filters, works from
// Git changes, reports coverage gaps on --audit, updates tests on --update without
// weakening valid assertions, refuses to report an unrun suite as a pass, and fails
// closed when the project has no suite or a dependency is missing.
func TestBrowserTestSkillContract(t *testing.T) {
	body := normalizeSpaces(read(t, filepath.Join(skillsRoot, "sop-browser-test", "SKILL.md")))
	for _, want := range []string{
		"Playwright",
		"--mobile", "--approvals", "--audit", "--update",
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
		"git diff --name-only",
		"coverage",
		"NO TEST",
		"regression",
		"Never delete or weaken a valid assertion",
		"lightweight",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("browser-test contract missing %q", want)
		}
	}
}

// TestBrowserTestSkillDrivesProjectSuite proves the skill's shell examples are the
// discovery and runner commands it needs — Git change detection, the project's own
// Playwright runner, and the `sop` calls it verifies against. It is the one command whose
// shell examples are not a pure SOP CLI client.
func TestBrowserTestSkillDrivesProjectSuite(t *testing.T) {
	body := read(t, filepath.Join(skillsRoot, "sop-browser-test", "SKILL.md"))
	var runner, sopLines, gitLines int
	for _, line := range bashCommandLines(body) {
		switch {
		case strings.HasPrefix(line, "sop "):
			sopLines++
		case strings.HasPrefix(line, "git "):
			gitLines++
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
	if gitLines == 0 {
		t.Error("browser-test must show the Git change detection it works from")
	}
	for _, forbidden := range append([]string{
		"puppeteer", "webdriver", "git commit", "git push", "git add",
	}, ModelLiterals...) {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Errorf("browser-test skill contains %q", forbidden)
		}
	}
}
