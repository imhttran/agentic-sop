package skill

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestBrowserTestSkillContract proves the shipped /sop-browser-test skill states its
// contract: it reuses the Playwright MCP, discovers the target rather than inventing it,
// drives real interactions, checks for errors, verifies backend state, tests approve and
// decline against separate disposable approvals, cleans up only what it created, and
// fails closed without the MCP or `sop`.
func TestBrowserTestSkillContract(t *testing.T) {
	body := normalizeSpaces(read(t, filepath.Join(skillsRoot, "sop-browser-test", "SKILL.md")))
	for _, want := range []string{
		"Playwright MCP",
		"--mobile", "--approvals",
		"sop approvals --json",
		"APPROVED", "DECLINED",
		"separate disposable",
		"fail closed", "PATH",
		"console",
		"screenshot",
		"CLEANUP", "production",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("browser-test contract missing %q", want)
		}
	}
}

// TestBrowserTestSkillBashInvokesSOPOnly proves the skill drives the browser through the
// MCP and the shell only through the `sop` CLI: no shell example runs a browser, a
// framework, or a server directly.
func TestBrowserTestSkillBashInvokesSOPOnly(t *testing.T) {
	body := read(t, filepath.Join(skillsRoot, "sop-browser-test", "SKILL.md"))
	for _, line := range bashCommandLines(body) {
		if !strings.HasPrefix(line, "sop ") {
			t.Errorf("browser-test shell example does not invoke sop: %q", line)
		}
	}
	for _, forbidden := range append([]string{
		"npx ", "npm ", "playwright test", "puppeteer", "webdriver",
		"git commit", "git push", "git add",
	}, ModelLiterals...) {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Errorf("browser-test skill contains %q", forbidden)
		}
	}
}
