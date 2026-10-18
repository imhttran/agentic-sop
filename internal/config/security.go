package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SecurityConfig is the optional security scanner add-on configuration. It is
// disabled unless explicitly enabled by configuration or SOP_SECURITY_ENABLED;
// an omitted block leaves existing behavior and configuration bytes unchanged.
type SecurityConfig struct {
	// Enabled is a pointer so an omitted value (disabled) is distinguishable from
	// an explicit false. The environment may fill it when the project omitted it.
	Enabled *bool `yaml:"enabled"`
	// Mode is report-only (the MVP default) or required. Report-only records a
	// verdict without blocking delivery; required blocks on FAIL/INDETERMINATE.
	Mode string `yaml:"mode"`
	// Scope is the scan scope. workspace is the MVP default and the only scope
	// that can satisfy a required full-scan policy.
	Scope string `yaml:"scope"`
	// RequiredCategories names the categories a required gate must cover.
	RequiredCategories []string `yaml:"required_categories"`
	// FailOn lists normalized severities that produce a FAIL verdict.
	FailOn []string `yaml:"fail_on"`
	// UnknownSeverity is the explicit policy for a finding whose provider severity
	// cannot be mapped: fail, indeterminate, or ignore. It never silently becomes
	// low severity.
	UnknownSeverity string `yaml:"unknown_severity"`
	// TimeoutSeconds bounds one adapter invocation. Zero means the default.
	TimeoutSeconds int `yaml:"timeout_seconds"`
	// MaxOutputBytes bounds one adapter's captured and persisted report. Zero
	// means the default.
	MaxOutputBytes int `yaml:"max_output_bytes"`
	// Baseline is the repository-relative reviewed baseline/suppression file. An
	// absent file means no suppressions.
	Baseline string `yaml:"baseline"`
	// Exclude declares repository-relative paths or path prefixes that are outside
	// the requested scan scope. Exclusions are recorded in the scan evidence; they
	// are a declared scope boundary, not a coverage failure.
	Exclude []string `yaml:"exclude"`
	// AdvisoryDatabasePath is the local advisory database path a cargo-audit-style
	// dependency adapter uses. It is explicit so a scan never downloads a database
	// implicitly. OSV-Scanner uses OSVDatabasePath because the two database formats
	// are not interchangeable.
	AdvisoryDatabasePath string `yaml:"advisory_database_path"`
	// OSVDatabasePath is the local OSV-Scanner offline database cache root. It is a
	// separate input from AdvisoryDatabasePath: an OSV database and a cargo-audit
	// advisory database have different layouts and must never be substituted for
	// one another.
	OSVDatabasePath string `yaml:"osv_database_path"`
	// StaleAdvisoryDatabase is the policy for a stale advisory database: deny
	// (the default) or allow. Deny makes a stale database INDETERMINATE; allow
	// keeps the verdict path and records a PARTIAL coverage diagnostic.
	StaleAdvisoryDatabase string `yaml:"stale_advisory_database"`
	// Auto enables automatic adapter selection. It is report-only configuration
	// policy and never changes a verdict by itself. Automatic selection fills only
	// unconfigured, enabled coverage; it never overrides an explicit disable.
	Auto bool `yaml:"auto"`
	// Categories enables or disables a whole category. A pointer distinguishes an
	// omitted entry (inherit the global enabled state) from an explicit false
	// (disable every adapter in the category). A category disable cannot be
	// re-enabled by automatic selection.
	Categories map[string]*bool `yaml:"categories"`
	// Adapters selects the adapters by stable ID and category. It is replaceable
	// through configuration alone; the runner never branches on a specific ID.
	Adapters []SecurityAdapterConfig `yaml:"adapters"`
}

