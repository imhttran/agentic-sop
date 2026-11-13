// Package commandpolicy classifies commands deterministically before they are
// executed: SAFE, REQUIRES_APPROVAL, or DENIED. It works on structured argument
// lists, never a shell string, and project policy can only tighten the defaults
// — it can never loosen a denied command.
package commandpolicy

import (
	"errors"
	"path/filepath"
	"strings"
)

// Class is a command's execution class.
type Class string

const (
	// Safe: read-only or verification commands that may run unattended.
	Safe Class = "SAFE"
	// RequiresApproval: commands that change shared state and need a human.
	RequiresApproval Class = "REQUIRES_APPROVAL"
	// Denied: commands that must never run unattended, such as force-pushes.
	Denied Class = "DENIED"
)

// rank orders classes by strictness so policy can only tighten.
func rank(c Class) int {
	switch c {
	case Safe:
		return 0
	case RequiresApproval:
		return 1
	case Denied:
		return 2
	default:
		return 2
	}
}

// readOnlyGit are git subcommands that only observe the repository.
var readOnlyGit = map[string]bool{
	"diff": true, "status": true, "log": true, "show": true, "rev-parse": true,
	"ls-files": true, "describe": true, "blame": true, "shortlog": true,
	"cat-file": true, "grep": true,
}

// safeGo are go subcommands that only build or test.
var safeGo = map[string]bool{
	"test": true, "build": true, "vet": true, "fmt": true, "list": true,
}

// gitGlobalWithValue are git global options that consume the next argument, so
// the subcommand can still be located after them.
var gitGlobalWithValue = map[string]bool{
	"-C": true, "-c": true, "--git-dir": true, "--work-tree": true,
	"--namespace": true,
}

// Classify applies the default policy to a structured command (program plus
// arguments). An empty command is an error; an unrecognized program is treated
// conservatively as requiring approval.
func Classify(argv []string) (Class, error) {
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return "", errors.New("commandpolicy: empty command")
	}

	switch filepath.Base(argv[0]) {
	case "git":
		return classifyGit(argv[1:]), nil
	case "go":
		return classifyGo(argv[1:]), nil
	case "cargo":
		// Cargo classification requires an explicit permission and a repository
		// root; the permission-aware entry point is CargoClassify. Without that
		// permission a Cargo command is never SAFE.
		return RequiresApproval, nil
	default:
		return RequiresApproval, nil
	}
}

func classifyGit(args []string) Class {
	// A push is recognised by any 'push' token, not by the subcommand guess: an
	// unrecognised value-taking global option (--attr-source HEAD) would otherwise
	// hide it. Fail closed: a configuration override on a push can force the update
	// (remote.*.push, push.*), so it is denied rather than approved.
	if idx := indexToken(args, "push"); idx >= 0 {
		return classifyPushInvocation(args[:idx], args[idx:])
	}
	// An unknown leading global option may take a value, so the subcommand cannot
	// be located reliably. Fail closed to approval rather than guess.
	if !knownGitGlobals(args) {
		return RequiresApproval
	}
	if gitConfigOverride(args) {
		// A global configuration override can make even a read-only subcommand
		// launch an external program: a pager, diff driver, fsmonitor, ssh command,
		// hook path, alias, or filter. It needs a human, never SAFE.
		return RequiresApproval
	}
	sub := gitSubcommand(args)
	switch {
	case sub == "":
		return RequiresApproval
	case readOnlyGit[sub]:
		return Safe
	default:
		return RequiresApproval
	}
}

// gitConfigOverride reports whether git's global-option prefix sets configuration
// inline: `-c key=value`, the attached `-ckey=value`, or `--config-env`. Those can
// name a command git later executes, so a read-only subcommand is no longer a
// read-only action.
//
// Only the global position is inspected, because that is the only place git reads
// these options. After the subcommand `-c` belongs to the subcommand: `git log -c`
// and `git grep -c` are ordinary read-only flags and stay SAFE.
func gitConfigOverride(args []string) bool {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			return false // the subcommand starts here; global options are done
		}
		switch {
		case a == "-c", a == "--config-env":
			return true
		case strings.HasPrefix(a, "--config-env="):
			return true
		case strings.HasPrefix(a, "-c"):
			return true // attached form: -ckey=value
		case gitGlobalWithValue[a]:
			i++ // this option consumes the following argument
		}
	}
	return false
}

