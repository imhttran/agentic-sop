package redact

import (
	"strings"
	"testing"
)

func TestModelInputWithPolicyContextualKeys(t *testing.T) {
	p := Policy{Enabled: true}
	cases := []struct{ name, in, want string }{
		{"opaque secret", "opaque_secret=opaque-live-value-42", "opaque_secret=[redacted]"},
		{"api key", "api_key=abc123def456", "api_key=[redacted]"},
		{"auth token", "auth_token: abc123def456", "auth_token: [redacted]"},
		{"client secret", "client_secret=abc123def456", "client_secret=[redacted]"},
		{"json", `{"opaque_secret":"opaque-live-value-42"}`, `{"opaque_secret":"[redacted]"}`},
		{"diff prefix", "+opaque_secret=opaque-live-value-42", "+opaque_secret=[redacted]"},
		{"spaced assignment", `opaque_secret = "value with spaces"`, `opaque_secret = "[redacted]"`},
		{"export", "export OPAQUE_SECRET=opaque-live-value-42", "export OPAQUE_SECRET=[redacted]"},
		{"authorization", "Authorization: Bearer abcdefghijkl", "Authorization: Bearer [redacted]"},
		{"suffixed password", "password1=opaque-live-value-42", "password1=[redacted]"},
		{"suffixed api key", "api_key2=opaque-live-value-42", "api_key2=[redacted]"},
		{"camel token", "tokenValue=opaque-live-value-42", "tokenValue=[redacted]"},
		{"camel secret key", "secretKey=opaque-live-value-42", "secretKey=[redacted]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ModelInputWithPolicy(tc.in, nil, p); got != tc.want {
				t.Errorf("ModelInputWithPolicy(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestModelInputWithPolicyContextualFalsePositives(t *testing.T) {
	p := Policy{Enabled: true}
	for _, in := range []string{
		"secretary=Jane",
		"secretary1=Jane",
		"keyboard=us",
		"monkey=banana",
		"public_key=abc123",
		"publicKey=abc123",
		"ssh_public_key=abc123",
		"https://example.com/path",
		"GET https://example.com/v1/status -> 200",
		"token budget exhausted",
		"sk-learn tutorial",
		"index 57138f2..1df0179 100644",
		"time: 12:30",
		"SOP_TEST_API_KEY=replace-me",
		"SOP_TEST_API_KEY=changeme",
	} {
		if got := ModelInputWithPolicy(in, nil, p); got != in {
			t.Errorf("ModelInputWithPolicy(%q) changed ordinary text to %q", in, got)
		}
	}
}

func TestModelInputWithPolicyInventory(t *testing.T) {
	p := Policy{Enabled: true, Values: []string{"bare-opaque-value-88"}}
	in := "ordinary text bare-opaque-value-88 more text"
	got := ModelInputWithPolicy(in, nil, p)
	if strings.Contains(got, "bare-opaque-value-88") || !strings.Contains(got, "[redacted]") {
		t.Fatalf("inventory value was not redacted: %q", got)
	}
	if ModelInput(in, nil) != in {
		t.Fatalf("default ModelInput should preserve the previous behavior: %q", ModelInput(in, nil))
	}
}

func TestModelInputWithPolicyKnownFormats(t *testing.T) {
	p := Policy{Enabled: true}
	for _, token := range []string{
		"glpat-abcdefghijklmnopqrst",
		"npm_abcdefghijklmnopqrstuvwxyz0123456789",
		"pypi-abcdefghijklmnop",
		"shpat_abcdefghijklmnopqrstuvwxyz012345",
		"ghs_abcdefghijklmnopqrst",
		"ghu_abcdefghijklmnopqrst",
		"AIzaabcdefghijklmnopqrstuvwxyz012345678",
		"SG.abcdefghijklmnop.qrstuvwxyz012345",
		"sk_live_abcdefghijklmnop",
		"rk_live_abcdefghijklmnop",
		"pk_live_abcdefghijklmnop",
	} {
		got := ModelInputWithPolicy("token "+token+" end", nil, p)
		if strings.Contains(got, token) || !strings.Contains(got, "[redacted]") {
			t.Errorf("known format %q was not redacted: %q", token, got)
		}
	}
	for _, in := range []string{
		"glpat-short",
		"npm_short",
		"AIza-short",
		"SG.short",
		"sk_live_short",
	} {
		if got := ModelInputWithPolicy(in, nil, p); got != in {
			t.Errorf("short pseudo-token %q was changed: %q", in, got)
		}
	}
}
