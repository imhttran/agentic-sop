// Package config defines the project configuration model loaded from
// .agent-sdlc/config.yaml.
//
// Configuration describes policy only. It never contains mutable task state,
// and it must not contain secrets: the schema has no secret fields and strict
// decoding rejects unknown keys, so credentials stay in the environment.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/imhttran/agentic-sop/internal/decision"
)

// Layout: the configuration lives alongside the SQLite state in one directory.
const (
	// DirName is the per-project directory holding sop state and configuration.
	DirName = ".agent-sdlc"
	// FileName is the configuration file name inside DirName.
	FileName = "config.yaml"
)

// CurrentVersion is the configuration schema version this build understands.
const CurrentVersion = 1

// DefaultMaxFixCycles bounds the review/fix loop when none is configured.
const DefaultMaxFixCycles = 3

// Default agent settings for new projects (plan AHV2008).
const (
	// DefaultHarness is the harness new projects start with: the tool-enabled
	// coding-agent path.
	DefaultHarness = "tool"
	// DefaultProvider is the model provider new projects start with.
	DefaultProvider = "ollama"
	// DefaultModel is the model new projects start with.
	DefaultModel = "deepseek-v4.1-flash:cloud"
	// LegacyHarness is the harness implied by the legacy command path. A
	// project that only sets provider: command keeps behaving as before.
	LegacyHarness = "command"
)

// Supported agent harnesses, providers, and review engines, kept here so a bad
// value fails at load time with a clear message instead of at first use.
var (
	supportedHarnesses = map[string]bool{"tool": true, "command": true}
	supportedProviders = map[string]bool{"command": true, "ollama": true, "llamacpp": true}
	supportedEngines   = map[string]bool{"self": true, "open-code-review": true}
	supportedSeverity  = map[string]bool{
		"critical": true, "high": true, "medium": true, "low": true, "info": true,
	}
)

// ErrNotFound is returned by Load when no configuration file exists. Callers
// treat a missing configuration as "use defaults", not as a failure.
var ErrNotFound = errors.New("config: not found")

// Config is the resolved project configuration: every default has been applied
// and the values have passed Validate.
type Config struct {
	Version    int            `yaml:"version"`
	Project    Project        `yaml:"project"`
	Agent      Agent          `yaml:"agent"`
	Validation Validation     `yaml:"validation"`
	Review     Review         `yaml:"review"`
	Quality    Quality        `yaml:"quality"`
	Human      Human          `yaml:"human"`
	Decision   DecisionConfig `yaml:"decision"`
	Features   Features       `yaml:"features"`
	Workflow   Workflow       `yaml:"workflow"`
}

// Project holds project metadata.
type Project struct {
	Name              string `yaml:"name"`
	IntegrationBranch string `yaml:"integration_branch"`
}

// Agent selects the execution harness, the model provider it drives, and,
// optionally, the model the provider serves. Credentials and endpoints stay in
// the environment.
type Agent struct {
	// Harness selects the execution layer. "tool" is the tool-enabled coding
	// agent that reads and modifies the repository through controlled tools;
	// "command" is the legacy externally-invoked command agent.
	Harness  string `yaml:"harness"`
	Provider string `yaml:"provider"`
	// Model names the model for the selected provider (ollama or llamacpp). It is
	// ignored by the command provider, and the provider's environment variable
	// (SOP_OLLAMA_MODEL / SOP_LLAMACPP_MODEL) overrides it when set.
	Model string `yaml:"model"`
}

// Validation holds the deterministic verification commands, run in order.
type Validation struct {
	Build []string `yaml:"build"`
	Test  []string `yaml:"test"`
	Lint  []string `yaml:"lint"`
}

// Review selects the review engine and whether it delegates semantic review.
type Review struct {
	Engine     string `yaml:"engine"`
	Delegation bool   `yaml:"delegation"`
}

// Quality holds the quality-gate policy. RequireTests is optional so an omitted
// value keeps the safe default (tests required) while an explicit false is
// still expressible.
type Quality struct {
	RequireTests *bool    `yaml:"require_tests"`
	MaxFixCycles int      `yaml:"max_fix_cycles"`
	FailOn       []string `yaml:"fail_on"`
}

// RequiresTests reports whether tests are required, defaulting to true.
func (q Quality) RequiresTests() bool {
	if q.RequireTests == nil {
		return true
	}
	return *q.RequireTests
}

// Human holds the human-gate policy. ApprovalBeforeCommit is optional so an
// omitted value defaults to requiring approval.
type Human struct {
	ApprovalBeforeCommit *bool `yaml:"approval_before_commit"`
}

