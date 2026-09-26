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
	"--exec-path": true, "--namespace": true,
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
	default:
		return RequiresApproval, nil
	}
}

func classifyGit(args []string) Class {
	sub := gitSubcommand(args)
	switch {
	case sub == "":
		return RequiresApproval
	case sub == "push":
		if hasForceFlag(args) {
			return Denied
		}
		return RequiresApproval
	case readOnlyGit[sub]:
		return Safe
	default:
		return RequiresApproval
	}
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

// hasForceFlag reports whether a git push would rewrite history.
func hasForceFlag(args []string) bool {
	for _, a := range args {
		switch {
		case a == "--force", a == "-f", a == "--force-with-lease":
			return true
		case strings.HasPrefix(a, "--force-with-lease="):
			return true
		}
	}
	return false
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
