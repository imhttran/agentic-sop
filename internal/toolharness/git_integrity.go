package toolharness

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// Git is a program launcher as much as a version-control tool: a repository or an
// ambient environment can name programs that git runs while it reads the working
// tree. SOP's git tools and per-turn fingerprints must not let that happen, and
// must not present the result as trustworthy when it cannot be prevented.
//
// This file neutralises every repository- or environment-configured git execution
// mechanism that git exposes a switch for, and detects the one it does not
// (clean/smudge filters), so a tool result is labelled rather than trusted.
// Containment in the OS sandbox (see isolation.go) is a separate control and is
// not, by itself, evidence that git's reported output is unmodified.

// hardenedGitArgs returns the arguments for one SOP-run git command with the
// hardening options applied. It deliberately omits the leading `git`: every caller
// passes the literal program name, so the harness keeps a single static exec site
// and a policy-reviewed `exec.CommandContext` call is not multiplied.
//
// `core.fsmonitor=false` is a global option and applies to every subcommand;
// `--no-ext-diff` and `--no-textconv` are diff options, so they are added only to
// `diff`, which is the only subcommand SOP runs that formats a diff (`git status`
// rejects them as unknown options).
//
// Measured on Git 2.50.1: without these options `diff.external`,
// `GIT_EXTERNAL_DIFF`, `core.fsmonitor`, and `textconv` each execute their
// configured program; with them, each is blocked.
func hardenedGitArgs(args []string) []string {
	out := make([]string, 0, len(args)+5)
	out = append(out, "-c", "core.fsmonitor=false")
	if len(args) > 0 && args[0] == "diff" {
		out = append(out, "diff", "--no-ext-diff", "--no-textconv")
		return append(out, args[1:]...)
	}
	return append(out, args...)
}

// gitInjectionEnvNames are environment variables that make git run a program or
// read configuration SOP did not choose. They are dropped from every git child
// so an ambient shell profile, wrapper, or CI variable cannot add an execution
// mechanism that the hardened argv does not know about.
var gitInjectionEnvNames = map[string]bool{
	"GIT_EXTERNAL_DIFF":     true, // diff driver program
	"GIT_CONFIG":            true, // config file override
	"GIT_CONFIG_COUNT":      true, // inline `-c` injection (paired with KEY/VALUE)
	"GIT_CONFIG_PARAMETERS": true, // older inline `-c` injection
	"GIT_ASKPASS":           true, // credential program
	"SSH_ASKPASS":           true, // credential program
	"GIT_PAGER":             true, // pager program
	"PAGER":                 true, // pager program
}

// gitChildEnv is the environment for a git child: the inherited environment
// without any variable that can inject configuration or an executable. It is
// never nil, because a nil child environment means "inherit everything".
func gitChildEnv() []string {
	inherited := os.Environ()
	env := make([]string, 0, len(inherited))
	for _, kv := range inherited {
		name, _, _ := strings.Cut(kv, "=")
		if gitInjectionEnvNames[name] ||
			strings.HasPrefix(name, "GIT_CONFIG_KEY_") ||
			strings.HasPrefix(name, "GIT_CONFIG_VALUE_") {
			continue
		}
		env = append(env, kv)
	}
	return env
}

// maxFilterProbePaths bounds how many paths one attribute probe names, so a large
// working tree cannot turn detection into an unbounded command line.
const maxFilterProbePaths = 100

// activeGitFilters reports the clean/smudge filter drivers that actually run for
// paths, sorted and de-duplicated. It is the one git execution mechanism with no
// disabling switch: the driver name is chosen by the repository, so git offers no
// global flag to turn filters off (measured: a `filter.<driver>.clean` still runs
// with every documented hardening flag).
//
// A driver counts as active only when both halves are present: an attribute
// assigns it to a path (`filter=<name>`) *and* a `filter.<name>.clean|smudge|
// process` command is configured. An attribute reference alone runs nothing — a
// `.gitattributes` naming `filter=lfs` with git-lfs absent or unconfigured leaves
// `git status` working normally — so reporting it would be a false positive.
//
// Detection is deliberately best-effort: a probe failure returns no names, and a
// caller must not read that as proof that no filter is configured. The result is
// used to label git output, never to justify trusting it.
func (h *Harness) activeGitFilters(ctx context.Context, paths []string) []string {
	referenced := h.attributeFilterNames(ctx, paths)
	if len(referenced) == 0 {
		return nil
	}
	configured := h.configuredFilterNames(ctx)
	active := make([]string, 0, len(referenced))
	for _, name := range referenced {
		if configured[name] {
			active = append(active, name)
		}
	}
	if len(active) == 0 {
		return nil
	}
	return active
}

