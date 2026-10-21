package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Experimental advisory retrieval (RDM-003). The layer is opt-in and DISABLED by
// default: with an omitted block and no SOP_ADVISORY_RETRIEVAL_* environment,
// Enabled is false and the resolved values are inert, so existing behavior
// (including the existing `rag:` block) is unchanged.
//
// Precedence matches the project-wide one: SOP_ADVISORY_RETRIEVAL_* environment >
// the `advisory_retrieval:` block in .agent-sdlc/config.yaml > the built-in
// defaults. It is resolved by AdvisoryRetrievalConfig.Resolved. Installer
// selection is out of scope; the executable is named directly.
const (
	EnvAdvisoryRetrievalEnabled     = "SOP_ADVISORY_RETRIEVAL_ENABLED"
	EnvAdvisoryRetrievalIndex       = "SOP_ADVISORY_RETRIEVAL_INDEX"
	EnvAdvisoryRetrievalOutcomes    = "SOP_ADVISORY_RETRIEVAL_OUTCOMES"
	EnvAdvisoryRetrievalCorpora     = "SOP_ADVISORY_RETRIEVAL_CORPORA"
	EnvAdvisoryRetrievalTimeout     = "SOP_ADVISORY_RETRIEVAL_TIMEOUT_MS"
	EnvAdvisoryRetrievalResults     = "SOP_ADVISORY_RETRIEVAL_MAX_RESULTS"
	EnvAdvisoryRetrievalTokens      = "SOP_ADVISORY_RETRIEVAL_TOKEN_BUDGET"
	EnvAdvisoryRetrievalCompatFloor = "SOP_ADVISORY_RETRIEVAL_COMPAT_FLOOR"

	// DefaultAdvisoryRetrievalTimeoutMS bounds one advisory retrieval request.
	DefaultAdvisoryRetrievalTimeoutMS = 2000
	// DefaultAdvisoryRetrievalMaxResults caps advisory evidence items.
	DefaultAdvisoryRetrievalMaxResults = 5
	// DefaultAdvisoryRetrievalTokenBudget bounds total advisory tokens injected.
	DefaultAdvisoryRetrievalTokenBudget = 2000
)

// AdvisoryRetrievalConfig is the experimental advisory retrieval block. It names the
// local SQLite index and the authoritative outcome-file directory directly, so the local
// path needs no subprocess, no database service, and no installer selection.
type AdvisoryRetrievalConfig struct {
	// Enabled is the master opt-in. It is false unless explicitly enabled by the
	// environment or the `advisory_retrieval:` block.
	Enabled bool `yaml:"enabled"`
	// CaptureTimeoutMS bounds capture index maintenance; zero uses 5000 ms.
	CaptureTimeoutMS int `yaml:"capture_timeout_ms"`
	// IndexPath is the local, rebuildable SQLite memory index.
	IndexPath string `yaml:"index_path"`
	// OutcomesDir is the directory of authoritative outcome documents used for drift
	// exclusion on the read path.
	OutcomesDir string `yaml:"outcomes_dir"`
	// AllowedCorpora is the authorized corpus allowlist. Empty forbids retrieval.
	AllowedCorpora []string `yaml:"allowed_corpora"`
	// TimeoutMS bounds one advisory retrieval request, in milliseconds.
	TimeoutMS int `yaml:"timeout_ms"`
	// MaxResults bounds the number of advisory evidence records.
	MaxResults int `yaml:"max_results"`
	// TokenBudget bounds total advisory tokens across mapped items.
	TokenBudget int `yaml:"token_budget"`
	// CompatibilityFloor is the minimum compatibility a record must carry to be
	// mapped. An empty floor accepts any record.
	CompatibilityFloor string `yaml:"compatibility_floor"`
}

// Timeout is the resolved per-request deadline.
func (a AdvisoryRetrievalConfig) Timeout() time.Duration {
	return time.Duration(a.TimeoutMS) * time.Millisecond
}

