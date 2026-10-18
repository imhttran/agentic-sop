package commandpolicy

import (
	"path/filepath"
	"testing"
)

// TestCargoRequiresPermission proves a supported Cargo command is not SAFE unless
// the relevant permission is enabled.
func TestCargoRequiresPermission(t *testing.T) {
	argv := []string{"cargo", "build"}
	class, err := CargoClassify(argv, false, t.TempDir())
	if err != nil {
		t.Fatalf("disabled classify error: %v", err)
	}
	if class == Safe {
		t.Fatal("a Cargo command must not be SAFE with the permission off")
	}
	class, err = CargoClassify(argv, true, t.TempDir())
	if err != nil {
		t.Fatalf("enabled classify error: %v", err)
	}
	if class != Safe {
		t.Fatalf("cargo build should be SAFE when enabled, got %s", class)
	}
	// Not a cargo command at all.
	if _, err := CargoClassify([]string{"go", "build"}, true, ""); err == nil {
		t.Fatal("non-cargo command must return an error")
	}
}

// TestCargoRejectsUnsupportedArguments proves unsupported subcommands and
// arguments, and shell syntax, are rejected before execution.
func TestCargoRejectsUnsupportedArguments(t *testing.T) {
	root := t.TempDir()
	bad := [][]string{
		{"cargo", "publish"},
		{"cargo", "build", "--unknown-flag"},
		{"cargo", "build", "-p", "crate"},
		{"cargo", "fmt"},
		{"cargo", "build", ";", "rm", "-rf", "/"},
		{"cargo", "build", "a|b"},
	}
	for _, argv := range bad {
		class, err := CargoClassify(argv, true, root)
		if err == nil && class == Safe {
			t.Fatalf("expected rejection for %v, got SAFE", argv)
		}
	}
	good := [][]string{
		{"cargo", "build"},
		{"cargo", "test", "--release"},
		{"cargo", "check"},
		{"cargo", "clippy", "--all-targets"},
		{"cargo", "fmt", "--check"},
	}
	for _, argv := range good {
		class, err := CargoClassify(argv, true, root)
		if err != nil || class != Safe {
			t.Fatalf("expected SAFE for %v, got %s err=%v", argv, class, err)
		}
	}
}

// TestCargoSafetyOrderingBeforePermission proves structural safety is checked
// before the permission flag: a dangerous form is rejected even on a path whose
// Cargo permission is off, rather than being reported as merely requiring
// approval. This keeps the checker's behavior independent of the permission.
func TestCargoSafetyOrderingBeforePermission(t *testing.T) {
	root := t.TempDir()
	for _, argv := range [][]string{
		{"cargo", "build", ";", "rm", "-rf", "/"},
		{"cargo", "publish"},
		{"cargo", "build", "--manifest-path", filepath.Join("..", "escape.toml")},
	} {
		class, err := CargoClassify(argv, false, root)
		if err == nil || class == Safe {
			t.Fatalf("disabled permission masked a rejected form %v: class=%s err=%v", argv, class, err)
		}
	}
	// A supported form with the permission off is still only RequiresApproval.
	class, err := CargoClassify([]string{"cargo", "build"}, false, root)
	if err != nil || class != RequiresApproval {
		t.Fatalf("supported disabled form = %s, %v; want RequiresApproval", class, err)
	}
}

// TestCargoRejectsManifestPathEscape proves a --manifest-path that resolves
// outside the repository is rejected.
func TestCargoRejectsManifestPathEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join("..", "elsewhere", "Cargo.toml")
	if _, err := CargoClassify([]string{"cargo", "build", "--manifest-path", outside}, true, root); err == nil {
		t.Fatalf("manifest path escape must be rejected: %s", outside)
	}
	abs := filepath.Join(string(filepath.Separator), "tmp", "Cargo.toml")
	if _, err := CargoClassify([]string{"cargo", "build", "--manifest-path=" + abs}, true, root); err == nil {
		t.Fatal("absolute manifest path outside root must be rejected")
	}
	inside := filepath.Join("crate", "Cargo.toml")
	class, err := CargoClassify([]string{"cargo", "build", "--manifest-path", inside}, true, root)
	if err != nil || class != Safe {
		t.Fatalf("in-repo manifest path should be SAFE, got %s err=%v", class, err)
	}
}
