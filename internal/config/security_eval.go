package config

import (
	"fmt"
	"strings"
)

// Security evaluation (Promptfoo) configuration. This extends the existing
// optional security block with a single optional AI-evaluation adapter. The
// evaluation adapter is disabled unless explicitly enabled, and the whole block
// stays omitted from committed bytes unless the operator writes it.

// Evaluation profile names. The local profile requires local providers and
// never falls back to a hosted provider.
const (
	SecurityEvalProfileLocal  = "local"
	SecurityEvalProfileRemote = "remote"
)

// Severity assignment modes for behavioral findings. Behavioral evaluation
// failures never receive an automatic CVSS severity; severity comes only from an
// explicit mapping.
const (
	SecurityEvalSeverityNone     = "none"
	SecurityEvalSeverityExplicit = "explicit"
)

// Data-handling choices for a remote provider. local-only refuses any remote
// provider; remote requires an explicit acknowledgement.
const (
	SecurityEvalDataLocalOnly     = "local-only"
	SecurityEvalDataRemoteAllowed = "remote-allowed"
)

// SecurityEvalConfig is the optional Promptfoo evaluation adapter configuration.
// It is nested under SecurityConfig and disabled unless Enabled is true.
type SecurityEvalConfig struct {
	// Enabled turns the evaluation adapter on. It is a pointer so an omitted
	// value (disabled) is distinguishable from an explicit false.
	Enabled *bool `yaml:"enabled"`
	// Profile is local or remote. An empty value means local.
	Profile string `yaml:"profile"`
	// Target names the evaluation target model under test. It is a model
	// identifier, never a credential.
	Target string `yaml:"target"`
	// PromptfooVersion is the pinned Promptfoo version. Ordinary execution never
	// downloads, so a mismatch is a load error.
	PromptfooVersion string `yaml:"promptfoo_version"`
	// InstallReference names the documentation section that gives the exact
	// install step. It is a reference, not an executed command.
	InstallReference string `yaml:"install_reference"`
	// AttackProvider is the provider that generates attacks. It must be local for
	// the local profile.
	AttackProvider string `yaml:"attack_provider"`
	// GradingProvider is the provider that grades results. It must be local for
	// the local profile.
	GradingProvider string `yaml:"grading_provider"`
	// TargetProvider is the provider that serves the target model. It must be
	// local for the local profile.
	TargetProvider string `yaml:"target_provider"`
	// ConfigPath is the repository-relative Promptfoo config file. It is explicit
	// so no file is fetched implicitly.
	ConfigPath string `yaml:"config_path"`
	// DatasetPath is the repository-relative red-team dataset. It is optional; an
	// omitted value means the dataset hash is recorded as unavailable.
	DatasetPath string `yaml:"dataset_path"`
	// RequestLimit bounds requests per case. Zero means the default.
	RequestLimit int `yaml:"request_limit"`
	// Concurrency bounds parallel cases. Zero means the default.
	Concurrency int `yaml:"concurrency"`
	// ProcessTimeoutSeconds bounds one Promptfoo invocation. Zero means the
	// security.timeout_seconds value.
	ProcessTimeoutSeconds int `yaml:"process_timeout_seconds"`
	// RemoteProvider acknowledges that a remote provider is used. The local
	// profile rejects it.
	RemoteProvider bool `yaml:"remote_provider"`
	// DataHandling is local-only or remote-allowed. The local profile requires
	// local-only.
	DataHandling string `yaml:"data_handling"`
	// TelemetryDisabled disables Promptfoo telemetry and sharing. The local profile
	// forces it. The zero value is false, so an operator must set it for a remote
	// profile or accept the documented default.
	TelemetryDisabled bool `yaml:"telemetry_disabled"`
	// ReportOnly marks an evaluation that records findings without blocking the
	// workflow. It is independent of security.mode.
	ReportOnly bool `yaml:"report_only"`
	// SeverityMode controls whether behavioral findings get a severity. The
	// default, none, never assigns CVSS or any automatic severity.
	SeverityMode string `yaml:"severity_mode"`
}

const (
	defaultSecurityEvalPromptfooVersion = "0.118.0"
	defaultSecurityEvalRequestLimit     = 10
	defaultSecurityEvalConcurrency      = 4
	defaultSecurityEvalProcessTimeout   = 900
)

// SecurityEvalEnabled reports whether the evaluation adapter is explicitly
// enabled.
func (c Config) SecurityEvalEnabled() bool {
	return c.Security.Evaluation.Enabled != nil && *c.Security.Evaluation.Enabled
}

