package toolharness

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/git"
)

// MutationObservation separates tool success from independently verified state
// change. Changed can be true after a failed operation; that is diagnostic
// evidence of partial work, not successful mutation progress.
type MutationObservation struct {
	Succeeded bool
	Verified  bool
	Changed   bool
}

// RunObservedMutation executes a mutation-capable tool exactly once, comparing
// its canonical file target before/after execution. Commands have unknown targets
// and use a repository-tree comparison. Run's ordinary result/error contract is
// preserved; unavailable observation never supplies positive mutation evidence.
func (h *Harness) RunObservedMutation(ctx context.Context, name string, args map[string]any) (string, MutationObservation, error) {
	target := h.root
	var scopeErr error
	switch name {
	case ToolRunCommand:
		// A command's target is unknown; compare the whole repository tree.
	case ToolCopyFile:
		// The mutation is the destination file.
		var p resolvedPath
		dest, _ := args["dest"].(string)
		p, scopeErr = h.resolveArg(map[string]any{"path": dest}, "path", true, accessWrite)
		target = p.abs
	default:
		var p resolvedPath
		p, scopeErr = h.resolveArg(args, "path", true, accessWrite)
		target = p.abs
	}
	before, beforeErr := h.mutationFingerprint(ctx, target)
	var result string
	var err error
	succeeded := false
	if name == ToolRunCommand {
		result, succeeded, err = h.runCommandWithStatus(ctx, args)
	} else {
		result, err = h.dispatch(ctx, name, args)
		succeeded = err == nil
	}
	auditErr := err
	if auditErr == nil && !succeeded {
		auditErr = errors.New("command exited unsuccessfully")
	}
	after, afterErr := h.mutationFingerprint(ctx, target)
	// Audit output may itself live in the repository. Record it after comparison
	// so observer-owned writes cannot manufacture tool mutation evidence.
	h.audit(name, args, result, auditErr)
	verified := scopeErr == nil && beforeErr == nil && afterErr == nil
	return result, MutationObservation{
		Succeeded: succeeded,
		Verified:  verified,
		Changed:   verified && before != after,
	}, err
}

// mutationFingerprint is the content-complete counterpart of the existing
// working-tree fingerprint for operation-level verification. It needs neither
// Git nor HEAD. Named targets include existence/type/mode/content; command trees
// exclude Git metadata and SOP's runtime state. Timestamps and inode identity
// are deliberately absent. Symlinks in a tree are hashed without following them.
func (h *Harness) mutationFingerprint(ctx context.Context, target string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, h.cfg.CommandTimeout)
	defer cancel()
	if target == "" {
		return "", errors.New("mutation target unavailable")
	}
	sum := sha256.New()
	err := filepath.WalkDir(target, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if target == h.root && path != target && (filepath.Base(path) == ".git" || filepath.Base(path) == stateDirRel) {
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if walkErr != nil {
			if path == target && errors.Is(walkErr, fs.ErrNotExist) {
				_, err := io.WriteString(sum, "absent")
				return err
			}
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(target, path)
		if err != nil {
			return err
		}
		mode := info.Mode()
		fmt.Fprintf(sum, "%q\x00%d\x00", filepath.ToSlash(rel), mode.Type()|mode.Perm()|mode&(fs.ModeSetuid|fs.ModeSetgid|fs.ModeSticky))
		switch {
		case mode.IsDir():
			// File tools never mutate directory contents. In particular, a failed
			// write to .agent-sdlc must not inspect the protected database inside it.
			if target != h.root {
				return filepath.SkipDir
			}
			return nil
		case mode&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(sum, "%q\x00", link)
			return nil
		case mode.IsRegular():
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			content := sha256.New()
			_, copyErr := io.Copy(content, mutationReader{ctx: ctx, reader: file})
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			sum.Write(content.Sum(nil))
			return nil
		default:
			return errors.New("unsupported file type in mutation observation")
		}
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sum.Sum(nil)), nil
}

type mutationReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r mutationReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

// RepositoryFingerprint observes repository content without Git metadata or SOP
// runtime state. An unavailable observation must be treated as unknown.
func (h *Harness) RepositoryFingerprint(ctx context.Context) (string, error) {
	return h.mutationFingerprint(ctx, h.root)
}

// ErrSnapshotUnavailable reports that a complete repository snapshot could not
// be taken. Callers must treat this as unknown mutation evidence, never as an
// empty snapshot that proves nothing changed.
var ErrSnapshotUnavailable = errors.New("repository snapshot unavailable")

// RepositoryPathFingerprints identifies individual file mutations across an
// invocation using Git-aware file discovery: every tracked file plus every
// untracked file that is not ignored. Ignored generated output is not hashed,
// while tracked-but-ignored files remain part of the snapshot. It excludes
// Git/SOP metadata, does not follow symlinks, and ignores directory entries so
// creating a parent directory for an excluded artifact cannot count as an
// implementation change.
//
// Discovery failure (not a Git working tree, or Git unavailable) is an error,
// not an empty snapshot, so an incomplete observation can never look successful.
func (h *Harness) RepositoryPathFingerprints(ctx context.Context) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, h.cfg.CommandTimeout)
	defer cancel()
	files, err := git.New(h.root).SnapshotFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSnapshotUnavailable, err)
	}
	paths := make(map[string]string, len(files))
	for _, listed := range files {
		rel, abs, err := h.snapshotPath(listed)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrSnapshotUnavailable, err)
		}
		if snapshotExcluded(rel) {
			continue
		}
		fingerprint, err := h.mutationFingerprint(ctx, abs)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrSnapshotUnavailable, rel, err)
		}
		paths[rel] = fingerprint
	}
	return paths, nil
}

