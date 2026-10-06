package model

import (
	"strings"
	"testing"
)

// These tests pin the Phase 7 bounded strategy-replanning configuration: the flag
// is OFF by default, the environment overrides the config, the bound is bounded
// and deterministic, and enabling replanning alone changes no agent selection.

func TestReplanEnabledResolution(t *testing.T) {
	cases := []struct {
		name   string
		config Route
		env    map[string]string
		want   bool
	}{
		{name: "default is off", want: false},
		{name: "config enables", config: Route{ReplanEnabled: boolPtr(true)}, want: true},
		{name: "config disables explicitly", config: Route{ReplanEnabled: boolPtr(false)}, want: false},
		{name: "environment overrides config", config: Route{ReplanEnabled: boolPtr(false)}, env: map[string]string{EnvReplanEnabled: "true"}, want: true},
		{name: "environment can disable a configured enable", config: Route{ReplanEnabled: boolPtr(true)}, env: map[string]string{EnvReplanEnabled: "false"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReplanEnabled(tc.config, envMap(tc.env))
			if err != nil {
				t.Fatalf("ReplanEnabled: %v", err)
			}
			if got != tc.want {
				t.Errorf("ReplanEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReplanEnabledInvalidEnv(t *testing.T) {
	_, err := ReplanEnabled(Route{}, envMap(map[string]string{EnvReplanEnabled: "maybe"}))
	if err == nil || !strings.Contains(err.Error(), EnvReplanEnabled) {
		t.Fatalf("error = %v, want one naming %s", err, EnvReplanEnabled)
	}
}

func TestMaxReplansResolution(t *testing.T) {
	cases := []struct {
		name   string
		config Route
		env    map[string]string
		want   int
	}{
		{name: "default", want: DefaultMaxReplans},
		{name: "config value", config: Route{MaxReplans: intPtr(3)}, want: 3},
		{name: "config zero disables", config: Route{MaxReplans: intPtr(0)}, want: 0},
		{name: "environment overrides config", config: Route{MaxReplans: intPtr(3)}, env: map[string]string{EnvMaxReplans: "2"}, want: 2},
		{name: "environment zero disables", config: Route{MaxReplans: intPtr(3)}, env: map[string]string{EnvMaxReplans: "0"}, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MaxReplans(tc.config, envMap(tc.env))
			if err != nil {
				t.Fatalf("MaxReplans: %v", err)
			}
			if got != tc.want {
				t.Errorf("MaxReplans = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestMaxReplansInvalidEnv(t *testing.T) {
	for _, v := range []string{"lots", "-1"} {
		_, err := MaxReplans(Route{}, envMap(map[string]string{EnvMaxReplans: v}))
		if err == nil || !strings.Contains(err.Error(), EnvMaxReplans) {
			t.Fatalf("MaxReplans(%q) error = %v, want one naming %s", v, err, EnvMaxReplans)
		}
	}
}

// TestReplanConfigDoesNotActivateRouting proves the replan fields are not routing
// values: enabling replanning alone neither makes the route table Configured() nor
// activates Resolve, so an existing installation's agent selection is unchanged.
func TestReplanConfigDoesNotActivateRouting(t *testing.T) {
	cfg := Route{ReplanEnabled: boolPtr(true), MaxReplans: intPtr(1)}
	if cfg.Configured() {
		t.Fatal("replan fields must not make the route table Configured()")
	}
	res, err := Resolve(Inputs{Config: cfg, Lookup: envMap(nil)})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Active {
		t.Fatalf("replan alone must not activate routing: %+v", res)
	}
}

func TestValidateRejectsNegativeMaxReplans(t *testing.T) {
	err := Route{MaxReplans: intPtr(-1)}.Validate()
	if err == nil || !strings.Contains(err.Error(), "max_replans") {
		t.Fatalf("Validate = %v, want an error naming max_replans", err)
	}
}
