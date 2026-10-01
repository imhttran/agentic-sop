//go:build windows

package dist

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/skill"
)

// These tests exercise install.ps1 on Windows, under the built-in Windows PowerShell
// (5.1, which is what a clean machine has) and PowerShell 7 when it is installed. Every
// run uses temporary directories — a scratch USERPROFILE, a scratch bin directory, and a
// controlled PATH — so no test touches the runner's real profile, PATH, skill roots, or
// installed sop binary. The Unix installer is covered by install_test.go, which is
// excluded on Windows, and the cross-platform contract is checked by contract_test.go.

// powershells returns the PowerShell executables to exercise.
func powershells(t *testing.T) []string {
	t.Helper()
	var found []string
	for _, name := range []string{"powershell", "pwsh"} {
		if path, err := exec.LookPath(name); err == nil {
			found = append(found, path)
		}
	}
	if len(found) == 0 {
		t.Skip("no PowerShell on this machine")
	}
	return found
}

// setEnv sets key in env, replacing any existing entry that differs only in case, because
// Windows environment variable names are case-insensitive and a duplicate PATH entry
// would make the child's PATH ambiguous.
func setEnv(env map[string]string, key, value string) {
	for k := range env {
		if strings.EqualFold(k, key) {
			delete(env, k)
		}
	}
	env[key] = value
}

