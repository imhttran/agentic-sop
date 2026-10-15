package testfast

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// scriptPath locates the wrapper under test relative to this package directory,
// skipping on platforms that cannot run a POSIX shell script.
func scriptPath(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("scripts/checks/test-fast.sh requires a POSIX shell")
	}
	p := filepath.Join("..", "..", "scripts", "checks", "test-fast.sh")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("test-fast.sh not found: %v", err)
	}
	return p
}

// shell returns an available POSIX shell, skipping if none exists.
func shell(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"bash", "sh"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	t.Skip("no POSIX shell available")
	return ""
}

// cleanEnv is the ambient environment with every test seam removed, so the
// caller controls the wrapper's inputs exactly.
func cleanEnv() []string {
	drop := []string{"SOP_TEST_PLATFORM=", "SOP_TEST_RAM_DIR=", "SOP_TEST_GO_BIN=",
		"SOP_TEST_TMPDIR=", "SOP_TEST_NO_RAM=", "SOP_TEST_MIN_FREE_MB=",
		"SOP_TEST_PARALLEL=", "SOP_TEST_TIMEOUT=", "MOCK_"}
	var out []string
	for _, kv := range os.Environ() {
		skip := false
		for _, p := range drop {
			if strings.HasPrefix(kv, p) {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, kv)
		}
	}
	return out
}

type result struct {
	stdout string
	stderr string
	code   int
}

// runScript runs the wrapper with a clean environment plus extraEnv and args.
func runScript(t *testing.T, extraEnv []string, args ...string) result {
	t.Helper()
	cmd := exec.Command(shell(t), append([]string{scriptPath(t)}, args...)...)
	cmd.Env = append(cleanEnv(), extraEnv...)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			t.Fatalf("running test-fast.sh: %v", err)
		}
	}
	return result{out.String(), errb.String(), code}
}

// plan parses the key=value lines printed by --print-plan.
func plan(t *testing.T, out string) map[string]string {
	t.Helper()
	m := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			m[k] = v
		}
	}
	return m
}

// writeMock writes an executable mock `go` script and returns its path.
func writeMock(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "mock-go")
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestPrintPlanLinuxUsesRAM exercises the Linux branch: with a usable RAM-backed
// directory and enough free space, the wrapper selects it.
func TestPrintPlanLinuxUsesRAM(t *testing.T) {
	ramDir := t.TempDir()
	sysDir := t.TempDir()

	res := runScript(t, []string{
		"SOP_TEST_PLATFORM=linux",
		"SOP_TEST_RAM_DIR=" + ramDir,
		"SOP_TEST_MIN_FREE_MB=1",
		"TMPDIR=" + sysDir,
	}, "--print-plan")
	if res.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", res.code, res.stderr)
	}
	p := plan(t, res.stdout)
	if p["platform"] != "linux" {
		t.Errorf("platform = %q, want linux", p["platform"])
	}
	if p["ram"] != "yes" || p["source"] != "ram" {
		t.Errorf("ram = %q source = %q, want yes/ram", p["ram"], p["source"])
	}
	if p["base"] != ramDir {
		t.Errorf("base = %q, want the RAM dir %q", p["base"], ramDir)
	}
}

// TestPrintPlanDarwinUsesSystemTemp exercises the macOS branch: macOS has no
// /dev/shm, so the ordinary system temp directory is used even if a RAM-backed
// path would otherwise be reachable.
func TestPrintPlanDarwinUsesSystemTemp(t *testing.T) {
	ramDir := t.TempDir()
	sysDir := t.TempDir()

	res := runScript(t, []string{
		"SOP_TEST_PLATFORM=darwin",
		"SOP_TEST_RAM_DIR=" + ramDir,
		"SOP_TEST_MIN_FREE_MB=1",
		"TMPDIR=" + sysDir,
	}, "--print-plan")
	if res.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", res.code, res.stderr)
	}
	p := plan(t, res.stdout)
	if p["platform"] != "darwin" {
		t.Errorf("platform = %q, want darwin", p["platform"])
	}
	if p["ram"] != "no" || p["source"] != "system" {
		t.Errorf("ram = %q source = %q, want no/system", p["ram"], p["source"])
	}
	if p["base"] != sysDir {
		t.Errorf("base = %q, want the system temp dir %q", p["base"], sysDir)
	}
}

// TestPrintPlanOtherPlatformUsesSystemTemp covers the remaining platforms.
func TestPrintPlanOtherPlatformUsesSystemTemp(t *testing.T) {
	ramDir := t.TempDir()
	sysDir := t.TempDir()

	res := runScript(t, []string{
		"SOP_TEST_PLATFORM=other",
		"SOP_TEST_RAM_DIR=" + ramDir,
		"SOP_TEST_MIN_FREE_MB=1",
		"TMPDIR=" + sysDir,
	}, "--print-plan")
	if res.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", res.code, res.stderr)
	}
	p := plan(t, res.stdout)
	if p["ram"] != "no" || p["source"] != "system" || p["base"] != sysDir {
		t.Errorf("got ram=%q source=%q base=%q, want no/system/%q", p["ram"], p["source"], p["base"], sysDir)
	}
}

