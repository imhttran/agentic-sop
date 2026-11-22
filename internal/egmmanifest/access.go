package egmmanifest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrUnsupported is returned when a check cannot be performed on this platform.
var ErrUnsupported = errors.New("egmmanifest: file permission checks are unsupported on this platform")

// CheckOutside verifies that target is not equal to or inside any forbidden root.
// Symlinks are resolved, including the parent of a path that does not exist yet, so a
// symlink cannot hide the real location.
func CheckOutside(target string, forbidden ...string) error {
	if strings.TrimSpace(target) == "" {
		return errors.New("egmmanifest: empty path")
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	realTarget, err := resolveMaybe(absTarget)
	if err != nil {
		return err
	}
	for _, f := range forbidden {
		if strings.TrimSpace(f) == "" {
			continue
		}
		absF, err := filepath.Abs(f)
		if err != nil {
			return err
		}
		realF, err := resolveMaybe(absF)
		if err != nil {
			return err
		}
		if within(realTarget, realF) {
			return fmt.Errorf("egmmanifest: path %s is inside forbidden root %s", absTarget, absF)
		}
	}
	return nil
}

func within(p, root string) bool {
	cleanP := filepath.Clean(p)
	cleanRoot := filepath.Clean(root)
	return cleanP == cleanRoot || strings.HasPrefix(cleanP, cleanRoot+string(filepath.Separator))
}

func resolveMaybe(p string) (string, error) {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r, nil
	}
	dir, base := filepath.Split(p)
	if dir == "" {
		return filepath.Clean(p), nil
	}
	r, err := filepath.EvalSymlinks(filepath.Clean(dir))
	if err != nil {
		return filepath.Clean(p), nil
	}
	return filepath.Join(r, base), nil
}

// CheckFileMode verifies that path is a regular file (not a symlink) with no group or
// other permissions. If want is non-zero, the exact permission bits must match.
//
// This is a portable Unix check. On Windows the mode bits are not meaningful and the
// function returns ErrUnsupported; the experiment targets macOS and Linux.
func CheckFileMode(path string, want os.FileMode) error {
	if runtime.GOOS == "windows" {
		return ErrUnsupported
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("egmmanifest: %s is a symlink", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("egmmanifest: %s is not a regular file", path)
	}
	got := info.Mode().Perm()
	if got&0o077 != 0 {
		return fmt.Errorf("egmmanifest: %s mode %04o grants group/other access", path, got)
	}
	if want != 0 && got != want {
		return fmt.Errorf("egmmanifest: %s mode %04o, want %04o", path, got, want)
	}
	return nil
}

// CheckDirMode verifies that path is a real directory (not a symlink) with no group or
// other permissions.
func CheckDirMode(path string, want os.FileMode) error {
	if runtime.GOOS == "windows" {
		return ErrUnsupported
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("egmmanifest: %s is a symlink", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("egmmanifest: %s is not a directory", path)
	}
	got := info.Mode().Perm()
	if got&0o077 != 0 {
		return fmt.Errorf("egmmanifest: %s mode %04o grants group/other access", path, got)
	}
	if want != 0 && got != want {
		return fmt.Errorf("egmmanifest: %s mode %04o, want %04o", path, got, want)
	}
	return nil
}
