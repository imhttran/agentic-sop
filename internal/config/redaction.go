package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/redact"
)

// Redaction is the optional, opt-in LLM02 C1 model-input redaction policy.
// An omitted block leaves existing behavior unchanged: the policy is disabled.
//
// The block contains no secret values. InventoryPath names an operator-owned
// file outside the repository; the values are loaded once at startup and never
// rendered, logged, or persisted by this package.
type Redaction struct {
	// Enabled turns on exact inventory redaction plus bounded contextual
	// credential-key and known-format detection at the model-input boundary.
	Enabled bool `yaml:"enabled"`
	// InventoryPath is an absolute path or a ~/ path outside the repository. It
	// is required when Enabled is true.
	InventoryPath string `yaml:"inventory_path"`
	// MaxValues, MinValueBytes, and MaxValueBytes bound the inventory file.
	MaxValues     int `yaml:"max_values"`
	MinValueBytes int `yaml:"min_value_bytes"`
	MaxValueBytes int `yaml:"max_value_bytes"`
}

// RedactionPolicy loads the configured inventory and returns the model-input
// policy. It is a no-op when disabled. When enabled, a missing, malformed,
// symlinked, non-regular, or non-0600 inventory fails closed; the error names
// the path or line but never includes a value.
func (c Config) RedactionPolicy(projectDir string) (redact.Policy, error) {
	if !c.Redaction.Enabled {
		return redact.Policy{}, nil
	}
	if strings.TrimSpace(projectDir) == "" {
		return redact.Policy{}, errors.New("config: project directory is required for redaction.inventory_path")
	}
	path, err := resolveInventoryPath(c.Redaction.InventoryPath)
	if err != nil {
		return redact.Policy{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return redact.Policy{}, fmt.Errorf("config: redaction.inventory_path %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return redact.Policy{}, fmt.Errorf("config: redaction.inventory_path %s is a symlink; refusing", path)
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return redact.Policy{}, fmt.Errorf("config: redaction.inventory_path %s: %w", path, err)
	}
	projectAbs, err := filepath.Abs(projectDir)
	if err != nil {
		return redact.Policy{}, fmt.Errorf("config: resolve project directory: %w", err)
	}
	projectReal := projectAbs
	if resolved, err := filepath.EvalSymlinks(projectAbs); err == nil {
		projectReal = resolved
	}
	rel, err := filepath.Rel(projectReal, real)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return redact.Policy{}, fmt.Errorf("config: redaction.inventory_path %s is inside the repository; it must be external", path)
	}
	values, err := redact.LoadInventory(real, redact.InventoryLimits{
		MaxValues:     c.Redaction.MaxValues,
		MinValueBytes: c.Redaction.MinValueBytes,
		MaxValueBytes: c.Redaction.MaxValueBytes,
	})
	if err != nil {
		return redact.Policy{}, fmt.Errorf("config: %w", err)
	}
	return redact.Policy{Enabled: true, Values: values}, nil
}

func resolveInventoryPath(value string) (string, error) {
	p := strings.TrimSpace(value)
	if p == "" {
		return "", errors.New("config: redaction.inventory_path is required when redaction.enabled is true")
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("config: resolve redaction.inventory_path: %w", err)
		}
		p = filepath.Join(home, p[2:])
	} else if strings.HasPrefix(p, "~") {
		return "", errors.New("config: redaction.inventory_path must use ~/ or an absolute path")
	}
	if !filepath.IsAbs(p) {
		return "", errors.New("config: redaction.inventory_path must be absolute or start with ~/; it must be outside the repository")
	}
	return filepath.Clean(p), nil
}

func (c *Config) applyRedactionDefaults() {
	if c.Redaction.MaxValues == 0 {
		c.Redaction.MaxValues = redact.DefaultInventoryMaxValues
	}
	if c.Redaction.MinValueBytes == 0 {
		c.Redaction.MinValueBytes = redact.DefaultInventoryMinValueBytes
	}
	if c.Redaction.MaxValueBytes == 0 {
		c.Redaction.MaxValueBytes = redact.DefaultInventoryMaxValueBytes
	}
}

func (c *Config) validateRedaction() error {
	r := c.Redaction
	if !r.Enabled {
		return nil
	}
	if strings.TrimSpace(r.InventoryPath) == "" {
		return errors.New("config: redaction.inventory_path is required when redaction.enabled is true")
	}
	if _, err := resolveInventoryPath(r.InventoryPath); err != nil {
		return err
	}
	if r.MaxValues < 1 || r.MaxValues > 10000 {
		return fmt.Errorf("config: redaction.max_values must be within [1,10000], got %d", r.MaxValues)
	}
	if r.MinValueBytes < 8 {
		return fmt.Errorf("config: redaction.min_value_bytes must be at least 8, got %d", r.MinValueBytes)
	}
	if r.MaxValueBytes < r.MinValueBytes || r.MaxValueBytes > 1<<20 {
		return fmt.Errorf("config: redaction.max_value_bytes must be within [%d,1048576], got %d", r.MinValueBytes, r.MaxValueBytes)
	}
	return nil
}
