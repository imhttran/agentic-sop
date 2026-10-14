package toolharness

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// stateDBRel is SOP's workflow database, relative to the repository root. It is
// the one file the harness must never read, write, create, list, or search:
// SOP is the sole owner of workflow state, and a model must not be able to
// inspect or tamper with it through a tool.
const stateDBRel = ".agent-sdlc/state.db"

// stateDirRel is SOP's state directory. Search skips it wholesale so a pattern
// can never surface the state database.
const stateDirRel = ".agent-sdlc"

// resolvedPath is a tool argument resolved to a canonical path inside the
// repository, together with its repository-relative, slash-separated form.
type resolvedPath struct {
	abs  string // canonical absolute path
	rel  string // path relative to the authorized root, slash-separated
	root string // the authorized root the path was resolved against
}

// readFile returns the file's contents, bounded by maxBytes.
func (h *Harness) readFile(args map[string]any) (string, error) {
	p, err := h.resolveArg(args, "path", true, accessRead)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(p.abs)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("read_file: %s is a directory", p.rel)
	}
	data, err := os.ReadFile(p.abs)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}
	return truncate(string(data), h.cfg.MaxOutputBytes), nil
}

// writeFile writes or creates a file. mustNotExist implements create_file.
func (h *Harness) writeFile(args map[string]any, mustNotExist bool) (string, error) {
	content, ok := args["content"].(string)
	if !ok {
		return "", errors.New(`missing required argument "content"`)
	}
	// A trailing separator names a directory; these tools act on files only. There
	// is no directory tool, and write_file creates missing parents, so a directory
	// path is a mistake to correct rather than a silent regular-file create.
	if raw, _ := args["path"].(string); strings.HasSuffix(strings.TrimSpace(raw), "/") {
		return "", errors.New(`path ends in "/" (a directory); these tools create regular files only. Write the file you want — write_file creates any missing parent directories — rather than creating a directory`)
	}
	p, err := h.resolveArg(args, "path", true, accessWrite)
	if err != nil {
		return "", err
	}
	if mustNotExist {
		if _, err := os.Lstat(p.abs); err == nil {
			return "", fmt.Errorf("create_file: %s already exists; use write_file to overwrite it", p.rel)
		}
	}
	if err := os.MkdirAll(filepath.Dir(p.abs), 0o755); err != nil {
		if blocked, ok := parentNotDir(filepath.Dir(p.abs)); ok {
			rel, rerr := filepath.Rel(p.root, blocked)
			if rerr != nil {
				rel = blocked
			}
			return "", fmt.Errorf("cannot create the directory for %s: a file already exists at %q; delete it with delete_file first, or choose a different path", p.rel, filepath.ToSlash(rel))
		}
		return "", fmt.Errorf("write %s: %w", p.rel, err)
	}
	if err := os.WriteFile(p.abs, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", p.rel, err)
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(content), p.rel), nil
}

// parentNotDir returns the first existing path component of dir (dir or one of
// its ancestors) that is not a directory, so a failed parent-directory create can
// explain the collision instead of surfacing a raw "not a directory" errno. It
// never deletes or modifies anything.
func parentNotDir(dir string) (string, bool) {
	for d := dir; ; {
		info, err := os.Lstat(d)
		if err == nil {
			if info.IsDir() {
				return "", false
			}
			return d, true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", false
		}
		d = parent
	}
}

// copyFile performs a byte-exact copy of one file from an authorized source root
// (default: the primary repository) into the writable repository root. It exists so
// a governed task can import a file deterministically — the file bytes never pass
// through a model — while staying inside the same confinement: the source is read
// through an authorized root, the destination is written through a writable root,
// and both are audited. It refuses a directory path on either side.
func (h *Harness) copyFile(args map[string]any) (string, error) {
	source, _ := args["source"].(string)
	if strings.TrimSpace(source) == "" {
		return "", errors.New(`missing required argument "source"`)
	}
	dest, _ := args["dest"].(string)
	if strings.TrimSpace(dest) == "" {
		return "", errors.New(`missing required argument "dest"`)
	}
	if strings.HasSuffix(strings.TrimSpace(source), "/") || strings.HasSuffix(strings.TrimSpace(dest), "/") {
		return "", errors.New(`copy_file copies a single file; a path ending in "/" names a directory`)
	}
	src, err := h.resolveArg(map[string]any{"path": source, "root": args["source_root"]}, "path", true, accessRead)
	if err != nil {
		return "", err
	}
	dst, err := h.resolveArg(map[string]any{"path": dest}, "path", true, accessWrite)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(src.abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("copy_file: source %q does not exist under root %q (resolved to %s); the source path is relative to that root", src.rel, src.root, src.abs)
		}
		return "", fmt.Errorf("copy_file: read %s: %w", src.rel, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("copy_file: %s is a directory", src.rel)
	}
	data, err := os.ReadFile(src.abs)
	if err != nil {
		return "", fmt.Errorf("copy_file: read %s: %w", src.rel, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst.abs), 0o755); err != nil {
		if blocked, ok := parentNotDir(filepath.Dir(dst.abs)); ok {
			rel, rerr := filepath.Rel(dst.root, blocked)
			if rerr != nil {
				rel = blocked
			}
			return "", fmt.Errorf("cannot create the directory for %s: a file already exists at %q; delete it with delete_file first, or choose a different path", dst.rel, filepath.ToSlash(rel))
		}
		return "", fmt.Errorf("copy_file: write %s: %w", dst.rel, err)
	}
	if err := os.WriteFile(dst.abs, data, 0o644); err != nil {
		return "", fmt.Errorf("copy_file: write %s: %w", dst.rel, err)
	}
	return fmt.Sprintf("copied %d bytes from %s to %s", len(data), src.rel, dst.rel), nil
}

