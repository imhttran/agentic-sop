package ollamaagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/redact"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// TestC1HardlinkAliasModelInput characterizes a pre-existing hardlink with an
// ordinary name under the approved C1 policy. It keeps three questions separate:
//
//  1. Path authorization: does read_file refuse the alias before reading it?
//  2. Raw tool output: did the harness return the synthetic content at all?
//  3. Model-input redaction: did C1 keep the content out of the model boundary?
//
// An alias to a lexically denied file is now refused by the Option C Phase 1
// identity policy before content is returned. Aliases to ordinary files (an
// inventory value, a contextual credential, or an unlisted value) are not
// protected identities; C1 is their content control: shape and contextual
// rules plus exact inventory values are redacted, while an unlisted opaque
// value remains a documented residual.
//
// Synthetic canaries only. The inventory is an external 0600 file outside the
// repository, loaded through the production inventory loader.
func TestC1HardlinkAliasModelInput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("C1 inventory loading is Unix-only; hardlink C1 coverage runs on Unix")
	}
	const (
		shapeValue      = "sk-hardlink-shape-not-a-real-secret"
		envOpaqueValue  = "hardlink-env-opaque-value-41"
		inventoryValue  = "hardlink-inventory-value-42"
		contextualValue = "hardlink-contextual-value-43"
		unlistedValue   = "hardlink-unlisted-value-44"
		ordinaryMarker  = "hardlink-ordinary-marker"
	)

	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	gitInit(t, repo)
	writeFile(t, repo, ".env", "SOP_TEST_API_KEY="+shapeValue+"\nSOP_OPAQUE_SECRET="+envOpaqueValue+"\n")
	writeFile(t, repo, ".env.example", "SOP_TEST_API_KEY=replace-me\n")
	writeFile(t, repo, ".env.sample", "SOP_TEST_API_KEY=replace-me\n")
	writeFile(t, repo, "notes.txt", ordinaryMarker+" ordinary repository content\n")
	writeFile(t, repo, "inventory.txt", "the value is "+inventoryValue+"\n")
	writeFile(t, repo, "contextual.txt", "opaque_secret="+contextualValue+"\n")
	writeFile(t, repo, "unlisted.txt", "the value is "+unlistedValue+"\n")

	aliases := map[string]string{
		"alias-env.txt":        ".env",
		"alias-inventory.txt":  "inventory.txt",
		"alias-contextual.txt": "contextual.txt",
		"alias-unlisted.txt":   "unlisted.txt",
	}
	for alias, target := range aliases {
		if err := os.Link(filepath.Join(repo, target), filepath.Join(repo, alias)); err != nil {
			t.Fatalf("create hardlink %s -> %s: %v", alias, target, err)
		}
	}
	if err := os.Symlink(".env", filepath.Join(repo, "env-symlink.txt")); err != nil {
		t.Fatalf("create symlink to .env: %v", err)
	}

	// The inventory is a regular 0600 file outside the repository and is loaded
	// through the production loader, not injected as a literal.
	inventoryPath := filepath.Join(root, "sensitive-values.txt")
	if err := os.WriteFile(inventoryPath, []byte(inventoryValue+"\n"), 0o600); err != nil {
		t.Fatalf("write external inventory: %v", err)
	}
	if err := os.Chmod(inventoryPath, 0o600); err != nil {
		t.Fatalf("set inventory permissions: %v", err)
	}
	values, err := redact.LoadInventory(inventoryPath, redact.InventoryLimits{})
	if err != nil {
		t.Fatalf("load external inventory: %v", err)
	}
	policy := redact.Policy{Enabled: true, Values: values}

	h := toolharness.New(repo, toolharness.DefaultConfig(), toolharness.NewAuditLog(0))
	ctx := context.Background()
	read := func(path string) (string, error) {
		return h.Run(ctx, toolharness.ToolReadFile, map[string]any{"path": path})
	}
	modelInput := func(out string, runErr error) string {
		return toolResultMessageWithPolicy(toolharness.ToolReadFile, out, runErr, policy)
	}

	t.Run("hardlink to .env is denied at the path level", func(t *testing.T) {
		out, runErr := read("alias-env.txt")
		if !errors.Is(runErr, toolharness.ErrSensitivePath) {
			t.Fatalf("hardlink alias to .env: want identity denial, got out=%q err=%v", out, runErr)
		}
		if out != "" {
			t.Fatalf("hardlink alias to .env returned content: %q", out)
		}
		for _, canary := range []string{shapeValue, envOpaqueValue} {
			if msg := modelInput(out, runErr); strings.Contains(msg, canary) {
				t.Fatalf("identity denial exposed %q: %q", canary, msg)
			}
		}
	})

	t.Run("inventory value is redacted through a hardlink", func(t *testing.T) {
		out, runErr := read("alias-inventory.txt")
		if runErr != nil {
			t.Fatalf("hardlink alias to the inventory file was refused: %v", runErr)
		}
		if !strings.Contains(out, inventoryValue) {
			t.Fatalf("raw tool output did not contain the inventory value: %q", out)
		}
		if msg := modelInput(out, nil); strings.Contains(msg, inventoryValue) {
			t.Fatalf("exact inventory value reached the model through a hardlink: %q", msg)
		}
	})

	t.Run("contextual credential value is redacted through a hardlink", func(t *testing.T) {
		out, runErr := read("alias-contextual.txt")
		if runErr != nil {
			t.Fatalf("hardlink alias to the contextual file was refused: %v", runErr)
		}
		if !strings.Contains(out, contextualValue) {
			t.Fatalf("raw tool output did not contain the contextual value: %q", out)
		}
		if msg := modelInput(out, nil); strings.Contains(msg, contextualValue) {
			t.Fatalf("contextual credential value reached the model through a hardlink: %q", msg)
		}
	})

	t.Run("unlisted opaque value remains an observed residual", func(t *testing.T) {
		out, runErr := read("alias-unlisted.txt")
		if runErr != nil {
			t.Fatalf("hardlink alias to the unlisted file was refused: %v", runErr)
		}
		if !strings.Contains(out, unlistedValue) {
			t.Fatalf("raw tool output did not contain the unlisted value: %q", out)
		}
		msg := modelInput(out, nil)
		if !strings.Contains(msg, unlistedValue) {
			t.Fatalf("C1 now redacts an unlisted opaque hardlink value; update the hardlink evidence and policy claim: %q", msg)
		}
		t.Logf("observed bounded residual: unlisted opaque value reached the model input through a hardlink")
	})

	t.Run("positive controls remain usable", func(t *testing.T) {
		for _, path := range []string{"notes.txt", ".env.example", ".env.sample"} {
			out, runErr := read(path)
			if runErr != nil {
				t.Fatalf("positive control %s was refused: %v", path, runErr)
			}
			msg := modelInput(out, nil)
			if path == "notes.txt" {
				if !strings.Contains(msg, ordinaryMarker) {
					t.Fatalf("ordinary file content is missing from the model input: %q", msg)
				}
				continue
			}
			if !strings.Contains(msg, "replace-me") {
				t.Fatalf("placeholder from %s is missing from the model input: %q", path, msg)
			}
		}
	})

	t.Run("credential paths and symlinks stay denied", func(t *testing.T) {
		for _, path := range []string{".env", "env-symlink.txt"} {
			out, runErr := read(path)
			if !errors.Is(runErr, toolharness.ErrSensitivePath) {
				t.Fatalf("%s was not denied by the credential-path policy: out=%q err=%v", path, out, runErr)
			}
			if msg := modelInput(out, runErr); strings.Contains(msg, shapeValue) {
				t.Fatalf("denial message for %s exposed a canary: %q", path, msg)
			}
		}
	})

	t.Run("C1 disabled preserves the pre-C1 residual", func(t *testing.T) {
		out, runErr := read("alias-unlisted.txt")
		if runErr != nil {
			t.Fatalf("hardlink alias was refused: %v", runErr)
		}
		msg := toolResultMessageWithPolicy(toolharness.ToolReadFile, out, runErr, redact.Policy{})
		if !strings.Contains(msg, unlistedValue) {
			t.Fatalf("default policy unexpectedly redacted the unlisted value: %q", msg)
		}
		out, runErr = read("alias-inventory.txt")
		if runErr != nil {
			t.Fatalf("inventory hardlink was refused: %v", runErr)
		}
		msg = toolResultMessageWithPolicy(toolharness.ToolReadFile, out, runErr, redact.Policy{})
		if !strings.Contains(msg, inventoryValue) {
			t.Fatalf("default policy applied an inventory value: %q", msg)
		}

		out, runErr = read("alias-env.txt")
		if !errors.Is(runErr, toolharness.ErrSensitivePath) {
			t.Fatalf("identity denial must not depend on C1: out=%q err=%v", out, runErr)
		}
	})
}
