package security

import (
	"regexp"
	"sort"
	"strings"
)

// ExcludeMatcher is the compiled form of a declared exclusion list. Compile once
// per scan so a workspace walk or a findings pass does not re-parse patterns.
//
// Semantics follow the gitignore rules Semgrep applies to `--exclude`:
//   - a leading "/" anchors the pattern to the repository root;
//   - a pattern without "/" matches a path segment at any depth;
//   - a pattern containing "/" is relative to the repository root;
//   - "*" and "?" do not cross "/", while "**" may;
//   - a pattern that matches a directory also excludes its descendants.
type ExcludeMatcher struct {
	patterns []compiledExclude
}

type compiledExclude struct {
	pattern     string
	re          *regexp.Regexp
	segmentOnly bool
}

// NewExcludeMatcher normalizes and compiles exclusion patterns. Invalid glob
// syntax is ignored so a malformed pattern never silently hides content; the
// configured list still appears in scope.excluded for review.
func NewExcludeMatcher(patterns []string) ExcludeMatcher {
	var compiled []compiledExclude
	seen := make(map[string]bool, len(patterns))
	for _, raw := range patterns {
		pattern := normalizeExcludePattern(raw)
		if pattern == "" || seen[pattern] {
			continue
		}
		seen[pattern] = true
		re, err := compileExcludePattern(pattern)
		if err != nil {
			continue
		}
		anchored := strings.HasPrefix(pattern, "/")
		segmentOnly := !anchored && !strings.Contains(pattern, "/")
		compiled = append(compiled, compiledExclude{pattern: pattern, re: re, segmentOnly: segmentOnly})
	}
	return ExcludeMatcher{patterns: compiled}
}

// Patterns returns the normalized patterns in compiled order.
func (m ExcludeMatcher) Patterns() []string {
	out := make([]string, 0, len(m.patterns))
	for _, pattern := range m.patterns {
		out = append(out, pattern.pattern)
	}
	return out
}

// Excluded reports whether a repository-relative path is inside the declared
// scope. The path and each of its ancestors are tested, so excluding a directory
// excludes everything below it.
func (m ExcludeMatcher) Excluded(p string) bool {
	candidate := normalizePathForMatch(p)
	if candidate == "" {
		return false
	}
	// A pattern without a slash matches any path segment, like gitignore.
	for _, pattern := range m.patterns {
		if !pattern.segmentOnly {
			continue
		}
		for _, segment := range strings.Split(candidate, "/") {
			if pattern.re.MatchString(segment) {
				return true
			}
		}
	}
	// A pattern with a slash, or an anchored pattern, is matched against the
	// path and each ancestor prefix so a directory excludes its descendants.
	for prefix := candidate; ; {
		for _, pattern := range m.patterns {
			if !pattern.segmentOnly && pattern.re.MatchString(prefix) {
				return true
			}
		}
		i := strings.LastIndexByte(prefix, '/')
		if i < 0 {
			return false
		}
		prefix = prefix[:i]
	}
}

// ExcludedPath is the convenience form for one-off checks. Hot paths should
// compile an ExcludeMatcher once.
func ExcludedPath(p string, patterns []string) bool {
	return NewExcludeMatcher(patterns).Excluded(p)
}

// NormalizeExcludes trims, deduplicates, sorts, and drops empty exclusion
// patterns. A deterministic list makes scope evidence and policy identity stable
// across scans.
func NormalizeExcludes(patterns []string) []string {
	seen := make(map[string]bool, len(patterns))
	out := make([]string, 0, len(patterns))
	for _, raw := range patterns {
		pattern := normalizeExcludePattern(raw)
		if pattern == "" || seen[pattern] {
			continue
		}
		seen[pattern] = true
		out = append(out, pattern)
	}
	sort.Strings(out)
	return out
}

func normalizeExcludePattern(raw string) string {
	pattern := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	pattern = strings.TrimPrefix(pattern, "./")
	for strings.HasSuffix(pattern, "/") {
		pattern = strings.TrimSuffix(pattern, "/")
	}
	if pattern == "" || pattern == "." {
		return ""
	}
	return pattern
}

func normalizePathForMatch(p string) string {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	for strings.HasSuffix(p, "/") {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

// compileExcludePattern translates one normalized pattern into an anchored
// regular expression. The leading "/" is removed because the match is already
// anchored to the repository-relative candidate.
func compileExcludePattern(pattern string) (*regexp.Regexp, error) {
	pattern = strings.TrimPrefix(pattern, "/")
	pattern = strings.TrimSuffix(pattern, "/")
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); {
		switch pattern[i] {
		case '*':
			j := i
			for j < len(pattern) && pattern[j] == '*' {
				j++
			}
			if j-i > 1 && j < len(pattern) && pattern[j] == '/' {
				// "**/" matches zero or more path segments.
				b.WriteString("(?:.*/)?")
				i = j + 1
				continue
			}
			// "**" may cross separators; a single "*" may not.
			if j-i > 1 {
				b.WriteString(".*")
			} else {
				b.WriteString("[^/]*")
			}
			i = j
		case '?':
			b.WriteString("[^/]")
			i++
		case '[':
			j := i + 1
			if j < len(pattern) && (pattern[j] == '!' || pattern[j] == '^') {
				j++
			}
			if j < len(pattern) && pattern[j] == ']' {
				j++
			}
			for j < len(pattern) && pattern[j] != ']' {
				j++
			}
			if j >= len(pattern) {
				// An unterminated class is a literal "[".
				b.WriteString(`\[`)
				i++
				continue
			}
			class := pattern[i+1 : j]
			if strings.HasPrefix(class, "!") {
				class = "^" + class[1:]
			}
			b.WriteString("[" + class + "]")
			i = j + 1
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
			i++
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}
