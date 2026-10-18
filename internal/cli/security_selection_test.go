package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecurityScanGlobalDisableDoesNotBypassRequiredGate(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
project:
  name: x
security:
  enabled: false
  mode: required
  required_categories: [sast]
  adapters:
    - id: semgrep
      category: sast
      command: [semgrep]
`)
	code, stdout, stderr := runCLI(t, dir, "security", "scan", "--json", "--run-id", "disabled-required")
	if code == exitOK {
		t.Fatalf("globally disabled required gate exited OK: %s", stdout)
	}
	if strings.Contains(stdout, "NOT_APPLICABLE") {
		t.Fatalf("global disable bypassed the required gate: %s", stdout)
	}
	var out struct {
		Scan struct {
			Verdict securityVerdictView `json:"verdict"`
			Reasons []string            `json:"reasons"`
		} `json:"scan"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if string(out.Scan.Verdict) != "INDETERMINATE" {
		t.Fatalf("verdict = %s, want INDETERMINATE", out.Scan.Verdict)
	}
	if !containsReason(out.Scan.Reasons, "globally disabled") {
		t.Fatalf("reasons = %v, want a global-disable explanation", out.Scan.Reasons)
	}
	if !stateExists(filepath.Join(dir, stateDirName, "runs", "disabled-required", "security", "scan.json")) {
		t.Fatal("disabled required gate wrote no evidence")
	}
	if stderr != "" && strings.Contains(stderr, "exec") {
		t.Fatalf("unexpected execution error: %s", stderr)
	}
}

type securityVerdictView string

func TestSecurityScanDisabledAdapterWithEnabledReplacement(t *testing.T) {
	dir := t.TempDir()
	response := `{"protocol_version":"security-scan/v1","adapter_id":"deps-new","category":"dependencies","status":"COMPLETED","exit_code":0,"coverage":"COMPLETE"}`
	script := securityScript(t, dir, response)
	writeConfig(t, dir, `
project:
  name: x
security:
  enabled: true
  mode: report-only
  adapters:
    - id: deps-old
      category: dependencies
      protocol: external
      languages: [go]
      enabled: false
      version: "1.0.0"
      command: [/nonexistent/deps-old]
    - id: deps-new
      category: dependencies
      protocol: external
      languages: [go]
      version: "1.0.0"
      command: ["`+script+`"]
`)
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.21\n")

	code, stdout, stderr := runCLI(t, dir, "security", "scan", "--json", "--run-id", "replacement")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	var out struct {
		Scan struct {
			Verdict        string                  `json:"verdict"`
			Adapters       []securityExecutionView `json:"adapters"`
			CoverageReport []securityCoverageView  `json:"coverage_by_category"`
			Reasons        []string                `json:"reasons"`
		} `json:"scan"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	oldDisabled := false
	newCompleted := false
	for _, adapter := range out.Scan.Adapters {
		if adapter.AdapterID == "deps-old" {
			oldDisabled = adapter.Disabled && adapter.Status == "SKIPPED"
		}
		if adapter.AdapterID == "deps-new" {
			newCompleted = !adapter.Disabled && adapter.Status == "COMPLETED"
		}
	}
	if !oldDisabled || !newCompleted {
		t.Fatalf("adapters = %+v, want deps-old disabled and deps-new completed", out.Scan.Adapters)
	}
	if containsReason(out.Scan.Reasons, "disabled coverage") {
		t.Fatalf("an enabled replacement was treated as a coverage gap: %v", out.Scan.Reasons)
	}
	for _, category := range out.Scan.CoverageReport {
		if category.Category != "dependencies" {
			continue
		}
		for _, language := range category.Languages {
			if language.Language == "go" && language.State != "covered" {
				t.Fatalf("go coverage = %s, want covered", language.State)
			}
		}
	}
}

type securityExecutionView struct {
	AdapterID string `json:"adapter_id"`
	Status    string `json:"status"`
	Disabled  bool   `json:"disabled"`
}

type securityCoverageView struct {
	Category  string `json:"category"`
	State     string `json:"state"`
	Languages []struct {
		Language string `json:"language"`
		State    string `json:"state"`
	} `json:"languages"`
}

func TestSecurityInspectShowsSelectionDisableAndCoverage(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
project:
  name: x
security:
  enabled: true
  categories:
    dependencies: false
  adapters:
    - id: semgrep
      category: sast
      command: [semgrep]
    - id: gitleaks
      category: secrets
      command: [gitleaks]
    - id: cargo-audit
      category: dependencies
      command: [cargo, audit]
`)
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.21\n")
	writeFile(t, dir, "main.go", "package main\n")
	writeFile(t, dir, "Cargo.toml", "[package]\nname = \"app\"\nversion = \"0.1.0\"\n")
	writeFile(t, dir, "Cargo.lock", "version = 3\n\n[[package]]\nname = \"clean\"\nversion = \"1.0.0\"\n")

	code, stdout, stderr := runCLI(t, dir, "security", "inspect", "--json")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	var out struct {
		Selected []struct {
			ID string `json:"id"`
		} `json:"selected"`
		Disabled []struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		} `json:"disabled"`
		Coverage []securityCoverageView `json:"coverage"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	selected := map[string]bool{}
	for _, adapter := range out.Selected {
		selected[adapter.ID] = true
	}
	if !selected["semgrep"] || !selected["gitleaks"] || selected["cargo-audit"] {
		t.Fatalf("selected = %v, want semgrep and gitleaks only", selected)
	}
	if len(out.Disabled) != 1 || out.Disabled[0].ID != "cargo-audit" || !strings.Contains(out.Disabled[0].Reason, "category") {
		t.Fatalf("disabled = %+v, want cargo-audit with a category reason", out.Disabled)
	}
	found := false
	for _, category := range out.Coverage {
		if category.Category != "dependencies" {
			continue
		}
		found = category.State == "disabled"
	}
	if !found {
		t.Fatalf("coverage = %+v, want dependencies disabled", out.Coverage)
	}
}

func containsReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if strings.Contains(reason, want) {
			return true
		}
	}
	return false
}

func TestSecurityInspectExternalReplacementIsVisible(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
project:
  name: x
security:
  enabled: true
  adapters:
    - id: private-deps
      category: dependencies
      protocol: external
      languages: [go]
      command: [./private-deps]
`)
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.21\n")
	writeFile(t, dir, "private-deps", "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(dir, "private-deps"), 0o755); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI(t, dir, "security", "inspect", "--json")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	var out struct {
		Selected []struct {
			ID        string   `json:"id"`
			Languages []string `json:"languages"`
		} `json:"selected"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if len(out.Selected) != 1 || out.Selected[0].ID != "private-deps" {
		t.Fatalf("selected = %+v, want private-deps", out.Selected)
	}
	if len(out.Selected[0].Languages) == 0 {
		t.Fatal("external adapter language coverage was not shown")
	}
}