// TestPrintPlanFallsBackWhenRAMUnavailable proves a missing RAM directory falls
// back safely to the system temp dir and says so on stderr.
func TestPrintPlanFallsBackWhenRAMUnavailable(t *testing.T) {
	sysDir := t.TempDir()
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	res := runScript(t, []string{
		"SOP_TEST_PLATFORM=linux",
		"SOP_TEST_RAM_DIR=" + missing,
		"TMPDIR=" + sysDir,
	}, "--print-plan")
	if res.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", res.code, res.stderr)
	}
	p := plan(t, res.stdout)
	if p["ram"] != "no" || p["base"] != sysDir {
		t.Errorf("got ram=%q base=%q, want no/%q", p["ram"], p["base"], sysDir)
	}
	if !strings.Contains(res.stderr, "unavailable") {
		t.Errorf("expected a fallback notice on stderr, got:\n%s", res.stderr)
	}
}

// TestPrintPlanFallsBackWhenRAMTooSmall proves the capacity check falls back
// rather than risk filling the RAM filesystem.
func TestPrintPlanFallsBackWhenRAMTooSmall(t *testing.T) {
	ramDir := t.TempDir()
	sysDir := t.TempDir()

	res := runScript(t, []string{
		"SOP_TEST_PLATFORM=linux",
		"SOP_TEST_RAM_DIR=" + ramDir,
		"SOP_TEST_MIN_FREE_MB=100000000", // ~95 TiB: never satisfied
		"TMPDIR=" + sysDir,
	}, "--print-plan")
	if res.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", res.code, res.stderr)
	}
	p := plan(t, res.stdout)
	if p["ram"] != "no" || p["base"] != sysDir {
		t.Errorf("got ram=%q base=%q, want no/%q", p["ram"], p["base"], sysDir)
	}
	if !strings.Contains(res.stderr, "below the") {
		t.Errorf("expected a capacity notice on stderr, got:\n%s", res.stderr)
	}
}

// TestExplicitTmpdirWins proves an operator-provided base overrides RAM
// selection on every platform.
func TestExplicitTmpdirWins(t *testing.T) {
	ramDir := t.TempDir()
	explicit := t.TempDir()

	res := runScript(t, []string{
		"SOP_TEST_PLATFORM=linux",
		"SOP_TEST_RAM_DIR=" + ramDir,
		"SOP_TEST_MIN_FREE_MB=1",
		"SOP_TEST_TMPDIR=" + explicit,
	}, "--print-plan")
	if res.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", res.code, res.stderr)
	}
	p := plan(t, res.stdout)
	if p["source"] != "override" || p["base"] != explicit {
		t.Errorf("got source=%q base=%q, want override/%q", p["source"], p["base"], explicit)
	}
}

// TestExplicitTmpdirMustExist proves a bad override fails loudly instead of
// silently using /tmp.
func TestExplicitTmpdirMustExist(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	res := runScript(t, []string{"SOP_TEST_TMPDIR=" + missing}, "--print-plan")
	if res.code == 0 {
		t.Fatalf("expected a non-zero exit for a missing --tmpdir, stderr:\n%s", res.stderr)
	}
	if !strings.Contains(res.stderr, "not an existing directory") {
		t.Errorf("expected a clear error, got:\n%s", res.stderr)
	}
}

// TestRunCleansOnlyItsOwnDirectory proves one invocation creates a unique dir
// under the base, points the child's TMPDIR at it, removes exactly that dir, and
// leaves the shared base intact.
func TestRunCleansOnlyItsOwnDirectory(t *testing.T) {
	base := t.TempDir()
	record := filepath.Join(t.TempDir(), "record")
	mock := writeMock(t, "#!/bin/sh\nprintf '%s\\n' \"$TMPDIR\" > \"$MOCK_RECORD\"\nexit 0\n")

	res := runScript(t, []string{
		"SOP_TEST_PLATFORM=darwin", // force the deterministic system-temp path
		"SOP_TEST_TMPDIR=" + base,
		"SOP_TEST_GO_BIN=" + mock,
		"MOCK_RECORD=" + record,
	})
	if res.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", res.code, res.stderr)
	}

	raw, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("mock did not record TMPDIR: %v", err)
	}
	used := strings.TrimSpace(string(raw))
	if used == "" || used == base {
		t.Fatalf("child TMPDIR = %q, want a unique dir under %q", used, base)
	}
	if filepath.Dir(used) != base {
		t.Fatalf("child TMPDIR %q is not directly under base %q", used, base)
	}
	if _, err := os.Stat(used); !os.IsNotExist(err) {
		t.Fatalf("per-invocation dir %q was not cleaned up (stat err: %v)", used, err)
	}
	if _, err := os.Stat(base); err != nil {
		t.Fatalf("shared base %q must survive: %v", base, err)
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("base %q should be empty after cleanup, found %d entries", base, len(entries))
	}
}

