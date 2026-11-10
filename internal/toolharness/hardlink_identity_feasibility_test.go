//go:build linux || darwin

package toolharness

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/sensitivepath"
)

// This file is a feasibility probe for a possible hardlink/path-identity
// policy. It does not change production enforcement. It measures whether
// os.SameFile can identify a protected inode through an ordinary-name alias,
// which lifecycle events invalidate a snapshot registry, how the existing file
// tools behave, and the bounded cost of a workspace scan.

type probeProtectedIdentity struct {
	rel  string
	info fs.FileInfo
}

func writeProbeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// buildIdentityRegistry walks the workspace once and records the identity of
// every lexically denied credential path. The cost is bounded by the walk and
// the number of denied paths, not by the number of credential values.
func buildIdentityRegistry(t *testing.T, root string) ([]probeProtectedIdentity, error) {
	t.Helper()
	var registry []probeProtectedIdentity
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == stateDirRel {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !sensitivepath.IsToolReadDenied(rel) {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		registry = append(registry, probeProtectedIdentity{rel: rel, info: info})
		return nil
	})
	return registry, err
}

func matchProtectedIdentity(registry []probeProtectedIdentity, info fs.FileInfo) (string, bool) {
	for _, entry := range registry {
		if os.SameFile(entry.info, info) {
			return entry.rel, true
		}
	}
	return "", false
}

func openAndMatch(t *testing.T, path string, registry []probeProtectedIdentity) (string, bool) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	return matchProtectedIdentity(registry, info)
}

func hardlinkFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeProbeFile(t, root, ".env", "SOP_TEST_API_KEY=hardlink-identity-canary-60\n")
	writeProbeFile(t, root, ".env.example", "SOP_TEST_API_KEY=replace-me\n")
	writeProbeFile(t, root, "notes.txt", "ordinary-content\n")
	return root
}

func TestHardlinkIdentityFeasibility(t *testing.T) {
	t.Run("pre-existing alias matches the protected inode", func(t *testing.T) {
		root := hardlinkFixture(t)
		if err := os.Link(filepath.Join(root, ".env"), filepath.Join(root, "alias.txt")); err != nil {
			t.Fatal(err)
		}
		registry, err := buildIdentityRegistry(t, root)
		if err != nil {
			t.Fatal(err)
		}
		rel, ok := openAndMatch(t, filepath.Join(root, "alias.txt"), registry)
		if !ok || rel != ".env" {
			t.Fatalf("pre-existing alias not identified: rel=%q ok=%t", rel, ok)
		}
	})

	t.Run("alias created after the scan matches a known inode", func(t *testing.T) {
		root := hardlinkFixture(t)
		registry, err := buildIdentityRegistry(t, root)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Link(filepath.Join(root, ".env"), filepath.Join(root, "late.txt")); err != nil {
			t.Fatal(err)
		}
		if _, ok := openAndMatch(t, filepath.Join(root, "late.txt"), registry); !ok {
			t.Fatal("a new alias to an already-known protected inode was not identified")
		}
	})

	t.Run("new credential created after the scan is not identified", func(t *testing.T) {
		root := hardlinkFixture(t)
		registry, err := buildIdentityRegistry(t, root)
		if err != nil {
			t.Fatal(err)
		}
		writeProbeFile(t, root, ".env.production", "SOP_TEST_API_KEY=late-credential\n")
		if err := os.Link(filepath.Join(root, ".env.production"), filepath.Join(root, "late-credential.txt")); err != nil {
			t.Fatal(err)
		}
		if _, ok := openAndMatch(t, filepath.Join(root, "late-credential.txt"), registry); ok {
			t.Fatal("a credential created after the scan was unexpectedly identified")
		}
		refreshed, err := buildIdentityRegistry(t, root)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := openAndMatch(t, filepath.Join(root, "late-credential.txt"), refreshed); !ok {
			t.Fatal("a rebuild did not identify the late credential")
		}
	})

	t.Run("replacement with the original inode kept alive", func(t *testing.T) {
		root := hardlinkFixture(t)
		// Keep the original inode alive through another link so the replacement
		// cannot reuse it. Immediate recreation often reuses the inode, which
		// makes identity look stable when it is only allocator behavior.
		if err := os.Link(filepath.Join(root, ".env"), filepath.Join(root, "original-link.txt")); err != nil {
			t.Fatal(err)
		}
		registry, err := buildIdentityRegistry(t, root)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(root, ".env")); err != nil {
			t.Fatal(err)
		}
		writeProbeFile(t, root, ".env", "SOP_TEST_API_KEY=replacement\n")
		if err := os.Link(filepath.Join(root, ".env"), filepath.Join(root, "replacement.txt")); err != nil {
			t.Fatal(err)
		}
		if _, ok := openAndMatch(t, filepath.Join(root, "replacement.txt"), registry); ok {
			t.Fatal("the stale registry identified the replacement inode")
		}
		if _, ok := openAndMatch(t, filepath.Join(root, "original-link.txt"), registry); !ok {
			t.Fatal("the stale registry lost the preserved original inode")
		}

		refreshed, err := buildIdentityRegistry(t, root)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := openAndMatch(t, filepath.Join(root, "replacement.txt"), refreshed); !ok {
			t.Fatal("a rebuild did not identify the replacement inode")
		}
		_, keptOld := openAndMatch(t, filepath.Join(root, "original-link.txt"), refreshed)
		t.Logf("replace-style refresh kept the old inode: %t", keptOld)

		merged := append(append([]probeProtectedIdentity(nil), registry...), refreshed...)
		if _, ok := openAndMatch(t, filepath.Join(root, "original-link.txt"), merged); !ok {
			t.Fatal("a merged registry lost the preserved original inode")
		}
	})

	t.Run("rename keeps identity and can deny an ordinary name", func(t *testing.T) {
		root := hardlinkFixture(t)
		registry, err := buildIdentityRegistry(t, root)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(root, ".env"), filepath.Join(root, "renamed.txt")); err != nil {
			t.Fatal(err)
		}
		if sensitivepath.IsToolReadDenied("renamed.txt") {
			t.Fatal("test fixture is wrong: renamed.txt is lexically denied")
		}
		if _, ok := openAndMatch(t, filepath.Join(root, "renamed.txt"), registry); !ok {
			t.Fatal("identity did not survive a rename")
		}
	})

	t.Run("symlink alias is already denied by resolved-path policy", func(t *testing.T) {
		root := hardlinkFixture(t)
		if err := os.Symlink(".env", filepath.Join(root, "link.txt")); err != nil {
			t.Fatal(err)
		}
		real, err := filepath.EvalSymlinks(filepath.Join(root, "link.txt"))
		if err != nil {
			t.Fatal(err)
		}
		rel, err := filepath.Rel(root, real)
		if err != nil {
			t.Fatal(err)
		}
		if !sensitivepath.IsToolReadDenied(filepath.ToSlash(rel)) {
			t.Fatal("resolved symlink target is not lexically denied")
		}
	})

	t.Run("ordinary hardlink and .env.example are not protected identities", func(t *testing.T) {
		root := hardlinkFixture(t)
		if err := os.Link(filepath.Join(root, "notes.txt"), filepath.Join(root, "notes-alias.txt")); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(filepath.Join(root, ".env.example"), filepath.Join(root, "example-alias.txt")); err != nil {
			t.Fatal(err)
		}
		registry, err := buildIdentityRegistry(t, root)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"notes-alias.txt", "example-alias.txt"} {
			if _, ok := openAndMatch(t, filepath.Join(root, name), registry); ok {
				t.Fatalf("%s was incorrectly identified as protected", name)
			}
		}
	})

	t.Run("hardlink to an outside credential is outside the workspace scan", func(t *testing.T) {
		root := hardlinkFixture(t)
		outside := t.TempDir()
		writeProbeFile(t, outside, ".env", "SOP_TEST_API_KEY=outside-canary\n")
		if err := os.Link(filepath.Join(outside, ".env"), filepath.Join(root, "outside-alias.txt")); err != nil {
			t.Skipf("hardlink across temporary directories is not supported: %v", err)
		}
		registry, err := buildIdentityRegistry(t, root)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := openAndMatch(t, filepath.Join(root, "outside-alias.txt"), registry); ok {
			t.Fatal("the workspace scan unexpectedly identified an outside credential")
		}
	})
}

