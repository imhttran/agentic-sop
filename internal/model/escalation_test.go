package model

import (
	"strings"
	"testing"
)

// envMap returns a lookup function over a fixed map, so the escalation resolvers
// can be tested without touching the process environment.
func envMap(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

// intPtr returns a pointer to an int, for the optional escalation bound.
func intPtr(n int) *int { return &n }

func TestEscalationEnabledResolution(t *testing.T) {
	cases := []struct {
		name   string
		config Route
		env    map[string]string
		want   bool
	}{
		{
			name: "default is off",
			want: false,
		},
		{
			name:   "config enables",
			config: Route{EscalationEnabled: boolPtr(true)},
			want:   true,
		},
		{
			name:   "config disables explicitly",
			config: Route{EscalationEnabled: boolPtr(false)},
			want:   false,
		},
		{
			name:   "environment overrides config",
			config: Route{EscalationEnabled: boolPtr(false)},
			env:    map[string]string{EnvEscalationEnabled: "true"},
			want:   true,
		},
		{
			name:   "environment can disable a configured enable",
			config: Route{EscalationEnabled: boolPtr(true)},
			env:    map[string]string{EnvEscalationEnabled: "false"},
			want:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := EscalationEnabled(tc.config, envMap(tc.env))
			if err != nil {
				t.Fatalf("EscalationEnabled: %v", err)
			}
			if got != tc.want {
				t.Errorf("EscalationEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEscalationEnabledInvalidEnv(t *testing.T) {
	_, err := EscalationEnabled(Route{}, envMap(map[string]string{EnvEscalationEnabled: "maybe"}))
	if err == nil || !strings.Contains(err.Error(), EnvEscalationEnabled) {
		t.Fatalf("error = %v, want one naming %s", err, EnvEscalationEnabled)
	}
}

func TestMaxEscalationsResolution(t *testing.T) {
	cases := []struct {
		name   string
		config Route
		env    map[string]string
		want   int
	}{
		{name: "default", want: DefaultMaxEscalations},
		{name: "config value", config: Route{MaxEscalations: intPtr(5)}, want: 5},
		{name: "config zero disables", config: Route{MaxEscalations: intPtr(0)}, want: 0},
		{name: "environment overrides config", config: Route{MaxEscalations: intPtr(5)}, env: map[string]string{EnvMaxEscalations: "1"}, want: 1},
		{name: "environment zero disables", config: Route{MaxEscalations: intPtr(5)}, env: map[string]string{EnvMaxEscalations: "0"}, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MaxEscalations(tc.config, envMap(tc.env))
			if err != nil {
				t.Fatalf("MaxEscalations: %v", err)
			}
			if got != tc.want {
				t.Errorf("MaxEscalations = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestMaxEscalationsInvalidEnv(t *testing.T) {
	for _, v := range []string{"lots", "-1"} {
		_, err := MaxEscalations(Route{}, envMap(map[string]string{EnvMaxEscalations: v}))
		if err == nil || !strings.Contains(err.Error(), EnvMaxEscalations) {
			t.Fatalf("MaxEscalations(%q) error = %v, want one naming %s", v, err, EnvMaxEscalations)
		}
	}
}

// TestEscalationConfigDoesNotActivateRouting proves the escalation fields are not
// routing values: enabling escalation alone neither makes the route table
// Configured() nor activates Resolve, so an existing installation's agent selection
// is unchanged.
func TestEscalationConfigDoesNotActivateRouting(t *testing.T) {
	cfg := Route{EscalationEnabled: boolPtr(true), MaxEscalations: intPtr(3)}
	if cfg.Configured() {
		t.Fatal("escalation fields must not make the route table Configured()")
	}
	res, err := Resolve(Inputs{Config: cfg, Lookup: envMap(nil)})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Active {
		t.Fatalf("escalation alone must not activate routing: %+v", res)
	}
}

func TestValidateRejectsNegativeMaxEscalations(t *testing.T) {
	err := Route{MaxEscalations: intPtr(-1)}.Validate()
	if err == nil || !strings.Contains(err.Error(), "max_escalations") {
		t.Fatalf("Validate = %v, want an error naming max_escalations", err)
	}
}