// TestFailureExitStatusPropagates proves a failing suite is reported as failing.
func TestFailureExitStatusPropagates(t *testing.T) {
	base := t.TempDir()
	mock := writeMock(t, "#!/bin/sh\nexit 3\n")
	res := runScript(t, []string{
		"SOP_TEST_TMPDIR=" + base,
		"SOP_TEST_GO_BIN=" + mock,
	})
	if res.code != 3 {
		t.Fatalf("exit = %d, want 3 (failures must not be masked)", res.code)
	}
	// The per-invocation dir is still cleaned up on failure.
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("base %q should be empty after a failed run, found %d entries", base, len(entries))
	}
}

// TestConcurrentInvocationsAreIsolated proves two simultaneous runs get distinct
// directories, each keeps its own file while both are live, and neither deletes
// the other's directory.
func TestConcurrentInvocationsAreIsolated(t *testing.T) {
	base := t.TempDir()
	records := []string{
		filepath.Join(t.TempDir(), "record-a"),
		filepath.Join(t.TempDir(), "record-b"),
	}
	barrier := filepath.Join(t.TempDir(), "go")

	mock := writeMock(t, `#!/bin/sh
printf '%s\n' "$TMPDIR" > "$MOCK_RECORD"
: > "$TMPDIR/owned-by-$$"
while [ ! -e "$MOCK_BARRIER" ]; do sleep 0.05; done
# Our own file must still be present once the other run has started too.
[ -e "$TMPDIR/owned-by-$$" ] || exit 9
exit 0
`)

	start := func(record string) *exec.Cmd {
		cmd := exec.Command(shell(t), scriptPath(t))
		cmd.Env = append(cleanEnv(),
			"SOP_TEST_TMPDIR="+base,
			"SOP_TEST_GO_BIN="+mock,
			"MOCK_RECORD="+record,
			"MOCK_BARRIER="+barrier,
		)
		if err := cmd.Start(); err != nil {
			t.Fatalf("start: %v", err)
		}
		return cmd
	}
	cmds := []*exec.Cmd{start(records[0]), start(records[1])}

	// Both runs must reach the barrier: their records written and their session
	// directories present, before either is released.
	var dirs []string
	deadline := time.Now().Add(20 * time.Second)
	for {
		dirs = dirs[:0]
		ok := true
		for _, r := range records {
			b, err := os.ReadFile(r)
			if err != nil {
				ok = false
				break
			}
			d := strings.TrimSpace(string(b))
			if _, err := os.Stat(d); err != nil {
				ok = false
				break
			}
			dirs = append(dirs, d)
		}
		if ok && len(dirs) == 2 {
			break
		}
		if time.Now().After(deadline) {
			for _, c := range cmds {
				_ = c.Process.Kill()
			}
			t.Fatalf("runs did not reach the barrier in time; dirs so far: %v", dirs)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if dirs[0] == dirs[1] {
		t.Fatalf("concurrent runs shared a directory: %q", dirs[0])
	}
	for _, d := range dirs {
		if _, err := os.Stat(d); err != nil {
			t.Fatalf("session dir %q should still exist while both runs are live: %v", d, err)
		}
	}

	// Release both and wait.
	if err := os.WriteFile(barrier, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for i, c := range cmds {
		if err := c.Wait(); err != nil {
			t.Fatalf("run %d failed: %v", i, err)
		}
	}

	// Each run removed its own dir; the shared base survives and is empty.
	for _, d := range dirs {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Errorf("session dir %q was not cleaned up", d)
		}
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("base %q should be empty after both runs, found %d entries", base, len(entries))
	}
}

// TestMakefileTargets proves `make test` still runs the plain, disk-backed suite
// and that `make test-fast` delegates to the wrapper.
func TestMakefileTargets(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	mk := string(data)

	// `test:` must still be exactly the ordinary suite.
	if m := regexp.MustCompile(`(?m)^test:\n\tgo test \./\.\.\.\n`).FindString(mk); m == "" {
		t.Errorf("Makefile `test:` target must remain `go test ./...`; got:\n%s", mk)
	}
	if !strings.Contains(mk, "scripts/checks/test-fast.sh") {
		t.Errorf("Makefile must define a test-fast target delegating to scripts/checks/test-fast.sh")
	}
}
