package security

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// gitleaksView is a staged, filtered source tree for one Gitleaks invocation.
// Gitleaks has no path-exclusion flag, so SOP builds a view containing only
// in-scope files. Excluded content is never linked into the view and is never
// read by the scanner. This also keeps excluded findings out of the report before
// the adapter's output bound is applied.
type gitleaksView struct {
	dir             string
	excludedFiles   int
	excludedDirs    int
	skippedSymlinks int
	skippedOther    int
}

// gitleaksSource is the repository source set Gitleaks may read. When scoped is
// true, files is the Git snapshot: tracked files plus untracked files that are
// not ignored. Ignored local files are absent. When scoped is false, the
// workspace is not a Git checkout and the adapter falls back to walking it.
type gitleaksSource struct {
	files  []string
	scoped bool
}

// stageGitleaksView stages a filtered source view for one Gitleaks invocation.
// A scoped source is staged from its exact file list, so ignored local files and
// nested worktree checkouts are never linked into the view. An unscoped source
// falls back to walking the workspace.
func stageGitleaksView(root, dir string, matcher ExcludeMatcher, source gitleaksSource) (gitleaksView, error) {
	view := gitleaksView{dir: dir}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return view, err
	}
	if source.scoped {
		return stageGitleaksFiles(root, dir, matcher, source.files, view)
	}
	return stageGitleaksWalk(root, dir, matcher, view)
}

// stageGitleaksFiles stages exactly the repository-source paths Git reported.
// A tracked file is staged even when an ignore rule matches it, so a committed
// .env remains a secret-scan target. An ignored untracked file is not reported
// by the snapshot and is never read.
func stageGitleaksFiles(root, dir string, matcher ExcludeMatcher, files []string, view gitleaksView) (gitleaksView, error) {
	for _, raw := range files {
		rel, _ := NormalizePath(raw)
		if rel == "" || !SafeRelativePath(rel) {
			view.skippedOther++
			continue
		}
		if matcher.Excluded(rel) {
			view.excludedFiles++
			continue
		}
		source := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Lstat(source)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// A tracked file deleted from the working tree has no content to scan.
				continue
			}
			return view, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			view.skippedSymlinks++
			continue
		}
		if !info.Mode().IsRegular() {
			view.skippedOther++
			continue
		}
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return view, err
		}
		if err := os.Link(source, target); err == nil {
			continue
		}
		if err := copyStagedFile(source, target, info.Mode().Perm()); err != nil {
			return view, err
		}
	}
	return view, nil
}

// stageGitleaksWalk stages every regular file under root that is inside the
// declared scope. It is the fallback for a workspace without Git metadata, where
// ignore rules and a source snapshot are unavailable.
func stageGitleaksWalk(root, dir string, matcher ExcludeMatcher, view gitleaksView) (gitleaksView, error) {
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if matcher.Excluded(relSlash) {
			if entry.IsDir() {
				view.excludedDirs++
				return filepath.SkipDir
			}
			view.excludedFiles++
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		target := filepath.Join(dir, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			view.skippedSymlinks++
			return nil
		}
		if !info.Mode().IsRegular() {
			view.skippedOther++
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.Link(path, target); err == nil {
			return nil
		}
		return copyStagedFile(path, target, info.Mode().Perm())
	})
	if err != nil {
		return view, fmt.Errorf("stage gitleaks view: %w", err)
	}
	return view, nil
}

func copyStagedFile(source, destination string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// viewRoots returns the view path forms Gitleaks may report: the literal temp
// path and its symlink-resolved form (macOS reports /private/var for /var).
func viewRoots(dir string) []string {
	roots := []string{dir}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil && resolved != dir {
		roots = append(roots, resolved)
	}
	return roots
}

// viewRelativePath maps a Gitleaks path back to a repository-relative path.
func viewRelativePath(file string, roots []string) (string, bool) {
	if !filepath.IsAbs(file) {
		path, _ := NormalizePath(file)
		return path, path != ""
	}
	for _, root := range roots {
		rel, err := filepath.Rel(root, file)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		path, _ := NormalizePath(filepath.ToSlash(rel))
		if path != "" {
			return path, true
		}
	}
	return "", false
}

// sanitizeViewPaths removes the temporary view root from a native report so the
// retained evidence never exposes a host-specific temporary path.
func sanitizeViewPaths(raw []byte, roots []string) []byte {
	for _, root := range roots {
		for _, prefix := range []string{filepath.ToSlash(root) + "/", root + string(filepath.Separator)} {
			raw = bytes.ReplaceAll(raw, []byte(prefix), nil)
		}
	}
	return raw
}
