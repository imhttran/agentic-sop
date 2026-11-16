package toolharness

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// This file pins the git-execution boundary (git_integrity.go). Every test pairs a
// positive control — the canary really does run when git is invoked plainly — with
// the assertion that the harness's own git path does not run it. A test that only
// asserted "no marker" would pass on a broken canary.

// probeMarker returns a marker path and an executable script that appends to it.
func probeMarker(t *testing.T, dir, body string) (marker, script string) {
	t.Helper()
	marker = filepath.Join(dir, "probe-marker")
	script = filepath.Join(dir, "probe.sh")
	content := "#!/bin/sh\nprintf 'RAN\\n' >> " + marker + "\n" + body + "\nexit 0\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return marker, script
}

func ran(t *testing.T, marker string) bool {
	t.Helper()
	_, err := os.Stat(marker)
	return err == nil
}

// clearMarker removes the marker so the next observation is independent.
func clearMarker(t *testing.T, marker string) {
	t.Helper()
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

// requirePOSIXShell skips a test on a platform where a `#!/bin/sh` probe cannot run.
func requirePOSIXShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("a shebang probe cannot execute on Windows")
	}
}

// isolateGitConfig points git at empty global and system configuration for the
// duration of a test. Without it these tests depend on the host: a machine with
// git-lfs installed configures `filter.lfs.*` globally, so "no driver configured"
// is false there. The harness's own git children inherit this environment, so the
// probes under test see the same isolated configuration.
//
// A host can also carry a *dangling* filter configuration — `filter.lfs.*` present
// with no git-lfs binary — which the W7 policy correctly treats as unverifiable and
// therefore untrusted. Tests that assert trustworthy behaviour on an ordinary
// repository must isolate the configuration rather than encode an assumption about
// which of those two host shapes they run on.
func isolateGitConfig(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
}

func commitRepoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	writeRepoFile(t, dir, name, content)
	runGit(t, dir, "add", name)
	runGit(t, dir, "commit", "-q", "-m", "init")
}

func runGitEnv(t *testing.T, dir string, extra []string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extra...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, out)
	}
}

// --- unit: argv and environment construction ---

func TestHardenedGitArgsNeutralisesHelperMechanisms(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "status disables fsmonitor and adds no diff flags",
			in:   []string{"status", "--short", "--branch"},
			want: []string{"-c", "core.fsmonitor=false", "status", "--short", "--branch"},
		},
		{
			name: "diff disables fsmonitor, external diff and textconv",
			in:   []string{"diff"},
			want: []string{"-c", "core.fsmonitor=false", "diff", "--no-ext-diff", "--no-textconv"},
		},
		{
			name: "diff keeps its own arguments after the hardening flags",
			in:   []string{"diff", "HEAD", "--", "."},
			want: []string{"-c", "core.fsmonitor=false", "diff", "--no-ext-diff", "--no-textconv", "HEAD", "--", "."},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hardenedGitArgs(tc.in); strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("hardenedGitArgs(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestGitChildEnvDropsInjectionVariables(t *testing.T) {
	for _, name := range []string{
		"GIT_EXTERNAL_DIFF", "GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS",
		"GIT_CONFIG_KEY_0", "GIT_CONFIG_VALUE_0", "GIT_ASKPASS", "SSH_ASKPASS", "GIT_PAGER", "PAGER",
	} {
		t.Setenv(name, "injected")
	}
	t.Setenv("GIT_KEPT_MARKER", "kept")

	env := gitChildEnv()
	if env == nil {
		t.Fatal("gitChildEnv returned nil, which would mean inherit everything")
	}
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if gitInjectionEnvNames[name] || strings.HasPrefix(name, "GIT_CONFIG_KEY_") || strings.HasPrefix(name, "GIT_CONFIG_VALUE_") {
			t.Errorf("gitChildEnv kept injection variable %q", name)
		}
	}
	if !containsEnv(env, "GIT_KEPT_MARKER=kept") {
		t.Error("gitChildEnv dropped an unrelated variable")
	}
}