// rewriteFile replaces every exact occurrence of from with to in one file, so an
// import rewrite is deterministic and separate from copying. It is idempotent: a
// file whose from string is already gone (because to is present) is a no-op rather
// than an error, so re-running an import is safe. It refuses an empty from, a from
// equal to to, and a file in which neither string appears, and never touches any
// other file.
func (h *Harness) rewriteFile(args map[string]any) (string, error) {
	from, _ := args["from"].(string)
	to, _ := args["to"].(string)
	if from == "" {
		return "", errors.New(`missing required argument "from"`)
	}
	if from == to {
		return "", errors.New(`"from" and "to" are identical; nothing to rewrite`)
	}
	p, err := h.resolveArg(args, "path", true, accessWrite)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(p.abs)
	if err != nil {
		return "", fmt.Errorf("rewrite_file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("rewrite_file: %s is a directory", p.rel)
	}
	data, err := os.ReadFile(p.abs)
	if err != nil {
		return "", fmt.Errorf("rewrite_file: %w", err)
	}
	text := string(data)
	switch n := strings.Count(text, from); {
	case n > 0:
		if err := os.WriteFile(p.abs, []byte(strings.ReplaceAll(text, from, to)), 0o644); err != nil {
			return "", fmt.Errorf("rewrite_file: %w", err)
		}
		return fmt.Sprintf("rewrote %d occurrence(s) of %q in %s", n, from, p.rel), nil
	case strings.Contains(text, to):
		return fmt.Sprintf("already rewritten: %s has no %q (idempotent no-op)", p.rel, from), nil
	default:
		return "", fmt.Errorf("rewrite_file: %s contains neither %q nor its replacement; nothing to rewrite", p.rel, from)
	}
}

// deleteFile removes a file. It refuses a directory — a recursive delete is not a
// controlled operation — and, like every tool, SOP's state database.
func (h *Harness) deleteFile(args map[string]any) (string, error) {
	p, err := h.resolveArg(args, "path", true, accessWrite)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(p.abs)
	if err != nil {
		return "", fmt.Errorf("delete_file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("delete_file: %s is a directory", p.rel)
	}
	if err := os.Remove(p.abs); err != nil {
		return "", fmt.Errorf("delete_file: %w", err)
	}
	return fmt.Sprintf("deleted %s", p.rel), nil
}

// listFiles lists a directory's immediate entries.
func (h *Harness) listFiles(args map[string]any) (string, error) {
	p, err := h.resolveArg(args, "path", false, accessRead)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(p.abs)
	if err != nil {
		return "", fmt.Errorf("list_files: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	var b strings.Builder
	listed := 0
	for _, e := range entries {
		// The state database is never exposed, even as a name.
		if h.isProtectedPath(filepath.Join(p.rel, e.Name())) {
			continue
		}
		if listed >= maxListEntries {
			fmt.Fprintf(&b, "… [%d more entries]\n", len(entries)-listed)
			break
		}
		listed++
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		b.WriteString(name)
		b.WriteByte('\n')
	}
	if b.Len() == 0 {
		return "(empty)", nil
	}
	return b.String(), nil
}

// searchFiles reports matching lines under a starting path.
func (h *Harness) searchFiles(args map[string]any) (string, error) {
	pattern, _ := args["pattern"].(string)
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "", errors.New(`missing required argument "pattern"`)
	}
	start, err := h.resolveArg(args, "path", false, accessRead)
	if err != nil {
		return "", err
	}
	base := start.root

	var matches []string
	walkErr := filepath.WalkDir(start.abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			if p != start.abs && (d.Name() == ".git" || d.Name() == stateDirRel) {
				return filepath.SkipDir
			}
			return nil
		}
		if h.isProtectedPath(relFrom(base, p)) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxSearchFileBytes {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil || bytes.IndexByte(data, 0) >= 0 {
			return nil // unreadable or binary
		}
		for i, line := range strings.Split(string(data), "\n") {
			if !strings.Contains(line, pattern) {
				continue
			}
			matches = append(matches, fmt.Sprintf("%s:%d: %s", relFrom(base, p), i+1, strings.TrimRight(line, "\r")))
			if len(matches) >= maxSearchMatches {
				return filepath.SkipAll
			}
		}
		return nil
	})
	if walkErr != nil {
		return "", fmt.Errorf("search_files: %w", walkErr)
	}
	if len(matches) == 0 {
		return "(no matches)", nil
	}
	return truncate(strings.Join(matches, "\n"), h.cfg.MaxOutputBytes), nil
}

// accessClass distinguishes a read from a write so protection can be applied
// uniformly (the state database is refused for both). It is retained as the
// single place a future access-dependent rule can branch.
type accessClass int

const (
	accessRead accessClass = iota
	accessWrite
)

// resolveArg extracts a path argument and resolves it inside the repository.
func (h *Harness) resolveArg(args map[string]any, key string, required bool, access accessClass) (resolvedPath, error) {
	root, err := h.selectRoot(rootSelector(args))
	if err != nil {
		return resolvedPath{}, err
	}
	raw, _ := args[key].(string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if required {
			return resolvedPath{}, fmt.Errorf("missing required argument %q", key)
		}
		return h.resolveIn(root, ".", access)
	}
	return h.resolveIn(root, raw, access)
}

// resolve canonicalizes raw against the repository root and enforces the
// boundary: no escapes above the root (including via symlinks), and the state
// database is refused for every access class.
func (h *Harness) resolve(raw string, access accessClass) (resolvedPath, error) {
	return h.resolveIn(h.roots[0], raw, access)
}

// resolveIn canonicalizes raw against the selected authorized root and enforces
// that root's boundary: no escapes above the root (including through symlinks),
// SOP's state database is refused for every access class, and a write into a
// read-only root is refused.
func (h *Harness) resolveIn(root authorizedRoot, raw string, access accessClass) (resolvedPath, error) {
	if access == accessWrite && root.mode != RootReadWrite {
		return resolvedPath{}, fmt.Errorf("%w: %s", errReadOnlyRoot, raw)
	}
	if strings.ContainsRune(raw, 0) {
		return resolvedPath{}, errors.New("path contains a NUL byte")
	}
	abs := raw
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root.path, abs)
	}
	abs = filepath.Clean(abs)

	real, err := evalExisting(abs)
	if err != nil {
		return resolvedPath{}, fmt.Errorf("resolve %s: %w", raw, err)
	}
	rel, err := filepath.Rel(root.path, real)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return resolvedPath{}, fmt.Errorf("path escapes the repository: %s", raw)
	}
	rel = filepath.ToSlash(rel)
	if h.isProtectedPath(rel) {
		return resolvedPath{}, fmt.Errorf("%w: %s", ErrProtectedPath, raw)
	}
	return resolvedPath{abs: real, rel: rel, root: root.path}, nil
}

