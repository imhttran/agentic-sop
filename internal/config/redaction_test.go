package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeRedactionInventory(t *testing.T, content string, mode os.FileMode) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sensitive-values.txt")
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRedactionValidate(t *testing.T) {
	c := Default()
	c.Redaction.Enabled = true
	c.Redaction.InventoryPath = "/tmp/sensitive-values"
	c.applyRedactionDefaults()
	if err := c.validateRedaction(); err != nil {
		t.Fatalf("valid redaction config rejected: %v", err)
	}

	c.Redaction.InventoryPath = ""
	if err := c.validateRedaction(); err == nil {
		t.Fatal("enabled redaction with no inventory path was accepted")
	}
	c.Redaction.InventoryPath = "relative/path"
	if err := c.validateRedaction(); err == nil {
		t.Fatal("relative inventory path was accepted")
	}
	c.Redaction.InventoryPath = "/tmp/sensitive-values"
	c.Redaction.MinValueBytes = 4
	if err := c.validateRedaction(); err == nil {
		t.Fatal("min_value_bytes below 8 was accepted")
	}
	c.Redaction.MinValueBytes = 8
	c.Redaction.MaxValueBytes = 4
	if err := c.validateRedaction(); err == nil {
		t.Fatal("max_value_bytes below min was accepted")
	}
}

func TestRedactionPolicyDisabled(t *testing.T) {
	c := Default()
	policy, err := c.RedactionPolicy(t.TempDir())
	if err != nil {
		t.Fatalf("disabled policy: %v", err)
	}
	if policy.Enabled || len(policy.Values) != 0 {
		t.Fatalf("disabled policy = %+v, want zero", policy)
	}
}

func TestRedactionPolicyLoadsExternalInventory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX 0600 inventory is Unix-only")
	}
	project := t.TempDir()
	path := writeRedactionInventory(t, "alpha-value-123\nbeta-value-456\n", 0o600)
	c := Default()
	c.Redaction = Redaction{
		Enabled:       true,
		InventoryPath: path,
		MaxValues:     10,
		MinValueBytes: 8,
		MaxValueBytes: 128,
	}
	policy, err := c.RedactionPolicy(project)
	if err != nil {
		t.Fatalf("RedactionPolicy: %v", err)
	}
	if !policy.Enabled || len(policy.Values) != 2 {
		t.Fatalf("policy = %+v, want enabled with two values", policy)
	}
}

func TestRedactionPolicyFailsClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX 0600 inventory is Unix-only")
	}
	project := t.TempDir()
	c := Default()
	c.Redaction = Redaction{
		Enabled:       true,
		MaxValues:     10,
		MinValueBytes: 8,
		MaxValueBytes: 128,
	}

	t.Run("missing", func(t *testing.T) {
		c.Redaction.InventoryPath = filepath.Join(t.TempDir(), "missing")
		if _, err := c.RedactionPolicy(project); err == nil {
			t.Fatal("missing inventory was accepted")
		}
	})
	t.Run("inside repository", func(t *testing.T) {
		path := filepath.Join(project, "inventory.txt")
		if err := os.WriteFile(path, []byte("alpha-value-123\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		c.Redaction.InventoryPath = path
		if _, err := c.RedactionPolicy(project); err == nil {
			t.Fatal("inventory inside the repository was accepted")
		}
	})
	t.Run("symlink", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "target")
		if err := os.WriteFile(target, []byte("alpha-value-123\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(dir, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		c.Redaction.InventoryPath = link
		if _, err := c.RedactionPolicy(project); err == nil {
			t.Fatal("symlink inventory was accepted")
		}
	})
	t.Run("permissions", func(t *testing.T) {
		c.Redaction.InventoryPath = writeRedactionInventory(t, "alpha-value-123\n", 0o644)
		if _, err := c.RedactionPolicy(project); err == nil {
			t.Fatal("0644 inventory was accepted")
		}
	})
}

func TestParseAcceptsRedactionBlock(t *testing.T) {
	const doc = `version: 1
project:
  name: test
agent:
  harness: tool
  provider: ollama
  model: qwen3:4b
review:
  engine: self
quality:
  require_tests: true
  fail_on: [critical, high]
human:
  approval_before_commit: true
workflow:
  mode: local
redaction:
  enabled: true
  inventory_path: /tmp/sensitive-values
`
	cfg, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !cfg.Redaction.Enabled || cfg.Redaction.InventoryPath != "/tmp/sensitive-values" {
		t.Fatalf("redaction block = %+v", cfg.Redaction)
	}
	if cfg.Redaction.MaxValues != 256 || cfg.Redaction.MinValueBytes != 8 || cfg.Redaction.MaxValueBytes != 4096 {
		t.Fatalf("redaction defaults = %+v", cfg.Redaction)
	}
}
