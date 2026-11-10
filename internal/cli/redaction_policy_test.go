package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
)

func writeCLIRedactionConfig(t *testing.T, project, inventory string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(project, config.DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	doc := `version: 1
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
  inventory_path: ` + inventory + `
`
	if err := os.WriteFile(config.Path(project), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRedactionPolicyFromProjectDisabled(t *testing.T) {
	policy, err := redactionPolicyFromProject(t.TempDir())
	if err != nil {
		t.Fatalf("missing config: %v", err)
	}
	if policy.Enabled {
		t.Fatal("missing config enabled redaction")
	}
}

func TestRedactionPolicyFromProjectLoadsExternalInventory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX 0600 inventory is Unix-only")
	}
	project := t.TempDir()
	inventoryDir := t.TempDir()
	inventory := filepath.Join(inventoryDir, "sensitive-values.txt")
	if err := os.WriteFile(inventory, []byte("alpha-value-123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeCLIRedactionConfig(t, project, inventory)
	policy, err := redactionPolicyFromProject(project)
	if err != nil {
		t.Fatalf("redactionPolicyFromProject: %v", err)
	}
	if !policy.Enabled || len(policy.Values) != 1 || policy.Values[0] != "alpha-value-123" {
		t.Fatalf("policy = %+v", policy)
	}
}

func TestRedactionPolicyFromProjectFailsClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX 0600 inventory is Unix-only")
	}
	project := t.TempDir()
	writeCLIRedactionConfig(t, project, filepath.Join(t.TempDir(), "missing"))
	if _, err := redactionPolicyFromProject(project); err == nil {
		t.Fatal("missing enabled inventory was accepted")
	}
}
