// Package redact decides whether free-form text carries a credential, so every
// surface that prints untrusted text makes that decision in one place
// (docs/plans/PROPOSAL-SHARED-REDACTION.md).
//
// It does not own how a surface renders a redaction: callers pass a Marker, or a
// plain string for whole-line redaction. Markers appear in persisted artifacts
// and in operator-facing text where keeping the key is deliberate context, so
// each surface keeps its own.
package redact

import (
	"regexp"
	"sort"
	"strings"
)

// Kind classifies what matched, so a surface can render its own marker per kind.
type Kind string

const (
	// KindAssignment is key=value or key: value where the key names a credential.
	KindAssignment Kind = "assignment"
	// KindBearer is a Bearer authorization value.
	KindBearer Kind = "bearer"
	// KindKey is a provider API key with a recognisable prefix (sk-, rk-, pk-).
	KindKey Kind = "key"
	// KindToken is a credential token with a recognisable prefix (ghp_, github_pat_, xox[bap]-).
	KindToken Kind = "token"
	// KindAccessKey is an AWS-style access key id.
	KindAccessKey Kind = "access-key"
	// KindJWT is a JSON Web Token.
	KindJWT Kind = "jwt"
	// KindPEM is a PEM private-key header.
	KindPEM Kind = "pem"
	// KindURLUserinfo is a password embedded in a URL.
	KindURLUserinfo Kind = "url-userinfo"
	// KindKnown is a secret value the caller already knows.
	KindKnown Kind = "known"
)

// Marker renders the replacement for one match.
type Marker func(Kind) string

// Fixed returns a Marker that renders the same text for every kind.
func Fixed(marker string) Marker {
	return func(Kind) string { return marker }
}

// rule is one credential shape. value is the capture group holding the secret; 0
// means the whole match is the secret.
type rule struct {
	re    *regexp.Regexp
	kind  Kind
	value int
}

// rules are applied in order, and an earlier rule's match wins. The assignment
// rule runs first so "api_key=<token>" is treated as one assignment. A value that
// is only an authorization scheme word is skipped, so "Authorization: Bearer
// <token>" is left to the bearer rule and the token is redacted rather than the
// word "Bearer".
var rules = []rule{
	{regexp.MustCompile("(?i)\\b(api[\\s_-]?key|access[_-]?key|private[_-]?key|secret(?:[_-]?key)?|token|password|passwd|credential|authorization)[a-z0-9 _-]*[\"']?\\s*[:=]\\s*[\"']?([^\\s\"',;}\\[\\]`]+)"), KindAssignment, 2},
	{regexp.MustCompile(`(?i)\bbearer\s+([A-Za-z0-9._~+/=-]{8,})`), KindBearer, 1},
	{regexp.MustCompile(`(?i)\b(?:sk|rk|pk)-[A-Za-z0-9_-]{8,}`), KindKey, 0},
	{regexp.MustCompile(`(?i)\bgh[pousr]_[A-Za-z0-9]{16,}`), KindToken, 0},
	{regexp.MustCompile(`(?i)\bgithub_pat_[A-Za-z0-9_]{16,}`), KindToken, 0},
	{regexp.MustCompile(`(?i)\bxox[abprs]-[A-Za-z0-9-]{10,}`), KindToken, 0},
	{regexp.MustCompile(`(?i)\bAKIA[0-9A-Z]{16}\b`), KindAccessKey, 0},
	{regexp.MustCompile(`(?i)\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`), KindJWT, 0},
	{regexp.MustCompile(`(?i)\beyJ[A-Za-z0-9_-]{20,}`), KindJWT, 0},
	{regexp.MustCompile(`(?i)-----BEGIN [A-Z ]{0,40}PRIVATE KEY-----`), KindPEM, 0},
	{regexp.MustCompile(`://[^/\s:@]+:([^/\s@]+)@`), KindURLUserinfo, 1},
}

// Shapes replaces every credential-shaped match with its marker. The key of an
// assignment is replaced too; use ShapesValues to keep it.
func Shapes(s string, m Marker) string {
	return render(s, m, false)
}

// ShapesValues replaces only the secret part of each match, keeping the key for
// context, so "api_key=<token>" renders as "api_key=<marker>".
func ShapesValues(s string, m Marker) string {
	return render(s, m, true)
}

// Line replaces a whole line that carries a credential, for surfaces that show a
// diagnostic as one unit and would rather lose the line than leak from it. A line
// with no credential is returned unchanged.
func Line(s, marker string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if matches(line) {
			lines[i] = marker
		}
	}
	return strings.Join(lines, "\n")
}

// Values replaces exact secret values the caller already knows, longest first, so
// a secret that is a substring of another is removed completely.
func Values(s string, secrets []string, m Marker) string {
	sorted := append([]string(nil), secrets...)
	sort.Slice(sorted, func(i, j int) bool { return len(sorted[i]) > len(sorted[j]) })
	for _, secret := range sorted {
		if strings.TrimSpace(secret) == "" {
			continue
		}
		s = strings.ReplaceAll(s, secret, m(KindKnown))
	}
	return s
}

