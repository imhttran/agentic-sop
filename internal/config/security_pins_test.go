package config

import (
	"strings"
	"testing"
)

func TestSecurityAdapterPinsValidate(t *testing.T) {
	good := strings.Repeat("a", 64)
	cases := []struct {
		name string
		a    SecurityAdapterConfig
		want string // "" means valid
	}{
		{"no pins", SecurityAdapterConfig{}, ""},
		{"valid pins", SecurityAdapterConfig{BinarySHA256: good, Rules: "rules", RulesSHA256: good}, ""},
		{"short binary pin", SecurityAdapterConfig{BinarySHA256: "abc"}, "binary_sha256"},
		{"uppercase binary pin", SecurityAdapterConfig{BinarySHA256: strings.Repeat("A", 64)}, "binary_sha256"},
		{"prefixed rules pin", SecurityAdapterConfig{Rules: "rules", RulesSHA256: "sha256:" + good[:57]}, "rules_sha256"},
		{"rules pin without rules", SecurityAdapterConfig{RulesSHA256: good}, "without rules"},
		{"valid platform pin", SecurityAdapterConfig{BinarySHA256ByPlatform: map[string]string{"darwin/arm64": good}}, ""},
		{"unknown platform key", SecurityAdapterConfig{BinarySHA256ByPlatform: map[string]string{"windows/amd64": good}}, "not a supported platform"},
		{"malformed platform pin", SecurityAdapterConfig{BinarySHA256ByPlatform: map[string]string{"linux/amd64": "abc"}}, "binary_sha256_by_platform"},
		{"both pin forms", SecurityAdapterConfig{BinarySHA256: good, BinarySHA256ByPlatform: map[string]string{"darwin/arm64": good}}, "both"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.a
			a.ID, a.Category, a.Protocol, a.Command = "gitleaks", SecurityCategorySecrets, SecurityProtocolBuiltin, []string{"gitleaks"}
			err := validateSecurityAdapterConfigs(SecurityConfig{Adapters: []SecurityAdapterConfig{a}}, false)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}
