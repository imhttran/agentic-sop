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
	if name != ToolRunCommand {
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
