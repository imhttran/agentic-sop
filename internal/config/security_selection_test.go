package config

import (
	"strings"
	"testing"
)

func TestSecurityAdapterEnableSemantics(t *testing.T) {
	cfg, err := parseSecurityConfig(t, `
security:
  enabled: true
  adapters:
    - id: semgrep
      category: sast
      command: [semgrep]
      enabled: false
    - id: gitleaks
      category: secrets
      command: [gitleaks]
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Security.AdapterEnabled(cfg.Security.Adapters[0]) {
		t.Fatal("explicit enabled: false was treated as enabled")
	}
	if !cfg.Security.AdapterEnabled(cfg.Security.Adapters[1]) {
		t.Fatal("omitted enabled was not treated as enabled")
	}
}

func TestSecurityCategoryDisableWins(t *testing.T) {
	cfg, err := parseSecurityConfig(t, `
security:
  enabled: true
  categories:
    dependencies: false
  adapters:
    - id: cargo-audit
      category: dependencies
      command: [cargo, audit]
      enabled: true
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Security.CategoryEnabled(SecurityCategoryDependencies) {
		t.Fatal("explicit category false was treated as enabled")
	}
	if cfg.Security.AdapterEnabled(cfg.Security.Adapters[0]) {
		t.Fatal("adapter enabled: true overrode a category disable")
	}
}

func TestSecurityCategoryKeyUnknownRejected(t *testing.T) {
	_, err := parseSecurityConfig(t, "security:\n  enabled: true\n  categories:\n    dependency: false\n")
	if err == nil || !strings.Contains(err.Error(), "unknown security.categories key") {
		t.Fatalf("error = %v, want unknown category key", err)
	}
}

func TestSecurityCategoryNullRejected(t *testing.T) {
	_, err := parseSecurityConfig(t, "security:\n  enabled: true\n  categories:\n    sast:\n")
	if err == nil || !strings.Contains(err.Error(), "must be true or false") {
		t.Fatalf("error = %v, want a null category error", err)
	}
}

func TestSecurityRequiredCategoryAllDisabledRejected(t *testing.T) {
	_, err := parseSecurityConfig(t, `
security:
  enabled: true
  mode: required
  required_categories: [secrets]
  adapters:
    - id: gitleaks
      category: secrets
      command: [gitleaks]
      enabled: false
`)
	if err == nil || !strings.Contains(err.Error(), "every configured adapter for it is disabled") {
		t.Fatalf("error = %v, want disabled required category error", err)
	}
}

func TestSecurityOSVAdapterDefaults(t *testing.T) {
	cfg, err := parseSecurityConfig(t, `
security:
  enabled: true
  osv_database_path: .agent-sdlc/osv-db
  adapters:
    - id: osv-scanner
      category: dependencies
      version: "2.7.0"
`)
	if err != nil {
		t.Fatal(err)
	}
	adapter := cfg.Security.Adapters[0]
	if adapter.Protocol != SecurityProtocolBuiltin {
		t.Errorf("protocol = %q", adapter.Protocol)
	}
	if len(adapter.Command) != 1 || adapter.Command[0] != "osv-scanner" {
		t.Errorf("command = %v", adapter.Command)
	}
	if cfg.Security.OSVDatabasePath != ".agent-sdlc/osv-db" {
		t.Errorf("osv_database_path = %q", cfg.Security.OSVDatabasePath)
	}
	if cfg.Security.AdvisoryDatabasePath != "" {
		t.Errorf("osv configuration leaked into advisory_database_path: %q", cfg.Security.AdvisoryDatabasePath)
	}
}

func TestSecurityOSVAdapterRejectsLockfileField(t *testing.T) {
	_, err := parseSecurityConfig(t, `
security:
  enabled: true
  adapters:
    - id: osv-scanner
      category: dependencies
      lockfile: Cargo.lock
`)
	if err == nil || !strings.Contains(err.Error(), "accepts inputs, not lockfile") {
		t.Fatalf("error = %v, want inputs/lockfile conflict", err)
	}
}

func TestSecurityAdapterLanguages(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   string
		accept bool
		want   string
	}{
		{
			name:   "osv go and rust",
			body:   "    - id: osv-scanner\n      category: dependencies\n      languages: [go, rust]\n      inputs: [go.mod, Cargo.lock]\n",
			accept: true,
		},
		{
			name: "unknown language",
			body: "    - id: osv-scanner\n      category: dependencies\n      languages: [python]\n",
			want: "unknown language",
		},
		{
			name: "cargo audit cannot claim go",
			body: "    - id: cargo-audit\n      category: dependencies\n      languages: [go]\n",
			want: "covers only rust",
		},
		{
			name: "gitleaks is repository wide",
			body: "    - id: gitleaks\n      category: secrets\n      languages: [go]\n",
			want: "does not accept languages",
		},
		{
			name: "repository wide conflicts with languages",
			body: "    - id: private\n      category: sast\n      protocol: external\n      command: [./private]\n      repository_wide: true\n      languages: [go]\n",
			want: "cannot set both repository_wide and languages",
		},
		{
			name: "external secrets is repository wide",
			body: "    - id: private-secrets\n      category: secrets\n      protocol: external\n      command: [./private]\n      languages: [go]\n",
			want: "secrets scanner",
		},
		{
			name:   "external declared coverage",
			body:   "    - id: private\n      category: dependencies\n      protocol: external\n      command: [./private]\n      languages: [go]\n",
			accept: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSecurityConfig(t, "security:\n  enabled: true\n  adapters:\n"+tc.body)
			if tc.accept {
				if err != nil {
					t.Fatalf("valid configuration rejected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestSecurityExistingConfigurationUnchanged(t *testing.T) {
	cfg, err := parseSecurityConfig(t, `
security:
  enabled: true
  adapters:
    - id: semgrep
      category: sast
      command: [semgrep]
      rules: .agent-sdlc/security-rules
      version: "1.136.0"
    - id: gitleaks
      category: secrets
      command: [gitleaks]
      version: "8.30.1"
    - id: cargo-audit
      category: dependencies
      command: [cargo, audit]
      database: .agent-sdlc/advisory-db
      lockfile: Cargo.lock
      version: "0.22.2"
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Security.Adapters) != 3 {
		t.Fatalf("adapters = %d, want 3", len(cfg.Security.Adapters))
	}
	for _, adapter := range cfg.Security.Adapters {
		if !cfg.Security.AdapterEnabled(adapter) {
			t.Errorf("adapter %s was unexpectedly disabled", adapter.ID)
		}
		if adapter.Enabled != nil {
			t.Errorf("adapter %s gained an explicit enabled value", adapter.ID)
		}
	}
}

func TestSecurityDisabledBlockAcceptsOSVWithoutCommand(t *testing.T) {
	cfg, err := parseSecurityConfig(t, `
security:
  enabled: false
  adapters:
    - id: osv-scanner
      category: dependencies
`)
	if err != nil {
		t.Fatalf("disabled OSV adapter without a command was rejected: %v", err)
	}
	if cfg.SecurityEnabled() {
		t.Fatal("disabled block was enabled")
	}
}
