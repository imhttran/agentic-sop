package dqharness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ReplayFixture is the auditable input for one executable case. Every field is
// required: a fixture without provenance is not evidence.
type ReplayFixture struct {
	CaseID              string            `json:"case_id"`
	Revision            string            `json:"starting_revision"`
	TaskInput           string            `json:"task_input"`
	VerificationCommand []string          `json:"verification_command"`
	Provenance          string            `json:"provenance"`
	Files               map[string]string `json:"files,omitempty"`
}

// Validate rejects an incomplete fixture. It is the guard that prevents an invented or
// partial replay from being scored.
func (f ReplayFixture) Validate() error {
	switch {
	case strings.TrimSpace(f.CaseID) == "":
		return errors.New("replay fixture: case_id is required")
	case strings.TrimSpace(f.Revision) == "":
		return errors.New("replay fixture: starting_revision is required")
	case strings.TrimSpace(f.TaskInput) == "":
		return errors.New("replay fixture: task_input is required")
	case len(f.VerificationCommand) == 0:
		return errors.New("replay fixture: verification_command is required")
	case strings.TrimSpace(f.Provenance) == "":
		return errors.New("replay fixture: provenance is required")
	}
	return nil
}

// minimalReplayEnv is the scrubbed child environment: no API keys, tokens, or provider
// configuration. Both backends reproduce this environment inside the sandbox.
func minimalReplayEnv(dir string) []string {
	return []string{
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
		"HOME=" + dir,
		"TMPDIR=" + dir,
	}
}

// isolationCommand builds the sandboxed command line for the detected backend. It returns
// the full argument list to pass to iso.Tool and the child environment. No backend
// returns an unsandboxed command: an unknown backend is an error, and the bubblewrap
// backend returns its canonicalization error so the child is never launched with a raw
// path.
func isolationCommand(backend IsolationBackend, dir string, target []string) (args, env []string, err error) {
	env = minimalReplayEnv(dir)
	switch backend {
	case BackendSeatbelt:
		args = seatbeltCommand(dir, target)
		return args, env, nil
	case BackendBubblewrap:
		_, args, err = bubblewrapCommand(dir, target)
		if err != nil {
			return nil, nil, err
		}
		return args, env, nil
	default:
		return nil, nil, fmt.Errorf("replay isolation unavailable: unknown backend %q", backend)
	}
}

// seatbeltProfile builds the macOS sandbox-exec Seatbelt profile. It denies network and
// all writes except the replay directory and /dev/null.
func seatbeltProfile(dir string) string {
	return fmt.Sprintf(`(version 1)
(allow default)
(deny network*)
(deny file-write*)
(allow file-write* (subpath %q))
(allow file-write-data (literal "/dev/null"))
`, dir)
}

// seatbeltCommand builds the macOS sandbox-exec arguments: the Seatbelt profile passed with
// -p, followed by the verification command.
func seatbeltCommand(dir string, target []string) []string {
	args := []string{"-p", seatbeltProfile(dir)}
	return append(args, target...)
}

// bubblewrapCommand builds the Linux bubblewrap arguments. It binds the whole filesystem
// read-only and makes only the replay directory writable, so ordinary file writes
// outside the replay directory fail.
//
// It deliberately does NOT use --dev or --proc. --dev mounts a fresh writable tmpfs over
// /dev, which shadows the read-only host /dev (and /dev/shm) and lets writes there
// succeed inside a private mount. Host immutability is not the same as confinement: the
// contract requires the write itself to fail, even when it lands in a private mount.
// Keeping the host /dev read-only means writes under /dev, including /dev/shm, fail.
//
// The replay directory is bound writable by its canonical path after the read-only root,
// so the writable bind is layered on top of the read-only bind. The command changes into
// the replay directory, unshares the network and other namespaces, clears the
// environment and re-adds only the scrubbed variables, then terminates option parsing
// with -- before the verification command. It returns the bound replay path, the
// argument list, and an error.
//
// It fails closed: when filepath.EvalSymlinks cannot canonicalize the replay directory it
// returns an actionable error and no arguments, so the caller never launches a child with
// a raw, uncanonicalized path.
func bubblewrapCommand(dir string, target []string) (bound string, args []string, err error) {
	bound, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return "", nil, fmt.Errorf("replay isolation: cannot canonicalize replay directory %q: %w", dir, err)
	}
	args = []string{
		"--ro-bind", "/", "/",
		"--bind", bound, bound,
		"--chdir", bound,
		"--unshare-net",
		"--unshare-pid",
		"--unshare-ipc",
		"--unshare-uts",
		"--die-with-parent",
		"--new-session",
		"--clearenv",
		"--setenv", "PATH", "/usr/bin:/bin:/usr/sbin:/sbin",
		"--setenv", "HOME", bound,
		"--setenv", "TMPDIR", bound,
		"--",
	}
	return bound, append(args, target...), nil
}

// IsolatedReplay runs one fixture under OS-level isolation and returns the verification
// exit code. Controls: a fresh temporary directory, a scrubbed environment (no
// credentials), denied network, writes confined to the temporary directory, a timeout,
// and unconditional cleanup.
func IsolatedReplay(ctx context.Context, parent string, f ReplayFixture, timeout time.Duration) (exitCode int, output string, err error) {
	if err := f.Validate(); err != nil {
		return -1, "", err
	}
	iso := DetectIsolation()
	if !iso.Available {
		return -1, "", fmt.Errorf("replay isolation unavailable: %s", iso.Reason)
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	dir, err := os.MkdirTemp(parent, "rdm004-replay-")
	if err != nil {
		return -1, "", err
	}
	defer os.RemoveAll(dir)

	for rel, content := range f.Files {
		clean := filepath.Clean(rel)
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			return -1, "", fmt.Errorf("replay fixture: unsafe file path %q", rel)
		}
		target := filepath.Join(dir, clean)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return -1, "", err
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return -1, "", err
		}
	}

	args, cmdEnv, err := isolationCommand(iso.Backend, dir, f.VerificationCommand)
	if err != nil {
		return -1, "", err
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, iso.Tool, args...)
	cmd.Dir = dir
	// A scrubbed environment: no API keys, tokens, or provider configuration.
	// The backend builders also re-create this reduced environment inside the
	// sandbox, so this slice is defense in depth.
	cmd.Env = cmdEnv
	out, runErr := cmd.CombinedOutput()
	if len(out) > 64*1024 {
		out = out[:64*1024]
	}
	if runCtx.Err() != nil {
		return -1, string(out), fmt.Errorf("replay timed out after %s", timeout)
	}
	if runErr != nil {
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			return ee.ExitCode(), string(out), nil
		}
		return -1, string(out), runErr
	}
	return 0, string(out), nil
}
