package security

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

const (
	// MaxDiagnosticBytes bounds one diagnostic string. Reports are untrusted.
	MaxDiagnosticBytes = 1024
	// MaxMessageBytes bounds one finding message.
	MaxMessageBytes = 4096
)

// NormalizeSeverity maps a provider severity to the normalized vocabulary while
// retaining the original string in the finding. Unknown is explicit: it is never
// coerced to low severity.
func NormalizeSeverity(provider, raw string) Severity {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "critical", "crit":
		return SeverityCritical
	case "high", "error", "err":
		return SeverityHigh
	case "medium", "moderate", "warning", "warn":
		return SeverityMedium
	case "low":
		return SeverityLow
	case "info", "informational", "note":
		return SeverityInfo
	case "unknown", "":
		return SeverityUnknown
	}
	// Gitleaks has no severity field. Its findings are secrets, which the MVP
	// treats as HIGH unless the report carries an explicit mapped severity.
	if strings.EqualFold(strings.TrimSpace(provider), "gitleaks") {
		return SeverityHigh
	}
	return SeverityUnknown
}

// SeverityBlocked reports whether a severity is named in the policy's fail_on
// list. Unknown severity is handled separately by the policy.
func SeverityBlocked(failOn []string, severity Severity) bool {
	want := strings.ToUpper(strings.TrimSpace(string(severity)))
	for _, raw := range failOn {
		if strings.ToUpper(strings.TrimSpace(raw)) == want {
			return true
		}
	}
	return false
}

// SafeRelativePath reports whether p is a normalized, non-escaping repository
// path. Empty is allowed for providers that report no location.
func SafeRelativePath(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return true
	}
	if strings.ContainsRune(p, 0) || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") {
		return false
	}
	if len(p) >= 2 && p[1] == ':' {
		return false // Windows drive letter
	}
	clean := filepath.ToSlash(filepath.Clean(p))
	clean = strings.TrimPrefix(clean, "./")
	return clean != "" && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

// NormalizePath returns a safe repository-relative path. An unsafe or absolute
// path is never persisted; callers receive an empty path and a reason.
func NormalizePath(p string) (string, string) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", ""
	}
	if !SafeRelativePath(p) {
		return "", "unsafe path rejected"
	}
	clean := filepath.ToSlash(filepath.Clean(strings.TrimPrefix(p, "./")))
	return clean, ""
}

// SanitizeText bounds and neutralizes untrusted report text. Control characters
// other than newline and tab are removed, and the result is byte-bounded.
func SanitizeText(s string, max int) string {
	if max <= 0 {
		max = MaxDiagnosticBytes
	}
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case unicode.IsControl(r):
			return -1
		default:
			return r
		}
	}, s)
	s = strings.TrimSpace(s)
	if len(s) > max {
		s = s[:max]
	}
	return s
}

// StableFingerprint returns a stable, non-secret fingerprint namespaced by the
// provider identity. The inputs must not include a secret value or match text.
func StableFingerprint(provider, ruleID, path string, line, column int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%d", provider, ruleID, filepath.ToSlash(path), line, column)))
	return provider + ":" + hex.EncodeToString(sum[:])
}

// DigestBytes returns the SHA-256 hex digest of b.
func DigestBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// DigestStrings hashes an ordered list of identity components deterministically.
func DigestStrings(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

// redactKeys are JSON object keys whose values may contain a detected secret.
// Provider reports are sanitized before any native report is retained.
var redactKeys = map[string]bool{
	"secret":       true,
	"secretvalue":  true,
	"secret_value": true,
	"match":        true,
	"matched":      true,
	"raw":          true,
	"lines":        true,
}

// SanitizeNativeReport redacts secret-bearing fields in a native provider report
// and returns the sanitized JSON plus its digest. A report that cannot be parsed
// as JSON is rejected rather than persisted.
func SanitizeNativeReport(raw []byte) ([]byte, string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, "", fmt.Errorf("native report is not valid JSON: %w", err)
	}
	value = redactValue(value)
	out, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, "", err
	}
	out = append(out, '\n')
	return out, DigestBytes(out), nil
}

func redactValue(v any) any {
	switch value := v.(type) {
	case map[string]any:
		for key, child := range value {
			if redactKeys[strings.ToLower(key)] {
				value[key] = "[REDACTED]"
				continue
			}
			value[key] = redactValue(child)
		}
		return value
	case []any:
		for i := range value {
			value[i] = redactValue(value[i])
		}
		return value
	default:
		return v
	}
}

// RedactValues removes exact known secret strings from untrusted text. It is a
// second line of defense behind provider-side redaction; it never logs or
// persists the values it replaces.
func RedactValues(text string, values []string) string {
	sorted := append([]string(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return len(sorted[i]) > len(sorted[j]) })
	for _, value := range sorted {
		if len(strings.TrimSpace(value)) < 4 {
			continue
		}
		text = strings.ReplaceAll(text, value, "[REDACTED]")
	}
	return text
}

// StringSet converts a slice to a deterministic set.
func StringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[strings.ToLower(strings.TrimSpace(value))] = true
	}
	return out
}

func isAbsPath(p string) bool { return filepath.IsAbs(p) }

func joinPath(root, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(root, filepath.FromSlash(p))
}
