package redact

import (
	"fmt"
	"strings"
	"testing"
)

// Enforcement coverage for the seven-format C1 enhancement approved by the
// repository operator (decision memory repository c7f143fe...; source
// operator:llm02-seven-format-2026-11-10).

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

func TestSevenFormatsRedactedWithC1(t *testing.T) {
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
				got := ModelInputWithPolicy(input, nil, Policy{Enabled: true})
				if got == input || !strings.Contains(got, "[redacted]") {
					t.Fatalf("C1 did not redact %s: %q", tc.name, got)
				}
				if strings.Contains(got, tc.value) {
					t.Fatalf("C1 left the value in place: %q", got)
				}
			})
		}
	}
}

// C1 is opt-in; with it disabled, the seven formats must remain unchanged.
func TestSevenFormatsC1DisabledUnchanged(t *testing.T) {
	for _, tc := range sevenFormatValid {
		input := "value: " + tc.value
		if got := ModelInputWithPolicy(input, nil, Policy{}); got != input {
			t.Fatalf("%s was redacted with C1 disabled: %q", tc.name, got)
		}
	}
}

// Malformed, truncated, and ordinary identifiers must remain unchanged.
func TestSevenFormatNegativeControlsRemainUnchanged(t *testing.T) {
	for _, value := range sevenFormatNegative {
		input := "value: " + value
		if got := ModelInputWithPolicy(input, nil, Policy{Enabled: true}); got != input {
			t.Fatalf("negative control %q was redacted: %q", value, got)
		}
	}
}

// A normal user:password Redis URI is covered by the existing A+B URL-userinfo
// shape; the new empty-user rule must not change that behavior.
func TestSevenFormatRedisUserInfoUnchangedBehavior(t *testing.T) {
	const input = "value: redis://user:password@cache.example:6379/0"
	got := ModelInputWithPolicy(input, nil, Policy{Enabled: true})
	if !strings.Contains(got, "[redacted]") {
		t.Fatalf("the existing URL-userinfo redaction did not apply: %q", got)
	}
}