// SecurityAdapterConfig is one configured adapter. Built-in adapters wrap a
// pinned scanner CLI; external adapters speak the versioned process protocol.
type SecurityAdapterConfig struct {
	ID       string   `yaml:"id"`
	Category string   `yaml:"category"`
	Protocol string   `yaml:"protocol"`
	Command  []string `yaml:"command"`
	// Enabled is a pointer so an omitted value inherits the category/global enable
	// state, while an explicit false disables this adapter and blocks automatic
	// selection from re-adding it.
	Enabled *bool `yaml:"enabled"`
	// Languages declares the Go/Rust coverage this adapter provides. Empty means
	// the built-in adapter's default metadata for built-ins, and "undeclared" for
	// an external adapter. An undeclared external adapter can never prove
	// language-specific required coverage.
	Languages []string `yaml:"languages"`
	// RepositoryWide declares that the adapter inspects the repository as a whole
	// rather than a language set. It is implicit for secrets scanners. It is
	// mutually exclusive with a non-empty Languages list.
	RepositoryWide *bool `yaml:"repository_wide"`
	// Inputs names explicit dependency inputs for OSV-Scanner (go.mod or
	// Cargo.lock). When empty, inputs are resolved from detected repository content.
	Inputs []string `yaml:"inputs"`
	// Rules is the local rules path for a Semgrep adapter. It is explicit so a
	// scan never downloads rules implicitly.
	Rules string `yaml:"rules"`
	// Database is the local advisory database path for a dependency adapter. It is
	// explicit so a scan never downloads or updates a database implicitly.
	Database string `yaml:"database"`
	// Lockfile optionally names one lockfile for a dependency adapter. When it is
	// empty the adapter resolves the repository's actual lockfiles from content.
	// It is always passed to the scanner explicitly; the adapter never generates
	// or modifies a lockfile.
	Lockfile string `yaml:"lockfile"`
	// Stale is this adapter's stale-database policy: deny (the default) or allow.
	// When empty the global security.stale_advisory_database value applies.
	Stale string `yaml:"stale"`
	// Version is the expected/pinned scanner version recorded as provenance. It is
	// descriptive unless an adapter chooses to enforce it.
	Version string `yaml:"version"`
}

// Security categories.
const (
	SecurityCategorySAST         = "sast"
	SecurityCategorySecrets      = "secrets"
	SecurityCategoryDependencies = "dependencies"
)

// Stale advisory database policies. The empty value means deny.
const (
	SecurityStaleDeny  = "deny"
	SecurityStaleAllow = "allow"
)

// Security policies.
const (
	SecurityModeReportOnly = "report-only"
	SecurityModeRequired   = "required"
	SecurityScopeWorkspace = "workspace"
	SecurityScopeChanged   = "changed-files"

	SecurityUnknownFail          = "fail"
	SecurityUnknownIndeterminate = "indeterminate"
	SecurityUnknownIgnore        = "ignore"

	SecurityProtocolBuiltin  = "builtin"
	SecurityProtocolExternal = "external"

	SecurityLanguageGo   = "go"
	SecurityLanguageRust = "rust"
)

// Environment configuration for the add-on. Like the other optional layers, the
// environment fills only values the project omitted, and an explicit project
// value always wins.
const (
	envSecurityEnabled  = "SOP_SECURITY_ENABLED"
	envSecurityMode     = "SOP_SECURITY_MODE"
	envSecurityBaseline = "SOP_SECURITY_BASELINE"
)

// SecurityEnabled reports whether the add-on is explicitly enabled.
func (c Config) SecurityEnabled() bool {
	return c.Security.Enabled != nil && *c.Security.Enabled
}

// SecurityGateActive reports whether the security stage must run. It is true when
// the add-on is enabled or when an already configured required gate is active. A
// global disable must not silently exempt an active required gate; the stage runs
// and reports INDETERMINATE instead of NOT_APPLICABLE.
func (c Config) SecurityGateActive() bool {
	if c.SecurityEnabled() {
		return true
	}
	return c.Security.RequiredGateActive()
}

// RequiredGateActive reports whether configuration declares a required security
// gate. A required mode without required categories is not an active gate.
func (s SecurityConfig) RequiredGateActive() bool {
	return strings.TrimSpace(s.Mode) == SecurityModeRequired && len(s.RequiredCategories) > 0
}