// attributeFilterNames returns the filter drivers that `.gitattributes` assigns to
// the given paths, whether or not a driver command is configured for them.
func (h *Harness) attributeFilterNames(ctx context.Context, paths []string) []string {
	paths = boundedPaths(paths, maxFilterProbePaths)
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"check-attr", "-z", "filter", "--"}, paths...)
	out, ok := h.gitProbe(ctx, args)
	if !ok {
		return nil
	}
	return parseFilterAttrs(out)
}

// configuredFilterNames returns the set of driver names that have an executable
// filter command configured. `filter.<name>.required` and descriptive keys do not
// count; only clean, smudge and process run a program.
func (h *Harness) configuredFilterNames(ctx context.Context) map[string]bool {
	out, ok := h.gitProbe(ctx, []string{"config", "--get-regexp", `^filter\..*\.(clean|smudge|process)$`})
	if !ok {
		return nil
	}
	names := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		key, _, _ := strings.Cut(strings.TrimSpace(line), " ")
		// key is filter.<name>.<clean|smudge|process>
		rest, found := strings.CutPrefix(key, "filter.")
		if !found {
			continue
		}
		i := strings.LastIndex(rest, ".")
		if i <= 0 {
			continue
		}
		names[rest[:i]] = true
	}
	return names
}

// gitProbe runs a read-only git query with the same hardening as every other
// SOP-run git command and reports whether it succeeded. A non-zero exit (for
// example `git config --get-regexp` with no matches) is not an error.
func (h *Harness) gitProbe(ctx context.Context, args []string) ([]byte, bool) {
	cmd := exec.CommandContext(ctx, "git", hardenedGitArgs(args)...)
	cmd.Dir = h.root
	cmd.Env = gitChildEnv()
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, false
		}
		return nil, false
	}
	return out, true
}

// parseFilterAttrs reads `git check-attr -z filter` output, which is a flat
// sequence of path, attribute, value triples, and returns the driver names that
// are actually applied. "unspecified" and "unset" mean no driver runs.
func parseFilterAttrs(out []byte) []string {
	fields := strings.Split(string(out), "\x00")
	seen := make(map[string]bool)
	var names []string
	for i := 0; i+2 < len(fields); i += 3 {
		value := strings.TrimSpace(fields[i+2])
		switch value {
		case "", "unspecified", "unset":
			continue
		}
		if !seen[value] {
			seen[value] = true
			names = append(names, value)
		}
	}
	sort.Strings(names)
	return names
}

// boundedPaths drops empty entries and caps the number of probe paths. It does not
// otherwise rewrite a path, so a name with leading or trailing spaces is probed as
// git reported it.
func boundedPaths(paths []string, limit int) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		out = append(out, p)
		if len(out) == limit {
			break
		}
	}
	return out
}

// filterOutputNote is superseded by gitOutputNote (git_filter_policy.go), which
// distinguishes a verified git-lfs filter from one whose integrity SOP cannot
// establish.

// gitTouchedPaths extracts the paths a git tool result mentions, so the filter
// probe can name real paths. Porcelain status lines and diff file headers are both
// understood. The parsing is strict: a line that is not recognisably one of the
// two forms (an exit-status line, a diff hunk header, a branch header) contributes
// no path, so the probe is never handed arbitrary result text.
func gitTouchedPaths(out string) []string {
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if p := porcelainStatusPath(line); p != "" {
			paths = append(paths, p)
			continue
		}
		if p := diffHeaderPath(line); p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// porcelainStatusPath extracts the path from one `git status --porcelain` line.
// It requires git's fixed shape — two status columns and a separator space — and
// rejects a line whose columns are not status characters, so free text such as the
// tool's "exit 0" header is not mistaken for a path. A rename reports its new path.
func porcelainStatusPath(line string) string {
	line = strings.TrimRight(line, "\r")
	if len(line) < 4 || line[2] != ' ' {
		return ""
	}
	if !isStatusChar(line[0]) || !isStatusChar(line[1]) || (line[0] == ' ' && line[1] == ' ') {
		return ""
	}
	rest := line[3:]
	if i := strings.Index(rest, " -> "); i >= 0 {
		rest = rest[i+len(" -> "):]
	}
	rest = strings.Trim(rest, `"`)
	if rest == "" || rest == stateDirRel || strings.HasPrefix(rest, stateDirRel+"/") {
		return ""
	}
	return rest
}

// isStatusChar reports whether b can appear in a porcelain status column. A space
// is valid in either column (for example a staged rename is "R ").
func isStatusChar(b byte) bool {
	return strings.IndexByte(" ACDMRTUXB?!", b) >= 0
}

// diffHeaderPath extracts the path from a `+++ b/path` or `--- a/path` diff
// header. The `/dev/null` side of an add or delete names no path.
func diffHeaderPath(line string) string {
	for _, prefix := range []string{"+++ ", "--- "} {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		p := strings.TrimSpace(line[len(prefix):])
		if p == "/dev/null" || p == "" {
			return ""
		}
		p = strings.TrimPrefix(p, "b/")
		p = strings.TrimPrefix(p, "a/")
		return p
	}
	return ""
}
