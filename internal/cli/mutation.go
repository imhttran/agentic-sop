package cli

import (
	"context"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/toolharness"
)

func snapshotRepository(ctx context.Context, dir string) (map[string]string, error) {
	paths, err := toolharness.New(dir, toolharness.DefaultConfig(), nil).RepositoryPathFingerprints(ctx)
	if err != nil {
		return nil, err
	}
	for path := range paths {
		if isSOPPath(path) {
			delete(paths, path)
		}
	}
	return paths, nil
}

// invocationChanges prefers the executing harness's observed paths. Providers
// without that evidence use before/after content snapshots, never the dirty-tree
// diff alone. An unavailable observation fails closed. An injected diff observer
// retains its existing observation contract; production always supplies snapshots.
func invocationChanges(ctx context.Context, dir string, d deps, before map[string]string, reported []string, diff string) ([]string, bool, error) {
	if reported != nil {
		paths := taskChangedFiles(reported)
		return paths, len(paths) > 0, nil
	}
	if d.snapshotRepository == nil {
		paths := taskChangedFiles(taskInvocationChanges(nil, diff))
		// An opaque injected diff can carry mutation evidence without file headers.
		observed := len(paths) > 0 || (len(changedFiles(diff)) == 0 && strings.TrimSpace(diff) != "")
		return paths, observed, nil
	}
	after, err := d.snapshotRepository(ctx, dir)
	if err != nil {
		return nil, false, err
	}
	var paths []string
	for path, fingerprint := range before {
		if after[path] != fingerprint {
			paths = append(paths, path)
		}
	}
	for path := range after {
		if _, existed := before[path]; !existed {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	paths = taskChangedFiles(paths)
	return paths, len(paths) > 0, nil
}