// CategoryEnabled reports whether a category is enabled. An omitted entry
// inherits the global enabled state (true when this method is consulted on an
// enabled block). An explicit false disables the whole category.
func (s SecurityConfig) CategoryEnabled(category string) bool {
	key := strings.ToLower(strings.TrimSpace(category))
	if value, ok := s.Categories[key]; ok && value != nil {
		return *value
	}
	return true
}

// AdapterEnabled reports whether one adapter is enabled after applying the
// category and adapter controls. A category or adapter explicit false wins; an
// omitted value inherits the enabled default.
func (s SecurityConfig) AdapterEnabled(adapter SecurityAdapterConfig) bool {
	category := strings.ToLower(strings.TrimSpace(adapter.Category))
	if category == "" {
		category = defaultCategoryForID(strings.ToLower(strings.TrimSpace(adapter.ID)))
	}
	if category != "" && !s.CategoryEnabled(category) {
		return false
	}
	return adapter.Enabled == nil || *adapter.Enabled
}

// defaultSecurityExcludes are always outside a source scan. Git metadata is not
// source, and SOP's own run state is evidence rather than project content; both
// must never be scanned or persisted as findings.
var defaultSecurityExcludes = []string{".git", ".agent-sdlc"}

// defaultCategoryForID returns the built-in category for a built-in adapter id.
// An external adapter always declares its category explicitly.
func defaultCategoryForID(id string) string {
	switch id {
	case "semgrep":
		return SecurityCategorySAST
	case "gitleaks":
		return SecurityCategorySecrets
	case "cargo-audit", "osv-scanner":
		return SecurityCategoryDependencies
	default:
		return ""
	}
}

// SecurityExcludes returns the effective, deterministic exclusion list: the
// built-in VCS/state exclusions followed by the operator's configured patterns.
// Duplicates are removed; the configured order is preserved.
func (c Config) SecurityExcludes() []string {
	out := append([]string(nil), defaultSecurityExcludes...)
	seen := make(map[string]bool, len(out)+len(c.Security.Exclude))
	for _, pattern := range out {
		seen[pattern] = true
	}
	for _, raw := range c.Security.Exclude {
		pattern := strings.TrimSpace(raw)
		if pattern == "" || seen[pattern] {
			continue
		}
		seen[pattern] = true
		out = append(out, pattern)
	}
	return out
}

// ResolveSecurityEnv fills and validates the security block for a Config that was
// built without a project file (for example config.Default()). It applies the
// same environment precedence and enabled-block defaults as Parse, so an
// environment-enabled add-on works in a configless project too. It is a no-op
// when the add-on is disabled.
func ResolveSecurityEnv(c *Config) error {
	if c == nil {
		return nil
	}
	if err := c.applySecurityEnv(); err != nil {
		return err
	}
	c.applySecurityDefaults()
	return c.validateSecurity()
}

// applySecurityEnv fills the security block from the environment before defaults
// are applied. An explicit project value wins.
func (c *Config) applySecurityEnv() error {
	if raw, ok := os.LookupEnv(envSecurityEnabled); ok {
		value, err := parseEnvBool(envSecurityEnabled, raw)
		if err != nil {
			return err
		}
		if c.Security.Enabled == nil {
			c.Security.Enabled = &value
		}
	}
	if raw, ok := os.LookupEnv(envSecurityMode); ok && strings.TrimSpace(raw) != "" && strings.TrimSpace(c.Security.Mode) == "" {
		c.Security.Mode = strings.TrimSpace(raw)
	}
	if raw, ok := os.LookupEnv(envSecurityBaseline); ok && strings.TrimSpace(raw) != "" && strings.TrimSpace(c.Security.Baseline) == "" {
		c.Security.Baseline = strings.TrimSpace(raw)
	}
	return nil
}