// envValue looks a variable up without depending on its case, because Windows
// environment variable names are case-insensitive and the inherited environment may
// spell PATH as "Path".
func envValue(env map[string]string, key string) string {
	for k, v := range env {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

func unsetEnv(env map[string]string, key string) {
	for k := range env {
		if strings.EqualFold(k, key) {
			delete(env, k)
		}
	}
}

// installEnv builds the child environment for one run: a scratch USERPROFILE (which is
// where the installer puts the agent skill roots), the real PATH with an optional scratch
// bin directory first, and SOP_BIN_DIR cleared so -BinDir or the documented default
// decides the install directory.
func installEnv(t *testing.T, home, pathFirst string, overrides map[string]string) []string {
	t.Helper()
	env := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	setEnv(env, "USERPROFILE", home)
	setEnv(env, "HOME", home)
	unsetEnv(env, "SOP_BIN_DIR")
	// Pin the Go caches to this machine's real ones: the toolchain derives GOPATH and the
	// build/module caches from the home directory, so the scratch USERPROFILE above would
	// otherwise send every `go build` inside the installer to an empty cache.
	for _, key := range []string{"GOPATH", "GOCACHE", "GOMODCACHE"} {
		if v := goEnv(t, key); v != "" {
			setEnv(env, key, v)
		}
	}
	if pathFirst != "" {
		path := pathFirst
		if current := envValue(env, "PATH"); current != "" {
			path = pathFirst + ";" + current
		}
		setEnv(env, "PATH", path)
	}
	for k, v := range overrides {
		setEnv(env, k, v)
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

// runInstallPS runs install.ps1 with the given environment.
func runInstallPS(t *testing.T, shell string, env []string, args ...string) (string, error) {
	t.Helper()
	script := filepath.Join(repoRoot(t), "install.ps1")
	argv := append([]string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script}, args...)
	cmd := exec.Command(shell, argv...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestWindowsInstallerParses proves the script is valid PowerShell before any behaviour
// is exercised: the language parser reports the errors without running it.
func TestWindowsInstallerParses(t *testing.T) {
	script := filepath.Join(repoRoot(t), "install.ps1")
	for _, shell := range powershells(t) {
		t.Run(filepath.Base(shell), func(t *testing.T) {
			program := strings.Join([]string{
				"$errors = $null",
				"[void][System.Management.Automation.Language.Parser]::ParseFile('" + script + "', [ref]$null, [ref]$errors)",
				"if ($errors) { $errors | ForEach-Object { $_.ToString() }; exit 1 }",
				"exit 0",
			}, "; ")
			out, err := exec.Command(shell, "-NoProfile", "-NonInteractive", "-Command", program).CombinedOutput()
			if err != nil {
				t.Fatalf("install.ps1 does not parse: %v\n%s", err, out)
			}
		})
	}
}

// TestWindowsInstallerHelp proves -Help documents every parameter.
func TestWindowsInstallerHelp(t *testing.T) {
	home := scratchHome(t)
	for _, shell := range powershells(t) {
		out, err := runInstallPS(t, shell, installEnv(t, home, "", nil), "-Help")
		if err != nil {
			t.Fatalf("%s -Help: %v\n%s", shell, err, out)
		}
		for _, want := range []string{"-Skills", "-Plugin", "-All", "-BinDir", "-DryRun", "-UninstallSkills", "-Force", "-Help"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: -Help does not document %s:\n%s", filepath.Base(shell), want, out)
			}
		}
	}
}

// TestWindowsInstallerDryRunChangesNothing proves -DryRun reports the whole plan without
// creating a single file.
func TestWindowsInstallerDryRunChangesNothing(t *testing.T) {
	home := scratchHome(t)
	// The agent directories exist, so -All has an environment to act on rather than
	// skipping both agents.
	for _, dir := range []string{".agents", ".claude"} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(home, "bin")

	for _, shell := range powershells(t) {
		out, err := runInstallPS(t, shell, installEnv(t, home, "", nil), "-BinDir", bin, "-All", "-DryRun")
		if err != nil {
			t.Fatalf("%s -DryRun -All: %v\n%s", shell, err, out)
		}
		if !strings.Contains(out, "would:") {
			t.Errorf("%s: dry run should print what it would do:\n%s", shell, out)
		}
		for _, path := range []string{
			bin,
			filepath.Join(bin, "sop.exe"),
			filepath.Join(home, ".agents", "skills"),
			filepath.Join(home, ".claude", "skills"),
		} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("%s: dry run created %s", filepath.Base(shell), path)
			}
		}
	}
}

// TestWindowsInstallerInstallsTheCLI proves the documented entry point produces a
// runnable sop.exe, twice over, and gives PATH guidance only when it is needed.
func TestWindowsInstallerInstallsTheCLI(t *testing.T) {
	shell := powershells(t)[0]
	home := scratchHome(t)
	bin := filepath.Join(home, "bin")
	env := installEnv(t, home, "", nil)

	out, err := runInstallPS(t, shell, env, "-BinDir", bin)
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	exe := filepath.Join(bin, "sop.exe")
	if _, err := os.Stat(exe); err != nil {
		t.Fatalf("sop.exe was not installed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Add this directory to PATH") {
		t.Errorf("expected PATH guidance for a bin dir that is not on PATH:\n%s", out)
	}

	// It runs.
	version := exec.Command(exe, "version")
	version.Dir = repoRoot(t)
	if vout, verr := version.CombinedOutput(); verr != nil {
		t.Errorf("installed sop.exe is not runnable: %v\n%s", verr, vout)
	}

	// Installing again succeeds and leaves a working binary in place.
	if out2, err2 := runInstallPS(t, shell, env, "-BinDir", bin); err2 != nil {
		t.Fatalf("re-install: %v\n%s", err2, out2)
	}
	if _, err := os.Stat(exe); err != nil {
		t.Fatalf("sop.exe missing after re-install: %v", err)
	}

	// With the bin directory on PATH, the guidance is replaced by a confirmation.
	out3, err3 := runInstallPS(t, shell, installEnv(t, home, bin, nil), "-BinDir", bin)
	if err3 != nil {
		t.Fatalf("install with the bin dir on PATH: %v\n%s", err3, out3)
	}
	if !strings.Contains(out3, "is on PATH") {
		t.Errorf("expected a confirmation when the bin dir is on PATH:\n%s", out3)
	}
}

// TestWindowsInstallerBinDirWithSpaces proves an installation directory containing
// spaces is handled as one path.
func TestWindowsInstallerBinDirWithSpaces(t *testing.T) {
	shell := powershells(t)[0]
	home := scratchHome(t)
	bin := filepath.Join(home, "SOP Test Home", "bin")

	out, err := runInstallPS(t, shell, installEnv(t, home, "", nil), "-BinDir", bin)
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	exe := filepath.Join(bin, "sop.exe")
	if _, err := os.Stat(exe); err != nil {
		t.Fatalf("sop.exe not installed in %q: %v\n%s", bin, err, out)
	}
	version := exec.Command(exe, "version")
	version.Dir = repoRoot(t)
	if vout, verr := version.CombinedOutput(); verr != nil {
		t.Errorf("sop.exe in a path with spaces is not runnable: %v\n%s", verr, vout)
	}
}

// TestWindowsInstallerCopiesSkillsAndPreservesForeign proves the Windows install copies
// the canonical skills, marks them as SOP's, and never overwrites a skill SOP does not
// own.
func TestWindowsInstallerCopiesSkillsAndPreservesForeign(t *testing.T) {
	home := scratchHome(t)
	claudeRoot := filepath.Join(home, ".claude", "skills")

	// A foreign skill at a SOP name, and an unrelated skill.
	foreign := filepath.Join(claudeRoot, "sop-review")
	if err := os.MkdirAll(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	foreignBody := filepath.Join(foreign, "SKILL.md")
	if err := os.WriteFile(foreignBody, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(claudeRoot, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	otherBody := filepath.Join(other, "SKILL.md")
	if err := os.WriteFile(otherBody, []byte("theirs\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runInstallPS(t, powershells(t)[0], installEnv(t, home, "", nil), "-BinDir", filepath.Join(home, "bin"), "-Skills", "claude")
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if !strings.Contains(out, "an existing non-SOP skill uses this name") {
		t.Errorf("expected the foreign skill to be reported as skipped:\n%s", out)
	}

	// The foreign content is untouched.
	for path, want := range map[string]string{foreignBody: "mine\n", otherBody: "theirs\n"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Errorf("unrelated file %s was changed or removed: %q (%v)", path, got, err)
		}
	}

	// Every other command is installed, has its SKILL.md, and carries the marker.
	for _, c := range skill.Commands {
		if c.Name == "sop-review" {
			continue
		}
		dir := filepath.Join(claudeRoot, c.Name)
		if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
			t.Errorf("%s: SKILL.md not installed: %v", c.Name, err)
		}
		marker, err := os.ReadFile(filepath.Join(dir, ".sop-managed"))
		if err != nil {
			t.Errorf("%s: no ownership marker: %v", c.Name, err)
			continue
		}
		if !strings.Contains(string(marker), "marker=agentic-sop") {
			t.Errorf("%s: the marker does not identify agentic-sop:\n%s", c.Name, marker)
		}
		if !strings.Contains(string(marker), "source=skills/"+c.Name) {
			t.Errorf("%s: the marker does not record its source:\n%s", c.Name, marker)
		}
	}

	// The installed copy is a copy, not a link, and matches the canonical file.
	canonical, err := os.ReadFile(filepath.Join(repoRoot(t), "skills", "sop-plan", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(filepath.Join(claudeRoot, "sop-plan", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical, installed) {
		t.Error("the installed skill is not a copy of the canonical skill")
	}
}

// TestWindowsInstallerRefreshesSOPOwnedSkills proves the install is idempotent and that a
// SOP-owned copy is restored from canonical, which is what update means on Windows.
func TestWindowsInstallerRefreshesSOPOwnedSkills(t *testing.T) {
	shell := powershells(t)[0]
	home := scratchHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(home, "bin")
	env := installEnv(t, home, "", nil)
	installed := filepath.Join(home, ".claude", "skills", "sop-plan", "SKILL.md")

	if out, err := runInstallPS(t, shell, env, "-BinDir", bin, "-Skills", "claude"); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}

	// A second install is a no-op: the copies are already current.
	out, err := runInstallPS(t, shell, env, "-BinDir", bin, "-Skills", "claude")
	if err != nil {
		t.Fatalf("re-install: %v\n%s", err, out)
	}
	if !strings.Contains(out, "ok: ") {
		t.Errorf("a second install should report the copies as current:\n%s", out)
	}

	// A changed installed copy is restored from canonical.
	if err := os.WriteFile(installed, []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := runInstallPS(t, shell, env, "-BinDir", bin, "-Skills", "claude"); err != nil {
		t.Fatalf("re-install after a change: %v\n%s", err, out)
	}
	canonical, err := os.ReadFile(filepath.Join(repoRoot(t), "skills", "sop-plan", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(installed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical, got) {
		t.Error("the changed installed copy was not restored from canonical")
	}

	// -Force refreshes even a current copy.
	out, err = runInstallPS(t, shell, env, "-BinDir", bin, "-Skills", "claude", "-Force")
	if err != nil {
		t.Fatalf("-Force install: %v\n%s", err, out)
	}
	if !strings.Contains(out, "updated: ") {
		t.Errorf("-Force should refresh the copies:\n%s", out)
	}
}

// TestWindowsInstallerUninstallRemovesOnlySOPOwned proves uninstall removes what SOP
// installed and nothing else, even at a SOP name.
func TestWindowsInstallerUninstallRemovesOnlySOPOwned(t *testing.T) {
	shell := powershells(t)[0]
	home := scratchHome(t)
	claudeRoot := filepath.Join(home, ".claude", "skills")

	// A foreign skill at a SOP name, which uninstall must leave alone.
	foreign := filepath.Join(claudeRoot, "sop-test")
	if err := os.MkdirAll(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	foreignBody := filepath.Join(foreign, "SKILL.md")
	if err := os.WriteFile(foreignBody, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(home, "bin")
	if out, err := runInstallPS(t, shell, installEnv(t, home, "", nil), "-BinDir", bin, "-Skills", "claude"); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	installed := filepath.Join(claudeRoot, "sop-review")
	if _, err := os.Stat(filepath.Join(installed, ".sop-managed")); err != nil {
		t.Fatalf("the SOP skill was not installed: %v", err)
	}

	out, err := runInstallPS(t, shell, installEnv(t, home, "", nil), "-UninstallSkills", "claude")
	if err != nil {
		t.Fatalf("uninstall: %v\n%s", err, out)
	}
	if _, err := os.Stat(installed); !os.IsNotExist(err) {
		t.Errorf("uninstall left the SOP skill in place: %v", err)
	}
	if got, err := os.ReadFile(foreignBody); err != nil || string(got) != "mine\n" {
		t.Errorf("uninstall touched a foreign skill: %q (%v)", got, err)
	}
	if !strings.Contains(out, "is not SOP-managed") {
		t.Errorf("uninstall should report the foreign skill it left:\n%s", out)
	}
}

// TestWindowsInstallerRequiresGo proves a missing toolchain fails with an actionable
// message instead of a confusing error.
func TestWindowsInstallerRequiresGo(t *testing.T) {
	home := scratchHome(t)
	env := installEnv(t, home, "", map[string]string{"PATH": filepath.Join(os.Getenv("SystemRoot"), "System32")})

	out, err := runInstallPS(t, powershells(t)[0], env, "-BinDir", filepath.Join(home, "bin"))
	if err == nil {
		t.Fatalf("expected a failure without the Go toolchain:\n%s", out)
	}
	if !strings.Contains(out, "Go toolchain") {
		t.Errorf("expected an actionable missing-Go message:\n%s", out)
	}
}

// TestWindowsInstallerPluginStepIsSafe proves the plugin step prints the official steps
// and never writes into Claude's configuration.
func TestWindowsInstallerPluginStepIsSafe(t *testing.T) {
	shell := powershells(t)[0]
	home := scratchHome(t)

	out, err := runInstallPS(t, shell, installEnv(t, home, "", nil), "-BinDir", filepath.Join(home, "bin"), "-Plugin", "claude")
	if err != nil {
		t.Fatalf("install -Plugin claude: %v\n%s", err, out)
	}
	for _, want := range []string{"--plugin-dir", "claude plugin marketplace add", "claude plugin install sop@agentic-sop", "/plugin marketplace add", "/plugin install sop@agentic-sop"} {
		if !strings.Contains(out, want) {
			t.Errorf("the plugin step does not document %q:\n%s", want, out)
		}
	}
	for _, path := range []string{filepath.Join(home, ".claude", "settings.json"), filepath.Join(home, ".claude", "plugins")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("the installer must not write %s", path)
		}
	}
}

// TestWindowsInstallerWithSeveralGoInstallationsOnPath proves a machine with more than
// one Go on PATH installs normally. A shell runs the first one, so the installer must
// too: CI's runner has two, and an earlier version passed the whole Get-Command result
// to the shell, which joined both paths into one unusable command.
func TestWindowsInstallerWithSeveralGoInstallationsOnPath(t *testing.T) {
	shell := powershells(t)[0]
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go on PATH")
	}
	home := scratchHome(t)

	// A shim earlier on PATH, so Get-Command sees two installations.
	shims := filepath.Join(home, "shims")
	if err := os.MkdirAll(shims, 0o755); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(shims, "go.cmd")
	if err := os.WriteFile(shim, []byte("@echo off\r\n\""+realGo+"\" %*\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(home, "bin")
	out, err := runInstallPS(t, shell, installEnv(t, home, shims, nil), "-BinDir", bin)
	if err != nil {
		t.Fatalf("install with two Go installations on PATH: %v\n%s", err, out)
	}
	exe := filepath.Join(bin, "sop.exe")
	if _, err := os.Stat(exe); err != nil {
		t.Fatalf("sop.exe was not installed: %v\n%s", err, out)
	}
	version := exec.Command(exe, "version")
	version.Dir = repoRoot(t)
	if vout, verr := version.CombinedOutput(); verr != nil {
		t.Errorf("installed sop.exe is not runnable: %v\n%s", verr, vout)
	}
}
