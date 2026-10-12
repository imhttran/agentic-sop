package cli

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/verify"
)

// jevCfg builds a config with JEV optionally enabled and risk-based verification
// optionally on.
func jevCfg(jevEnabled, riskBased bool) config.Config {
	cfg := config.Config{Quality: config.Quality{JEV: config.JEV{Enabled: boolPtr(jevEnabled)}}}
	if riskBased {
		cfg.Verification = config.Verification{RiskBased: boolPtr(true)}
	}
	return cfg
}

// TestShouldRunJEV proves the tier-gated JEV contract: JEV must be enabled, and with
// risk-based verification on it runs only for the HIGH tier (LOW/MEDIUM keep it
// optional). With the feature off, an enabled JEV runs exactly as before.
func TestShouldRunJEV(t *testing.T) {
	cases := []struct {
		name      string
		jev       bool
		riskBased bool
		tier      verify.Tier
		want      bool
	}{
		{"JEV disabled never runs", false, true, verify.High, false},
		{"enabled, feature off, any tier runs", true, false, verify.Low, true},
		{"enabled, feature off, unset tier runs", true, false, "", true},
		{"enabled, risk-based, LOW does not run", true, true, verify.Low, false},
		{"enabled, risk-based, MEDIUM does not run", true, true, verify.Medium, false},
		{"enabled, risk-based, HIGH runs", true, true, verify.High, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldRunJEV(jevCfg(tc.jev, tc.riskBased), tc.tier); got != tc.want {
				t.Errorf("shouldRunJEV = %v, want %v", got, tc.want)
			}
		})
	}
}