// applySecurityDefaults fills omitted settings for an enabled add-on. A disabled
// block is deliberately left untouched so an unaffected configuration is
// byte-identical to the previous format.
func (c *Config) applySecurityDefaults() {
	if !c.SecurityEnabled() {
		return
	}
	if strings.TrimSpace(c.Security.Mode) == "" {
		c.Security.Mode = SecurityModeReportOnly
	}
	if strings.TrimSpace(c.Security.Scope) == "" {
		c.Security.Scope = SecurityScopeWorkspace
	}
	if len(c.Security.FailOn) == 0 {
		c.Security.FailOn = []string{"critical", "high"}
	}
	if strings.TrimSpace(c.Security.UnknownSeverity) == "" {
		c.Security.UnknownSeverity = SecurityUnknownIndeterminate
	}
	// A stale advisory database defaults to deny, so an omitted value is
	// fail-closed and never silently accepts stale evidence.
	if strings.TrimSpace(c.Security.StaleAdvisoryDatabase) == "" {
		c.Security.StaleAdvisoryDatabase = SecurityStaleDeny
	}
	if c.Security.TimeoutSeconds == 0 {
		c.Security.TimeoutSeconds = 300
	}
	if c.Security.MaxOutputBytes == 0 {
		c.Security.MaxOutputBytes = 1 << 20
	}
	if len(c.Security.Adapters) == 0 {
		c.Security.Adapters = []SecurityAdapterConfig{
			{ID: "semgrep", Category: SecurityCategorySAST, Protocol: SecurityProtocolBuiltin},
			{ID: "gitleaks", Category: SecurityCategorySecrets, Protocol: SecurityProtocolBuiltin},
		}
	}
	for key, value := range c.Security.Categories {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if normalized != key {
			delete(c.Security.Categories, key)
			c.Security.Categories[normalized] = value
		}
	}
	for i := range c.Security.Adapters {
		a := &c.Security.Adapters[i]
		a.ID = strings.TrimSpace(a.ID)
		a.Category = strings.ToLower(strings.TrimSpace(a.Category))
		a.Protocol = strings.ToLower(strings.TrimSpace(a.Protocol))
		if a.Protocol == "" {
			a.Protocol = SecurityProtocolBuiltin
		}
		if a.Category == "" {
			a.Category = defaultCategoryForID(strings.ToLower(a.ID))
		}
		a.Languages = normalizeLanguages(a.Languages)
		for i := range a.Inputs {
			a.Inputs[i] = strings.TrimSpace(strings.ReplaceAll(a.Inputs[i], "\\", "/"))
		}
		sort.Strings(a.Inputs)
		if len(a.Command) == 0 {
			switch a.ID {
			case "semgrep":
				a.Command = []string{"semgrep"}
			case "gitleaks":
				a.Command = []string{"gitleaks"}
			case "cargo-audit":
				// cargo-audit is a Cargo subcommand; the argv is explicit so no
				// shell is involved.
				a.Command = []string{"cargo", "audit"}
			case "osv-scanner":
				a.Command = []string{"osv-scanner"}
			}
		}
	}
}

// validateDisabledSecurity validates explicit policy values in a block that does
// not currently run the add-on. It does not apply defaults or require adapters.
func validateDisabledSecurity(s SecurityConfig) error {
	if mode := strings.TrimSpace(s.Mode); mode != "" && mode != SecurityModeReportOnly && mode != SecurityModeRequired {
		return fmt.Errorf("config: unknown security.mode %q (want report-only, required)", s.Mode)
	}
	if scope := strings.TrimSpace(s.Scope); scope != "" && scope != SecurityScopeWorkspace && scope != SecurityScopeChanged {
		return fmt.Errorf("config: unknown security.scope %q (want workspace, changed-files)", s.Scope)
	}
	for _, sev := range s.FailOn {
		switch strings.ToLower(strings.TrimSpace(sev)) {
		case "critical", "high", "medium", "low", "info":
		default:
			return fmt.Errorf("config: unknown security.fail_on severity %q (want critical, high, medium, low, info)", sev)
		}
	}
	switch unknown := strings.TrimSpace(s.UnknownSeverity); unknown {
	case "", SecurityUnknownFail, SecurityUnknownIndeterminate, SecurityUnknownIgnore:
	default:
		return fmt.Errorf("config: unknown security.unknown_severity %q (want fail, indeterminate, ignore)", unknown)
	}
	if err := validateSecurityStale(s.StaleAdvisoryDatabase); err != nil {
		return err
	}
	if err := validateSecurityAdvisoryDatabasePath(s.AdvisoryDatabasePath); err != nil {
		return err
	}
	if err := validateSecurityAdvisoryDatabasePath(s.OSVDatabasePath); err != nil {
		return err
	}
	if b := strings.TrimSpace(s.Baseline); b != "" {
		if strings.HasPrefix(b, "/") || strings.HasPrefix(b, "\\") || strings.Contains(b, "..") {
			return fmt.Errorf("config: security.baseline must be a repository-relative path without '..', got %q", b)
		}
	}
	if err := validateSecurityExcludes(s.Exclude); err != nil {
		return err
	}
	if err := validateSecurityCategories(s.Categories); err != nil {
		return err
	}
	for _, raw := range s.RequiredCategories {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case SecurityCategorySAST, SecurityCategorySecrets, SecurityCategoryDependencies:
		default:
			return fmt.Errorf("config: unknown security.required_categories entry %q (want sast, secrets, dependencies)", raw)
		}
	}
	return validateSecurityAdapterConfigs(s, false)
}

