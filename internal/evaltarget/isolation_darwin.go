//go:build darwin

package evaltarget

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const sandboxExecPath = "/usr/bin/sandbox-exec"

func isolationAvailable() bool {
	info, err := os.Stat(sandboxExecPath)
	return err == nil && !info.IsDir()
}

func isolationKind() string { return "sandbox-exec" }

// isolationCommand runs argv under a deny-by-default sandbox profile. The case
// root is writable; system tooling and the executable itself are read-only. The
// ordinary workspace and the user's home are not readable. All network access is
// denied.
func isolationCommand(ctx context.Context, root, dir string, argv, env []string) (*exec.Cmd, error) {
	return darwinIsolationCommand(ctx, root, dir, argv, env, "(deny network*)")
}

// liveIsolationCommand is the opt-in, evaluation-only live variant. It keeps the
// file and environment restrictions and permits exactly one network destination:
// the fixed loopback inference endpoint. Every other network destination remains
// denied by the default-deny profile.
func liveIsolationCommand(ctx context.Context, root, dir string, argv, env []string) (*exec.Cmd, error) {
	return darwinIsolationCommand(ctx, root, dir, argv, env, liveNetworkRule())
}

// liveNetworkRule renders the single-endpoint Seatbelt rule. Seatbelt accepts
// "localhost:<port>" (not a numeric literal), which matches the loopback address
// the live decider dials.
func liveNetworkRule() string {
	return fmt.Sprintf("(allow network-outbound (remote ip %q))", LiveEndpointHostPort)
}

// darwinIsolationCommand builds the deny-by-default profile with networkRule
// appended. The profile is otherwise identical for the default and live variants.
func darwinIsolationCommand(ctx context.Context, root, dir string, argv, env []string, networkRule string) (*exec.Cmd, error) {
	if !isolationAvailable() {
		return nil, ErrIsolationUnavailable
	}
	exe, err := exec.LookPath(argv[0])
	if err != nil {
		return nil, fmt.Errorf("evaltarget: resolve command %q: %w", argv[0], err)
	}
	if !filepath.IsAbs(exe) {
		exe, err = filepath.Abs(exe)
		if err != nil {
			return nil, err
		}
	}
	exeDir := filepath.Dir(exe)
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
		exeDir = filepath.Dir(exe)
	}
	var exeReads strings.Builder
	for p := exe; p != "/" && p != "."; p = filepath.Dir(p) {
		fmt.Fprintf(&exeReads, " (literal %q)", p)
	}
	fmt.Fprintf(&exeReads, " (literal %q)", exeDir)
	profile := fmt.Sprintf(`(version 1)
(deny default)
(allow process*)
(allow file-read* (literal "/") (subpath %q) (subpath "/usr") (subpath "/usr/local") (subpath "/System") (subpath "/Library") (subpath "/bin") (subpath "/usr/bin") (subpath "/usr/sbin") (subpath "/sbin") (subpath "/dev") (subpath "/etc") (subpath "/private/etc") (subpath "/private/tmp") (subpath "/private/var/folders") (subpath "/private/var/select") (subpath "/opt/homebrew")%s)
(allow file-write* (subpath %q) (subpath "/private/tmp") (subpath "/private/var/folders"))
(allow sysctl-read)
(allow mach-lookup)
%s
`, root, exeReads.String(), root, networkRule)
	args := append([]string{"-p", profile, "--"}, argv...)
	cmd := exec.CommandContext(ctx, sandboxExecPath, args...)
	cmd.Dir = dir
	cmd.Env = env
	return cmd, nil
}
