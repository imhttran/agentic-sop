package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
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