// RequiresApprovalBeforeCommit reports whether a human must approve before
// committing, defaulting to true.
func (h Human) RequiresApprovalBeforeCommit() bool {
	if h.ApprovalBeforeCommit == nil {
		return true
	}
	return *h.ApprovalBeforeCommit
}

// DecisionConfig holds the decision-layer policy. The layer is disabled by
// default: the deterministic rules are the baseline, and a Jev adapter must
// demonstrate value before it is trusted (plan T037).
type DecisionConfig struct {
	Provider   string              `yaml:"provider"` // deterministic | jev
	Enabled    bool                `yaml:"enabled"`
	Thresholds decision.Thresholds `yaml:"thresholds"`
}

// Features toggles optional capabilities.
type Features struct {
	JevDecisions bool `yaml:"jev_decisions"`
}

// Workflow configures how a task's completion is represented. "local" completes
// a task at LOCAL_DONE — the local lifecycle ran but no PR, CI, or merge did;
// "pull-request" is reserved for the remote lifecycle.
type Workflow struct {
	Mode string `yaml:"mode"`
}

// Default returns the built-in configuration, the same values the generated
// template documents. New projects start on the tool harness with Ollama and
// deepseek-v4.1-flash:cloud (plan AHV2008).
func Default() Config {
	requireTests := true
	approval := true
	return Config{
		Version: CurrentVersion,
		Project: Project{IntegrationBranch: "main"},
		Agent: Agent{
			Harness:  DefaultHarness,
			Provider: DefaultProvider,
			Model:    DefaultModel,
		},
		Review: Review{Engine: "self"},
		Quality: Quality{
			RequireTests: &requireTests,
			MaxFixCycles: DefaultMaxFixCycles,
			FailOn:       []string{"critical", "high"},
		},
		Human: Human{ApprovalBeforeCommit: &approval},
		Decision: DecisionConfig{
			Provider:   "deterministic",
			Thresholds: decision.Thresholds{RouteToStrongModel: 0.7, RequireHuman: 0.4},
		},
		Workflow: Workflow{Mode: "local"},
	}
}

// Path returns the configuration file path for a project directory.
func Path(projectDir string) string {
	return filepath.Join(projectDir, DirName, FileName)
}

// Load reads and parses the configuration at path, returning ErrNotFound when
// the file does not exist.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	return Parse(data)
}

// LoadDir loads the configuration for a project directory.
func LoadDir(projectDir string) (*Config, error) {
	return Load(Path(projectDir))
}

// Parse parses and validates configuration bytes. Unknown keys, malformed YAML,
// missing required fields, and unsupported versions are all reported as errors.
func Parse(data []byte) (*Config, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var c Config
	if err := dec.Decode(&c); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("config: file is empty")
		}
		return nil, fmt.Errorf("config: parse: %w", err)
	}

	// Reject a second document so a stray "---" cannot silently shadow config.
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return nil, errors.New("config: multiple YAML documents are not supported")
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("config: parse: %w", err)
	}

	c.applyDefaults()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// applyDefaults fills omitted settings. A field whose zero value is also a
// valid explicit value uses a pointer (RequireTests, ApprovalBeforeCommit);
// the others use their zero value as "unset".
//
// Precedence is defaults < explicit config file values: an explicitly
// configured value is never overwritten. In particular a project that sets
// agent.provider: command is never coerced onto the tool/Ollama defaults, so
// legacy command projects keep the command path (plan AHV2008).
func (c *Config) applyDefaults() {
	if c.Version == 0 {
		c.Version = CurrentVersion
	}
	if strings.TrimSpace(c.Project.IntegrationBranch) == "" {
		c.Project.IntegrationBranch = "main"
	}

	// Agent defaults: fill only what the project omitted. A harness is inferred
	// from an explicit provider only when the harness itself is unset and the
	// provider is the legacy command provider, otherwise the harness default is
	// the tool harness.
	provider := strings.TrimSpace(c.Agent.Provider)
	if provider == "" {
		c.Agent.Provider = DefaultProvider
		provider = DefaultProvider
	}
	if strings.TrimSpace(c.Agent.Harness) == "" {
		if provider == "command" {
			c.Agent.Harness = LegacyHarness
		} else {
			c.Agent.Harness = DefaultHarness
		}
	}
	if strings.TrimSpace(c.Agent.Model) == "" {
		// The model is only meaningful for model-serving providers. Leaving it
		// unset for the command provider avoids asserting a model the command
		// path ignores (and keeps the command path's behavior unchanged).
		if provider != "command" {
			c.Agent.Model = DefaultModel
		}
	}

	if strings.TrimSpace(c.Review.Engine) == "" {
		c.Review.Engine = "self"
	}
	if c.Quality.MaxFixCycles == 0 {
		c.Quality.MaxFixCycles = DefaultMaxFixCycles
	}
	if len(c.Quality.FailOn) == 0 {
		c.Quality.FailOn = []string{"critical", "high"}
	}
	if strings.TrimSpace(c.Decision.Provider) == "" {
		c.Decision.Provider = "deterministic"
	}
	if c.Decision.Thresholds == (decision.Thresholds{}) {
		c.Decision.Thresholds = decision.Thresholds{RouteToStrongModel: 0.7, RequireHuman: 0.4}
	}
	if strings.TrimSpace(c.Workflow.Mode) == "" {
		c.Workflow.Mode = "local"
	}
}