// applySecurityEvalDefaults fills omitted values for an enabled evaluation block.
// A disabled block is deliberately left untouched.
func (c *Config) applySecurityEvalDefaults() {
	e := &c.Security.Evaluation
	if e.Enabled == nil || !*e.Enabled {
		return
	}
	if strings.TrimSpace(e.Profile) == "" {
		e.Profile = SecurityEvalProfileLocal
	}
	if strings.TrimSpace(e.PromptfooVersion) == "" {
		e.PromptfooVersion = defaultSecurityEvalPromptfooVersion
	}
	if strings.TrimSpace(e.InstallReference) == "" {
		e.InstallReference = "docs/security-promptfoo.md#installation"
	}
	if e.RequestLimit == 0 {
		e.RequestLimit = defaultSecurityEvalRequestLimit
	}
	if e.Concurrency == 0 {
		e.Concurrency = defaultSecurityEvalConcurrency
	}
	if e.ProcessTimeoutSeconds == 0 {
		if c.Security.TimeoutSeconds > 0 {
			e.ProcessTimeoutSeconds = c.Security.TimeoutSeconds
		} else {
			e.ProcessTimeoutSeconds = defaultSecurityEvalProcessTimeout
		}
	}
	if strings.TrimSpace(e.DataHandling) == "" {
		e.DataHandling = SecurityEvalDataLocalOnly
	}
	if strings.TrimSpace(e.SeverityMode) == "" {
		e.SeverityMode = SecurityEvalSeverityNone
	}
	// The local profile requires local, telemetry-disabled operation and refuses
	// remote providers. Setting these here is the explicit local default; a remote
	// provider selection is a load error, never a silent fallback.
	if strings.ToLower(strings.TrimSpace(e.Profile)) == SecurityEvalProfileLocal {
		e.RemoteProvider = false
		e.DataHandling = SecurityEvalDataLocalOnly
		e.TelemetryDisabled = true
	}
	for _, f := range []*string{&e.Profile, &e.Target, &e.AttackProvider, &e.GradingProvider, &e.TargetProvider, &e.ConfigPath, &e.DatasetPath, &e.DataHandling, &e.SeverityMode, &e.PromptfooVersion} {
		*f = strings.TrimSpace(*f)
	}
}

// validateSecurityEval validates the evaluation block. It runs both when the
// add-on is enabled and when it is disabled, so a later enable cannot carry a bad
// setting. It never treats a credential field as configuration.
func validateSecurityEval(s SecurityConfig) error {
	e := s.Evaluation
	if e.Enabled == nil && !securityEvalNonEmpty(e) {
		return nil
	}
	switch profile := strings.ToLower(strings.TrimSpace(e.Profile)); profile {
	case "", SecurityEvalProfileLocal, SecurityEvalProfileRemote:
	default:
		return fmt.Errorf("config: unknown security.evaluation.profile %q (want local, remote)", e.Profile)
	}
	switch data := strings.ToLower(strings.TrimSpace(e.DataHandling)); data {
	case "", SecurityEvalDataLocalOnly, SecurityEvalDataRemoteAllowed:
	default:
		return fmt.Errorf("config: unknown security.evaluation.data_handling %q (want local-only, remote-allowed)", e.DataHandling)
	}
	switch sev := strings.ToLower(strings.TrimSpace(e.SeverityMode)); sev {
	case "", SecurityEvalSeverityNone, SecurityEvalSeverityExplicit:
	default:
		return fmt.Errorf("config: unknown security.evaluation.severity_mode %q (want none, explicit)", e.SeverityMode)
	}
	if v := strings.TrimSpace(e.PromptfooVersion); v != "" && v != defaultSecurityEvalPromptfooVersion {
		return fmt.Errorf("config: security.evaluation.promptfoo_version %q is not the pinned version %s", e.PromptfooVersion, defaultSecurityEvalPromptfooVersion)
	}
	if e.RequestLimit < 0 {
		return fmt.Errorf("config: security.evaluation.request_limit must be positive, got %d", e.RequestLimit)
	}
	if e.Concurrency < 0 {
		return fmt.Errorf("config: security.evaluation.concurrency must be positive, got %d", e.Concurrency)
	}
	if e.ProcessTimeoutSeconds < 0 {
		return fmt.Errorf("config: security.evaluation.process_timeout_seconds must be positive, got %d", e.ProcessTimeoutSeconds)
	}
	if err := validateSecurityEvalPath("config_path", e.ConfigPath); err != nil {
		return err
	}
	if err := validateSecurityEvalPath("dataset_path", e.DatasetPath); err != nil {
		return err
	}
	// The local profile requires local providers, local data handling, and
	// telemetry disabled. A hosted selection is a load error, never a silent
	// fallback to a hosted provider.
	if strings.ToLower(strings.TrimSpace(e.Profile)) == SecurityEvalProfileLocal {
		if e.RemoteProvider {
			return fmt.Errorf("config: security.evaluation.profile %q rejects a remote provider; the local profile never falls back to a hosted provider", SecurityEvalProfileLocal)
		}
		if strings.ToLower(strings.TrimSpace(e.DataHandling)) == SecurityEvalDataRemoteAllowed {
			return fmt.Errorf("config: security.evaluation.profile %q requires data_handling %q", SecurityEvalProfileLocal, SecurityEvalDataLocalOnly)
		}
	}
	if isEvalEnabled(e) {
		if strings.TrimSpace(e.Target) == "" {
			return fmt.Errorf("config: security.evaluation.target is required when evaluation is enabled")
		}
		if strings.TrimSpace(e.ConfigPath) == "" {
			return fmt.Errorf("config: security.evaluation.config_path is required when evaluation is enabled")
		}
		switch strings.ToLower(strings.TrimSpace(e.Profile)) {
		case "", SecurityEvalProfileLocal:
			if strings.TrimSpace(e.AttackProvider) == "" {
				return fmt.Errorf("config: security.evaluation.attack_provider is required for the local profile")
			}
			if strings.TrimSpace(e.GradingProvider) == "" {
				return fmt.Errorf("config: security.evaluation.grading_provider is required for the local profile")
			}
			if strings.TrimSpace(e.TargetProvider) == "" {
				return fmt.Errorf("config: security.evaluation.target_provider is required for the local profile")
			}
		case SecurityEvalProfileRemote:
			if !e.RemoteProvider {
				return fmt.Errorf("config: security.evaluation.profile %q requires remote_provider: true", SecurityEvalProfileRemote)
			}
			if strings.ToLower(strings.TrimSpace(e.DataHandling)) != SecurityEvalDataRemoteAllowed {
				return fmt.Errorf("config: security.evaluation.profile %q requires data_handling %q", SecurityEvalProfileRemote, SecurityEvalDataRemoteAllowed)
			}
		}
	}
	return nil
}

