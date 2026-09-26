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

// Supported agent providers and review engines, kept here so a bad value fails
// at load time with a clear message instead of at first use.
var (
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
	Version    int        `yaml:"version"`
	Project    Project    `yaml:"project"`
	Agent      Agent      `yaml:"agent"`
	Validation Validation `yaml:"validation"`
	Review     Review     `yaml:"review"`
	Quality    Quality    `yaml:"quality"`
	Human      Human      `yaml:"human"`
}

// Project holds project metadata.
type Project struct {
	Name              string `yaml:"name"`
	IntegrationBranch string `yaml:"integration_branch"`
}

// Agent selects the model provider. Credentials and endpoints stay in the
// environment.
type Agent struct {
	Provider string `yaml:"provider"`
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

// Default returns the built-in configuration, the same values the generated
// template documents.
func Default() Config {
	requireTests := true
	approval := true
	return Config{
		Version: CurrentVersion,
		Project: Project{IntegrationBranch: "main"},
		Agent:   Agent{Provider: "command"},
		Review:  Review{Engine: "self"},
		Quality: Quality{
			RequireTests: &requireTests,
			MaxFixCycles: DefaultMaxFixCycles,
			FailOn:       []string{"critical", "high"},
		},
		Human: Human{ApprovalBeforeCommit: &approval},
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
func (c *Config) applyDefaults() {
	if c.Version == 0 {
		c.Version = CurrentVersion
	}
	if strings.TrimSpace(c.Project.IntegrationBranch) == "" {
		c.Project.IntegrationBranch = "main"
	}
	if strings.TrimSpace(c.Agent.Provider) == "" {
		c.Agent.Provider = "command"
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
}

// Validate reports the first policy violation in the configuration.
func (c *Config) Validate() error {
	if c.Version != CurrentVersion {
		return fmt.Errorf("config: unsupported version %d (want %d)", c.Version, CurrentVersion)
	}
	if strings.TrimSpace(c.Project.Name) == "" {
		return errors.New("config: project.name is required")
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
  # command | ollama | llamacpp
  provider: command

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
`, CurrentVersion, name, DefaultMaxFixCycles)
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