// validateSecurityStale accepts an empty value (deny), deny, or allow. Any other
// value is a focused load-time error so a later enable cannot carry a bad policy.
func validateSecurityStale(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", SecurityStaleDeny, SecurityStaleAllow:
		return nil
	default:
		return fmt.Errorf("config: unknown security.stale_advisory_database %q (want deny, allow)", value)
	}
}

// validateSecurityCategories rejects unknown category keys and empty entries so
// a typo cannot silently leave a category enabled.
func validateSecurityCategories(categories map[string]*bool) error {
	for raw, value := range categories {
		category := strings.ToLower(strings.TrimSpace(raw))
		switch category {
		case SecurityCategorySAST, SecurityCategorySecrets, SecurityCategoryDependencies:
		default:
			return fmt.Errorf("config: unknown security.categories key %q (want sast, secrets, dependencies)", raw)
		}
		if value == nil {
			return fmt.Errorf("config: security.categories.%s must be true or false", raw)
		}
	}
	return nil
}

// normalizeLanguages trims, lowercases, deduplicates, and sorts language names.
func normalizeLanguages(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, raw := range values {
		value := strings.ToLower(strings.TrimSpace(raw))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// validateSecurityAdapterLanguages checks the declared language set. Unknown
// languages are rejected so a typo cannot be mistaken for coverage.
func validateSecurityAdapterLanguages(languages []string) error {
	for _, raw := range languages {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case SecurityLanguageGo, SecurityLanguageRust:
		default:
			return fmt.Errorf("unknown language %q (want go, rust)", raw)
		}
	}
	return nil
}

// validateSecurityInputs requires repository-relative dependency input paths.
func validateSecurityInputs(inputs []string) error {
	for _, raw := range inputs {
		if strings.TrimSpace(raw) == "" {
			return fmt.Errorf("input path must not be empty")
		}
		if err := validateSecurityLockfileValue(raw); err != nil {
			return err
		}
	}
	return nil
}

// validateSecurityAdapterConfigs validates adapter-level fields shared by the
// enabled and disabled paths. enabledOnly adds command and category rules that
// only matter when the adapter can actually run.
func validateSecurityAdapterConfigs(s SecurityConfig, enabledOnly bool) error {
	seen := make(map[string]bool, len(s.Adapters))
	for i, a := range s.Adapters {
		id := strings.TrimSpace(a.ID)
		if id == "" {
			return fmt.Errorf("config: security.adapters[%d].id is required", i)
		}
		if seen[id] {
			return fmt.Errorf("config: duplicate security adapter id %q", id)
		}
		seen[id] = true
		protocol := strings.ToLower(strings.TrimSpace(a.Protocol))
		if protocol == "" {
			protocol = SecurityProtocolBuiltin
		}
		category := strings.ToLower(strings.TrimSpace(a.Category))
		if category == "" {
			category = defaultCategoryForID(strings.ToLower(id))
		}
		switch category {
		case SecurityCategorySAST, SecurityCategorySecrets, SecurityCategoryDependencies:
		default:
			return fmt.Errorf("config: security adapter %q has unknown category %q (want sast, secrets, dependencies)", id, a.Category)
		}
		if err := validateSecurityAdapterLanguages(a.Languages); err != nil {
			return fmt.Errorf("config: security adapter %q: %w", id, err)
		}
		if a.RepositoryWide != nil && *a.RepositoryWide && len(a.Languages) > 0 {
			return fmt.Errorf("config: security adapter %q cannot set both repository_wide and languages", id)
		}
		if err := validateSecurityInputs(a.Inputs); err != nil {
			return fmt.Errorf("config: security adapter %q: %w", id, err)
		}
		if strings.ContainsRune(a.Database, 0) {
			return fmt.Errorf("config: security adapter %q database contains a NUL byte", id)
		}
		if err := validateSecurityDatabaseValue(a.Database); err != nil {
			return fmt.Errorf("config: security adapter %q: %w", id, err)
		}
		if err := validateSecurityStaleValue(a.Stale); err != nil {
			return fmt.Errorf("config: security adapter %q: %w", id, err)
		}
		if strings.TrimSpace(a.Lockfile) != "" {
			if err := validateSecurityLockfileValue(a.Lockfile); err != nil {
				return fmt.Errorf("config: security adapter %q: %w", id, err)
			}
		}
		switch protocol {
		case SecurityProtocolBuiltin:
			switch strings.ToLower(id) {
			case "semgrep":
				if category != SecurityCategorySAST {
					return fmt.Errorf("config: security adapter %q must use category %q", id, SecurityCategorySAST)
				}
				if err := rejectRepositoryWide(id, a); err != nil {
					return err
				}
				if len(a.Inputs) > 0 {
					return fmt.Errorf("config: security adapter %q does not accept inputs", id)
				}
			case "gitleaks":
				if category != SecurityCategorySecrets {
					return fmt.Errorf("config: security adapter %q must use category %q", id, SecurityCategorySecrets)
				}
				if len(a.Languages) > 0 {
					return fmt.Errorf("config: security adapter %q is repository-wide and does not accept languages", id)
				}
				if a.RepositoryWide != nil && !*a.RepositoryWide {
					return fmt.Errorf("config: security adapter %q is repository-wide; repository_wide cannot be false", id)
				}
				if len(a.Inputs) > 0 {
					return fmt.Errorf("config: security adapter %q does not accept inputs", id)
				}
			case "cargo-audit":
				if category != SecurityCategoryDependencies {
					return fmt.Errorf("config: security adapter %q must use category %q", id, SecurityCategoryDependencies)
				}
				for _, language := range a.Languages {
					if language != SecurityLanguageRust {
						return fmt.Errorf("config: security adapter %q covers only rust, got language %q", id, language)
					}
				}
				if err := rejectRepositoryWide(id, a); err != nil {
					return err
				}
				if len(a.Inputs) > 0 {
					return fmt.Errorf("config: security adapter %q accepts lockfile, not inputs", id)
				}
			case "osv-scanner":
				if category != SecurityCategoryDependencies {
					return fmt.Errorf("config: security adapter %q must use category %q", id, SecurityCategoryDependencies)
				}
				if err := rejectRepositoryWide(id, a); err != nil {
					return err
				}
				if strings.TrimSpace(a.Lockfile) != "" {
					return fmt.Errorf("config: security adapter %q accepts inputs, not lockfile", id)
				}
			default:
				return fmt.Errorf("config: unknown built-in security adapter %q (want semgrep, gitleaks, cargo-audit, osv-scanner; use protocol: external for a private adapter)", id)
			}
		case SecurityProtocolExternal:
			if category == SecurityCategorySecrets {
				if len(a.Languages) > 0 {
					return fmt.Errorf("config: security adapter %q is a secrets scanner and is repository-wide; languages does not apply", id)
				}
				if a.RepositoryWide != nil && !*a.RepositoryWide {
					return fmt.Errorf("config: security adapter %q is repository-wide; repository_wide cannot be false", id)
				}
			}
			if len(a.Inputs) > 0 {
				return fmt.Errorf("config: external security adapter %q does not accept inputs", id)
			}
		default:
			return fmt.Errorf("config: security adapter %q has unknown protocol %q (want builtin, external)", id, a.Protocol)
		}
		if enabledOnly && s.AdapterEnabled(a) && (len(a.Command) == 0 || strings.TrimSpace(a.Command[0]) == "") {
			return fmt.Errorf("config: security adapter %q requires a command", id)
		}
		if len(a.Command) > 0 {
			for _, arg := range a.Command {
				if strings.ContainsRune(arg, 0) {
					return fmt.Errorf("config: security adapter %q command contains a NUL byte", id)
				}
			}
		}
	}
	return nil
}

func rejectRepositoryWide(id string, a SecurityAdapterConfig) error {
	if a.RepositoryWide != nil && *a.RepositoryWide {
		return fmt.Errorf("config: security adapter %q is language-scoped; repository_wide does not apply", id)
	}
	return nil
}

// validateSecurityAdvisoryDatabasePath validates the global advisory database
// path. An absolute path is allowed because a shared local database commonly
// lives outside the repository; a relative path stays repository-relative.
func validateSecurityAdvisoryDatabasePath(value string) error {
	return validateSecurityDatabaseValue(value)
}

// validateSecurityDatabaseValue validates an explicit local advisory database
// path. An absolute path is allowed because a shared local database commonly
// lives outside the repository; a relative path stays repository-relative and
// may not escape it.
func validateSecurityDatabaseValue(value string) error {
	p := strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if p == "" {
		return nil
	}
	if strings.ContainsRune(p, 0) {
		return fmt.Errorf("database contains a NUL byte")
	}
	if filepath.IsAbs(p) {
		return nil
	}
	for _, segment := range strings.Split(p, "/") {
		if segment == ".." {
			return fmt.Errorf("database must stay in the repository or be absolute, got %q", value)
		}
	}
	return nil
}

// validateSecurityLockfileValue requires a repository-relative lockfile path. A
// dependency adapter must never read a lockfile outside the workspace.
func validateSecurityLockfileValue(value string) error {
	p := strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if p == "" {
		return nil
	}
	if strings.ContainsRune(p, 0) {
		return fmt.Errorf("lockfile contains a NUL byte")
	}
	if strings.HasPrefix(p, "/") || (len(p) >= 2 && p[1] == ':') {
		return fmt.Errorf("lockfile must be repository-relative, got %q", value)
	}
	for _, segment := range strings.Split(strings.TrimPrefix(p, "/"), "/") {
		if segment == ".." {
			return fmt.Errorf("lockfile must not contain '..', got %q", value)
		}
	}
	return nil
}

// validateSecurityStaleValue validates a per-adapter stale-database policy.
func validateSecurityStaleValue(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", SecurityStaleDeny, SecurityStaleAllow:
		return nil
	default:
		return fmt.Errorf("unknown stale policy %q (want deny, allow)", value)
	}
}

