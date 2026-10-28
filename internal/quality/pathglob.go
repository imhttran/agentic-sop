package quality

import (
	"path"
	"path/filepath"
	"strings"
)

// Path glob matching shared by the PostgreSQL policy and verification.scoped.

// MatchPath reports whether a repository-relative path matches a glob in which
// `**` spans whole segments (see matchGlob). A blank path never matches.
func MatchPath(pattern, path string) bool {
	cleaned := filepath.ToSlash(strings.TrimSpace(path))
	cleaned = strings.TrimPrefix(cleaned, "./")
	cleaned = strings.TrimSuffix(cleaned, "/")
	if cleaned == "" {
		return false
	}
	return matchGlob(pattern, strings.Split(cleaned, "/"))
}

// matchGlob matches a slash-separated glob pattern against the path's segments.
// `**` matches zero or more whole segments; `*`/`?`/character classes match
// within one segment via filepath.Match. The matcher is anchored at both ends
// (the entire path must match the pattern), which makes the semantics simple to
// reason about and impossible to trigger by an unrelated substring.
func matchGlob(pattern string, segments []string) bool {
	// Split the pattern on `**`, keeping order; each literal chunk must match a
	// contiguous run of segments, and `**` allows any gap between chunks.
	chunks := splitDoubleStar(pattern)
	return matchChunks(chunks, segments)
}

// chunk is either a literal segment sequence (doubleStar == false) or a `**`
// wildcard (doubleStar == true).
type chunk struct {
	doubleStar bool
	segments   []string // only for literal chunks
}

// splitDoubleStar splits a pattern into an alternating list of literal-segment
// chunks and `**` wildcards. Consecutive `**` collapse into one.
func splitDoubleStar(pattern string) []chunk {
	parts := strings.Split(pattern, "**")
	var chunks []chunk
	for _, p := range parts {
		p = strings.Trim(p, "/")
		if p == "" {
			chunks = append(chunks, chunk{doubleStar: true})
			continue
		}
		chunks = append(chunks, chunk{segments: strings.Split(p, "/")})
	}
	// Collapse runs of consecutive `**` chunks into a single wildcard.
	var out []chunk
	for _, c := range chunks {
		if c.doubleStar && len(out) > 0 && out[len(out)-1].doubleStar {
			continue
		}
		out = append(out, c)
	}
	return out
}

// matchChunks matches the chunk sequence against the whole segment list.
func matchChunks(chunks []chunk, segments []string) bool {
	if len(chunks) == 0 {
		return len(segments) == 0
	}
	c := chunks[0]
	if c.doubleStar {
		// `**` consumes zero or more segments; try every split.
		for i := 0; i <= len(segments); i++ {
			if matchChunks(chunks[1:], segments[i:]) {
				return true
			}
		}
		return false
	}
	// Literal chunk: must match the next segments, one-for-one.
	if len(segments) < len(c.segments) {
		return false
	}
	for i, want := range c.segments {
		if !matchSegmentGlob(want, segments[i]) {
			return false
		}
	}
	return matchChunks(chunks[1:], segments[len(c.segments):])
}

// matchSegmentGlob matches one path segment against a pattern segment using the
// standard `*` / `?` / character-class semantics. A bare name must match exactly.
//
// A wildcard never matches a leading dot in a segment (POSIX/shell semantics):
// `path.Match` would otherwise let `*` match a dotfile-like segment such as
// `.pgvector`, so that case is rejected explicitly. Matching uses `path.Match`
// (not `filepath.Match`) so the result is platform-independent.
func matchSegmentGlob(pattern, segment string) bool {
	if pattern == segment {
		return true
	}
	if strings.HasPrefix(segment, ".") && !strings.HasPrefix(pattern, ".") {
		return false
	}
	ok, err := path.Match(pattern, segment)
	return err == nil && ok
}