func containsEnv(env []string, want string) bool {
	for _, kv := range env {
		if kv == want {
			return true
		}
	}
	return false
}

func TestParseFilterAttrs(t *testing.T) {
	// Three NUL-terminated fields per path: path, attribute, value.
	out := []byte("a.txt\x00filter\x00cf\x00b.txt\x00filter\x00unspecified\x00c.txt\x00filter\x00lfs\x00d.txt\x00filter\x00cf\x00")
	got := parseFilterAttrs(out)
	if strings.Join(got, ",") != "cf,lfs" {
		t.Errorf("parseFilterAttrs = %v, want [cf lfs]", got)
	}
	if nil != parseFilterAttrs(nil) {
		t.Error("empty output should report no filters")
	}
}

func TestGitTouchedPathsHandlesStatusAndDiff(t *testing.T) {
	status := "exit 0\n## main\n M f.txt\n?? new.go\nR  old.txt -> renamed.txt"
	got := gitTouchedPaths(status)
	want := []string{"f.txt", "new.go", "renamed.txt"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("status paths = %v, want %v", got, want)
	}

	diff := "exit 0\ndiff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n-a\n+b"
	got = gitTouchedPaths(diff)
	if len(got) == 0 {
		t.Fatal("no paths extracted from a diff")
	}
	for _, p := range got {
		if p != "x.go" {
			t.Errorf("diff path = %q, want x.go", p)
		}
	}
}

// --- integration: the harness's git path runs no repository helper ---

func TestGitStatusDoesNotRunRepositoryFsmonitor(t *testing.T) {
	needSandbox(t)
	requirePOSIXShell(t)
	dir := t.TempDir()
	gitInitRepo(t, dir)
	commitRepoFile(t, dir, "f.txt", "x: y\n")

	marker, script := probeMarker(t, dir, "")
	runGit(t, dir, "config", "core.fsmonitor", script)

	// Positive control: a plain `git status` runs the configured hook.
	clearMarker(t, marker)
	_ = exec.Command("git", "-C", dir, "status", "--porcelain").Run()
	if !ran(t, marker) {
		t.Fatal("positive control failed: plain git status did not run the canary")
	}

	// The harness path must not.
	clearMarker(t, marker)
	writeRepoFile(t, dir, "f.txt", "x: z\n")
	h := newTestHarness(t, dir, nil)
	if out, err := h.Run(context.Background(), ToolGitStatus, nil); err != nil {
		t.Fatalf("git_status: %v (%s)", err, out)
	}
	if ran(t, marker) {
		t.Error("git_status executed the repository-configured fsmonitor")
	}
}

func TestGitDiffDoesNotRunExternalDiffDriver(t *testing.T) {
	needSandbox(t)
	requirePOSIXShell(t)
	dir := t.TempDir()
	gitInitRepo(t, dir)
	commitRepoFile(t, dir, "f.txt", "x: y\n")
	writeRepoFile(t, dir, "f.txt", "x: z\n")

	marker, script := probeMarker(t, dir, "")
	runGit(t, dir, "config", "diff.external", script)

	clearMarker(t, marker)
	_ = exec.Command("git", "-C", dir, "diff").Run()
	if !ran(t, marker) {
		t.Fatal("positive control failed: plain git diff did not run the canary")
	}

	clearMarker(t, marker)
	h := newTestHarness(t, dir, nil)
	if out, err := h.Run(context.Background(), ToolGitDiff, nil); err != nil {
		t.Fatalf("git_diff: %v (%s)", err, out)
	}
	if ran(t, marker) {
		t.Error("git_diff executed the repository-configured diff.external")
	}
}

