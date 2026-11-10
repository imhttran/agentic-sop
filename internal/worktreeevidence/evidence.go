// Package worktreeevidence preserves and verifies the SOP lifecycle evidence a
// worktree holds before an SOP-managed removal.
//
// The archive layout reuses the repository's existing worktree-evidence
// convention: <archiveRoot>/<plan-id>/ contains the worktree's .agent-sdlc
// contents. The caller supplies archiveRoot; the Git adapter derives it from the
// main repository's .agent-sdlc/archive/worktree-evidence directory.
//
// This package protects SOP-managed removal only. A direct `git worktree remove`
// bypasses it and must perform the same evidence preservation manually.
package worktreeevidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/durablefs"
)

const (
	// StateDirName is SOP's per-project state directory.
	StateDirName = ".agent-sdlc"
	// ArchiveDirName is the existing worktree-evidence archive directory name.
	ArchiveDirName = "worktree-evidence"
)

// ErrArchiveInsideWorktree is returned when the requested archive location is
// inside the worktree being removed.
var ErrArchiveInsideWorktree = errors.New("worktree evidence archive must be outside the worktree")

type planMeta struct {
	PlanID string `json:"plan_id"`
}

// HasLifecycleEvidence reports whether worktree contains ignored SOP lifecycle
// state that must be preserved before removal. Tracked configuration and the
// security baseline alone are not lifecycle evidence.
func HasLifecycleEvidence(worktree string) (bool, error) {
	stateDir := filepath.Join(worktree, StateDirName)
	info, err := os.Lstat(stateDir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("worktreeevidence: %s is not a directory", stateDir)
	}
	if _, err := os.Lstat(filepath.Join(stateDir, "state.db")); err == nil {
		return true, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	for _, name := range []string{"runs", "archive", "context", "memory"} {
		entries, err := os.ReadDir(filepath.Join(stateDir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return false, err
		}
		if len(entries) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// ArchivePath returns the conventional archive destination for worktree under
// archiveRoot. The identity is the worktree's recorded plan id, or its directory
// name when no plan id is recorded.
func ArchivePath(worktree, archiveRoot string) (string, error) {
	identity, err := archiveIdentity(worktree)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(archiveRoot)
	if err != nil {
		return "", err
	}
	archive := filepath.Join(root, identity)
	outside, err := outside(worktree, archive)
	if err != nil {
		return "", err
	}
	if !outside {
		return "", fmt.Errorf("%w: %s", ErrArchiveInsideWorktree, archive)
	}
	return archive, nil
}

// Preserve copies the worktree's .agent-sdlc contents to the conventional
// archive and verifies the copy. It is idempotent for a matching archive and
// refuses to overwrite an existing archive that does not match.
func Preserve(worktree, archiveRoot string) (string, error) {
	has, err := HasLifecycleEvidence(worktree)
	if err != nil {
		return "", err
	}
	if !has {
		return "", nil
	}
	archive, err := ArchivePath(worktree, archiveRoot)
	if err != nil {
		return "", err
	}
	src := filepath.Join(worktree, StateDirName)
	if info, err := os.Stat(archive); err == nil {
		if !info.IsDir() {
			return "", fmt.Errorf("worktreeevidence: archive %s is not a directory", archive)
		}
		if err := verifyTree(src, archive); err != nil {
			return "", fmt.Errorf("worktreeevidence: archive %s exists and does not match: %w", archive, err)
		}
		return archive, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(archive), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(archive), "."+filepath.Base(archive)+".tmp-")
	if err != nil {
		return "", err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(tmp)
		}
	}()
	if err := copyTree(src, tmp); err != nil {
		return "", fmt.Errorf("worktreeevidence: preserve %s: %w", src, err)
	}
	if err := verifyTree(src, tmp); err != nil {
		return "", fmt.Errorf("worktreeevidence: verify preserved %s: %w", src, err)
	}
	if err := os.Rename(tmp, archive); err != nil {
		return "", fmt.Errorf("worktreeevidence: publish archive %s: %w", archive, err)
	}
	keep = true
	return archive, nil
}

// Verify checks that every file in the worktree's .agent-sdlc is present with
// identical content in the conventional archive. It returns nil when the
// worktree holds no lifecycle evidence. The archive must be outside the worktree
// and must not contain symlinks: a link back into the worktree would satisfy a
// checksum without being an independent copy.
func Verify(worktree, archiveRoot string) error {
	has, err := HasLifecycleEvidence(worktree)
	if err != nil {
		return err
	}
	if !has {
		return nil
	}
	archive, err := ArchivePath(worktree, archiveRoot)
	if err != nil {
		return err
	}
	if err := verifyTree(filepath.Join(worktree, StateDirName), archive); err != nil {
		return fmt.Errorf("worktreeevidence: archive %s: %w", archive, err)
	}
	return nil
}

func archiveIdentity(worktree string) (string, error) {
	metaPath := filepath.Join(worktree, StateDirName, "plan.meta.json")
	if info, err := os.Lstat(metaPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("worktreeevidence: %s must be a regular file", metaPath)
		}
		data, err := os.ReadFile(metaPath)
		if err != nil {
			return "", err
		}
		var meta planMeta
		if json.Unmarshal(data, &meta) == nil {
			id := strings.TrimSpace(meta.PlanID)
			if id != "" {
				if err := safeIdentity(id); err != nil {
					return "", err
				}
				return id, nil
			}
		}
	}
	base := filepath.Base(filepath.Clean(worktree))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "", errors.New("worktreeevidence: cannot derive an archive identity")
	}
	if err := safeIdentity(base); err != nil {
		return "", err
	}
	return base, nil
}

func safeIdentity(id string) error {
	if id == "." || id == ".." || strings.ContainsAny(id, `/\\`) {
		return fmt.Errorf("worktreeevidence: unsafe archive identity %q", id)
	}
	return nil
}

// outside reports whether archive is outside worktree, resolving existing
// symlinks where possible.
func outside(worktree, archive string) (bool, error) {
	work := canonical(worktree)
	arch := canonical(archive)
	rel, err := filepath.Rel(work, arch)
	if err != nil {
		return false, err
	}
	inside := rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
	return !inside, nil
}

func canonical(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	if real, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		return filepath.Join(real, filepath.Base(path))
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("unsupported symlink in SOP state: %s", path)
		case info.Mode().IsRegular():
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if err := durablefs.WriteFile(target, data); err != nil {
				return err
			}
			return os.Chmod(target, info.Mode().Perm())
		default:
			return fmt.Errorf("unsupported file type in SOP state: %s", path)
		}
	})
}

func verifyTree(src, dst string) error {
	if err := rejectArchiveLinks(dst); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			ti, err := os.Lstat(target)
			if err != nil {
				return err
			}
			if !ti.IsDir() {
				return fmt.Errorf("%s is not a directory", target)
			}
			return nil
		case info.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("unsupported symlink in SOP state: %s", path)
		case info.Mode().IsRegular():
			want, err := digestFile(path)
			if err != nil {
				return err
			}
			got, err := digestFile(target)
			if err != nil {
				return err
			}
			if got != want {
				return fmt.Errorf("%s does not match %s", target, path)
			}
			return nil
		default:
			return fmt.Errorf("unsupported file type in SOP state: %s", path)
		}
	})
}

// rejectArchiveLinks refuses an archive tree that contains symlinks or special
// files. Verification must read an independent copy; a symlink can point back
// into the worktree and satisfy a checksum while leaving no preserved evidence.
func rejectArchiveLinks(root string) error {
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("archive root %s is a symlink", root)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", root)
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("archive contains a symlink: %s", path)
		}
		if !d.IsDir() && !d.Type().IsRegular() {
			return fmt.Errorf("archive contains an unsupported file: %s", path)
		}
		return nil
	})
}

func digestFile(path string) (string, error) {
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