// isProtectedPath reports whether rel names SOP's state database. It compares
// the cleaned, slash-separated path so a backslash or redundant ./ variant
// cannot slip through.
func (h *Harness) isProtectedPath(rel string) bool {
	rel = filepath.ToSlash(filepath.Clean(rel))
	return rel == stateDBRel
}

// rel renders p relative to the primary repository root.
func (h *Harness) rel(p string) string { return relFrom(h.root, p) }

// relFrom renders p relative to base, slash-separated.
func relFrom(base, p string) string {
	r, err := filepath.Rel(base, p)
	if err != nil {
		return p
	}
	return filepath.ToSlash(r)
}

// evalExisting resolves symlinks on the deepest existing ancestor of abs and
// rejoins the non-existent remainder, so a symlinked directory cannot smuggle a
// path outside the repository.
func evalExisting(abs string) (string, error) {
	remainder := ""
	cur := abs
	for {
		if _, err := os.Lstat(cur); err == nil {
			real, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return "", err
			}
			if remainder != "" {
				real = filepath.Join(real, remainder)
			}
			return real, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs, nil
		}
		remainder = filepath.Join(filepath.Base(cur), remainder)
		cur = parent
	}
}

// truncate bounds s to at most n bytes, marking the cut.
func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("\n… [truncated %d bytes]", len(s)-n)
}