// Resolved applies the environment over this block, fills defaults, normalizes the
// corpus allowlist deterministically, and validates the result. It never enables
// advisory retrieval implicitly: an omitted block with no environment resolves to
// disabled and returns without validation.
func (a AdvisoryRetrievalConfig) Resolved(getenv func(string) string) (AdvisoryRetrievalConfig, error) {
	if getenv == nil {
		getenv = os.Getenv
	}

	if v := strings.TrimSpace(getenv(EnvAdvisoryRetrievalEnabled)); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return AdvisoryRetrievalConfig{}, fmt.Errorf("advisory_retrieval: %s: invalid boolean %q (want true or false)", EnvAdvisoryRetrievalEnabled, v)
		}
		a.Enabled = b
	}
	if v := strings.TrimSpace(getenv(EnvAdvisoryRetrievalIndex)); v != "" {
		a.IndexPath = v
	}
	if v := strings.TrimSpace(getenv(EnvAdvisoryRetrievalOutcomes)); v != "" {
		a.OutcomesDir = v
	}
	if v := strings.TrimSpace(getenv(EnvAdvisoryRetrievalCorpora)); v != "" {
		a.AllowedCorpora = splitCorpora(v)
	}
	if v := strings.TrimSpace(getenv(EnvAdvisoryRetrievalTimeout)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return AdvisoryRetrievalConfig{}, fmt.Errorf("advisory_retrieval: %s: invalid timeout %q (want a positive integer of milliseconds)", EnvAdvisoryRetrievalTimeout, v)
		}
		a.TimeoutMS = n
	}
	if v := strings.TrimSpace(getenv(EnvAdvisoryRetrievalResults)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return AdvisoryRetrievalConfig{}, fmt.Errorf("advisory_retrieval: %s: invalid result limit %q (want a positive integer)", EnvAdvisoryRetrievalResults, v)
		}
		a.MaxResults = n
	}
	if v := strings.TrimSpace(getenv(EnvAdvisoryRetrievalTokens)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return AdvisoryRetrievalConfig{}, fmt.Errorf("advisory_retrieval: %s: invalid token budget %q (want a positive integer)", EnvAdvisoryRetrievalTokens, v)
		}
		a.TokenBudget = n
	}
	if v := strings.TrimSpace(getenv(EnvAdvisoryRetrievalCompatFloor)); v != "" {
		a.CompatibilityFloor = v
	}

	if a.CaptureTimeoutMS < 0 {
		return AdvisoryRetrievalConfig{}, fmt.Errorf("advisory_retrieval: capture_timeout_ms must not be negative")
	}
	if a.CaptureTimeoutMS == 0 {
		a.CaptureTimeoutMS = 5000
	}
	if a.TimeoutMS <= 0 {
		a.TimeoutMS = DefaultAdvisoryRetrievalTimeoutMS
	}
	if a.MaxResults <= 0 {
		a.MaxResults = DefaultAdvisoryRetrievalMaxResults
	}
	if a.TokenBudget <= 0 {
		a.TokenBudget = DefaultAdvisoryRetrievalTokenBudget
	}
	a.AllowedCorpora = normalizeCorpora(a.AllowedCorpora)

	if !a.Enabled {
		return a, nil
	}

	if strings.TrimSpace(a.IndexPath) == "" {
		return AdvisoryRetrievalConfig{}, fmt.Errorf("advisory_retrieval: index_path is required when %s is true", EnvAdvisoryRetrievalEnabled)
	}
	if !filepath.IsAbs(a.IndexPath) {
		return AdvisoryRetrievalConfig{}, fmt.Errorf("advisory_retrieval: index_path must be an absolute path, got %q", a.IndexPath)
	}
	if strings.TrimSpace(a.OutcomesDir) == "" {
		return AdvisoryRetrievalConfig{}, fmt.Errorf("advisory_retrieval: outcomes_dir is required when %s is true", EnvAdvisoryRetrievalEnabled)
	}
	if !filepath.IsAbs(a.OutcomesDir) {
		return AdvisoryRetrievalConfig{}, fmt.Errorf("advisory_retrieval: outcomes_dir must be an absolute path, got %q", a.OutcomesDir)
	}
	if len(a.AllowedCorpora) == 0 {
		return AdvisoryRetrievalConfig{}, fmt.Errorf("advisory_retrieval: a non-empty allowed_corpora is required when %s is true", EnvAdvisoryRetrievalEnabled)
	}

	return a, nil
}

// Corpora returns the resolved corpora as retrievalboundary.Corpus values. It does
// not validate that the strings are known corpora; Validate does that via
// retrievalboundary.Validate on the built query.
func (a AdvisoryRetrievalConfig) Corpora() []string {
	return append([]string(nil), a.AllowedCorpora...)
}
