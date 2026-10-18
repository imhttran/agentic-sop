package security

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/git"
)

// ComputeContentIdentity records the exact committed and working-tree content a
// scan observes. Git is the authority when the workspace is a repository. A
// non-repository workspace falls back to a deterministic file-tree digest and is
// marked dirty, because no commit/tree identity exists.
func ComputeContentIdentity(ctx context.Context, root string) (ContentIdentity, error) {
	repo := git.New(root)
	status, err := repo.Status(ctx)
	if err != nil {
		digest, derr := workspaceDigest(root)
		if derr != nil {
			return ContentIdentity{}, derr
		}
		return ContentIdentity{Digest: digest, Dirty: true}, nil
	}
	identity := ContentIdentity{Dirty: status == git.Dirty}
	if head, herr := repo.Head(ctx); herr == nil {
		identity.Commit = head
	}
	if tree, terr := repo.Tree(ctx); terr == nil {
		identity.Tree = tree
	}
	if identity.Dirty {
		diff, derr := repo.DiffAll(ctx)
		if derr != nil {
			return ContentIdentity{}, fmt.Errorf("security: content identity: %w", derr)
		}
		identity.Digest = DigestStrings(identity.Commit, identity.Tree, diff)
	}
	return identity, nil
}

// ComputeScope resolves the requested scope. Workspace is the default. A
// changed-files scope lists tracked and untracked changes and excludes SOP's own
// state and Git metadata, so the optimization never scans its own evidence.
func ComputeScope(ctx context.Context, root, mode string) (Scope, error) {
	if strings.TrimSpace(mode) == "" || mode == "workspace" {
		return Scope{Mode: "workspace"}, nil
	}
	if mode != "changed-files" {
		return Scope{}, fmt.Errorf("security: unsupported scope %q", mode)
	}
	files, err := git.New(root).ChangedFiles(ctx)
	if err != nil {
		return Scope{}, fmt.Errorf("security: changed-files scope: %w", err)
	}
	kept := make([]string, 0, len(files))
	seen := make(map[string]bool, len(files))
	for _, file := range files {
		path, _ := NormalizePath(file)
		if path == "" {
			continue
		}
		if path == ".agent-sdlc" || strings.HasPrefix(path, ".agent-sdlc/") || path == ".git" || strings.HasPrefix(path, ".git/") {
			continue
		}
		if !seen[path] {
			seen[path] = true
			kept = append(kept, path)
		}
	}
	sort.Strings(kept)
	return Scope{Mode: "changed-files", Files: kept}, nil
}

// relativeFindingPath converts an absolute provider path into a safe
// repository-relative path. A path outside the workspace is rejected rather
// than persisted.
func relativeFindingPath(root, path string) (string, string) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "workspace root is invalid"
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "finding path is invalid"
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", "finding path cannot be resolved"
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "finding path escapes the workspace"
	}
	return NormalizePath(filepath.ToSlash(rel))
}

// workspaceDigest computes a deterministic digest of a non-Git workspace. It
// skips .git and .agent-sdlc, hashes regular-file content, and never follows
// symbolic links (so a link cannot pull content from outside the workspace).
func workspaceDigest(root string) (string, error) {
	hash := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if rel == ".git" || rel == ".agent-sdlc" {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		_, _ = io.WriteString(hash, rel+"\x00"+fmt.Sprintf("%d", info.Size())+"\x00")
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(hash, file)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("security: workspace digest: %w", err)
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
