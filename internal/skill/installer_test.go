package skill

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The SOP skills are installed by shell scripts. These tests run those scripts against
// a scratch HOME (t.TempDir), so the developer's real ~/.agents and ~/.claude skill
// directories are never touched, and prove the installers are idempotent, scoped, and
// safe against unrelated entries.

const installerScript = "../../scripts/install-skills.sh"

// scratchHome returns a temporary HOME with the given agent markers created (for
// example ".agents" for Zed, ".claude" for Claude Code).
func scratchHome(t *testing.T, markers ...string) string {
	t.Helper()
	home := t.TempDir()
	for _, m := range markers {
		if err := os.MkdirAll(filepath.Join(home, m), 0o755); err != nil {
			t.Fatalf("create marker %s: %v", m, err)
		}
	}
	return home
}

// runInstaller runs an installer script with HOME redirected to home and returns its
// combined output.
func runInstaller(t *testing.T, home, script string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{script}, args...)...)
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// assertLinked proves every SOP command is a symlink in root whose SKILL.md resolves
// (a broken link would be invisible to the agent).
func assertLinked(t *testing.T, root string) {
	t.Helper()
	for _, s := range zedSkills {
		path := filepath.Join(root, s.name)
		fi, err := os.Lstat(path)
		if err != nil {
			t.Errorf("missing %s: %v", path, err)
			continue
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s is not a symlink", path)
			continue
		}
		if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil {
			t.Errorf("%s/SKILL.md does not resolve: %v", path, err)
		}
	}
}

// assertAbsent proves no SOP command is present in root.
func assertAbsent(t *testing.T, root string) {
	t.Helper()
	for _, s := range zedSkills {
		path := filepath.Join(root, s.name)
		if _, err := os.Lstat(path); err == nil {
			t.Errorf("%s should not exist", path)
		}
	}
}

// TestInstallerZedIsIdempotent installs for Zed twice: the second run must change
// nothing and must leave the same seven working links.
func TestInstallerZedIsIdempotent(t *testing.T) {
	home := scratchHome(t, ".agents")
	root := filepath.Join(home, ".agents", "skills")

	if out, err := runInstaller(t, home, installerScript, "zed"); err != nil {
		t.Fatalf("install zed: %v\n%s", err, out)
	}
	assertLinked(t, root)

	out, err := runInstaller(t, home, installerScript, "zed")
	if err != nil {
		t.Fatalf("re-install zed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "0 changed") {
		t.Errorf("second install should be a no-op, got:\n%s", out)
	}
	assertLinked(t, root)
}

// TestInstallerClaudeIsIdempotent is the same contract for Claude Code's skills root.
func TestInstallerClaudeIsIdempotent(t *testing.T) {
	home := scratchHome(t, ".claude")
	root := filepath.Join(home, ".claude", "skills")

	if out, err := runInstaller(t, home, installerScript, "claude"); err != nil {
		t.Fatalf("install claude: %v\n%s", err, out)
	}
	assertLinked(t, root)

	out, err := runInstaller(t, home, installerScript, "claude")
	if err != nil {
		t.Fatalf("re-install claude: %v\n%s", err, out)
	}
	if !strings.Contains(out, "0 changed") {
		t.Errorf("second install should be a no-op, got:\n%s", out)
	}
	assertLinked(t, root)
}

// TestInstallerAllInstallsPresentAgents proves `all` installs every agent whose
// directory exists, into that agent's own root.
func TestInstallerAllInstallsPresentAgents(t *testing.T) {
	home := scratchHome(t, ".agents", ".claude")

	out, err := runInstaller(t, home, installerScript, "all")
	if err != nil {
		t.Fatalf("install all: %v\n%s", err, out)
	}
	assertLinked(t, filepath.Join(home, ".agents", "skills"))
	assertLinked(t, filepath.Join(home, ".claude", "skills"))
}

// TestInstallerAllSkipsAbsentAgent proves `all` never creates configuration for an
// agent this machine does not have.
func TestInstallerAllSkipsAbsentAgent(t *testing.T) {
	home := scratchHome(t, ".agents") // no ~/.claude

	out, err := runInstaller(t, home, installerScript, "all")
	if err != nil {
		t.Fatalf("install all: %v\n%s", err, out)
	}
	assertLinked(t, filepath.Join(home, ".agents", "skills"))
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Error("all must not create ~/.claude when Claude is absent")
	}
	if !strings.Contains(out, "skip: claude") {
		t.Errorf("all should report skipping the absent agent, got:\n%s", out)
	}
}

// TestInstallerExplicitTargetWithNoMarker proves naming an agent installs it even when
// its directory is not present yet (the platform is filesystem-based).
func TestInstallerExplicitTargetWithNoMarker(t *testing.T) {
	home := t.TempDir() // no markers at all

	out, err := runInstaller(t, home, installerScript, "claude")
	if err != nil {
		t.Fatalf("install claude: %v\n%s", err, out)
	}
	assertLinked(t, filepath.Join(home, ".claude", "skills"))
}

// TestInstallerNoSupportedAgentFails proves `all` fails loudly rather than claiming
// success when no agent environment can be determined.
func TestInstallerNoSupportedAgentFails(t *testing.T) {
	home := t.TempDir()

	out, err := runInstaller(t, home, installerScript, "all")
	if err == nil {
		t.Fatal("`all` with no agent present should exit non-zero")
	}
	if !strings.Contains(out, "no supported agent environment") {
		t.Errorf("expected an actionable error, got:\n%s", out)
	}
}

