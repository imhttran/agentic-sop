package cli

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/taskfile"
	"github.com/imhttran/agentic-sop/internal/verify"
)

func boolPtr(b bool) *bool { return &b }

// fullValidationFixture is the standard configured set a project holds.
func fullValidationFixture() config.Validation {
	return config.Validation{Build: []string{"full-build"}, Test: []string{"full-test"}, Lint: []string{"full-lint"}}
}

// TestSelectValidationDisabledIsUnchanged proves risk-based verification is off by
// default: the full configured set runs exactly as before, for any execution mode.
func TestSelectValidationDisabledIsUnchanged(t *testing.T) {
	full := fullValidationFixture()
	cfg := config.Config{Validation: full}
	for _, spec := range []*taskfile.Spec{{}, {ExecutionMode: domain.ExecutionVerifyFirst}} {
		if got := selectValidation(cfg, spec); !reflect.DeepEqual(got, full) {
			t.Errorf("selectValidation(disabled) = %+v, want the full set %+v", got, full)
		}
	}
}

// TestSelectValidationLowUsesTargetedSet proves a verify-first (LOW) task runs the
// configured targeted set when risk-based verification is enabled.
func TestSelectValidationLowUsesTargetedSet(t *testing.T) {
	cfg := config.Config{
		Validation: fullValidationFixture(),
		Verification: config.Verification{
			RiskBased: boolPtr(true),
			Low:       config.VerificationLow{Build: []string{"low-build"}, Test: []string{"low-test"}},
		},
	}
	spec := &taskfile.Spec{ExecutionMode: domain.ExecutionVerifyFirst}
	want := config.Validation{Build: []string{"low-build"}, Test: []string{"low-test"}}
	if got := selectValidation(cfg, spec); !reflect.DeepEqual(got, want) {
		t.Errorf("selectValidation(LOW) = %+v, want the targeted set %+v", got, want)
	}
}

// TestSelectValidationLowWithoutTargetedFallsBackToFull proves a LOW tier never
// reduces verification to nothing when no targeted set is configured.
func TestSelectValidationLowWithoutTargetedFallsBackToFull(t *testing.T) {
	full := fullValidationFixture()
	cfg := config.Config{Validation: full, Verification: config.Verification{RiskBased: boolPtr(true)}}
	spec := &taskfile.Spec{ExecutionMode: domain.ExecutionVerifyFirst}
	if got := selectValidation(cfg, spec); !reflect.DeepEqual(got, full) {
		t.Errorf("selectValidation(LOW, no targeted set) = %+v, want the full set %+v", got, full)
	}
}

// TestSelectValidationMediumIsBuildAndTest proves an ordinary (MEDIUM) task runs the
// build+test set without lint when risk-based verification is enabled.
func TestSelectValidationMediumIsBuildAndTest(t *testing.T) {
	full := fullValidationFixture()
	cfg := config.Config{Validation: full, Verification: config.Verification{RiskBased: boolPtr(true)}}
	want := config.Validation{Build: full.Build, Test: full.Test}
	if got := selectValidation(cfg, &taskfile.Spec{}); !reflect.DeepEqual(got, want) {
		t.Errorf("selectValidation(MEDIUM) = %+v, want build+test %+v", got, want)
	}
}

// TestVerificationTierDisabledIsMedium proves the tier is the safe default when the
// feature is off.
func TestVerificationTierDisabledIsMedium(t *testing.T) {
	if got := verificationTier(config.Config{}, &taskfile.Spec{}); got != verify.Medium {
		t.Errorf("verificationTier(disabled) = %q, want %q", got, verify.Medium)
	}
}

// TestVerificationTierVerifyFirstIsLow proves a verify-first task derives LOW.
func TestVerificationTierVerifyFirstIsLow(t *testing.T) {
	cfg := config.Config{Verification: config.Verification{RiskBased: boolPtr(true)}}
	spec := &taskfile.Spec{ExecutionMode: domain.ExecutionVerifyFirst}
	if got := verificationTier(cfg, spec); got != verify.Low {
		t.Errorf("verificationTier(verify-first) = %q, want %q", got, verify.Low)
	}
}

