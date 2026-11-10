package git

import (
	"context"
	"fmt"
	"strings"
)

// StatusEntry is one changed path reported by git status --porcelain.
type StatusEntry struct {
	// Status is the two-letter porcelain status (for example " M" or "??").
	Status string
	// Path is the repository-relative path. For a rename or copy both the old
	// and new paths are returned as separate entries.
	Path string
}

// StatusPaths lists changed paths, including untracked files. It is read-only.
// It is used where a caller must distinguish its own generated audit files from
// source changes.
func (a *Adapter) StatusPaths(ctx context.Context) ([]StatusEntry, error) {
	out, err := run(ctx, a.dir, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, fmt.Errorf("status paths: %w", err)
	}
	tokens := strings.Split(out, "\x00")
	var entries []StatusEntry
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		// A status entry is "XY <path>"; anything shorter or without the spacer is
		// not an entry (for example the bare source path of a rename record).
		if len(token) < 4 || token[2] != ' ' {
			continue
		}
		status := token[:2]
		path := token[3:]
		if path == "" {
			continue
		}
		entries = append(entries, StatusEntry{Status: status, Path: path})
		// With -z a rename or copy is "XY <new>\0<old>\0"; the original path is
		// the next NUL-separated token and carries no status prefix.
		if strings.ContainsAny(status, "RC") && i+1 < len(tokens) {
			if old := tokens[i+1]; old != "" {
				entries = append(entries, StatusEntry{Status: status, Path: old})
			}
			i++
		}
	}
	return entries, nil
}
