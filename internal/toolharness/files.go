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
	abs string // canonical absolute path
	rel string // path relative to the repository root, slash-separated
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
		return "", fmt.Errorf("write %s: %w", p.rel, err)
	}
	if err := os.WriteFile(p.abs, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", p.rel, err)
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(content), p.rel), nil
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
		if h.isProtectedPath(h.rel(p)) {
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
			matches = append(matches, fmt.Sprintf("%s:%d: %s", h.rel(p), i+1, strings.TrimRight(line, "\r")))
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
	raw, _ := args[key].(string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if required {
			return resolvedPath{}, fmt.Errorf("missing required argument %q", key)
		}
		return h.resolve(".", access)
	}
	return h.resolve(raw, access)
}

// resolve canonicalizes raw against the repository root and enforces the
// boundary: no escapes above the root (including via symlinks), and the state
// database is refused for every access class.
func (h *Harness) resolve(raw string, access accessClass) (resolvedPath, error) {
	if strings.ContainsRune(raw, 0) {
		return resolvedPath{}, errors.New("path contains a NUL byte")
	}
	abs := raw
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(h.root, abs)
	}
	abs = filepath.Clean(abs)

	real, err := evalExisting(abs)
	if err != nil {
		return resolvedPath{}, fmt.Errorf("resolve %s: %w", raw, err)
	}
	rel, err := filepath.Rel(h.root, real)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return resolvedPath{}, fmt.Errorf("path escapes the repository: %s", raw)
	}
	rel = filepath.ToSlash(rel)
	if h.isProtectedPath(rel) {
		return resolvedPath{}, fmt.Errorf("%w: %s", ErrProtectedPath, raw)
	}
	return resolvedPath{abs: real, rel: rel}, nil
}

// isProtectedPath reports whether rel names SOP's state database. It compares
// the cleaned, slash-separated path so a backslash or redundant ./ variant
// cannot slip through.
func (h *Harness) isProtectedPath(rel string) bool {
	rel = filepath.ToSlash(filepath.Clean(rel))
	return rel == stateDBRel
}

// rel renders p relative to the repository root.
func (h *Harness) rel(p string) string {
	r, err := filepath.Rel(h.root, p)
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
