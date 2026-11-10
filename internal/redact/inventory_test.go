package redact

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeInventory(t *testing.T, content string, mode os.FileMode) string {
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

func TestLoadInventory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX 0600 inventory is Unix-only")
	}
	path := writeInventory(t, "alpha-value-123\r\n\r\nbeta-value-456\nalpha-value-123\n", 0o600)
	values, err := LoadInventory(path, InventoryLimits{})
	if err != nil {
		t.Fatalf("LoadInventory: %v", err)
	}
	if len(values) != 2 || values[0] != "alpha-value-123" || values[1] != "beta-value-456" {
		t.Fatalf("LoadInventory values = %v", values)
	}
}

func TestLoadInventoryRejectsUnsafeFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX 0600 inventory is Unix-only")
	}
	t.Run("missing", func(t *testing.T) {
		if _, err := LoadInventory(filepath.Join(t.TempDir(), "missing"), InventoryLimits{}); err == nil {
			t.Fatal("missing inventory was accepted")
		}
	})
	t.Run("permissions", func(t *testing.T) {
		path := writeInventory(t, "alpha-value-123\n", 0o644)
		if _, err := LoadInventory(path, InventoryLimits{}); err == nil {
			t.Fatal("0644 inventory was accepted")
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
		if _, err := LoadInventory(link, InventoryLimits{}); err == nil {
			t.Fatal("symlink inventory was accepted")
		}
	})
	t.Run("directory", func(t *testing.T) {
		if _, err := LoadInventory(t.TempDir(), InventoryLimits{}); err == nil {
			t.Fatal("directory inventory was accepted")
		}
	})
	t.Run("short value", func(t *testing.T) {
		path := writeInventory(t, "short\n", 0o600)
		if _, err := LoadInventory(path, InventoryLimits{MinValueBytes: 8}); err == nil {
			t.Fatal("short inventory value was accepted")
		}
	})
	t.Run("too many values", func(t *testing.T) {
		path := writeInventory(t, "alpha-value-123\nbeta-value-456\n", 0o600)
		if _, err := LoadInventory(path, InventoryLimits{MaxValues: 1}); err == nil {
			t.Fatal("oversized inventory count was accepted")
		}
	})
}

func TestLoadInventoryErrorsDoNotLeakValues(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX 0600 inventory is Unix-only")
	}
	const value = "alpha-value-123"
	path := writeInventory(t, value+"\n", 0o644)
	_, err := LoadInventory(path, InventoryLimits{})
	if err == nil {
		t.Fatal("0644 inventory was accepted")
	}
	if strings.Contains(err.Error(), value) {
		t.Fatalf("inventory error leaked the value: %v", err)
	}
}
