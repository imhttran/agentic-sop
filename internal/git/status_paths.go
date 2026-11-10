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
	var entries []StatusEntry
	for _, token := range strings.Split(out, "\x00") {
		if token == "" || len(token) < 4 {
			continue
		}
		status := token[:2]
		path := token[3:]
		if path == "" {
			continue
		}
		entries = append(entries, StatusEntry{Status: status, Path: path})
	}
	return entries, nil
}