// snapshotPath validates one Git-listed path and returns its repository-relative
// slash form and absolute path. Git does not normally emit paths outside the
// work tree, but containment is still verified so a corrupt or hostile listing
// cannot widen the observation boundary. The parent is resolved (not the leaf),
// so a symlinked directory cannot smuggle the read outside the repository while
// a symlink file itself is still observed as a link rather than dereferenced.
func (h *Harness) snapshotPath(listed string) (string, string, error) {
	if listed == "" || strings.ContainsRune(listed, 0) {
		return "", "", errors.New("snapshot path is empty or contains a NUL byte")
	}
	if filepath.IsAbs(listed) {
		return "", "", fmt.Errorf("snapshot path is absolute: %q", listed)
	}
	clean := filepath.Clean(filepath.FromSlash(listed))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("snapshot path escapes the repository: %q", listed)
	}
	abs := filepath.Join(h.root, clean)
	if !pathWithin(abs, h.root) {
		return "", "", fmt.Errorf("snapshot path escapes the repository: %q", listed)
	}
	parent, err := evalExisting(filepath.Dir(abs))
	if err != nil {
		return "", "", fmt.Errorf("resolve snapshot path %q: %w", listed, err)
	}
	if !pathWithin(parent, h.root) {
		return "", "", fmt.Errorf("snapshot path escapes the repository: %q", listed)
	}
	return filepath.ToSlash(clean), abs, nil
}

// snapshotExcluded reports whether rel is Git metadata or SOP's runtime state.
func snapshotExcluded(rel string) bool {
	return rel == ".git" || strings.HasPrefix(rel, ".git/") ||
		rel == stateDirRel || strings.HasPrefix(rel, stateDirRel+"/")
}

// RunCommandChecked preserves the command result while exposing its real exit
// status independently of display text. It executes and audits exactly once.
func (h *Harness) RunCommandChecked(ctx context.Context, args map[string]any) (string, bool, error) {
	result, succeeded, err := h.runCommandWithStatus(ctx, args)
	auditErr := err
	if auditErr == nil && !succeeded {
		auditErr = errors.New("command exited unsuccessfully")
	}
	h.audit(ToolRunCommand, args, result, auditErr)
	return result, succeeded, err
}
