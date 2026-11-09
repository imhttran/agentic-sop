package security

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
)

// textOnlyDetection builds a disposable repository with no Go/Rust manifest or
// source content. The synthetic secret string is a canary, never a real secret.
func textOnlyDetection(t *testing.T) Detection {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"README.md":   "# text only fixture\n",
		"config.yaml": "name: gitleaks-readiness-fixture\n",
		"secret.txt":  "synthetic_canary = \"sop-not-a-real-secret\"\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	d, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Languages) != 0 {
		t.Fatalf("languages = %v, want a text/config-only repository", d.Languages)
	}
	return d
}

func gitleaksSelected(selection Selection) bool {
	for _, adapter := range selection.Adapters {
		if adapter.Config.ID == AdapterIDGitleaks {
			return true
		}
	}
	return false
}

func TestGitleaksAutoSelectionWithoutDetectedLanguage(t *testing.T) {
	detection := textOnlyDetection(t)
	cfg := config.SecurityConfig{Enabled: boolPtr(true), Auto: true}
	selection := ResolveSelection(cfg, detection, true)
	if !gitleaksSelected(selection) {
		t.Fatalf("gitleaks was not auto-selected for a text/config-only repository: %v", selectedIDs(selection))
	}
	for _, adapter := range selection.Adapters {
		if adapter.Config.ID != AdapterIDGitleaks {
			continue
		}
		if !adapter.RepositoryWide || len(adapter.Languages) != 0 {
			t.Fatalf("gitleaks selection = %+v, want a repository-wide adapter with no language claim", adapter)
		}
	}
	var secrets *CategoryCoverage
	for i := range selection.Coverage {
		if selection.Coverage[i].Category == CategorySecrets {
			secrets = &selection.Coverage[i]
		}
	}
	if secrets == nil || secrets.State != CoverageStateCovered || !secrets.RepositoryWide {
		t.Fatalf("secrets coverage = %+v, want repository-wide covered", secrets)
	}
}

func TestGitleaksAutoAdapterConfigWithoutDetectedLanguage(t *testing.T) {
	detection := textOnlyDetection(t)
	adapters := AutoAdapterConfigs(detection, nil, "")
	ids := map[string]bool{}
	for _, adapter := range adapters {
		ids[adapter.ID] = true
	}
	if !ids[AdapterIDGitleaks] {
		t.Fatalf("auto adapter configs = %v, want gitleaks", ids)
	}
	if ids[AdapterIDSemgrep] || ids[AdapterIDCargoAudit] {
		t.Fatalf("auto adapter configs added language-scoped adapters without detected languages: %v", ids)
	}
}

func TestGitleaksReadinessCandidateWithoutDetectedLanguage(t *testing.T) {
	detection := textOnlyDetection(t)
	candidates := SelectCandidatesWithDatabases(detection, nil, "", "")
	if len(candidates) != 1 || candidates[0].ID != AdapterIDGitleaks || candidates[0].Status != CapabilityProposed {
		t.Fatalf("candidates = %+v, want only proposed gitleaks", candidates)
	}
}

func TestGitleaksCapabilityWithoutDetectedLanguage(t *testing.T) {
	detection := textOnlyDetection(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	available := ClassifyCapabilitiesWithDatabases(detection, []config.SecurityAdapterConfig{{
		ID: AdapterIDGitleaks, Category: config.SecurityCategorySecrets, Command: []string{exe},
	}}, "", "")
	if len(available) != 1 || available[0].Status != CapabilityAvailable {
		t.Fatalf("available classification = %+v, want available gitleaks", available)
	}
	unavailable := ClassifyCapabilitiesWithDatabases(detection, []config.SecurityAdapterConfig{{
		ID: AdapterIDGitleaks, Category: config.SecurityCategorySecrets, Command: []string{"sop-gitleaks-unavailable-canary"},
	}}, "", "")
	if len(unavailable) != 1 || unavailable[0].Status != CapabilityUnavailable {
		t.Fatalf("unavailable classification = %+v, want unavailable gitleaks (not unsupported)", unavailable)
	}
}

func TestGitleaksSelectionWithSupportedLanguageControl(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/control\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	detection, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if !detection.HasLanguage(LanguageGo) {
		t.Fatalf("control languages = %v, want go", detection.Languages)
	}
	cfg := config.SecurityConfig{Enabled: boolPtr(true), Auto: true}
	selection := ResolveSelection(cfg, detection, true)
	if !gitleaksSelected(selection) {
		t.Fatalf("gitleaks was not selected for the supported-language control: %v", selectedIDs(selection))
	}
}

func TestGitleaksFailureIsNotLanguageSkipped(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("text/config-only fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: AdapterIDGitleaks, Category: config.SecurityCategorySecrets, Protocol: config.SecurityProtocolBuiltin,
		Command: []string{"gitleaks"}, Version: "8.99.0",
	}, BuildOptions{
		Executor: fakeExecutor{run: func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
			return nil, []byte("synthetic scanner failure"), 2, errors.New("exit status 2")
		}},
		Timeout: time.Second, MaxOutputBytes: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusFailed {
		t.Fatalf("status = %s, want %s", exec.Status, StatusFailed)
	}
	if exec.NoApplicableContent {
		t.Fatal("a failing Gitleaks scan was treated as no-applicable-content")
	}
}
