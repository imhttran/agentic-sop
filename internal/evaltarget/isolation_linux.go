//go:build linux

package evaltarget

import (
	"context"
	"os/exec"
	"path/filepath"
)

func isolationAvailable() bool {
	_, err := exec.LookPath("bwrap")
	return err == nil
}

func isolationKind() string { return "bwrap" }

// isolationCommand runs argv under bubblewrap with a private mount and network
// namespace. Only the case root is writable; the rest of the system is read-only.
func isolationCommand(ctx context.Context, root, dir string, argv, env []string) (*exec.Cmd, error) {
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		return nil, ErrIsolationUnavailable
	}
	args := []string{
		"--die-with-parent",
		"--unshare-all",
		"--new-session",
		"--proc", "/proc",
		"--dev", "/dev",
		"--bind", root, root,
		"--chdir", dir,
	}
	for _, ro := range []string{"/usr", "/bin", "/sbin", "/lib", "/lib64", "/etc"} {
		if _, err := filepath.EvalSymlinks(ro); err == nil {
			args = append(args, "--ro-bind", ro, ro)
		}
	}
	args = append(args, "--")
	args = append(args, argv...)
	cmd := exec.CommandContext(ctx, bwrap, args...)
	cmd.Dir = dir
	cmd.Env = env
	return cmd, nil
}
