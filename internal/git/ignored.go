package git

import (
	"context"
	"fmt"
	"strings"
)

// IgnoredPaths returns the repository-relative paths that Git excludes from the
// working-tree snapshot: untracked paths matching an ignore rule. It uses the
// same --exclude-standard resolution as SnapshotFiles, so the two results are
// disjoint by construction: a path Git tracks is never reported here, even when
// an ignore rule matches it.
//
// A directory whose entire content is ignored is reported once with a trailing
// separator instead of once per file, which keeps a large build or runtime
// directory to a single entry. Output uses NUL separators so paths containing
// spaces or newlines are preserved exactly.
//
// Any error means the ignored set is unavailable. Callers must treat that as
// "unknown", never as an empty set.
func (a *Adapter) IgnoredPaths(ctx context.Context) ([]string, error) {
	if err := a.ValidateRepository(ctx); err != nil {
		return nil, fmt.Errorf("ignored paths: %w", err)
	}
	out, err := run(ctx, a.dir, "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory")
	if err != nil {
		return nil, fmt.Errorf("ignored paths: %w", err)
	}
	seen := make(map[string]bool)
	var paths []string
	for _, entry := range strings.Split(out, "\x00") {
		if entry == "" || seen[entry] {
			continue
		}
		seen[entry] = true
		paths = append(paths, entry)
	}
	return paths, nil
}
