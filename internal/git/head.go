package git

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// This file is the read-only committed-content boundary. It exposes the exact
// bytes of a regular file committed at HEAD, so a caller can give bounded review
// context without ever reading the working tree. Because the content comes from
// the commit, a staged or unstaged modification of the same path can never be
// presented as committed content. Reading a path from HEAD also says nothing
// about which task, author, or invocation produced the commit: last-commit
// provenance is descriptive only and must never be treated as task ownership.
//
// Every path is passed as one literal argument and is never evaluated by a shell.

var (
	// ErrHeadPathAbsent reports that a path is not a regular file committed at
	// HEAD: it is absent, a directory, a symbolic link, or a submodule. A caller
	// must omit the path rather than fall back to the working-tree version.
	ErrHeadPathAbsent = errors.New("git: path is not a regular file committed at HEAD")
	// ErrHeadFileTooLarge reports that the committed blob exceeds the caller's
	// byte limit. It is returned without reading the blob.
	ErrHeadFileTooLarge = errors.New("git: committed file exceeds the read limit")
)

// HeadEntry describes the regular file committed at one repository-relative path.
type HeadEntry struct {
	// Mode is the Git tree mode; it is "100644" or "100755" for a regular file.
	Mode string
	// Size is the committed blob size in bytes.
	Size int64
	// SHA is the blob object id the content is read from.
	SHA string
}

// cleanBlobPath validates and normalizes a repository-relative path for a HEAD
// lookup. It rejects an empty, absolute, escaping, or NUL-bearing path, so a
// caller-supplied value can never address anything outside the repository.
func cleanBlobPath(path string) (string, error) {
	if path == "" || strings.HasPrefix(path, "/") || strings.ContainsRune(path, 0) {
		return "", fmt.Errorf("git: unsafe committed path %q", path)
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	clean = strings.TrimPrefix(clean, "./")
	if clean == "" || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("git: unsafe committed path %q", path)
	}
	return clean, nil
}

// isHexObjectID reports whether s is a hexadecimal Git object id (40 characters
// for SHA-1, 64 for SHA-256). Only a well-formed object id is ever handed to
// cat-file, so a malformed revision from an unexpected ls-tree record can never
// reach the reader.
func isHexObjectID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		default:
			return false
		}
	}
	return true
}

// HeadEntry returns the tree entry for path at HEAD. A path that is not a regular
// file in the commit reports ErrHeadPathAbsent.
func (a *Adapter) HeadEntry(ctx context.Context, path string) (HeadEntry, error) {
	clean, err := cleanBlobPath(path)
	if err != nil {
		return HeadEntry{}, err
	}
	out, err := run(ctx, a.dir, "--literal-pathspecs", "ls-tree", "-l", "HEAD", "--", clean)
	if err != nil {
		return HeadEntry{}, fmt.Errorf("head entry: %w", err)
	}
	// One exact literal path yields at most one record:
	// "<mode> SP <type> SP <sha> SP <size> TAB <path>". Any other shape (no
	// record, several records, a tree, a symlink, or a submodule) is treated as
	// absent, so only a committed regular file is ever read.
	meta, _, ok := strings.Cut(strings.TrimRight(out, "\n"), "\t")
	if !ok {
		return HeadEntry{}, ErrHeadPathAbsent
	}
	fields := strings.Fields(meta)
	if len(fields) != 4 {
		return HeadEntry{}, ErrHeadPathAbsent
	}
	mode, typ, sha, sizeText := fields[0], fields[1], fields[2], fields[3]
	if typ != "blob" || (mode != "100644" && mode != "100755") {
		return HeadEntry{}, ErrHeadPathAbsent
	}
	if !isHexObjectID(sha) {
		return HeadEntry{}, ErrHeadPathAbsent
	}
	size, err := strconv.ParseInt(sizeText, 10, 64)
	if err != nil {
		return HeadEntry{}, fmt.Errorf("head entry size: %w", err)
	}
	return HeadEntry{Mode: mode, Size: size, SHA: sha}, nil
}

// HeadFile returns the metadata and the exact committed bytes of the regular file
// committed at path. It never reads the working tree, so staged and unstaged
// modifications are excluded by construction. A path that is not a committed
// regular file reports ErrHeadPathAbsent; a blob larger than maxBytes reports
// ErrHeadFileTooLarge without being read.
func (a *Adapter) HeadFile(ctx context.Context, path string, maxBytes int64) (HeadEntry, []byte, error) {
	entry, err := a.HeadEntry(ctx, path)
	if err != nil {
		return HeadEntry{}, nil, err
	}
	if entry.Size > maxBytes {
		return entry, nil, ErrHeadFileTooLarge
	}
	// Read by the object id the entry validated, so the content is exactly the
	// object whose mode, type, and size were checked.
	out, err := run(ctx, a.dir, "cat-file", "blob", entry.SHA)
	if err != nil {
		return HeadEntry{}, nil, fmt.Errorf("head blob: %w", err)
	}
	return entry, []byte(out), nil
}

// LastCommit returns "<full-sha> <subject>" for the most recent commit that
// touched path. It is read-only provenance: it does not establish that a task,
// author, or invocation owns the commit. An empty result (no commit affects the
// path) is not an error.
func (a *Adapter) LastCommit(ctx context.Context, path string) (string, error) {
	clean, err := cleanBlobPath(path)
	if err != nil {
		return "", err
	}
	out, err := run(ctx, a.dir, "--literal-pathspecs", "log", "-1", "--format=%H %s", "--", clean)
	if err != nil {
		return "", fmt.Errorf("last commit: %w", err)
	}
	return strings.TrimSpace(out), nil
}
