package config

import (
	"strings"
	"testing"
)

func parseSecurityConfig(t *testing.T, security string) (*Config, error) {
	t.Helper()
	doc := "version: 1\nproject:\n  name: test\n" + security
	return Parse([]byte(doc))
}

func TestSecurityDisabledByDefault(t *testing.T) {
	cfg, err := parseSecurityConfig(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SecurityEnabled() {
		t.Fatal("security add-on is enabled by default")
	}
	if len(cfg.Security.Adapters) != 0 || cfg.Security.Mode != "" || cfg.Security.Scope != "" {
		t.Errorf("disabled block was mutated: %+v", cfg.Security)
	}
}

func TestSecurityEnabledDefaults(t *testing.T) {
	cfg, err := parseSecurityConfig(t, "security:\n  enabled: true\n")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SecurityEnabled() {
		t.Fatal("security add-on was not enabled")
	}
	if cfg.Security.Mode != SecurityModeReportOnly {
		t.Errorf("mode = %q, want report-only", cfg.Security.Mode)
	}
	if cfg.Security.Scope != SecurityScopeWorkspace {
		t.Errorf("scope = %q, want workspace", cfg.Security.Scope)
	}
	if len(cfg.Security.FailOn) != 2 || cfg.Security.FailOn[0] != "critical" || cfg.Security.FailOn[1] != "high" {
		t.Errorf("fail_on = %v", cfg.Security.FailOn)
	}
	if cfg.Security.UnknownSeverity != SecurityUnknownIndeterminate {
		t.Errorf("unknown severity = %q", cfg.Security.UnknownSeverity)
	}
	if cfg.Security.TimeoutSeconds != 300 || cfg.Security.MaxOutputBytes != 1<<20 {
		t.Errorf("bounds = %d/%d", cfg.Security.TimeoutSeconds, cfg.Security.MaxOutputBytes)
	}
	if len(cfg.Security.Adapters) != 2 {
		t.Fatalf("adapters = %d, want 2 built-ins", len(cfg.Security.Adapters))
	}
	if cfg.Security.Adapters[0].ID != "semgrep" || cfg.Security.Adapters[0].Category != SecurityCategorySAST {
		t.Errorf("first adapter = %+v", cfg.Security.Adapters[0])
	}
	if cfg.Security.Adapters[1].ID != "gitleaks" || cfg.Security.Adapters[1].Category != SecurityCategorySecrets {
		t.Errorf("second adapter = %+v", cfg.Security.Adapters[1])
	}
}

func TestSecurityRequiredNeedsCategories(t *testing.T) {
	_, err := parseSecurityConfig(t, "security:\n  enabled: true\n  mode: required\n")
	if err == nil || !strings.Contains(err.Error(), "required_categories") {
		t.Fatalf("error = %v, want required_categories", err)
	}
}

func TestSecurityUnknownBuiltinAdapterRejected(t *testing.T) {
	_, err := parseSecurityConfig(t, "security:\n  enabled: true\n  adapters:\n    - id: mystery\n      category: sast\n")
	if err == nil || !strings.Contains(err.Error(), "unknown built-in") {
		t.Fatalf("error = %v, want unknown built-in adapter", err)
	}
}

func TestSecurityExternalAdapterRequiresCommand(t *testing.T) {
	_, err := parseSecurityConfig(t, "security:\n  enabled: true\n  adapters:\n    - id: private\n      category: sast\n      protocol: external\n")
	if err == nil || !strings.Contains(err.Error(), "requires a command") {
		t.Fatalf("error = %v, want command requirement", err)
	}
}

func TestSecurityRequiredCategoryNeedsAdapter(t *testing.T) {
	_, err := parseSecurityConfig(t, "security:\n  enabled: true\n  mode: required\n  required_categories:\n    - secrets\n  adapters:\n    - id: semgrep\n      category: sast\n")
	if err == nil || !strings.Contains(err.Error(), "no configured adapter") {
		t.Fatalf("error = %v, want missing adapter", err)
	}
}

func TestSecurityBaselineMustStayInWorkspace(t *testing.T) {
	for _, path := range []string{"/abs.json", "../outside.json"} {
		_, err := parseSecurityConfig(t, "security:\n  enabled: true\n  baseline: "+path+"\n")
		if err == nil {
			t.Errorf("baseline %q was accepted", path)
		}
	}
}

func TestSecurityDisabledBlockStillValidatesExplicitValues(t *testing.T) {
	_, err := parseSecurityConfig(t, "security:\n  enabled: false\n  mode: nonsense\n")
	if err == nil {
		t.Fatal("an explicit invalid mode was accepted in a disabled block")
	}
}

func TestSecurityEnvironmentEnablesWhenProjectOmitted(t *testing.T) {
	t.Setenv(envSecurityEnabled, "true")
	cfg, err := parseSecurityConfig(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SecurityEnabled() {
		t.Fatal("SOP_SECURITY_ENABLED=true did not enable the add-on")
	}
	if cfg.Security.Mode != SecurityModeReportOnly || len(cfg.Security.Adapters) != 2 {
		t.Errorf("environment defaults = %+v", cfg.Security)
	}
}

func TestResolveSecurityEnvOnDefaultConfig(t *testing.T) {
	t.Setenv(envSecurityEnabled, "true")
	cfg := Default()
	if err := ResolveSecurityEnv(&cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.SecurityEnabled() || cfg.Security.Mode != SecurityModeReportOnly {
		t.Errorf("resolved security = %+v", cfg.Security)
	}
}

func TestSecurityExplicitProjectValueWinsOverEnvironment(t *testing.T) {
	t.Setenv(envSecurityEnabled, "true")
	cfg, err := parseSecurityConfig(t, "security:\n  enabled: false\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SecurityEnabled() {
		t.Fatal("explicit enabled: false was overwritten by the environment")
	}
}