// TestInstallerUnknownTargetFails proves an unknown target is rejected.
func TestInstallerUnknownTargetFails(t *testing.T) {
	home := t.TempDir()

	out, err := runInstaller(t, home, installerScript, "codex")
	if err == nil {
		t.Fatal("an unknown target should exit non-zero")
	}
	if !strings.Contains(out, "unknown argument") {
		t.Errorf("expected an unknown-argument error, got:\n%s", out)
	}
}

// TestInstallerDryRunChangesNothing proves --dry-run reports without touching disk.
func TestInstallerDryRunChangesNothing(t *testing.T) {
	home := scratchHome(t, ".claude")

	out, err := runInstaller(t, home, installerScript, "claude", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "would:") {
		t.Errorf("dry-run should print the actions it would take, got:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills")); !os.IsNotExist(err) {
		t.Error("dry-run must not create the skills root")
	}
}

// TestInstallerUninstallIsScoped proves uninstall removes only SOP links and leaves an
// unrelated skill untouched.
func TestInstallerUninstallIsScoped(t *testing.T) {
	home := scratchHome(t, ".claude")
	root := filepath.Join(home, ".claude", "skills")
	other := filepath.Join(root, "other", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(other), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}

	if out, err := runInstaller(t, home, installerScript, "claude"); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if out, err := runInstaller(t, home, installerScript, "claude", "--uninstall"); err != nil {
		t.Fatalf("uninstall: %v\n%s", err, out)
	}

	assertAbsent(t, root)
	if body, err := os.ReadFile(other); err != nil || string(body) != "keep me" {
		t.Errorf("unrelated skill was removed or changed: %q (%v)", body, err)
	}
}

// TestInstallerPreservesForeignEntryAtSOPName proves a real (non-SOP) entry at a SOP
// name is skipped by both install and uninstall, never replaced or deleted.
func TestInstallerPreservesForeignEntryAtSOPName(t *testing.T) {
	home := scratchHome(t, ".claude")
	root := filepath.Join(home, ".claude", "skills")
	foreign := filepath.Join(root, "sop", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(foreign), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreign, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runInstaller(t, home, installerScript, "claude")
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if !strings.Contains(out, "skip:") {
		t.Errorf("a foreign entry at a SOP name should be skipped, got:\n%s", out)
	}
	if body, err := os.ReadFile(foreign); err != nil || string(body) != "mine" {
		t.Errorf("installer clobbered a foreign entry at a SOP name: %q (%v)", body, err)
	}

	if out, err := runInstaller(t, home, installerScript, "claude", "--uninstall"); err != nil {
		t.Fatalf("uninstall: %v\n%s", err, out)
	}
	if body, err := os.ReadFile(foreign); err != nil || string(body) != "mine" {
		t.Errorf("uninstall removed a foreign entry at a SOP name: %q (%v)", body, err)
	}
}

// TestInstallerProjectScope proves --project installs into the project's own root and
// leaves the user root alone.
func TestInstallerProjectScope(t *testing.T) {
	home := scratchHome(t, ".claude")
	project := t.TempDir()

	out, err := runInstaller(t, home, installerScript, "claude", "--project", project)
	if err != nil {
		t.Fatalf("install --project: %v\n%s", err, out)
	}
	assertLinked(t, filepath.Join(project, ".claude", "skills"))
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills")); !os.IsNotExist(err) {
		t.Error("--project must not install into the user root")
	}
}

// TestInstallerForwardersDelegate proves the thin wrappers drive the shared installer
// with the right target.
func TestInstallerForwardersDelegate(t *testing.T) {
	home := scratchHome(t, ".agents", ".claude")
	cases := []struct {
		script string
		target string
	}{
		{"../../scripts/install-zed-skills.sh", "zed"},
		{"../../scripts/install-claude-skills.sh", "claude"},
	}
	for _, c := range cases {
		out, err := runInstaller(t, home, c.script, "--dry-run")
		if err != nil {
			t.Fatalf("%s: %v\n%s", c.script, err, out)
		}
		if !strings.Contains(out, "-> "+c.target) {
			t.Errorf("%s should install %q, got:\n%s", c.script, c.target, out)
		}
	}
}

// TestInstallerKnowsEveryTargetRoot proves the shared installer knows both agent
// layouts (the roots are the only real difference between the two adapters).
func TestInstallerKnowsEveryTargetRoot(t *testing.T) {
	body := read(t, installerScript)
	for _, want := range []string{".agents/skills", ".claude/skills", "zed", "claude", "all"} {
		if !strings.Contains(body, want) {
			t.Errorf("install-skills.sh does not know %q", want)
		}
	}
}

// TestInstallScriptsAreThinAndSafe proves the installers never invoke a provider or
// mutate a repository: they only link the canonical skills/ tree.
func TestInstallScriptsAreThinAndSafe(t *testing.T) {
	scripts := []string{
		installerScript,
		"../../scripts/install-zed-skills.sh",
		"../../scripts/install-claude-skills.sh",
	}
	forbidden := append([]string{}, providerBinaries...)
	forbidden = append(forbidden, "git ", "sed -i", "rm -rf", "go install")
	for _, script := range scripts {
		body := read(t, script)
		for _, bad := range forbidden {
			if strings.Contains(body, bad) {
				t.Errorf("%s must not contain %q", script, bad)
			}
		}
	}
}
