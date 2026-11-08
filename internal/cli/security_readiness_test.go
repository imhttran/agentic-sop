package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/security"
)

// readinessConfig loads the project config from dir, applying defaults.
func readinessConfig(t *testing.T, dir string) config.Config {
	t.Helper()
	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}

// TestSecurityReadinessDisabledIsNoOp asserts a disabled add-on runs no probe.
func TestSecurityReadinessDisabledIsNoOp(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "project:\n  name: x\n")
	cfg := readinessConfig(t, dir)
	if cfg.SecurityGateActive() {
		t.Fatal("disabled add-on reported an active gate")
	}
	var out bytes.Buffer
	if got := runSecurityReadiness(cfg, dir, &out); got != nil {
		t.Fatalf("disabled add-on produced a readiness result: %+v", got)
	}
	if out.Len() != 0 {
		t.Fatalf("disabled add-on wrote output: %q", out.String())
	}
}

// TestSecurityReadinessReady asserts a satisfiable required gate reports ready.
func TestSecurityReadinessReady(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package main\n")
	command, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	writeConfig(t, dir, fmt.Sprintf("project:\n  name: x\nsecurity:\n  enabled: true\n  mode: required\n  required_categories: [sast]\n  adapters:\n    - id: fixture-sast\n      category: sast\n      protocol: external\n      version: \"1.0.0\"\n      languages: [go]\n      command: [%q]\n", command))
	cfg := readinessConfig(t, dir)
	var out bytes.Buffer
	got := runSecurityReadiness(cfg, dir, &out)
	if got == nil || !got.Enabled || !got.Required {
		t.Fatalf("readiness = %+v, want enabled required", got)
	}
	if !got.Ready {
		t.Fatalf("readiness not ready: %v", got.Reasons)
	}
	if readinessBlocks(got) {
		t.Fatal("ready result blocked")
	}
}

// TestSecurityReadinessMissingToolFailsClosed asserts a required gate with an
// unavailable adapter is reported before IMPLEMENT with actionable text.
func TestSecurityReadinessMissingToolFailsClosed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package main\n")
	missing := filepath.Join(dir, "definitely-missing-tool-xyz")
	writeConfig(t, dir, fmt.Sprintf("project:\n  name: x\nsecurity:\n  enabled: true\n  mode: required\n  required_categories: [sast]\n  adapters:\n    - id: fixture-sast\n      category: sast\n      protocol: external\n      version: \"1.0.0\"\n      languages: [go]\n      command: [%q]\n", missing))
	cfg := readinessConfig(t, dir)
	var out bytes.Buffer
	got := runSecurityReadiness(cfg, dir, &out)
	if got == nil || got.Ready {
		t.Fatalf("readiness = %+v, want not ready", got)
	}
	if !readinessBlocks(got) {
		t.Fatal("missing required tool did not block")
	}
	if !strings.Contains(out.String(), "security readiness (required): not ready") {
		t.Fatalf("output = %q, want a readiness diagnostic", out.String())
	}
	if !strings.Contains(strings.Join(got.Reasons, ";"), "fixture-sast") {
		t.Fatalf("diagnostics = %v, want the adapter named", got.Reasons)
	}
	// The existing classifier turns this into the documented security failure.
	cls := failure.Classify(failure.Evidence{Source: "SECURITY", SecurityFailed: true, SecurityVerdict: "NOT_READY", SecurityReason: readinessReason(got)})
	if cls.Kind != failure.SecurityScanFailure || cls.Disposition != failure.Block {
		t.Fatalf("classification = %+v, want SECURITY_SCAN_FAILURE/BLOCK", cls)
	}
}

// TestSecurityReadinessMissingToolReportOnlyDoesNotBlock asserts a report-only
// task is never blocked by a missing advisory tool.
func TestSecurityReadinessMissingToolReportOnlyDoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package main\n")
	missing := filepath.Join(dir, "definitely-missing-tool-xyz")
	writeConfig(t, dir, fmt.Sprintf("project:\n  name: x\nsecurity:\n  enabled: true\n  mode: report-only\n  adapters:\n    - id: fixture-sast\n      category: sast\n      protocol: external\n      version: \"1.0.0\"\n      languages: [go]\n      command: [%q]\n", missing))
	cfg := readinessConfig(t, dir)
	var out bytes.Buffer
	got := runSecurityReadiness(cfg, dir, &out)
	if got == nil || got.Required {
		t.Fatalf("readiness = %+v, want report-only (not required)", got)
	}
	if readinessBlocks(got) {
		t.Fatal("report-only task was blocked by a missing advisory tool")
	}
}

// TestSecurityReadinessSingleDetectionPerCall asserts one readiness invocation
// performs exactly one content inspection. Classification reuses the detection
// result instead of walking the repository again.
func TestSecurityReadinessSingleDetectionPerCall(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package main\n")
	missing := filepath.Join(dir, "definitely-missing-tool-xyz")
	writeConfig(t, dir, fmt.Sprintf("project:\n  name: x\nsecurity:\n  enabled: true\n  mode: required\n  required_categories: [sast]\n  adapters:\n    - id: fixture-sast\n      category: sast\n      protocol: external\n      version: \"1.0.0\"\n      languages: [go]\n      command: [%q]\n", missing))
	cfg := readinessConfig(t, dir)

	original := detectRepositoryScoped
	t.Cleanup(func() { detectRepositoryScoped = original })
	detections := 0
	detectRepositoryScoped = func(root string, excludes []string) (security.Detection, error) {
		detections++
		return original(root, excludes)
	}

	var out bytes.Buffer
	if got := runSecurityReadiness(cfg, dir, &out); got == nil {
		t.Fatal("readiness probe returned no result")
	}
	if detections != 1 {
		t.Fatalf("repository detections = %d, want 1 per readiness call", detections)
	}
}

// TestSecurityReadinessReevaluatesEachCall asserts readiness is inspected on
// every call. A retained result would keep reporting a prerequisite failure that
// no longer holds, or hide one that appeared after an earlier check.
func TestSecurityReadinessReevaluatesEachCall(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package main\n")
	command := filepath.Join(dir, "fixture-sast")
	writeConfig(t, dir, fmt.Sprintf("project:\n  name: x\nsecurity:\n  enabled: true\n  mode: required\n  required_categories: [sast]\n  adapters:\n    - id: fixture-sast\n      category: sast\n      protocol: external\n      version: \"1.0.0\"\n      languages: [go]\n      command: [%q]\n", command))
	cfg := readinessConfig(t, dir)

	var out bytes.Buffer
	first := runSecurityReadiness(cfg, dir, &out)
	if first == nil || first.Ready {
		t.Fatalf("first readiness = %+v, want not ready while the adapter is absent", first)
	}

	// Make the adapter executable. The repository content identity and the
	// security configuration are unchanged, so only a fresh inspection can see it.
	if err := os.WriteFile(command, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	second := runSecurityReadiness(cfg, dir, &out)
	if second == nil || !second.Ready {
		t.Fatalf("second readiness = %+v, want ready after the adapter appeared", second)
	}
}
