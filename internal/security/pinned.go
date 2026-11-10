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
// ponytail: the hash is taken on the resolved absolute path and the inner
// executor runs that same path, so a writer racing between the two is not
// excluded; executing an opened file descriptor would close that window.
type pinnedExecutor struct {
	inner        Executor
	binarySHA256 string
}

func (p pinnedExecutor) Run(ctx context.Context, dir string, argv []string, stdin []byte, maxOut int) ([]byte, []byte, int, error) {
	argv, err := p.check(dir, argv)
	if err != nil {
		return nil, nil, -1, err
	}
	return p.inner.Run(ctx, dir, argv, stdin, maxOut)
}

func (p pinnedExecutor) RunEnv(ctx context.Context, dir string, argv []string, env []string, stdin []byte, maxOut int) ([]byte, []byte, int, error) {
	argv, err := p.check(dir, argv)
	if err != nil {
		return nil, nil, -1, err
	}
	inner, ok := p.inner.(EnvExecutor)
	if !ok {
		return nil, nil, -1, errors.New("executor does not support an explicit environment")
	}
	return inner.RunEnv(ctx, dir, argv, env, stdin, maxOut)
}

// check verifies the binary pin and returns argv with argv[0] replaced by the
// absolute path that was hashed, so the executed file is the checked file.
func (p pinnedExecutor) check(dir string, argv []string) ([]string, error) {
	if p.binarySHA256 != "" { // BuildAdapter wraps only when this is set
		if len(argv) == 0 {
			return nil, errors.New("security: empty command")
		}
		// The child runs with Dir=dir, so a relative path with a separator is
		// relative to dir, not to this process's working directory.
		name := argv[0]
		if strings.ContainsRune(name, filepath.Separator) && !filepath.IsAbs(name) {
			name = filepath.Join(dir, name)
		}
		path, err := exec.LookPath(name)
		if err != nil {
			return nil, fmt.Errorf("binary pin: %w", ErrUnavailable)
		}
		if path, err = filepath.Abs(path); err != nil {
			return nil, fmt.Errorf("binary pin: %w", ErrUnavailable)
		}
		got, err := fileSHA256(path)
		if err != nil {
			return nil, fmt.Errorf("binary pin: cannot read the resolved executable: %w", ErrUnavailable)
		}
		if got != p.binarySHA256 {
			return nil, fmt.Errorf("binary pin mismatch: the resolved executable is not the pinned binary (sha256 %s)", got)
		}
		argv = append([]string{path}, argv[1:]...)
	}
	return argv, nil
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
