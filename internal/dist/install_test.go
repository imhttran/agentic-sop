// Package dist validates SOP's distribution layer as an artifact: the root installer
// (install.sh), the per-agent skill installation it delegates to, and the Claude Code
// plugin package under integrations/claude/.
//
// Every test runs against temporary directories — a scratch HOME, a scratch bin
// directory, and (for the plugin) the packaged tree itself. Nothing here touches the
// developer's real ~/.agents, ~/.claude, or installed sop binary, and nothing edits a
// shell startup file.
package dist

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/skill"
)

// repoRoot is the checkout under test (internal/dist -> ../..).
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

// scratchHome returns a temporary HOME for an installer run. It is deliberately not
// t.TempDir: a Go build under a fresh HOME can leave a read-only module cache behind,
// which makes the framework's own TempDir cleanup fail the test. A tolerant cleanup
// keeps that out of the test result, and installerEnv pins the Go caches elsewhere
// anyway.
func scratchHome(t *testing.T) string {
	t.Helper()
	home, err := os.MkdirTemp("", "sop-dist-home-")
	if err != nil {
		t.Fatalf("scratch home: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	return home
}

// installerEnv builds the environment for a scratch run: a temporary HOME, a PATH that
// puts the scratch bin directory first, and the developer's real Go caches so a build
// is fast and no module cache lands in the scratch HOME. SOP_BIN_DIR and GOBIN are
// cleared so the default bin directory can only be reached through --bin-dir.
func installerEnv(t *testing.T, home, binDir string, overrides map[string]string) []string {
	t.Helper()
	env := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	delete(env, "SOP_BIN_DIR")
	delete(env, "GOBIN")
	env["HOME"] = home
	for _, key := range []string{"GOMODCACHE", "GOCACHE"} {
		if v := goEnv(t, key); v != "" {
			env[key] = v
		}
	}
	if binDir != "" {
		env["PATH"] = binDir + string(os.PathListSeparator) + env["PATH"]
	}
	for k, v := range overrides {
		env[k] = v
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

// goEnv reads one value from the developer's real Go environment.
func goEnv(t *testing.T, key string) string {
	t.Helper()
	out, err := exec.Command("go", "env", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// runInstaller runs the root installer from the checkout under test.
func runInstaller(t *testing.T, env []string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{filepath.Join(repoRoot(t), "install.sh")}, args...)...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// sopLinks are the commands the skill installer must link for an agent.
func sopLinks() []string {
	var names []string
	for _, c := range skill.Commands {
		names = append(names, c.Name)
	}
	return names
}

func TestInstallerHelp(t *testing.T) {
	out, err := runInstaller(t, installerEnv(t, scratchHome(t), "", nil), "--help")
	if err != nil {
		t.Fatalf("--help: %v\n%s", err, out)
	}
	for _, want := range []string{"--skills", "--plugin", "--bin-dir", "--dry-run", "./install.sh"} {
		if !strings.Contains(out, want) {
			t.Errorf("--help does not document %q:\n%s", want, out)
		}
	}
}

func TestInstallerRejectsBadArguments(t *testing.T) {
	for _, args := range [][]string{
		{"--bogus"},
		{"--skills", "codex"},
		{"--plugin", "zed"},
		{"--bin-dir"},
	} {
		out, err := runInstaller(t, installerEnv(t, scratchHome(t), "", nil), args...)
		if err == nil {
			t.Errorf("%v: expected a non-zero exit\n%s", args, out)
			continue
		}
		if !strings.Contains(out, "install:") {
			t.Errorf("%v: expected an actionable message, got:\n%s", args, out)
		}
	}
}

// TestInstallerDryRunChangesNothing proves --dry-run reports the whole plan, including
// the plugin steps, without creating a single file.
func TestInstallerDryRunChangesNothing(t *testing.T) {
	home := scratchHome(t)
	bin := filepath.Join(home, "bin")
	// The agent directories exist, so `all` has an environment to detect and the dry run
	// is about the plan rather than about a missing agent.
	for _, dir := range []string{".agents", ".claude"} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	out, err := runInstaller(t, installerEnv(t, home, "", nil),
		"--bin-dir", bin, "--skills", "all", "--plugin", "claude", "--dry-run")
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "would:") {
		t.Errorf("dry run should print what it would do:\n%s", out)
	}
	if !strings.Contains(out, "plugin marketplace add") {
		t.Errorf("dry run should print the official plugin steps:\n%s", out)
	}
	for _, path := range []string{
		bin,
		filepath.Join(home, "bin", "sop"),
		filepath.Join(home, ".agents", "skills"),
		filepath.Join(home, ".claude", "skills"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("dry run created %s", path)
		}
	}
}

// TestInstallerInstallsTheCLI proves the documented entry point really produces a
// runnable sop binary, twice over, and gives PATH guidance that appears only when it is
// needed.
func TestInstallerInstallsTheCLI(t *testing.T) {
	home := scratchHome(t)
	bin := filepath.Join(home, "bin")
	env := installerEnv(t, home, "", nil)

	out, err := runInstaller(t, env, "--bin-dir", bin)
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	installed := filepath.Join(bin, "sop")
	info, err := os.Stat(installed)
	if err != nil {
		t.Fatalf("sop not installed: %v\n%s", err, out)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("installed sop is not executable: %v", info.Mode())
	}
	if !strings.Contains(out, "Add this directory to PATH") {
		t.Errorf("expected PATH guidance for a bin dir that is not on PATH:\n%s", out)
	}

	// It runs.
	cmd := exec.Command(installed, "version")
	cmd.Dir = repoRoot(t)
	if vout, verr := cmd.CombinedOutput(); verr != nil {
		t.Errorf("installed sop is not runnable: %v\n%s", verr, vout)
	}

	// Installing again succeeds and leaves a working binary in place.
	out2, err := runInstaller(t, env, "--bin-dir", bin)
	if err != nil {
		t.Fatalf("re-install: %v\n%s", err, out2)
	}
	if _, err := os.Stat(installed); err != nil {
		t.Fatalf("sop missing after re-install: %v", err)
	}

	// With the bin dir on PATH the guidance is replaced by a confirmation.
	onPath := installerEnv(t, home, bin, nil)
	out3, err := runInstaller(t, onPath, "--bin-dir", bin)
	if err != nil {
		t.Fatalf("install on PATH: %v\n%s", err, out3)
	}
	if !strings.Contains(out3, "is on PATH") {
		t.Errorf("expected a confirmation when the bin dir is on PATH:\n%s", out3)
	}
	if strings.Contains(out3, "Add this directory to PATH") {
		t.Errorf("no PATH guidance was needed:\n%s", out3)
	}
}

// TestInstallerBinDirWithSpaces proves an installation directory containing spaces is
// handled as one path, not split into arguments.
func TestInstallerBinDirWithSpaces(t *testing.T) {
	home := scratchHome(t)
	bin := filepath.Join(home, "bin dir")

	out, err := runInstaller(t, installerEnv(t, home, "", nil), "--bin-dir", bin)
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	installed := filepath.Join(bin, "sop")
	info, err := os.Stat(installed)
	if err != nil {
		t.Fatalf("sop not installed in %q: %v\n%s", bin, err, out)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("installed sop is not executable")
	}
}

// TestInstallerDelegatesSkillInstallation proves --skills is delegated per agent and
// lands in that agent's own skills root, without installing into the others.
func TestInstallerDelegatesSkillInstallation(t *testing.T) {
	cases := []struct {
		skills string
		roots  []string
	}{
		{"zed", []string{".agents/skills"}},
		{"claude", []string{".claude/skills"}},
		{"all", []string{".agents/skills", ".claude/skills"}},
	}
	for _, tc := range cases {
		t.Run(tc.skills, func(t *testing.T) {
			home := scratchHome(t)
			// Both agent directories exist, so `all` has an environment to detect.
			for _, dir := range []string{".agents", ".claude"} {
				if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			bin := filepath.Join(home, "bin")
			out, err := runInstaller(t, installerEnv(t, home, "", nil), "--bin-dir", bin, "--skills", tc.skills)
			if err != nil {
				t.Fatalf("install --skills %s: %v\n%s", tc.skills, err, out)
			}
			for _, root := range tc.roots {
				for _, name := range sopLinks() {
					path := filepath.Join(home, root, name)
					fi, err := os.Lstat(path)
					if err != nil {
						t.Errorf("%s: missing %s: %v", tc.skills, path, err)
						continue
					}
					if fi.Mode()&os.ModeSymlink == 0 {
						t.Errorf("%s: %s is not a symlink", tc.skills, path)
					}
				}
			}
			for _, other := range []string{".agents/skills", ".claude/skills"} {
				if containsString(tc.roots, other) {
					continue
				}
				if _, err := os.Stat(filepath.Join(home, other)); err == nil && !isDirEmpty(t, filepath.Join(home, other)) {
					t.Errorf("--skills %s should not install into %s", tc.skills, other)
				}
			}
		})
	}
}

// TestInstallerSkillsAllWithoutAnAgentFailsLoudly proves a skills step that cannot find
// an agent reports it and still leaves the CLI installed: one step failing never hides
// what the others achieved.
func TestInstallerSkillsAllWithoutAnAgentFailsLoudly(t *testing.T) {
	home := scratchHome(t) // neither ~/.agents nor ~/.claude exists
	bin := filepath.Join(home, "bin")

	out, err := runInstaller(t, installerEnv(t, home, "", nil), "--bin-dir", bin, "--skills", "all")
	if err == nil {
		t.Fatalf("expected a non-zero exit when no agent environment exists:\n%s", out)
	}
	if !strings.Contains(out, "no supported agent environment") {
		t.Errorf("expected the skill installer's actionable message:\n%s", out)
	}
	if !strings.Contains(out, "Finished with errors") {
		t.Errorf("expected the installer to report a failed step:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(bin, "sop")); err != nil {
		t.Errorf("the CLI should still have been installed: %v", err)
	}
}

// TestInstallerPreservesUnrelatedSkills proves installation never overwrites or removes
// a skill SOP does not own.
func TestInstallerPreservesUnrelatedSkills(t *testing.T) {
	home := scratchHome(t)
	foreign := filepath.Join(home, ".claude", "skills", "other")
	if err := os.MkdirAll(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	body := filepath.Join(foreign, "SKILL.md")
	if err := os.WriteFile(body, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A real (non-symlink) entry at a SOP name is foreign too.
	atSOPName := filepath.Join(home, ".agents", "skills", "sop")
	if err := os.MkdirAll(atSOPName, 0o755); err != nil {
		t.Fatal(err)
	}
	atSOPBody := filepath.Join(atSOPName, "SKILL.md")
	if err := os.WriteFile(atSOPBody, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(home, "bin")
	if out, err := runInstaller(t, installerEnv(t, home, "", nil), "--bin-dir", bin, "--skills", "all"); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}

	for _, path := range []string{body, atSOPBody} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != "mine\n" {
			t.Errorf("unrelated file %s was changed or removed: %q (%v)", path, got, err)
		}
	}
	// The remaining SOP commands are still installed where nothing foreign blocked them.
	if _, err := os.Lstat(filepath.Join(home, ".claude", "skills", "sop-review")); err != nil {
		t.Errorf("sop-review was not installed: %v", err)
	}
}

// TestInstallerRequiresGo proves a missing toolchain fails with an actionable message
// instead of a confusing shell error.
func TestInstallerRequiresGo(t *testing.T) {
	// Precondition: no go in the minimal PATH this test uses.
	for _, dir := range []string{"/usr/bin", "/bin"} {
		if _, err := os.Stat(filepath.Join(dir, "go")); err == nil {
			t.Skipf("go is installed in %s on this host; the minimal PATH cannot exclude it", dir)
		}
	}
	home := scratchHome(t)
	env := installerEnv(t, home, "", map[string]string{"PATH": "/usr/bin:/bin"})

	out, err := runInstaller(t, env, "--bin-dir", filepath.Join(home, "bin"))
	if err == nil {
		t.Fatalf("expected a failure without the Go toolchain:\n%s", out)
	}
	if !strings.Contains(out, "Go toolchain") {
		t.Errorf("expected an actionable missing-Go message:\n%s", out)
	}
}

// TestInstallerPluginStepIsSafeInEveryMode proves the plugin step never writes into
// Claude Code's configuration: it prepares (or reports) the package and prints the
// official steps, and it works the same way with and without the claude CLI present.
func TestInstallerPluginStepIsSafeInEveryMode(t *testing.T) {
	home := scratchHome(t)
	bin := filepath.Join(home, "bin")

	out, err := runInstaller(t, installerEnv(t, home, "", nil), "--bin-dir", bin, "--plugin", "claude")
	if err != nil {
		t.Fatalf("install --plugin claude: %v\n%s", err, out)
	}
	for _, want := range []string{
		"--plugin-dir",
		"claude plugin marketplace add",
		"claude plugin install sop@agentic-sop",
		"/plugin marketplace add",
		"/plugin install sop@agentic-sop",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plugin step does not document %q:\n%s", want, out)
		}
	}
	// Nothing was written into a Claude configuration directory.
	for _, path := range []string{filepath.Join(home, ".claude", "settings.json"), filepath.Join(home, ".claude", "plugins")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("the installer must not write %s", path)
		}
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func isDirEmpty(t *testing.T, dir string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true
	}
	return len(entries) == 0
}