// EnvSecrets returns the values of environment variables whose names name a
// secret, longest first. A short value is skipped so common small strings are not
// redacted by accident; a real credential is longer than that.
func EnvSecrets(env []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, kv := range env {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || len(value) < 4 || seen[value] || !IsSecretName(name) {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// secretNameMarkers mark an environment variable or assignment key as naming a
// credential. One vocabulary for every surface: a name that is secret here is
// secret everywhere.
//
// These match as substrings, not whole segments, and that is deliberate: the cost
// of over-redacting a harmless value (GIT_AUTHOR_DATE, MONKEY) is a little lost
// context, while the cost of missing one is a leaked credential. A name this list
// has never seen is the case it has to survive.
var secretNameMarkers = []string{
	"TOKEN", "SECRET", "PASSWORD", "PASSWD", "APIKEY", "API_KEY",
	"ACCESS_KEY", "PRIVATE_KEY", "CREDENTIAL", "SESSION", "AUTHORIZATION",
	"AUTH", "KEY",
}

// IsSecretName reports whether name names a credential. Separators are
// normalised, so --api-key, api.key and API_KEY all match.
func IsSecretName(name string) bool {
	normalized := strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(strings.TrimSpace(name)))
	for _, marker := range secretNameMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func matches(s string) bool {
	for _, r := range rules {
		if r.re.MatchString(s) {
			return true
		}
	}
	return false
}

// isSchemeWord reports whether v is an authorization scheme rather than a secret.
func isSchemeWord(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "bearer", "basic", "digest", "negotiate":
		return true
	}
	return false
}

// hit is one credential match, with the span to replace under each rendering.
type hit struct {
	matchStart, matchEnd int
	valueStart, valueEnd int
	kind                 Kind
}

// findHits collects matches from every rule. Rules are ordered by confidence, so
// a match that overlaps one an earlier rule claimed is dropped: text a rule has
// already claimed is never re-examined, which is what keeps a written marker
// from being matched again.
func findHits(s string) []hit {
	return findHitsWithRules(s, rules)
}

func findHitsWithRules(s string, ruleSet []rule) []hit {
	var hits []hit
	for _, r := range ruleSet {
		for _, idx := range r.re.FindAllStringSubmatchIndex(s, -1) {
			mStart, mEnd := idx[0], idx[1]
			vStart, vEnd := mStart, mEnd
			if r.value > 0 {
				if len(idx) <= 2*r.value+1 || idx[2*r.value] < 0 {
					continue
				}
				vStart, vEnd = idx[2*r.value], idx[2*r.value+1]
			}
			// A scheme word is not the secret: the value it introduces is a token
			// shape the next rule knows how to redact.
			if r.kind == KindAssignment && isSchemeWord(s[vStart:vEnd]) {
				continue
			}
			if overlaps(hits, mStart, mEnd) {
				continue
			}
			hits = append(hits, hit{mStart, mEnd, vStart, vEnd, r.kind})
		}
	}
	return hits
}

func overlaps(hits []hit, start, end int) bool {
	for _, h := range hits {
		if start < h.matchEnd && h.matchStart < end {
			return true
		}
	}
	return false
}

// render replaces each hit with its marker, in text order. valueOnly keeps the
// part of the match that is not the secret.
func render(s string, m Marker, valueOnly bool) string {
	return renderWithRules(s, m, valueOnly, rules)
}

func renderWithRules(s string, m Marker, valueOnly bool, ruleSet []rule) string {
	hits := findHitsWithRules(s, ruleSet)
	if len(hits) == 0 {
		return s
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].matchStart < hits[j].matchStart })
	var b strings.Builder
	last := 0
	for _, h := range hits {
		start, end := h.matchStart, h.matchEnd
		if valueOnly {
			start, end = h.valueStart, h.valueEnd
		}
		if start < last || end > len(s) {
			continue
		}
		b.WriteString(s[last:start])
		b.WriteString(m(h.kind))
		last = end
	}
	b.WriteString(s[last:])
	return b.String()
}

// exactHits returns a span for every occurrence of each non-blank secret.
func exactHits(s string, secrets []string) []hit {
	var hits []hit
	for _, v := range secrets {
		if strings.TrimSpace(v) == "" {
			continue
		}
		for i := 0; ; {
			j := strings.Index(s[i:], v)
			if j < 0 {
				break
			}
			start := i + j
			hits = append(hits, hit{matchStart: start, matchEnd: start + len(v), kind: KindKnown})
			i = start + len(v)
		}
	}
	return hits
}

// renderSpans replaces each run of overlapping hit spans with one marker, taken
// from the run's first hit. The run extends to the furthest end it reaches, so no
// part of an overlapping match survives.
func renderSpans(s string, hits []hit, m Marker) string {
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].matchStart < hits[j].matchStart })
	var b strings.Builder
	last := 0
	for i := 0; i < len(hits); {
		start, end, kind := hits[i].matchStart, hits[i].matchEnd, hits[i].kind
		j := i + 1
		for ; j < len(hits) && hits[j].matchStart < end; j++ {
			end = max(end, hits[j].matchEnd)
		}
		if start >= last {
			b.WriteString(s[last:start])
			b.WriteString(m(kind))
			last = end
		}
		i = j
	}
	b.WriteString(s[last:])
	return b.String()
}
