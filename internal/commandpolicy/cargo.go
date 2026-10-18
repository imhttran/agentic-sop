package commandpolicy

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Cargo command authorization (SOP-001).
//
// This package never authorizes a Cargo command on its own: the caller passes an
// explicit permission flag. When the permission is OFF, every Cargo command
// resolves to RequiresApproval and a caller that executes only Safe commands
// refuses it. When the permission is ON, the classifier admits only a small,
// supported set of Cargo forms and rejects everything else BEFORE execution:
//
//   - cargo build, cargo test, cargo check, cargo clippy
//   - cargo fmt --check
//
// An unsupported subcommand, an unsupported argument, shell syntax, and a
// --manifest-path that resolves outside the repository root are rejected. The
// classifier is deterministic and path-aware; it never executes anything.

// cargoSupportedSubcommands are the Cargo subcommands this classifier admits.
var cargoSupportedSubcommands = map[string]bool{
	"build":  true,
	"test":   true,
	"check":  true,
	"clippy": true,
}

// cargoSupportedArgs are the exact arguments admitted after a supported
// subcommand. They are build/test switches; anything else is rejected. Options
// that consume a separate value (for example -p NAME) are deliberately not
// admitted, so the supported surface stays small and auditable.
var cargoSupportedArgs = map[string]bool{
	"--all-targets":         true,
	"--all-features":        true,
	"--no-default-features": true,
	"--workspace":           true,
	"--locked":              true,
	"--offline":             true,
	"--release":             true,
	"-q":                    true,
	"--quiet":               true,
	"--check":               true,
	"--all":                 true,
}

// CargoClassify applies the Cargo authorization rules. When enabled is false,
// every Cargo command resolves to RequiresApproval, so a caller that executes only
// Safe commands refuses it. When enabled is true, a supported form resolves to
// Safe; an unsupported form returns an error so the caller rejects it before
// execution.
//
// root is the repository root that a --manifest-path must stay within. An empty
// root keeps only the lexical parent-escape check.
func CargoClassify(argv []string, enabled bool, root string) (Class, error) {
	if len(argv) == 0 || filepath.Base(argv[0]) != "cargo" {
		return Denied, errors.New("commandpolicy: not a cargo command")
	}
	args := argv[1:]
	if len(args) == 0 {
		return Denied, fmt.Errorf("commandpolicy: cargo requires a supported subcommand")
	}
	// Structural safety is checked BEFORE the permission flag. A malformed or
	// dangerous form is therefore rejected even on a path whose permission is off,
	// instead of being reported as merely "requires approval". It also keeps the
	// error ordering stable: shell syntax and the subcommand are validated before
	// arguments or the permission can mask them.
	if strings.ContainsAny(strings.Join(args, " "), ";&|<>$`\n\r*?[]{}()~") {
		return Denied, fmt.Errorf("commandpolicy: shell syntax is not allowed in a cargo command")
	}
	sub := args[0]
	if !cargoSupportedSubcommands[sub] && sub != "fmt" {
		return Denied, fmt.Errorf("commandpolicy: unsupported cargo subcommand %q", sub)
	}
	rest := args[1:]
	if sub == "fmt" {
		if _, err := cargoFmt(rest); err != nil {
			return Denied, err
		}
	} else if err := checkCargoArgs(argv, rest, root); err != nil {
		return Denied, err
	}
	if !enabled {
		// The form is supported and safe to inspect, but the caller did not grant
		// permission for Cargo on this path.
		return RequiresApproval, nil
	}
	return Safe, nil
}

// cargoFmt admits only `cargo fmt` with --check.
func cargoFmt(rest []string) (Class, error) {
	sawCheck := false
	for _, a := range rest {
		switch {
		case a == "--check":
			sawCheck = true
		case cargoSupportedArgs[a]:
			// admitted
		default:
			return RequiresApproval, fmt.Errorf("commandpolicy: unsupported cargo fmt argument %q", a)
		}
	}
	if !sawCheck {
		return RequiresApproval, fmt.Errorf("commandpolicy: cargo fmt is only authorized with --check")
	}
	return Safe, nil
}

// checkCargoArgs validates the arguments of a supported subcommand and enforces
// --manifest-path containment.
func checkCargoArgs(argv, rest []string, root string) error {
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		switch {
		case a == "--manifest-path" || a == "--manifest_path":
			if i+1 >= len(rest) {
				return fmt.Errorf("commandpolicy: --manifest-path requires a value")
			}
			if err := manifestPathWithin(root, rest[i+1]); err != nil {
				return err
			}
			i++
		case strings.HasPrefix(a, "--manifest-path=") || strings.HasPrefix(a, "--manifest_path="):
			eq := strings.IndexByte(a, '=')
			if err := manifestPathWithin(root, a[eq+1:]); err != nil {
				return err
			}
		case cargoSupportedArgs[a]:
			// admitted
		default:
			return fmt.Errorf("commandpolicy: unsupported cargo argument %q in %q", a, strings.Join(argv, " "))
		}
	}
	return nil
}

// manifestPathWithin reports an error when the manifest path is absolute outside
// root or climbs out of root via ".." segments.
func manifestPathWithin(root, p string) error {
	if strings.TrimSpace(p) == "" {
		return fmt.Errorf("commandpolicy: --manifest-path is empty")
	}
	clean := filepath.Clean(p)
	if root == "" {
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("commandpolicy: --manifest-path %q escapes the repository", p)
		}
		return nil
	}
	canonical := root
	if abs, err := filepath.Abs(canonical); err == nil {
		canonical = abs
	}
	candidate := clean
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(canonical, candidate)
	}
	if abs, err := filepath.Abs(candidate); err == nil {
		candidate = abs
	}
	rel, err := filepath.Rel(canonical, candidate)
	if err != nil {
		return fmt.Errorf("commandpolicy: --manifest-path %q is not comparable to the repository", p)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("commandpolicy: --manifest-path %q escapes the repository", p)
	}
	return nil
}