func classifyGo(args []string) Class {
	if len(args) > 0 && safeGo[args[0]] {
		return Safe
	}
	return RequiresApproval
}

// gitSubcommand returns the first non-flag argument, skipping global options and
// the values they consume.
func gitSubcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			if gitGlobalWithValue[a] {
				i++
			}
			continue
		}
		return a
	}
	return ""
}

// indexToken returns the first index of token in args, or -1.
func indexToken(args []string, token string) int {
	for i, a := range args {
		if a == token {
			return i
		}
	}
	return -1
}

// pushPrefixInert are the global options that cannot configure, redirect, or
// reinterpret a push. Any other token before the push subcommand denies it.
var pushPrefixInert = map[string]bool{
	"--no-pager": true, "--no-optional-locks": true,
	"--literal-pathspecs": true, "--no-replace-objects": true,
}

// classifyPushInvocation applies to any command whose tokens contain 'push'.
// The tokens before it are global options the parser may misread (a -c, a -C, an
// unknown value-taking option, or a word that is really the subcommand), so any
// token outside pushPrefixInert denies the invocation.
func classifyPushInvocation(prefix, push []string) Class {
	for _, a := range prefix {
		if !pushPrefixInert[a] {
			return Denied
		}
	}
	return classifyPush(push)
}

// knownGitGlobals reports whether every leading global option is one this parser
// understands, so the subcommand is located correctly.
func knownGitGlobals(args []string) bool {
	for i := 0; i < len(args) && strings.HasPrefix(args[i], "-"); i++ {
		a := args[i]
		switch {
		case gitGlobalWithValue[a]:
			i++ // consumes the following argument
		case pushPrefixInert[a], a == "--exec-path":
			// bare --exec-path prints the path and exits. --exec-path=<dir> points
			// command lookup at a directory the model can write, so it is unknown.
		case strings.HasPrefix(a, "-c"), strings.HasPrefix(a, "--config-env"):
			// config forms are judged by gitConfigOverride
		default:
			return false
		}
	}
	return true
}

// pushSafeFlags are the push options that do not change which refs move.
var pushSafeFlags = map[string]bool{
	"-u": true, "--set-upstream": true, "-v": true, "--verbose": true,
	"-n": true, "--dry-run": true, "--": true,
}

// classifyPush fails closed. A plain push needs approval. A push that could
// rewrite or delete a remote ref is denied: any option outside pushSafeFlags
// (git accepts bundled short flags such as -fu, unambiguous long-option prefixes
// such as --force-with, and --mirror or --delete), and any refspec with a leading
// '+' (forced update) or ':' (deletion).
func classifyPush(args []string) Class {
	seen := false
	for _, a := range args {
		if !seen {
			seen = a == "push"
			continue
		}
		switch {
		case strings.HasPrefix(a, "-"):
			if !pushSafeFlags[a] {
				return Denied
			}
		case strings.HasPrefix(a, "+"), strings.HasPrefix(a, ":"):
			return Denied
		}
	}
	return RequiresApproval
}

// Policy is project configuration that tightens the default classes. It can
// raise a command's strictness but never lower it.
type Policy struct {
	// Deny lists argv prefixes that are always denied.
	Deny [][]string
	// Approve lists argv prefixes that always require approval.
	Approve [][]string
}

// Classify applies the defaults and then any tightening from the policy.
func (p Policy) Classify(argv []string) (Class, error) {
	base, err := Classify(argv)
	if err != nil {
		return "", err
	}

	class := base
	if matchesAny(argv, p.Approve) {
		class = tighten(class, RequiresApproval)
	}
	if matchesAny(argv, p.Deny) {
		class = tighten(class, Denied)
	}
	return class, nil
}

// tighten returns the stricter of two classes.
func tighten(a, b Class) Class {
	if rank(a) >= rank(b) {
		return a
	}
	return b
}

// matchesAny reports whether argv starts with any of the given prefixes.
func matchesAny(argv []string, prefixes [][]string) bool {
	for _, p := range prefixes {
		if hasPrefix(argv, p) {
			return true
		}
	}
	return false
}

// hasPrefix reports whether argv begins with the whole prefix, token by token.
func hasPrefix(argv, prefix []string) bool {
	if len(prefix) == 0 || len(argv) < len(prefix) {
		return false
	}
	for i := range prefix {
		if argv[i] != prefix[i] {
			return false
		}
	}
	return true
}