func TestGitDiffDoesNotRunTextconvDriver(t *testing.T) {
	needSandbox(t)
	requirePOSIXShell(t)
	dir := t.TempDir()
	gitInitRepo(t, dir)
	commitRepoFile(t, dir, "f.txt", "x: y\n")
	writeRepoFile(t, dir, "f.txt", "x: z\n")

	marker, script := probeMarker(t, dir, "")
	runGit(t, dir, "config", "diff.tc.textconv", script)
	writeRepoFile(t, dir, ".gitattributes", "f.txt diff=tc\n")

	clearMarker(t, marker)
	_ = exec.Command("git", "-C", dir, "diff").Run()
	if !ran(t, marker) {
		t.Fatal("positive control failed: plain git diff did not run the canary")
	}

	clearMarker(t, marker)
	h := newTestHarness(t, dir, nil)
	if out, err := h.Run(context.Background(), ToolGitDiff, nil); err != nil {
		t.Fatalf("git_diff: %v (%s)", err, out)
	}
	if ran(t, marker) {
		t.Error("git_diff executed the repository-configured textconv driver")
	}
}

func TestGitDiffDropsExternalDiffEnvironment(t *testing.T) {
	needSandbox(t)
	requirePOSIXShell(t)
	dir := t.TempDir()
	gitInitRepo(t, dir)
	commitRepoFile(t, dir, "f.txt", "x: y\n")
	writeRepoFile(t, dir, "f.txt", "x: z\n")

	marker, script := probeMarker(t, dir, "")
	t.Setenv("GIT_EXTERNAL_DIFF", script)

	clearMarker(t, marker)
	_ = exec.Command("git", "-C", dir, "diff").Run()
	if !ran(t, marker) {
		t.Fatal("positive control failed: plain git diff did not run GIT_EXTERNAL_DIFF")
	}

	clearMarker(t, marker)
	h := newTestHarness(t, dir, nil)
	if out, err := h.Run(context.Background(), ToolGitDiff, nil); err != nil {
		t.Fatalf("git_diff: %v (%s)", err, out)
	}
	if ran(t, marker) {
		t.Error("git_diff executed GIT_EXTERNAL_DIFF from the inherited environment")
	}
}

// TestGitOutputLabelsActiveCleanFilter covers the residual git_integrity.go cannot
// neutralise: a clean filter runs on status and diff with every documented flag.
// The result must be labelled, and the harness must not present the output as raw.
func TestGitOutputLabelsActiveCleanFilter(t *testing.T) {
	needSandbox(t)
	requirePOSIXShell(t)
	dir := t.TempDir()
	gitInitRepo(t, dir)
	commitRepoFile(t, dir, "f.txt", "x: y\n")
	writeRepoFile(t, dir, "f.txt", "x: z\n")

	_, script := probeMarker(t, dir, "cat")
	runGit(t, dir, "config", "filter.cf.clean", script)
	writeRepoFile(t, dir, ".gitattributes", "f.txt filter=cf\n")

	h := newTestHarness(t, dir, nil)
	out, err := h.Run(context.Background(), ToolGitStatus, nil)
	if err != nil {
		t.Fatalf("git_status: %v (%s)", err, out)
	}
	if !strings.Contains(out, "filter driver(s) cf") {
		t.Errorf("git_status did not label the active clean filter: %q", out)
	}

	out, err = h.Run(context.Background(), ToolGitDiff, nil)
	if err != nil {
		t.Fatalf("git_diff: %v (%s)", err, out)
	}
	if !strings.Contains(out, "filter driver(s) cf") {
		t.Errorf("git_diff did not label the active clean filter: %q", out)
	}
}

// TestGitOutputUnlabelledWithoutFilters is the negative control for the label: an
// ordinary repository must not carry the notice, so the label stays meaningful.
func TestGitOutputUnlabelledWithoutFilters(t *testing.T) {
	needSandbox(t)
	isolateGitConfig(t)
	dir := t.TempDir()
	gitInitRepo(t, dir)
	commitRepoFile(t, dir, "f.txt", "x: y\n")
	writeRepoFile(t, dir, "f.txt", "x: z\n")

	h := newTestHarness(t, dir, nil)
	out, err := h.Run(context.Background(), ToolGitStatus, nil)
	if err != nil {
		t.Fatalf("git_status: %v (%s)", err, out)
	}
	if strings.Contains(out, "filter driver(s)") {
		t.Errorf("unexpected filter notice on an ordinary repository: %q", out)
	}
}

