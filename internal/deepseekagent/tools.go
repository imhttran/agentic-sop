package deepseekagent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
)

// toolbox implements the controlled repository tools. Every path is resolved
// against the canonical repository root before use, so the model cannot read or
// write outside the repository, and writes to SOP's own state are refused.
type toolbox struct {
	root     string // canonical repository root
	stateDir string // config.DirName, relative to root
	cfg      Config
}

func newToolbox(root string, cfg Config) *toolbox {
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		canonical = root
	}
	if abs, err := filepath.Abs(canonical); err == nil {
		canonical = abs
	}
	return &toolbox{root: canonical, stateDir: config.DirName, cfg: cfg}
}

// run dispatches one tool call.
func (t *toolbox) run(ctx context.Context, name string, args map[string]any) (string, error) {
	switch name {
	case "read_file":
		return t.readFile(args)
	case "write_file":
		return t.writeFile(args, false)
	case "create_file":
		return t.writeFile(args, true)
	case "list_files":
		return t.listFiles(args)
	case "search_files":
		return t.searchFiles(args)
	case "run_command":
		return t.runCommand(ctx, args)
	case "git_status":
		return t.git(ctx, "status", "--short", "--branch")
	case "git_diff":
		return t.git(ctx, "diff")
	default:
		return "", fmt.Errorf("unsupported tool %q", name)
	}
}

func (t *toolbox) readFile(args map[string]any) (string, error) {
	path, err := t.resolveArg(args, "path", true, false)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("read_file: %s is a directory", t.rel(path))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}
	return truncate(string(data), t.cfg.MaxOutputBytes), nil
}

func (t *toolbox) writeFile(args map[string]any, mustNotExist bool) (string, error) {
	content, ok := args["content"].(string)
	if !ok {
		return "", errors.New(`missing required argument "content"`)
	}
	path, err := t.resolveArg(args, "path", true, true)
	if err != nil {
		return "", err
	}
	if mustNotExist {
		if _, err := os.Lstat(path); err == nil {
			return "", fmt.Errorf("create_file: %s already exists; use write_file to overwrite it", t.rel(path))
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("write %s: %w", t.rel(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", t.rel(path), err)
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(content), t.rel(path)), nil
}

func (t *toolbox) listFiles(args map[string]any) (string, error) {
	path, err := t.resolveArg(args, "path", false, false)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("list_files: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	var b strings.Builder
	for i, e := range entries {
		if i >= maxListEntries {
			fmt.Fprintf(&b, "… [%d more entries]\n", len(entries)-i)
			break
		}
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

func (t *toolbox) searchFiles(args map[string]any) (string, error) {
	pattern, _ := args["pattern"].(string)
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "", errors.New(`missing required argument "pattern"`)
	}
	start, err := t.resolveArg(args, "path", false, false)
	if err != nil {
		return "", err
	}

	var matches []string
	walkErr := filepath.WalkDir(start, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			if p != start && (d.Name() == ".git" || d.Name() == t.stateDir) {
				return filepath.SkipDir
			}
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
			matches = append(matches, fmt.Sprintf("%s:%d: %s", t.rel(p), i+1, strings.TrimRight(line, "\r")))
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
	return truncate(strings.Join(matches, "\n"), t.cfg.MaxOutputBytes), nil
}

// git runs a fixed, read-only git command (already allow-listed by construction).
func (t *toolbox) git(ctx context.Context, args ...string) (string, error) {
	return t.exec(ctx, append([]string{"git"}, args...))
}

// resolveArg extracts a path argument and resolves it inside the repository.
func (t *toolbox) resolveArg(args map[string]any, key string, required, forWrite bool) (string, error) {
	raw, _ := args[key].(string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if required {
			return "", fmt.Errorf("missing required argument %q", key)
		}
		return t.root, nil
	}
	return t.resolve(raw, forWrite)
}

// resolve canonicalizes raw against the repository root and enforces the
// boundary: no escapes above the root (including via symlinks) and no writes to
// SOP's state directory.
func (t *toolbox) resolve(raw string, forWrite bool) (string, error) {
	if strings.ContainsRune(raw, 0) {
		return "", errors.New("path contains a NUL byte")
	}
	abs := raw
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(t.root, abs)
	}
	abs = filepath.Clean(abs)

	real, err := evalExisting(abs)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", raw, err)
	}
	rel, err := filepath.Rel(t.root, real)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the repository: %s", raw)
	}
	if forWrite && t.protected(rel) {
		return "", fmt.Errorf("refusing to modify SOP state: %s", raw)
	}
	return real, nil
}

// protected reports whether rel is SOP's internal state area.
func (t *toolbox) protected(rel string) bool {
	return rel == t.stateDir || strings.HasPrefix(rel, t.stateDir+string(filepath.Separator))
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

// rel renders p relative to the repository root.
func (t *toolbox) rel(p string) string {
	r, err := filepath.Rel(t.root, p)
	if err != nil {
		return p
	}
	return r
}
