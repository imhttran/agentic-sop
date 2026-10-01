package dist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/skill"
)

// The two installers implement the same conceptual contract on their own platform:
// install.sh on macOS/Linux, install.ps1 on Windows. These cross-platform checks keep
// them from drifting apart, and keep SOP policy out of both.
const (
	installShPath = "../../install.sh"
	installPSPath = "../../install.ps1"
)

// TestSkillTreeMatchesTheCommandSurface proves the canonical skills/ tree and the command
// surface every validator and installer works from are the same set. A skill added,
// renamed, or dropped in one place and not the others fails here.
func TestSkillTreeMatchesTheCommandSurface(t *testing.T) {
	skillsDir := filepath.Join(repoRoot(t), "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		t.Fatalf("read skills/: %v", err)
	}

	onDisk := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(skillsDir, entry.Name(), "SKILL.md")); err != nil {
			t.Errorf("skills/%s has no SKILL.md, so it is not a command", entry.Name())
			continue
		}
		onDisk[entry.Name()] = true
	}

	for _, c := range skill.Commands {
		if !onDisk[c.Name] {
			t.Errorf("the command surface declares %q, which skills/ does not ship", c.Name)
			continue
		}
		delete(onDisk, c.Name)
	}
	for name := range onDisk {
		t.Errorf("skills/ ships %q, which the command surface does not declare", name)
	}
}

// TestInstallersShareTheDistributionContract proves both installers offer the same
// operations and observe the same boundaries: no provider access, no model, and no SOP
// policy. It is the cross-platform half of the distribution coverage; the Windows
// installer's behaviour is exercised in install_windows_test.go.
func TestInstallersShareTheDistributionContract(t *testing.T) {
	sh := read(t, installShPath)
	ps := read(t, installPSPath)

	// The Unix installer's documented flags.
	for _, flag := range []string{"--skills", "--plugin", "--bin-dir", "--dry-run", "--help", "--all"} {
		if !strings.Contains(sh, flag) {
			t.Errorf("install.sh does not offer %s", flag)
		}
	}
	// The PowerShell installer's documented parameters, with the values they accept.
	for _, param := range []string{
		"[ValidateSet('none', 'zed', 'claude', 'all')][string]$Skills = 'none'",
		"[ValidateSet('none', 'claude')][string]$Plugin = 'none'",
		"[switch]$All",
		"[string]$BinDir",
		"[switch]$DryRun",
		"[ValidateSet('none', 'zed', 'claude', 'all')][string]$UninstallSkills = 'none'",
		"[switch]$Force",
		"[switch]$Help",
	} {
		if !strings.Contains(ps, param) {
			t.Errorf("install.ps1 does not declare %s", param)
		}
	}

	// Both work from the canonical skills/ tree and the one Claude plugin package.
	for name, body := range map[string]string{"install.sh": sh, "install.ps1": ps} {
		for _, want := range []string{"skills", "integrations", "claude", ".claude-plugin"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s does not reference %q", name, want)
			}
		}
	}

	// The Windows installer copies rather than symlinks, so it records ownership. The
	// marker name is part of the contract that uninstall and update rely on.
	if !strings.Contains(ps, "'.sop-managed'") {
		t.Error("install.ps1 does not use the .sop-managed ownership marker")
	}
	for _, want := range []string{".agents", ".claude"} {
		if !strings.Contains(ps, want) {
			t.Errorf("install.ps1 does not reference the %s skill root", want)
		}
	}

	// An installer never resolves a provider, and names no model or policy setting: it
	// installs SOP and stops. The adapters it installs are validated separately.
	for name, body := range map[string]string{"install.sh": sh, "install.ps1": ps} {
		lower := strings.ToLower(body)
		for _, bin := range skill.ProviderBinaries {
			if strings.Contains(lower, bin) {
				t.Errorf("%s names the provider runtime %q", name, bin)
			}
		}
		for _, knob := range skill.PolicyKnobs {
			if strings.Contains(body, knob) {
				t.Errorf("%s restates the SOP policy setting %q", name, knob)
			}
		}
		for _, literal := range skill.ModelLiterals {
			if strings.Contains(lower, literal) {
				t.Errorf("%s names a model (%q)", name, literal)
			}
		}
	}

	// PowerShell specifics: nothing may reinterpret text as code, and the installer
	// itself never mutates PATH (the guidance it prints is text).
	for _, forbidden := range []string{"Invoke-Expression", "iex ", "setx "} {
		if strings.Contains(ps, forbidden) {
			t.Errorf("install.ps1 contains %q", forbidden)
		}
	}

	// A cheap structural proxy for the PowerShell syntax, because this test also runs
	// where PowerShell is unavailable. The real check is the PowerShell language parser
	// in install_windows_test.go, which parses the file before running any behaviour.
	if open, close := strings.Count(ps, "{"), strings.Count(ps, "}"); open != close {
		t.Errorf("install.ps1 has %d opening and %d closing braces", open, close)
	}
	if open, close := strings.Count(ps, "("), strings.Count(ps, ")"); open != close {
		t.Errorf("install.ps1 has %d opening and %d closing parentheses", open, close)
	}
	if open, close := strings.Count(ps, "@'"), countLinesStartingWith(ps, "'@"); open != close {
		t.Errorf("install.ps1 has %d here-string openers and %d closers", open, close)
	}
}

// countLinesStartingWith counts the lines that begin with prefix. A PowerShell here-string
// closes only with a delimiter at the start of a line.
func countLinesStartingWith(body, prefix string) int {
	n := 0
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, prefix) {
			n++
		}
	}
	return n
}
