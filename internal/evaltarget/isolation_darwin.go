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
// ordinary workspace and the user's home are not readable.
func isolationCommand(ctx context.Context, root, dir string, argv, env []string) (*exec.Cmd, error) {
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
(deny network*)
`, root, exeReads.String(), root)
	args := append([]string{"-p", profile, "--"}, argv...)
	cmd := exec.CommandContext(ctx, sandboxExecPath, args...)
	cmd.Dir = dir
	cmd.Env = env
	return cmd, nil
}