// Validate reports the first policy violation in the configuration.
func (c *Config) Validate() error {
	if c.Version != CurrentVersion {
		return fmt.Errorf("config: unsupported version %d (want %d)", c.Version, CurrentVersion)
	}
	if strings.TrimSpace(c.Project.Name) == "" {
		return errors.New("config: project.name is required")
	}
	if !supportedHarnesses[strings.TrimSpace(c.Agent.Harness)] {
		return fmt.Errorf("config: unknown agent.harness %q (want %s)",
			c.Agent.Harness, strings.Join(sortedKeys(supportedHarnesses), ", "))
	}
	if !supportedProviders[strings.TrimSpace(c.Agent.Provider)] {
		return fmt.Errorf("config: unknown agent.provider %q (want %s)",
			c.Agent.Provider, strings.Join(sortedKeys(supportedProviders), ", "))
	}
	if !supportedEngines[strings.TrimSpace(c.Review.Engine)] {
		return fmt.Errorf("config: unknown review.engine %q (want %s)",
			c.Review.Engine, strings.Join(sortedKeys(supportedEngines), ", "))
	}
	if c.Quality.MaxFixCycles < 0 {
		return fmt.Errorf("config: quality.max_fix_cycles must not be negative, got %d", c.Quality.MaxFixCycles)
	}
	for _, sev := range c.Quality.FailOn {
		if !supportedSeverity[strings.TrimSpace(sev)] {
			return fmt.Errorf("config: unknown quality.fail_on severity %q (want %s)",
				sev, strings.Join(sortedKeys(supportedSeverity), ", "))
		}
	}
	switch strings.TrimSpace(c.Decision.Provider) {
	case "deterministic", "jev":
	default:
		return fmt.Errorf("config: unknown decision.provider %q (want deterministic, jev)", c.Decision.Provider)
	}
	t := c.Decision.Thresholds
	if t.RouteToStrongModel < 0 || t.RouteToStrongModel > 1 || t.RequireHuman < 0 || t.RequireHuman > 1 {
		return errors.New("config: decision thresholds must be within [0,1]")
	}
	switch strings.TrimSpace(c.Workflow.Mode) {
	case "local", "pull-request":
	default:
		return fmt.Errorf("config: unknown workflow.mode %q (want local, pull-request)", c.Workflow.Mode)
	}
	return nil
}

// Template renders the documented default configuration for `sop init`.
func Template(projectName string) string {
	name := strings.TrimSpace(projectName)
	if name == "" {
		name = "project"
	}
	return fmt.Sprintf(`# SOP project configuration.
#
# This file describes policy, never mutable task state, and never secrets.
# Agent credentials come from the environment (for example SOP_LLAMACPP_API_KEY).
version: %d

project:
  name: %q
  integration_branch: main

agent:
  # tool | command
  # tool: the tool-enabled coding agent that edits the repository through
  # controlled tools. command: the legacy externally-invoked command agent
  # (for example scripts/sop-agent.sh).
  harness: %s
  # command | ollama | llamacpp
  provider: %s
  # model names the model for ollama/llamacpp; the provider's env var
  # (for example SOP_OLLAMA_MODEL) overrides it.
  model: %s

validation:
  build:
    - go build ./...
  test:
    - go test ./...
  lint:
    - go vet ./...

review:
  # self | open-code-review
  engine: self
  delegation: false

quality:
  require_tests: true
  max_fix_cycles: %d
  fail_on:
    - critical
    - high

human:
  approval_before_commit: true

workflow:
  # local | pull-request
  mode: local
`, CurrentVersion, name, DefaultHarness, DefaultProvider, DefaultModel, DefaultMaxFixCycles)
}

// sortedKeys returns a map's keys in a stable order for error messages.
func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