// validateSecurityExcludes rejects an exclusion that could escape the workspace
// or be interpreted as an absolute path. A leading "/" anchors the pattern to
// the repository root (gitignore semantics); it is not a host filesystem path.
// Glob metacharacters are permitted so an operator can declare a fixture tree;
// `..` is rejected regardless of position.
func validateSecurityExcludes(exclude []string) error {
	for _, raw := range exclude {
		pattern := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
		if pattern == "" {
			return fmt.Errorf("config: security.exclude contains an empty pattern")
		}
		if strings.ContainsRune(pattern, 0) {
			return fmt.Errorf("config: security.exclude %q contains a NUL byte", raw)
		}
		if len(pattern) >= 2 && pattern[1] == ':' {
			return fmt.Errorf("config: security.exclude must be repository-relative, got %q", raw)
		}
		for _, segment := range strings.Split(strings.TrimPrefix(pattern, "/"), "/") {
			if segment == ".." {
				return fmt.Errorf("config: security.exclude must not contain '..', got %q", raw)
			}
		}
	}
	return nil
}

// validateSecurity rejects a malformed enabled block at load time. A disabled or
// omitted block has nothing to execute, but explicit policy values are still
// checked so a later enable cannot carry a bad setting.
func (c *Config) validateSecurity() error {
	s := c.Security
	// A disabled block executes nothing, but an explicitly written policy value must
	// still be well-formed so a later enable cannot silently carry a bad setting.
	if !c.SecurityEnabled() {
		return validateDisabledSecurity(s)
	}
	switch strings.TrimSpace(s.Mode) {
	case SecurityModeReportOnly, SecurityModeRequired:
	default:
		return fmt.Errorf("config: unknown security.mode %q (want report-only, required)", s.Mode)
	}
	switch strings.TrimSpace(s.Scope) {
	case SecurityScopeWorkspace, SecurityScopeChanged:
	default:
		return fmt.Errorf("config: unknown security.scope %q (want workspace, changed-files)", s.Scope)
	}
	if s.TimeoutSeconds <= 0 {
		return fmt.Errorf("config: security.timeout_seconds must be positive, got %d", s.TimeoutSeconds)
	}
	if s.MaxOutputBytes <= 0 {
		return fmt.Errorf("config: security.max_output_bytes must be positive, got %d", s.MaxOutputBytes)
	}
	if strings.TrimSpace(s.Mode) == SecurityModeRequired && len(s.RequiredCategories) == 0 {
		return fmt.Errorf("config: security.mode %q requires at least one security.required_categories entry", s.Mode)
	}
	if len(s.FailOn) == 0 {
		return fmt.Errorf("config: security.fail_on must not be empty when the add-on is enabled")
	}
	for _, sev := range s.FailOn {
		switch strings.ToLower(strings.TrimSpace(sev)) {
		case "critical", "high", "medium", "low", "info":
		default:
			return fmt.Errorf("config: unknown security.fail_on severity %q (want critical, high, medium, low, info)", sev)
		}
	}
	switch strings.TrimSpace(s.UnknownSeverity) {
	case SecurityUnknownFail, SecurityUnknownIndeterminate, SecurityUnknownIgnore:
	default:
		return fmt.Errorf("config: unknown security.unknown_severity %q (want fail, indeterminate, ignore)", s.UnknownSeverity)
	}
	if err := validateSecurityStale(s.StaleAdvisoryDatabase); err != nil {
		return err
	}
	if err := validateSecurityAdvisoryDatabasePath(s.AdvisoryDatabasePath); err != nil {
		return err
	}
	if err := validateSecurityAdvisoryDatabasePath(s.OSVDatabasePath); err != nil {
		return err
	}
	if b := strings.TrimSpace(s.Baseline); b != "" {
		if strings.HasPrefix(b, "/") || strings.HasPrefix(b, "\\") || strings.Contains(b, "..") {
			return fmt.Errorf("config: security.baseline must be a repository-relative path without '..', got %q", b)
		}
	}
	if err := validateSecurityExcludes(s.Exclude); err != nil {
		return err
	}
	if err := validateSecurityCategories(s.Categories); err != nil {
		return err
	}
	if err := validateSecurityAdapterConfigs(s, true); err != nil {
		return err
	}

	byCategory := make(map[string]bool, len(s.Adapters))
	enabledByCategory := make(map[string]bool, len(s.Adapters))
	for _, a := range s.Adapters {
		category := strings.ToLower(strings.TrimSpace(a.Category))
		if category == "" {
			category = defaultCategoryForID(strings.ToLower(strings.TrimSpace(a.ID)))
		}
		byCategory[category] = true
		if s.AdapterEnabled(a) {
			enabledByCategory[category] = true
		}
	}
	for _, raw := range s.RequiredCategories {
		category := strings.ToLower(strings.TrimSpace(raw))
		switch category {
		case SecurityCategorySAST, SecurityCategorySecrets, SecurityCategoryDependencies:
		default:
			return fmt.Errorf("config: unknown security.required_categories entry %q (want sast, secrets, dependencies)", raw)
		}
		if !byCategory[category] {
			return fmt.Errorf("config: security.required_categories names %q but no configured adapter provides it", category)
		}
		if !enabledByCategory[category] {
			return fmt.Errorf("config: security.required_categories names %q but every configured adapter for it is disabled", category)
		}
	}
	return nil
}
