package sopclient

import (
	"strings"
)

// The single safe-summary point for activity Detail, applied at Store.activity
// so no consumer can bypass it.

// maxDetailLen bounds a Detail so a mis-recorded dump is never shown verbatim.
const maxDetailLen = 200

// redacted replaces any span suppressed by sanitizeDetail.
const redacted = "[redacted]"

// Case-insensitive markers of a credential, env dump, or prompt text.
var secretMarkers = []string{
	"api_key", "apikey", "api-key",
	"secret", "token", "password", "passwd",
	"authorization", "bearer ", "private_key", "privatekey",
	"-----begin", "access_key", "accesskey",
}

var envDumpMarkers = []string{
	"environ", "env=", "env var", "envvar",
	"os.environ", "$env:", "printenv", "getenv",
}

var promptMarkers = []string{
	"prompt:", "system prompt", "user prompt", "messages=",
	"role: system", "role: user", "role: assistant",
}

// sanitizeDetail redacts anything that looks like a secret, env dump, or prompt
// wholesale, truncates long free-form values, and otherwise returns SOP's text
// unchanged.
func sanitizeDetail(detail string) string {
	s := strings.TrimSpace(detail)
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	for _, marker := range secretMarkers {
		if strings.Contains(lower, marker) {
			return redacted
		}
	}
	for _, marker := range envDumpMarkers {
		if strings.Contains(lower, marker) {
			return redacted
		}
	}
	for _, marker := range promptMarkers {
		if strings.Contains(lower, marker) {
			return redacted
		}
	}
	// A multi-line blob is a dump, not a one-line summary.
	if strings.ContainsAny(s, "\n\r") {
		return redacted
	}
	if len(s) > maxDetailLen {
		s = truncate(s, maxDetailLen) + "..."
	}
	return s
}

// truncate returns the first n runes of s, never splitting a UTF-8 rune.
func truncate(s string, n int) string {
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}
