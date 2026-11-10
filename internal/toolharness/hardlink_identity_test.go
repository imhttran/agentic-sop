//go:build linux || darwin

package toolharness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// These tests pin the approved LLM02 Option C Phase 1 contract: a bounded
// protected-inode registry plus descriptor identity verification for the three
// direct content tools. They use the same synthetic fixture as the feasibility
// probe. They do not change production behavior.

func readDenied(t *testing.T, h *Harness, path string) {
	t.Helper()
	out, err := h.Run(context.Background(), ToolReadFile, map[string]any{"path": path})
	if !errors.Is(err, ErrSensitivePath) {
		t.Fatalf("read_file %s: want identity denial, got out=%q err=%v", path, out, err)
	}
	if out != "" {
		t.Fatalf("read_file %s returned content despite denial: %q", path, out)
	}
}

func TestIdentityEnforcementDirectAndLateAliases(t *testing.T) {
	root := hardlinkFixture(t)
	h := New(root, DefaultConfig(), NewAuditLog(0))
	ctx := context.Background()

	readDenied(t, h, ".env")

	// Trigger the one-time registry build with an ordinary read.
	out, err := h.Run(ctx, ToolReadFile, map[string]any{"path": "notes.txt"})
	if err != nil || !strings.Contains(out, "ordinary-content") {
		t.Fatalf("ordinary read failed: out=%q err=%v", out, err)
	}

	if err := os.Link(filepath.Join(root, ".env"), filepath.Join(root, "late.txt")); err != nil {
		t.Fatal(err)
	}
	readDenied(t, h, "late.txt")

	if err := os.Symlink(".env", filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	readDenied(t, h, "link.txt")
}

func TestIdentityEnforcementCoversSearchAndCopy(t *testing.T) {
	root := hardlinkFixture(t)
	const canary = "hardlink-identity-canary-60"
	if err := os.Link(filepath.Join(root, ".env"), filepath.Join(root, "alias.txt")); err != nil {
		t.Fatal(err)
	}
	h := New(root, DefaultConfig(), NewAuditLog(0))
	ctx := context.Background()

	out, err := h.Run(ctx, ToolSearchFiles, map[string]any{"path": ".", "pattern": canary})
	if err != nil {
		t.Fatalf("search_files: %v", err)
	}
	if out != "(no matches)" {
		t.Fatalf("search_files exposed protected content: %q", out)
	}

	out, err = h.Run(ctx, ToolCopyFile, map[string]any{"source": "alias.txt", "dest": "copied.txt"})
	if !errors.Is(err, ErrSensitivePath) || out != "" {
		t.Fatalf("copy_file: want identity denial, got out=%q err=%v", out, err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "copied.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("copy_file created a destination despite denial: %v", statErr)
	}
}

func TestIdentityEnforcementRefreshMergesReplacement(t *testing.T) {
	root := hardlinkFixture(t)
	h := New(root, DefaultConfig(), NewAuditLog(0))
	ctx := context.Background()

	if _, err := h.Run(ctx, ToolReadFile, map[string]any{"path": "notes.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(root, ".env"), filepath.Join(root, "old-alias.txt")); err != nil {
		t.Fatal(err)
	}
	readDenied(t, h, "old-alias.txt")

	// Keep the old inode alive through old-alias.txt, then replace .env through
	// model tools so the write refresh must add the new identity.
	if _, err := h.Run(ctx, ToolDeleteFile, map[string]any{"path": ".env"}); err != nil {
		t.Fatalf("delete .env: %v", err)
	}
	if _, err := h.Run(ctx, ToolCreateFile, map[string]any{"path": ".env", "content": "SOP_TEST_API_KEY=replacement\n"}); err != nil {
		t.Fatalf("create .env: %v", err)
	}
	if err := os.Link(filepath.Join(root, ".env"), filepath.Join(root, "new-alias.txt")); err != nil {
		t.Fatal(err)
	}

	readDenied(t, h, "new-alias.txt")
	readDenied(t, h, "old-alias.txt")
}

func TestIdentityEnforcementPositiveControls(t *testing.T) {
	root := hardlinkFixture(t)
	writeProbeFile(t, root, ".env.sample", "SOP_TEST_API_KEY=replace-me\n")
	h := New(root, DefaultConfig(), NewAuditLog(0))
	ctx := context.Background()

	for _, path := range []string{"notes.txt", ".env.example", ".env.sample"} {
		out, err := h.Run(ctx, ToolReadFile, map[string]any{"path": path})
		if err != nil {
			t.Fatalf("positive control %s: %v", path, err)
		}
		if path == "notes.txt" && !strings.Contains(out, "ordinary-content") {
			t.Fatalf("ordinary content missing: %q", out)
		}
		if path != "notes.txt" && !strings.Contains(out, "replace-me") {
			t.Fatalf("placeholder missing from %s: %q", path, out)
		}
	}
}

func TestIdentityEnforcementConcurrentReplacement(t *testing.T) {
	root := hardlinkFixture(t)
	h := New(root, DefaultConfig(), NewAuditLog(0))
	ctx := context.Background()
	if _, err := h.Run(ctx, ToolReadFile, map[string]any{"path": "notes.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(root, ".env"), filepath.Join(root, "alias.txt")); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			tmp := filepath.Join(root, "replacement.tmp")
			if err := os.WriteFile(tmp, []byte("SOP_TEST_API_KEY=concurrent-replacement\n"), 0o644); err != nil {
				return
			}
			_ = os.Rename(tmp, filepath.Join(root, ".env"))
		}
	}()

	for i := 0; i < 50; i++ {
		out, err := h.Run(ctx, ToolReadFile, map[string]any{"path": "alias.txt"})
		if !errors.Is(err, ErrSensitivePath) || out != "" {
			close(stop)
			wg.Wait()
			t.Fatalf("concurrent replacement exposed the alias: out=%q err=%v", out, err)
		}
	}
	close(stop)
	wg.Wait()
}

func TestIdentityEnforcementLateCredentialIsDocumentedResidual(t *testing.T) {
	root := hardlinkFixture(t)
	h := New(root, DefaultConfig(), NewAuditLog(0))
	ctx := context.Background()
	if _, err := h.Run(ctx, ToolReadFile, map[string]any{"path": "notes.txt"}); err != nil {
		t.Fatal(err)
	}

	// A credential created outside the model tools after the snapshot is not a
	// known protected identity. This pins the documented residual; a fresh
	// harness (a new run) rebuilds the registry and denies it.
	writeProbeFile(t, root, ".env.production", "SOP_TEST_API_KEY=late-credential\n")
	if err := os.Link(filepath.Join(root, ".env.production"), filepath.Join(root, "late-credential.txt")); err != nil {
		t.Fatal(err)
	}
	out, err := h.Run(ctx, ToolReadFile, map[string]any{"path": "late-credential.txt"})
	if err != nil || !strings.Contains(out, "late-credential") {
		t.Fatalf("documented late-credential residual changed: out=%q err=%v", out, err)
	}

	fresh := New(root, DefaultConfig(), NewAuditLog(0))
	readDenied(t, fresh, "late-credential.txt")
}

func TestIdentityRegistryBoundsAndFailClosed(t *testing.T) {
	t.Run("scan entry bound", func(t *testing.T) {
		root := t.TempDir()
		writeProbeFile(t, root, ".env.one", "x")
		writeProbeFile(t, root, ".env.two", "x")
		if _, err := scanProtectedIdentities([]authorizedRoot{{path: root}}, 1, 100); err == nil {
			t.Fatal("want a scan entry bound error")
		}
	})

	t.Run("protected identity bound", func(t *testing.T) {
		root := t.TempDir()
		writeProbeFile(t, root, ".env.one", "x")
		writeProbeFile(t, root, ".env.two", "x")
		if _, err := scanProtectedIdentities([]authorizedRoot{{path: root}}, 100, 1); err == nil {
			t.Fatal("want a protected identity bound error")
		}
	})

	t.Run("unavailable registry fails closed", func(t *testing.T) {
		root := hardlinkFixture(t)
		h := New(root, DefaultConfig(), NewAuditLog(0))
		h.identity.once.Do(func() {})
		h.identity.err = fmt.Errorf("%w: injected", ErrIdentityRegistryUnavailable)
		out, err := h.Run(context.Background(), ToolReadFile, map[string]any{"path": "notes.txt"})
		if !errors.Is(err, ErrIdentityRegistryUnavailable) || out != "" {
			t.Fatalf("want fail-closed registry error, got out=%q err=%v", out, err)
		}
	})
}
