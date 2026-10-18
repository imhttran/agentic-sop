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

func TestSecurityExcludesMustStayInWorkspace(t *testing.T) {
	for _, path := range []string{"C:/abs", "../outside", "internal/../outside"} {
		_, err := parseSecurityConfig(t, "security:\n  enabled: true\n  exclude:\n    - "+path+"\n")
		if err == nil {
			t.Errorf("exclude %q was accepted", path)
		}
	}
}

func TestSecurityExcludeLeadingSlashAnchorsToRoot(t *testing.T) {
	cfg, err := parseSecurityConfig(t, "security:\n  enabled: true\n  exclude:\n    - /sop\n")
	if err != nil {
		t.Fatalf("root-anchored exclusion was rejected: %v", err)
	}
	if got := cfg.SecurityExcludes(); got[len(got)-1] != "/sop" {
		t.Fatalf("excludes = %v, want root-anchored /sop", got)
	}
}

func TestSecurityExcludesAreAdditiveAndDeterministic(t *testing.T) {
	cfg, err := parseSecurityConfig(t, "security:\n  enabled: true\n  exclude:\n    - internal/security/testdata\n    - .git\n    - internal/security/testdata\n")
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.SecurityExcludes()
	want := []string{".git", ".agent-sdlc", "internal/security/testdata"}
	if len(got) != len(want) {
		t.Fatalf("excludes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("excludes = %v, want %v", got, want)
		}
	}
}

func TestSecurityExcludesDoNotImplicitlyExcludeEnv(t *testing.T) {
	cfg, err := parseSecurityConfig(t, "security:\n  enabled: true\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, pattern := range cfg.SecurityExcludes() {
		if pattern == ".env" || pattern == "/.env" || pattern == ".env.*" || pattern == "/.env.*" {
			t.Fatalf("default exclusions unexpectedly hide .env files: %v", cfg.SecurityExcludes())
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

func TestSecurityCargoAuditAdapterAccepted(t *testing.T) {
	cfg, err := parseSecurityConfig(t, "security:\n  enabled: true\n  advisory_database_path: .agent-sdlc/advisory-db\n  stale_advisory_database: deny\n  adapters:\n    - id: cargo-audit\n      category: dependencies\n      protocol: builtin\n      command: [cargo, audit]\n      database: .agent-sdlc/advisory-db\n      lockfile: Cargo.lock\n      stale: allow\n      version: \"0.21.2\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Security.Adapters) != 1 || cfg.Security.Adapters[0].Category != SecurityCategoryDependencies {
		t.Fatalf("adapters = %+v", cfg.Security.Adapters)
	}
	if cfg.Security.Adapters[0].Database != ".agent-sdlc/advisory-db" || cfg.Security.Adapters[0].Stale != "allow" {
		t.Fatalf("adapter = %+v", cfg.Security.Adapters[0])
	}
}

func TestSecurityCargoAuditRequiresDependenciesCategory(t *testing.T) {
	_, err := parseSecurityConfig(t, "security:\n  enabled: true\n  adapters:\n    - id: cargo-audit\n      category: sast\n")
	if err == nil || !strings.Contains(err.Error(), "dependencies") {
		t.Fatalf("error = %v, want category mismatch", err)
	}
}

func TestSecurityCargoAuditDefaults(t *testing.T) {
	cfg, err := parseSecurityConfig(t, "security:\n  enabled: true\n  adapters:\n    - id: cargo-audit\n")
	if err != nil {
		t.Fatal(err)
	}
	adapter := cfg.Security.Adapters[0]
	if adapter.Category != SecurityCategoryDependencies {
		t.Errorf("category = %q", adapter.Category)
	}
	if len(adapter.Command) != 2 || adapter.Command[0] != "cargo" || adapter.Command[1] != "audit" {
		t.Errorf("command = %v", adapter.Command)
	}
}

func TestSecurityAdapterPathsStaySafe(t *testing.T) {
	for name, body := range map[string]string{
		"lockfile escapes":  "security:\n  enabled: true\n  adapters:\n    - id: cargo-audit\n      category: dependencies\n      command: [cargo, audit]\n      lockfile: ../Cargo.lock\n",
		"lockfile absolute": "security:\n  enabled: true\n  adapters:\n    - id: cargo-audit\n      category: dependencies\n      command: [cargo, audit]\n      lockfile: /Cargo.lock\n",
		"database escapes":  "security:\n  enabled: true\n  adapters:\n    - id: cargo-audit\n      category: dependencies\n      command: [cargo, audit]\n      database: ../advisory-db\n",
		"stale unknown":     "security:\n  enabled: true\n  adapters:\n    - id: cargo-audit\n      category: dependencies\n      command: [cargo, audit]\n      stale: sometimes\n",
	} {
		if _, err := parseSecurityConfig(t, body); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestSecurityRequiredDependenciesCategoryAccepted(t *testing.T) {
	if _, err := parseSecurityConfig(t, "security:\n  enabled: true\n  mode: required\n  required_categories: [dependencies]\n  adapters:\n    - id: cargo-audit\n      category: dependencies\n      command: [cargo, audit]\n"); err != nil {
		t.Fatalf("required dependencies category rejected: %v", err)
	}
}

func TestSecurityGlobalAdvisoryDatabaseMayBeAbsolute(t *testing.T) {
	if _, err := parseSecurityConfig(t, "security:\n  enabled: true\n  advisory_database_path: /var/lib/advisory-db\n"); err != nil {
		t.Fatalf("absolute advisory database path rejected: %v", err)
	}
}