// TestRiskBasedLowRunsTargetedSetWithoutAgent is the end-to-end proof: with risk-based
// verification enabled, a verify-first task's LOW tier runs the targeted set (which
// passes) instead of the full set (whose test would fail), so the task passes with no
// agent call and the report records the targeted command, not the failing one.
func TestRiskBasedLowRunsTargetedSetWithoutAgent(t *testing.T) {
	dir := t.TempDir()
	seedVerifyFirstTask(t, dir)
	writeConfig(t, dir, "project:\n  name: x\n"+
		"validation:\n  build:\n    - \"true\"\n  test:\n    - \"exit 1\"\n  lint:\n    - \"true\"\n"+
		"verification:\n  risk_based: true\n  low:\n    build:\n      - \"true\"\n    test:\n      - \"true\"\n")
	a := &countingAgent{}

	code, stdout, stderr := runInjectedCLI(t, dir, "", a, "run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if a.calls != 0 {
		t.Errorf("a passing LOW verify-first task must not invoke the agent; got %d call(s)", a.calls)
	}
	report, err := os.ReadFile(filepath.Join(dir, stateDirName, "runs", "S001", "report.json"))
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if strings.Contains(string(report), "exit 1") {
		t.Errorf("the failing full-set test ran; the LOW targeted set was not used:\n%s", report)
	}
}

// TestScopeValidation pins scoped test selection: only a fully covered change is
// narrowed, and every uncertain case runs the full test set.
func TestScopeValidation(t *testing.T) {
	full := fullValidationFixture()
	cfg := config.Config{Validation: full, Verification: config.Verification{Scoped: []config.ScopedRule{
		{Paths: []string{"crates/core/**"}, Test: []string{"test-core"}},
		{Paths: []string{"crates/cli/**", "crates/core/src/shared.rs"}, Test: []string{"test-cli", "test-core"}},
		{Paths: []string{"crates/**/generated.rs"}, Test: []string{"test-generated"}},
	}}}
	many := make([]string, runpkg.MaxTaskChangedFiles)
	cases := []struct {
		name    string
		changed []string
		tier    verify.Tier
		want    []string
	}{
		{"covered", []string{"crates/core/src/lib.rs"}, verify.Medium, []string{"test-core"}},
		{"union deduplicated", []string{"crates/core/src/shared.rs", "crates/cli/main.rs"}, verify.Medium, []string{"test-core", "test-cli"}},
		{"broad and interior-wildcard rules both apply", []string{"crates/core/src/generated.rs"}, verify.Medium, []string{"test-core", "test-generated"}},
		{"one uncovered", []string{"crates/core/src/lib.rs", "README.md"}, verify.Medium, full.Test},
		{"empty change set", nil, verify.Medium, full.Test},
		{"possibly truncated", many, verify.Medium, full.Test},
		{"high tier", []string{"crates/core/src/lib.rs"}, verify.High, full.Test},
		{"low tier keeps its own set", []string{"crates/core/src/lib.rs"}, verify.Low, full.Test},
	}
	for _, c := range cases {
		got := scopeValidation(cfg, full, c.tier, c.changed)
		if !reflect.DeepEqual(got.Test, c.want) || !reflect.DeepEqual(got.Build, full.Build) || !reflect.DeepEqual(got.Lint, full.Lint) {
			t.Errorf("%s: got %+v, want test %v with build/lint unchanged", c.name, got, c.want)
		}
	}
	if got := (&runSession{}).scope(cfg, full, verify.Medium, nil); !reflect.DeepEqual(got, full) {
		t.Errorf("unscoped session narrowed validation: %+v", got)
	}
}

// TestFinalValidationGate proves a plan with scoped rules cannot complete unless
// the full configured set passes, and that it is a no-op without scoped rules.
func TestFinalValidationGate(t *testing.T) {
	dir := t.TempDir()
	d := deps{readDiff: func(context.Context, string) (string, error) { return "diff", nil }}
	rules := config.Verification{Scoped: []config.ScopedRule{{Paths: []string{"**"}, Test: []string{"exit 0"}}}}
	var stderr strings.Builder

	failing := config.Config{Validation: config.Validation{Test: []string{"exit 1"}}, Verification: rules}
	if finalValidationGate(context.Background(), dir, failing, d, newRunSession(), &stderr) {
		t.Fatal("final gate passed with a failing full suite")
	}
	if !strings.Contains(stderr.String(), "exit 1") {
		t.Errorf("failure output does not name the failing command: %q", stderr.String())
	}
	passing := config.Config{Validation: config.Validation{Test: []string{"exit 0"}}, Verification: rules}
	if !finalValidationGate(context.Background(), dir, passing, d, newRunSession(), &stderr) {
		t.Fatal("final gate failed with a passing full suite")
	}
	failing.Verification = config.Verification{}
	if !finalValidationGate(context.Background(), dir, failing, d, newRunSession(), &stderr) {
		t.Fatal("final gate ran without scoped rules")
	}
}
