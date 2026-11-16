package toolharness

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// W7 (decision packet Option C): only a *verified* git-lfs filter is accepted as
// an expected transformation. Any other executable filter driver means SOP cannot
// establish the integrity of git's reported status/diff, so the result is labelled
// and never trusted for mutation detection.
//
// This is separate from the W1 hardening. The hardening stops the mechanisms git
// exposes a switch for; a clean filter has no switch, so the answer is to verify
// the one expected configuration and fail closed for everything else.

// errGitOutputUntrusted reports that a repository-configured filter driver makes
// git's reported status/diff — and therefore a working-tree fingerprint —
// untrustworthy. Callers fail closed: a change check reports "changed" rather than
// "unchanged", so a filter can never make SOP claim a task changed nothing.
var errGitOutputUntrusted = errors.New("git output integrity cannot be established")

// gitLFSDriver is the driver name `git lfs install` writes.
const gitLFSDriver = "lfs"

// gitOutputIntegrity records what SOP knows about the git output it is about to
// return or hash.
//
// applied is path-accurate: the drivers that are both assigned to the reported
// paths and configured, so the note describes what actually ran. unverified is
// repository-wide: a configured driver that is not a verified git-lfs configuration
// can be selected by an attribute at any time — including one the model just wrote —
// and a filter can hide a change from `git status` entirely. Deciding from the
// reported paths alone would therefore miss exactly the case that matters.
type gitOutputIntegrity struct {
	applied    []string
	unverified []string
}

// trusted reports whether git's output may be relied on for mutation detection.
func (g gitOutputIntegrity) trusted() bool { return len(g.unverified) == 0 }

// gitOutputIntegrityOf inspects the repository and the paths a result named.
func (h *Harness) gitOutputIntegrityOf(ctx context.Context, paths []string) gitOutputIntegrity {
	return gitOutputIntegrity{
		applied:    h.activeGitFilters(ctx, paths),
		unverified: h.unverifiedFilterDrivers(ctx),
	}
}

// unverifiedFilterDrivers returns the configured filter drivers that are not a
// verified git-lfs configuration, sorted. An empty result means every configured
// driver is either absent or the standard git-lfs one.
func (h *Harness) unverifiedFilterDrivers(ctx context.Context) []string {
	configured := h.configuredFilterNames(ctx)
	if len(configured) == 0 {
		return nil
	}
	names := make([]string, 0, len(configured))
	for name := range configured {
		names = append(names, name)
	}
	sort.Strings(names)
	var unverified []string
	for _, name := range names {
		if !h.verifiedGitLFS(ctx, name) {
			unverified = append(unverified, name)
		}
	}
	return unverified
}

// verifiedGitLFS reports whether driver is the git-lfs configuration `git lfs
// install` writes: the expected driver name, a command that is exactly git-lfs's,
// and a git-lfs binary resolved outside the repository. All three are required, so
// a repository cannot pass verification by naming a driver `lfs`, by shipping its
// own `git-lfs`, or by pointing the driver at an arbitrary program.
func (h *Harness) verifiedGitLFS(ctx context.Context, driver string) bool {
	if driver != gitLFSDriver {
		return false
	}
	if !h.gitLFSBinaryOutsideRepo() {
		return false
	}
	command, ok := h.gitFilterCommand(ctx, driver)
	if !ok {
		return false
	}
	return isStandardGitLFSCommand(command)
}

// gitFilterCommand returns the command git will actually run for driver. git
// prefers the long-running `process` helper when it is configured.
func (h *Harness) gitFilterCommand(ctx context.Context, driver string) (string, bool) {
	for _, key := range []string{"process", "clean"} {
		if v, ok := h.gitConfigValue(ctx, "filter."+driver+"."+key); ok {
			return v, true
		}
	}
	return "", false
}

// gitConfigValue reads one git configuration value, reporting false when it is
// unset.
func (h *Harness) gitConfigValue(ctx context.Context, key string) (string, bool) {
	out, ok := h.gitProbe(ctx, []string{"config", "--get", key})
	if !ok {
		return "", false
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return "", false
	}
	return v, true
}

// isStandardGitLFSCommand accepts only the exact command strings `git lfs install`
// writes. A prefix match would accept `git-lfs clean -- %f; rm -rf /`, so the
// comparison is on the whole command after field normalisation.
func isStandardGitLFSCommand(command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 || fields[0] != "git-lfs" {
		return false
	}
	switch strings.Join(fields[1:], " ") {
	case "filter-process", "clean -- %f", "smudge -- %f":
		return true
	}
	return false
}

// gitLFSBinaryOutsideRepo requires a resolvable git-lfs that is not inside the
// repository, so a repository cannot ship the program its own filter runs.
func (h *Harness) gitLFSBinaryOutsideRepo() bool {
	found, err := exec.LookPath("git-lfs")
	if err != nil {
		return false
	}
	binary, err := filepath.Abs(found)
	if err != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(binary); err == nil {
		binary = resolved
	}
	root, err := filepath.EvalSymlinks(h.root)
	if err != nil {
		return false
	}
	return !underAny(binary, root)
}

// gitOutputNote states, in the tool result, that a repository-configured filter
// transformed the content and whether SOP could establish its integrity. A verified
// git-lfs filter is named as the expected transformation; anything else says the
// output cannot be trusted. Both cases are visible: silently transforming output is
// what this control exists to prevent.
func gitOutputNote(g gitOutputIntegrity) string {
	if len(g.unverified) > 0 {
		return fmt.Sprintf("\n[git output: configured filter driver(s) %s; output integrity could not be established, so this content is not guaranteed to match the files on disk]",
			strings.Join(g.unverified, ", "))
	}
	if len(g.applied) > 0 {
		return fmt.Sprintf("\n[git output: verified git-lfs clean filter applied (%s); status and diff show repository-clean content, not raw working-tree bytes]",
			strings.Join(g.applied, ", "))
	}
	return ""
}