func TestHardlinkAliasToolDenial(t *testing.T) {
	root := hardlinkFixture(t)
	const canary = "hardlink-identity-canary-60"
	if err := os.Link(filepath.Join(root, ".env"), filepath.Join(root, "alias.txt")); err != nil {
		t.Fatal(err)
	}
	h := New(root, DefaultConfig(), NewAuditLog(0))
	ctx := context.Background()

	out, err := h.Run(ctx, ToolReadFile, map[string]any{"path": "alias.txt"})
	if !errors.Is(err, ErrSensitivePath) || out != "" {
		t.Fatalf("read_file alias: want identity denial, got err=%v out=%q", err, out)
	}

	out, err = h.Run(ctx, ToolSearchFiles, map[string]any{"path": ".", "pattern": canary})
	if err != nil {
		t.Fatalf("search_files alias: %v", err)
	}
	if strings.Contains(out, canary) || strings.Contains(out, "alias.txt") {
		t.Fatalf("search_files exposed the protected alias: %q", out)
	}
	if out != "(no matches)" {
		t.Fatalf("search_files returned %q, want no matches", out)
	}

	out, err = h.Run(ctx, ToolCopyFile, map[string]any{"source": "alias.txt", "dest": "copied.txt"})
	if !errors.Is(err, ErrSensitivePath) || out != "" {
		t.Fatalf("copy_file alias: want identity denial, got err=%v out=%q", err, out)
	}
	if _, statErr := os.Stat(filepath.Join(root, "copied.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("copy_file created a destination despite the identity denial: %v", statErr)
	}
}

func TestHardlinkIdentityRegistryCost(t *testing.T) {
	root := t.TempDir()
	for dir := 0; dir < 50; dir++ {
		for file := 0; file < 100; file++ {
			writeProbeFile(t, root, fmt.Sprintf("tree/%02d/file-%04d.txt", dir, file), "ordinary\n")
		}
	}
	for i := 0; i < 100; i++ {
		writeProbeFile(t, root, fmt.Sprintf(".env.bench%03d", i), "SOP_TEST_API_KEY=bench\n")
	}
	writeProbeFile(t, root, "notes.txt", "ordinary\n")

	start := time.Now()
	registry, err := buildIdentityRegistry(t, root)
	buildCost := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry) != 100 {
		t.Fatalf("registry entries = %d, want 100", len(registry))
	}

	info, err := os.Stat(filepath.Join(root, "notes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	const checks = 10000
	start = time.Now()
	for i := 0; i < checks; i++ {
		matchProtectedIdentity(registry, info)
	}
	perCheck := time.Since(start) / checks

	t.Logf("registry entries=%d build=%s per-check=%s", len(registry), buildCost, perCheck)
	if buildCost > 5*time.Second {
		t.Fatalf("registry build took %s, want <= 5s", buildCost)
	}
	if perCheck > time.Millisecond {
		t.Fatalf("identity check took %s, want <= 1ms", perCheck)
	}
}
