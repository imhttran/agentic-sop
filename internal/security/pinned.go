package security

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// pinnedExecutor checks an adapter's optional binary pin immediately before
// every execution, then delegates. A failed check returns an
// error before any process starts, so the adapter reports a non-clean failure.
// On success the observed binary is the pinned one, and the pin is part of the
// adapter config digest that provenance records.
// The check copies the resolved binary into a private directory while hashing
// the same bytes, then executes that copy. A writer racing the original cannot
// change what runs.
// shortcut: each scan copies the scanner binary once, which costs time on large
// binaries; a binary on a noexec temp mount fails to start (ErrUnavailable).
// Upgrade: execute an opened descriptor where the platform allows it.
type pinnedExecutor struct {
	inner        Executor
	binarySHA256 string
}

func (p pinnedExecutor) Run(ctx context.Context, dir string, argv []string, stdin []byte, maxOut int) ([]byte, []byte, int, error) {
	argv, cleanup, err := p.check(dir, argv)
	if err != nil {
		return nil, nil, -1, err
	}
	defer cleanup()
	return p.inner.Run(ctx, dir, argv, stdin, maxOut)
}

func (p pinnedExecutor) RunEnv(ctx context.Context, dir string, argv []string, env []string, stdin []byte, maxOut int) ([]byte, []byte, int, error) {
	argv, cleanup, err := p.check(dir, argv)
	if err != nil {
		return nil, nil, -1, err
	}
	defer cleanup()
	inner, ok := p.inner.(EnvExecutor)
	if !ok {
		return nil, nil, -1, errors.New("executor does not support an explicit environment")
	}
	return inner.RunEnv(ctx, dir, argv, env, stdin, maxOut)
}

// check verifies the binary pin and returns argv with argv[0] replaced by a
// private copy of the verified bytes, plus a cleanup that removes the copy.
func (p pinnedExecutor) check(dir string, argv []string) ([]string, func(), error) {
	noop := func() {}
	if p.binarySHA256 != "" { // BuildAdapter wraps only when this is set
		if len(argv) == 0 {
			return nil, noop, errors.New("security: empty command")
		}
		// The child runs with Dir=dir, so a relative path with a separator is
		// relative to dir, not to this process's working directory.
		name := argv[0]
		if strings.ContainsRune(name, filepath.Separator) && !filepath.IsAbs(name) {
			name = filepath.Join(dir, name)
		}
		path, err := exec.LookPath(name)
		if err != nil {
			return nil, noop, fmt.Errorf("binary pin: %w", ErrUnavailable)
		}
		if path, err = filepath.Abs(path); err != nil {
			return nil, noop, fmt.Errorf("binary pin: %w", ErrUnavailable)
		}
		staged, cleanup, err := stageVerified(path, p.binarySHA256)
		if err != nil {
			return nil, noop, err
		}
		return append([]string{staged}, argv[1:]...), cleanup, nil
	}
	return argv, noop, nil
}

// stageVerified copies src into a new private directory while hashing the same
// bytes, and fails if the copy's digest is not want. The copy is what gets executed.
func stageVerified(src, want string) (string, func(), error) {
	in, err := os.Open(src)
	if err != nil {
		return "", nil, fmt.Errorf("binary pin: cannot read the resolved executable: %w", ErrUnavailable)
	}
	defer in.Close()
	dir, err := os.MkdirTemp("", "sop-pinned-")
	if err != nil {
		return "", nil, fmt.Errorf("binary pin: %w", ErrUnavailable)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	dst := filepath.Join(dir, filepath.Base(src))
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("binary pin: %w", ErrUnavailable)
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), in); err != nil {
		_ = out.Close()
		cleanup()
		return "", nil, fmt.Errorf("binary pin: cannot copy the resolved executable: %w", ErrUnavailable)
	}
	if err := out.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("binary pin: %w", ErrUnavailable)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		cleanup()
		return "", nil, fmt.Errorf("binary pin mismatch: the resolved executable is not the pinned binary (sha256 %s)", got)
	}
	return dst, cleanup, nil
}

// checkRulesPin verifies an optional rules pin against the exact resolved
// rules path an adapter passes to its scanner and that path's identity. It
// refuses a tree the digest cannot see: rulesIdentity skips non-regular
// entries, but a scanner would follow a symlink to unhashed content.
func checkRulesPin(rules, identity, pin string) error {
	if pin == "" {
		return nil
	}
	if _, err := regularTree(rules); err != nil {
		return fmt.Errorf("rules pin: %s", SanitizeText(err.Error(), MaxDiagnosticBytes))
	}
	if identity != pin {
		return fmt.Errorf("rules pin mismatch: rules digest %s is not the pinned digest", identity)
	}
	return nil
}

// regularTree returns an error if path is a symlink or contains any entry that
// is neither a directory nor a regular file.
func regularTree(path string) (string, error) {
	return "", filepath.WalkDir(path, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && !entry.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file", filepath.Base(p))
		}
		return nil
	})
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
