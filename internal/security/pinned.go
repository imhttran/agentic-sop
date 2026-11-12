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
	"sort"
	"strings"
)

// pinnedExecutor checks an adapter's optional binary pin immediately before
// every execution, then delegates. A failed check returns an
// error before any process starts, so the adapter reports a non-clean failure.
// On success the observed binary is the pinned one, and the pin is part of the
// adapter config digest that provenance records.
// The check copies the resolved binary into a private directory while hashing
// the same bytes, then executes that copy. A writer racing the original cannot
// change what runs. A configured rules pin is staged the same way: the scanner
// reads a verified private copy, not the writable original.
// shortcut: each scan copies the scanner binary and, when pinned, the rules tree;
// this costs time on large inputs; a binary on a noexec temp mount fails to start
// (ErrUnavailable).
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

// stageRules copies a rules file or directory into a private directory while
// computing the same digest rulesIdentity produces, and fails unless the copy's
// digest is want. The copy is what the scanner reads, so a writer racing the
// original cannot change the rules that are loaded. It returns the digest of the
// staged bytes, which provenance records as the observed rules identity.
//
// A symlinked rules root or any symlink or non-regular entry inside the tree is
// refused. rulesIdentity skips entries it cannot hash, but a scanner would follow
// a symlink to unhashed content, so the pin must not be satisfiable that way.
//
// The staged tree lives in a 0700 directory owned by this process. A writer with
// the same user identity that can reach that directory is outside the pin's trust
// boundary: the pin protects the writable acquisition/workspace path.
func stageRules(src, want string) (string, string, func(), error) {
	info, err := os.Lstat(src)
	if err != nil {
		return "", "", nil, fmt.Errorf("rules pin: %s", SanitizeText(err.Error(), MaxDiagnosticBytes))
	}
	if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
		return "", "", nil, fmt.Errorf("rules pin: %s is not a regular file", filepath.Base(src))
	}
	dir, err := os.MkdirTemp("", "sop-rules-")
	if err != nil {
		return "", "", nil, fmt.Errorf("rules pin: %w", ErrUnavailable)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	h := sha256.New()
	staged := dir
	if !info.IsDir() {
		// Keep the base name so the scanner sees the same file name it was given.
		// A scanner's --config accepts a file for a file rules path, so return the
		// staged file, not its containing directory.
		staged = filepath.Join(dir, filepath.Base(src))
		if err := copyHashedRule(src, staged, h); err != nil {
			cleanup()
			return "", "", nil, fmt.Errorf("rules pin: %s", SanitizeText(err.Error(), MaxDiagnosticBytes))
		}
	} else {
		files, err := regularRuleFiles(src)
		if err != nil {
			cleanup()
			return "", "", nil, fmt.Errorf("rules pin: %s", SanitizeText(err.Error(), MaxDiagnosticBytes))
		}
		// rulesIdentity hashes sorted slash-separated relative names with NUL
		// framing. WalkDir order is lexical per directory but not globally, so sort
		// the collected set. TestRulesPinStagesVerifiedTree pins this coupling with a
		// tree whose sorted order differs from WalkDir order.
		sort.Strings(files)
		for _, rel := range files {
			_, _ = io.WriteString(h, rel+"\x00")
			dst := filepath.Join(dir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
				cleanup()
				return "", "", nil, fmt.Errorf("rules pin: %s", SanitizeText(err.Error(), MaxDiagnosticBytes))
			}
			if err := copyHashedRule(filepath.Join(src, filepath.FromSlash(rel)), dst, h); err != nil {
				cleanup()
				return "", "", nil, fmt.Errorf("rules pin: %s", SanitizeText(err.Error(), MaxDiagnosticBytes))
			}
			_, _ = io.WriteString(h, "\x00")
		}
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		cleanup()
		return "", "", nil, fmt.Errorf("rules pin mismatch: rules digest %s is not the pinned digest", got)
	}
	return staged, got, cleanup, nil
}

// regularRuleFiles returns the slash-separated relative paths of every regular
// file under root. A symlink or other non-regular entry is an error: the scanner
// would follow it to content the digest does not cover.
func regularRuleFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file", filepath.Base(p))
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	return files, err
}

// copyHashedRule copies one rule file and feeds exactly the copied bytes to h.
func copyHashedRule(src, dst string, h io.Writer) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(io.MultiWriter(out, h), in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
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