func isEvalEnabled(e SecurityEvalConfig) bool {
	return e.Enabled != nil && *e.Enabled
}

func securityEvalNonEmpty(e SecurityEvalConfig) bool {
	return strings.TrimSpace(e.Profile) != "" ||
		strings.TrimSpace(e.Target) != "" ||
		strings.TrimSpace(e.ConfigPath) != "" ||
		strings.TrimSpace(e.PromptfooVersion) != "" ||
		strings.TrimSpace(e.AttackProvider) != "" ||
		strings.TrimSpace(e.GradingProvider) != "" ||
		strings.TrimSpace(e.TargetProvider) != "" ||
		strings.TrimSpace(e.DataHandling) != "" ||
		strings.TrimSpace(e.SeverityMode) != "" ||
		e.RemoteProvider || e.TelemetryDisabled || e.ReportOnly ||
		e.RequestLimit != 0 || e.Concurrency != 0 || e.ProcessTimeoutSeconds != 0
}

// validateSecurityEvalPath requires a repository-relative file path that cannot
// escape the workspace.
func validateSecurityEvalPath(field, value string) error {
	p := strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if p == "" {
		return nil
	}
	if strings.ContainsRune(p, 0) {
		return fmt.Errorf("config: security.evaluation.%s contains a NUL byte", field)
	}
	if strings.HasPrefix(p, "/") || (len(p) >= 2 && p[1] == ':') {
		return fmt.Errorf("config: security.evaluation.%s must be repository-relative, got %q", field, value)
	}
	for _, segment := range strings.Split(p, "/") {
		if segment == ".." {
			return fmt.Errorf("config: security.evaluation.%s must not contain '..', got %q", field, value)
		}
	}
	return nil
}

// credentialKeyNames are configuration keys that would commit a credential. A
// committed value under any of these keys fails validation.
var credentialKeyNames = map[string]bool{
	"api_key": true, "apikey": true, "api-key": true,
	"token": true, "secret": true, "password": true,
	"credential": true, "credentials": true,
	"access_key": true, "access-key": true, "secret_key": true,
}

// IsCredentialKey reports whether a configuration key names a credential. It is
// exported so callers and tests share one definition.
func IsCredentialKey(key string) bool {
	return credentialKeyNames[strings.ToLower(strings.TrimSpace(key))]
}
