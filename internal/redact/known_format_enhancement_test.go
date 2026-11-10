package redact

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// Test-only candidate patterns for the seven-format enhancement. They are not
// wired into production. The current gap tests document the NEEDS_HUMAN state;
// the proposed-pattern tests prove the bounded rules would detect the intended
// values and reject the negative controls once authorized.

var proposedSevenFormatRules = []*regexp.Regexp{
	regexp.MustCompile(`\bya29\.[A-Za-z0-9_-]{20,}\b`),
	regexp.MustCompile(`\bASIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\bsk_test_[A-Za-z0-9]{16,}\b`),
	regexp.MustCompile(`\bwhsec_[A-Za-z0-9]{16,}\b`),
	regexp.MustCompile(`\bdckr_pat_[A-Za-z0-9_-]{16,}\b`),
	regexp.MustCompile(`\bAGE-SECRET-KEY-1[A-Z0-9]{20,}\b`),
	regexp.MustCompile(`\brediss?://:[^@\s/]{8,}@[^\s"'<>]+`),
}

type sevenFormatValue struct {
	name  string
	value string
}

var sevenFormatValid = []sevenFormatValue{
	{"google-oauth", "ya29.a0AfH6SMBexampleexampleexampleexample"},
	{"aws-session", "ASIAABCDEFGHIJKLMNOP"},
	{"stripe-test", "sk_test_abcdefghijklmnop"},
	{"stripe-webhook", "whsec_abcdefghijklmnopqrst"},
	{"docker-pat", "dckr_pat_abcdefghijklmnopqrst"},
	{"age-secret", "AGE-SECRET-KEY-1ABCDEFGHIJKLMNOPQRSTUVWXYZ012345"},
	{"redis-empty-user", "redis://:secretpassword@cache.example:6379/0"},
}

var sevenFormatNegative = []string{
	"ya29.short",
	"ya29example",
	"ASIAABCDEFGHIJKLMNO",
	"asiaABCDEFGHIJKLMNOP",
	"sk_test_short",
	"sk_test",
	"whsec_short",
	"whsec",
	"dckr_pat_short",
	"dckr_pat",
	"AGE-SECRET-KEY-1SHORT",
	"AGE-SECRET-KEY-1",
	"redis://:short@host",
	"0123456789abcdef0123456789abcdef01234567",
	"123e4567-e89b-12d3-a456-426614174000",
	"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	"SGVsbG8gV29ybGQhIFRoaXMgaXMgYW4gb3JkaW5hcnkgYmFzZTY0IHN0cmluZw==",
	"internal/toolharness/isolation.go",
	"maxIdentityScanEntries",
}

func proposedSevenFormatRedact(s string) string {
	for _, re := range proposedSevenFormatRules {
		s = re.ReplaceAllString(s, "[redacted]")
	}
	return s
}

func TestSevenFormatProposedPatterns(t *testing.T) {
	contexts := []struct {
		name   string
		format string
	}{
		{"ordinary-file", "file content:\n%s\n"},
		{"git-diff", "diff --git a/x b/x\n+%s\n"},
		{"command-output", "exit 0\n%s\n"},
		{"user-prompt", "Task:\n%s\n"},
	}
	for _, tc := range sevenFormatValid {
		for _, ctx := range contexts {
			t.Run(tc.name+"/"+ctx.name, func(t *testing.T) {
				input := fmt.Sprintf(ctx.format, tc.value)
				got := proposedSevenFormatRedact(input)
				if got == input || !strings.Contains(got, "[redacted]") {
					t.Fatalf("candidate pattern did not redact %s: %q", tc.name, got)
				}
				if strings.Contains(got, tc.value) {
					t.Fatalf("candidate pattern left the value in place: %q", got)
				}
			})
		}
	}
}

func TestSevenFormatProposedPatternsRejectNegativeControls(t *testing.T) {
	for _, value := range sevenFormatNegative {
		input := "value: " + value
		if got := proposedSevenFormatRedact(input); got != input {
			t.Fatalf("candidate pattern matched a negative control %q: %q", value, got)
		}
	}
	// The empty-user Redis rule must not match a normal user:password URI.
	const userInfo = "value: redis://user:password@cache.example:6379/0"
	if got := proposedSevenFormatRedact(userInfo); got != userInfo {
		t.Fatalf("candidate Redis rule matched a non-empty user URI: %q", got)
	}
}

// TestSevenFormatsCurrentlyUnredacted records the NEEDS_HUMAN gap. When the
// seven patterns are authorized and wired into production, this test must be
// updated to assert redaction instead.
func TestSevenFormatsCurrentlyUnredacted(t *testing.T) {
	for _, tc := range sevenFormatValid {
		input := "value: " + tc.value
		if got := ModelInputWithPolicy(input, nil, Policy{Enabled: true}); got != input {
			t.Fatalf("%s is already redacted; update the approval status and this test: %q", tc.name, got)
		}
	}
}

// C1 is opt-in; with it disabled, the seven formats must remain unchanged even
// after a future implementation.
func TestSevenFormatsC1DisabledUnchanged(t *testing.T) {
	for _, tc := range sevenFormatValid {
		input := "value: " + tc.value
		if got := ModelInputWithPolicy(input, nil, Policy{}); got != input {
			t.Fatalf("%s was redacted with C1 disabled: %q", tc.name, got)
		}
	}
}

// Malformed, truncated, and ordinary identifiers must remain unchanged; this is
// a permanent negative-control guard for the future implementation.
func TestSevenFormatNegativeControlsRemainUnchanged(t *testing.T) {
	for _, value := range sevenFormatNegative {
		input := "value: " + value
		if got := ModelInputWithPolicy(input, nil, Policy{Enabled: true}); got != input {
			t.Fatalf("negative control %q was redacted: %q", value, got)
		}
	}
}