// TestGitOutputUnlabelledWhenAttributeNamesAnUnconfiguredDriver pins the accuracy
// of the label. A `.gitattributes` can assign `filter=lfs` while no `filter.lfs.*`
// command is configured and git-lfs is not installed; git then runs nothing and
// `status` works normally. Labelling that would tell the model its output was
// transformed when it was not.
func TestGitOutputUnlabelledWhenAttributeNamesAnUnconfiguredDriver(t *testing.T) {
	needSandbox(t)
	isolateGitConfig(t)
	dir := t.TempDir()
	gitInitRepo(t, dir)
	commitRepoFile(t, dir, "f.txt", "x: y\n")
	writeRepoFile(t, dir, ".gitattributes", "f.txt filter=lfs diff=lfs merge=lfs -text\n")
	writeRepoFile(t, dir, "f.txt", "x: z\n")

	// Precondition: the attribute is present but no driver command is configured.
	if out, err := exec.Command("git", "-C", dir, "check-attr", "filter", "--", "f.txt").Output(); err != nil || !strings.Contains(string(out), "lfs") {
		t.Fatalf("precondition: check-attr = %q err=%v, want an lfs attribute", out, err)
	}
	if err := exec.Command("git", "-C", dir, "config", "--get", "filter.lfs.clean").Run(); err == nil {
		t.Fatal("precondition: filter.lfs.clean is configured despite the isolated git config")
	}

	h := newTestHarness(t, dir, nil)
	out, err := h.Run(context.Background(), ToolGitStatus, nil)
	if err != nil {
		t.Fatalf("git_status: %v (%s)", err, out)
	}
	if strings.Contains(out, "filter driver(s)") {
		t.Errorf("labelled a filter that no configured driver runs: %q", out)
	}
}

// TestConfiguredFilterNames covers the configuration probe directly, so the
// attribute/config intersection is testable without a working filter script.
func TestConfiguredFilterNames(t *testing.T) {
	isolateGitConfig(t)
	dir := t.TempDir()
	gitInitRepo(t, dir)
	commitRepoFile(t, dir, "f.txt", "x: y\n")

	h := newTestHarness(t, dir, nil)
	ctx := context.Background()
	if got := h.configuredFilterNames(ctx); len(got) != 0 {
		t.Errorf("fresh repo configured filters = %v, want none", got)
	}

	runGit(t, dir, "config", "filter.cf.clean", "/bin/cat")
	runGit(t, dir, "config", "filter.proc.process", "/bin/cat")
	runGit(t, dir, "config", "filter.lfs.required", "true") // not an executable key

	got := h.configuredFilterNames(ctx)
	for _, want := range []string{"cf", "proc"} {
		if !got[want] {
			t.Errorf("configuredFilterNames missing %q: %v", want, got)
		}
	}
	if got["lfs"] {
		t.Errorf("filter.<name>.required must not count as an executable driver: %v", got)
	}
}

// TestFingerprintStillObservableWithHardening proves the hardening did not break
// the per-turn fingerprint: a real change is still detected.
func TestFingerprintStillObservableWithHardening(t *testing.T) {
	needSandbox(t)
	isolateGitConfig(t)
	dir := t.TempDir()
	gitInitRepo(t, dir)
	commitRepoFile(t, dir, "f.txt", "x: y\n")

	h := newTestHarness(t, dir, nil)
	ctx := context.Background()
	base, err := h.WorkingTreeFingerprint(ctx)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if changed, err := h.ChangedSince(ctx, base); err != nil || changed {
		t.Fatalf("unchanged tree: changed=%v err=%v, want false", changed, err)
	}
	writeRepoFile(t, dir, "f.txt", "x: z\n")
	if changed, err := h.ChangedSince(ctx, base); err != nil || !changed {
		t.Errorf("modified tree: changed=%v err=%v, want true", changed, err)
	}
}
