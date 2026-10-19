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

	profile := fmt.Sprintf(`(version 1)
(allow default)
(deny network*)
(deny file-write*)
(allow file-write* (subpath %q))
(allow file-write-data (literal "/dev/null"))
`, dir)

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := append([]string{"-p", profile}, f.VerificationCommand...)
	cmd := exec.CommandContext(runCtx, iso.Tool, args...)
	cmd.Dir = dir
	// A scrubbed environment: no API keys, tokens, or provider configuration.
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + dir, "TMPDIR=" + dir}
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
