package toolharness

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// W7 (Option C) contract: a verified git-lfs filter is accepted and mutation
// detection keeps working; any other executable filter driver makes git output
// untrusted, so status/diff are labelled and fingerprints fail closed to "changed".

// gitLFSGlobalConfig is the configuration `git lfs install` writes, restricted to
// the clean/smudge pair a pass-through probe can run safely.
const gitLFSGlobalConfig = "[filter \"lfs\"]\n\tclean = git-lfs clean -- %f\n\tsmudge = git-lfs smudge -- %f\n\trequired = true\n"

// writeConfigFile writes a git config body and returns its path.
func writeConfigFile(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "gitconfig")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeExecutable creates an executable shell script and returns its path.
func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// isolateGitConfigTo points git at one global config file and no system config,
// so the test does not depend on what the host or CI runner installed.
func isolateGitConfigTo(t *testing.T, globalConfig string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
}

// withGitLFSOnPath puts a pass-through `git-lfs` in dir and prepends dir to PATH.
func withGitLFSOnPath(t *testing.T, dir string) {
	t.Helper()
	writeExecutable(t, filepath.Join(dir, "git-lfs"), "exec cat")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// --- verified git-lfs: accepted, and mutation detection survives ---------------

func TestVerifiedGitLFSFilterIsAccepted(t *testing.T) {
	requirePOSIXShell(t)
	repo := t.TempDir()
	bin := t.TempDir() // outside the repository
	gitInitRepo(t, repo)
	commitRepoFile(t, repo, "f.txt", "x: y\n")

	isolateGitConfigTo(t, writeConfigFile(t, t.TempDir(), gitLFSGlobalConfig))
	withGitLFSOnPath(t, bin)

	h := newTestHarness(t, repo, nil)
	ctx := context.Background()

	if got := h.unverifiedFilterDrivers(ctx); len(got) != 0 {
		t.Fatalf("unverifiedFilterDrivers = %v, want none for a standard git-lfs config", got)
	}

	// The positive control for Option C: a git-lfs repository keeps working
	// mutation detection instead of being permanently "changed".
	base, err := h.WorkingTreeFingerprint(ctx)
	if err != nil {
		t.Fatalf("fingerprint on a verified git-lfs repository: %v", err)
	}
	if changed, err := h.ChangedSince(ctx, base); err != nil || changed {
		t.Fatalf("unchanged tree: changed=%v err=%v, want false", changed, err)
	}

	// The filter is selected by an attribute, so the model is told which expected
	// transformation produced the content.
	writeRepoFile(t, repo, ".gitattributes", "f.txt filter=lfs\n")
	writeRepoFile(t, repo, "f.txt", "x: z\n")
	out, err := h.Run(ctx, ToolGitStatus, nil)
	if err != nil {
		t.Fatalf("git_status: %v (%s)", err, out)
	}
	if !strings.Contains(out, "verified git-lfs clean filter applied") {
		t.Errorf("git_status did not name the verified git-lfs filter: %q", out)
	}
	if strings.Contains(out, "integrity could not be established") {
		t.Errorf("a verified git-lfs filter was reported as untrusted: %q", out)
	}
}

// TestGlobalGitLFSConfigLeavesUnrelatedRepositoriesTrusted is the negative control
// for the repository-wide decision: the config exists, but nothing selects it, so
// an unrelated repository must not be labelled or fail closed.
func TestGlobalGitLFSConfigLeavesUnrelatedRepositoriesTrusted(t *testing.T) {
	requirePOSIXShell(t)
	repo := t.TempDir()
	gitInitRepo(t, repo)
	commitRepoFile(t, repo, "f.txt", "x: y\n")

	isolateGitConfigTo(t, writeConfigFile(t, t.TempDir(), gitLFSGlobalConfig))
	withGitLFSOnPath(t, t.TempDir())

	h := newTestHarness(t, repo, nil)
	ctx := context.Background()
	if got := h.unverifiedFilterDrivers(ctx); len(got) != 0 {
		t.Fatalf("unverifiedFilterDrivers = %v, want none", got)
	}
	base, err := h.WorkingTreeFingerprint(ctx)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if changed, err := h.ChangedSince(ctx, base); err != nil || changed {
		t.Fatalf("unchanged tree: changed=%v err=%v, want false", changed, err)
	}
}

// --- unverified driver: fail closed, and never silently trusted ----------------

func TestUnverifiedFilterFailsClosed(t *testing.T) {
	repo := t.TempDir()
	gitInitRepo(t, repo)
	commitRepoFile(t, repo, "f.txt", "x: y\n")

	// Start from a trusted state so the baseline is a real fingerprint.
	isolateGitConfigTo(t, writeConfigFile(t, t.TempDir(), ""))
	h := newTestHarness(t, repo, nil)
	ctx := context.Background()
	base, err := h.WorkingTreeFingerprint(ctx)
	if err != nil {
		t.Fatalf("baseline fingerprint: %v", err)
	}

	// Now configure a non-git-lfs filter driver. Nothing in the tree changes.
	runGit(t, repo, "config", "filter.cf.clean", "cat")

	if got := h.unverifiedFilterDrivers(ctx); strings.Join(got, ",") != "cf" {
		t.Fatalf("unverifiedFilterDrivers = %v, want [cf]", got)
	}
	if _, err := h.WorkingTreeFingerprint(ctx); !errors.Is(err, errGitOutputUntrusted) {
		t.Fatalf("fingerprint err = %v, want errGitOutputUntrusted", err)
	}
	// A filter can hide a change, so an unchanged tree must still report changed.
	if changed, err := h.ChangedSince(ctx, base); err != nil || !changed {
		t.Fatalf("ChangedSince = (%v, %v), want (true, nil)", changed, err)
	}
	if changed, err := h.WorkingTreeChanged(ctx); err != nil || !changed {
		t.Fatalf("WorkingTreeChanged = (%v, %v), want (true, nil)", changed, err)
	}
}

// TestUnverifiedFilterIsLabelledWithoutAttributeSelection covers the hiding case:
// with no attribute selecting the driver, `git status` reports nothing, so a
// path-accurate label would be absent and the model would silently trust the
// result. The repository-wide decision must still mark it.
func TestUnverifiedFilterIsLabelledWithoutAttributeSelection(t *testing.T) {
	repo := t.TempDir()
	gitInitRepo(t, repo)
	commitRepoFile(t, repo, "f.txt", "x: y\n")

	isolateGitConfigTo(t, writeConfigFile(t, t.TempDir(), ""))
	runGit(t, repo, "config", "filter.cf.clean", "cat")

	h := newTestHarness(t, repo, nil)
	out, err := h.Run(context.Background(), ToolGitStatus, nil)
	if err != nil {
		t.Fatalf("git_status: %v (%s)", err, out)
	}
	if !strings.Contains(out, "output integrity could not be established") {
		t.Errorf("untrusted output was not labelled: %q", out)
	}
}

// --- verification cannot be satisfied by a repository ------------------------

func TestRepoLocalGitLFSBinaryIsNotVerified(t *testing.T) {
	requirePOSIXShell(t)
	repo := t.TempDir()
	gitInitRepo(t, repo)
	commitRepoFile(t, repo, "f.txt", "x: y\n")

	isolateGitConfigTo(t, writeConfigFile(t, t.TempDir(), gitLFSGlobalConfig))
	// The repository ships its own git-lfs and puts itself on PATH.
	writeExecutable(t, filepath.Join(repo, "git-lfs"), "exec cat")
	t.Setenv("PATH", repo+string(os.PathListSeparator)+os.Getenv("PATH"))

	h := newTestHarness(t, repo, nil)
	ctx := context.Background()
	if h.verifiedGitLFS(ctx, gitLFSDriver) {
		t.Fatal("a repository-local git-lfs was accepted as verified")
	}
	if got := h.unverifiedFilterDrivers(ctx); strings.Join(got, ",") != "lfs" {
		t.Fatalf("unverifiedFilterDrivers = %v, want [lfs]", got)
	}
}

func TestNonStandardLFSCommandIsNotVerified(t *testing.T) {
	requirePOSIXShell(t)
	repo := t.TempDir()
	gitInitRepo(t, repo)
	commitRepoFile(t, repo, "f.txt", "x: y\n")

	isolateGitConfigTo(t, writeConfigFile(t, t.TempDir(), "[filter \"lfs\"]\n\tclean = cat\n"))
	withGitLFSOnPath(t, t.TempDir()) // a real git-lfs is available, but not used

	h := newTestHarness(t, repo, nil)
	if h.verifiedGitLFS(context.Background(), gitLFSDriver) {
		t.Fatal("a driver named lfs with a non-git-lfs command was accepted")
	}
}

func TestIsStandardGitLFSCommand(t *testing.T) {
	cases := []struct {
		command string
		want    bool
	}{
		{"git-lfs filter-process", true},
		{"git-lfs clean -- %f", true},
		{"git-lfs smudge -- %f", true},
		{"  git-lfs   clean   --   %f  ", true},
		{"cat", false},
		{"./git-lfs clean -- %f", false},
		{"git-lfs clean -- %f; rm -rf /", false},
		{"git-lfs clean -- %f && curl evil", false},
		{"sh -c git-lfs clean -- %f", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isStandardGitLFSCommand(tc.command); got != tc.want {
			t.Errorf("isStandardGitLFSCommand(%q) = %v, want %v", tc.command, got, tc.want)
		}
	}
}
