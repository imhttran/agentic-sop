package security

import (
	"path"
	"sort"
	"strings"
)

// NormalizeExcludes trims, deduplicates, sorts, and drops empty exclusion
// patterns. A deterministic list makes scope evidence and policy identity stable
// across scans.
func NormalizeExcludes(patterns []string) []string {
	seen := make(map[string]bool, len(patterns))
	out := make([]string, 0, len(patterns))
	for _, raw := range patterns {
		pattern := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
		pattern = strings.TrimPrefix(pattern, "./")
		pattern = strings.TrimSuffix(pattern, "/")
		if pattern == "" || pattern == "." || seen[pattern] {
			continue
		}
		seen[pattern] = true
		out = append(out, pattern)
	}
	sort.Strings(out)
	return out
}

// ExcludedPath reports whether a repository-relative path falls inside one of
// the declared exclusions. A leading "/" anchors a pattern to the repository
// root (gitignore semantics). A literal pattern matches the path itself or any
// descendant (so a directory name excludes its tree). A pattern containing glob
// metacharacters is matched with path.Match; a trailing "/**" is a subtree
// shorthand. Matching is path-only and never reads the filesystem.
func ExcludedPath(p string, patterns []string) bool {
	p = strings.TrimPrefix(strings.TrimSpace(strings.ReplaceAll(p, "\\", "/")), "./")
	if p == "" {
		return false
	}
	for _, raw := range patterns {
		pattern := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
		pattern = strings.TrimPrefix(pattern, "./")
		pattern = strings.TrimSuffix(pattern, "/")
		if pattern == "" || pattern == "." {
			continue
		}
		anchored := strings.HasPrefix(pattern, "/")
		if anchored {
			pattern = strings.TrimPrefix(pattern, "/")
		}
		if strings.HasSuffix(pattern, "/**") {
			prefix := strings.TrimSuffix(pattern, "/**")
			if p == prefix || strings.HasPrefix(p, prefix+"/") {
				return true
			}
			continue
		}
		if !strings.ContainsAny(pattern, "*?[") {
			if p == pattern || strings.HasPrefix(p, pattern+"/") {
				return true
			}
			continue
		}
		if anchored {
			// A root-anchored glob applies to the first path element only, so
			// "/sop-*" never excludes a nested cmd/sop-* source directory.
			first := p
			if i := strings.IndexByte(first, '/'); i >= 0 {
				first = first[:i]
			}
			if ok, _ := path.Match(pattern, first); ok {
				return true
			}
			continue
		}
		// A relative glob may name a directory (e.g. internal/*/testdata), so
		// test the path and each of its ancestors.
		for candidate := p; candidate != ""; {
			if ok, _ := path.Match(pattern, candidate); ok {
				return true
			}
			i := strings.LastIndexByte(candidate, '/')
			if i < 0 {
				break
			}
			candidate = candidate[:i]
		}
	}
	return false
}
