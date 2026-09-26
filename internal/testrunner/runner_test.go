package testrunner

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunPassingCommand(t *testing.T) {
	res := New(t.TempDir()).Run(context.Background(), Check{Category: Build, Command: "echo hello"})

	if res.Status != Pass {
		t.Fatalf("status = %s, want PASS (stderr=%q)", res.Status, res.Stderr)
	}
	if res.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", res.ExitCode)
	}
	if !strings.Contains(res.Stdout, "hello") {
		t.Errorf("stdout = %q, want it to contain hello", res.Stdout)
	}
	if res.Category != Build || res.Command != "echo hello" {
		t.Errorf("result lost metadata: %+v", res)
	}
	if res.Duration < 0 {
		t.Errorf("duration = %v, want non-negative", res.Duration)
	}
}

func TestRunFailingCommand(t *testing.T) {
	res := New(t.TempDir()).Run(context.Background(), Check{Category: UnitTest, Command: "exit 3"})

	if res.Status != Fail {
		t.Fatalf("status = %s, want FAIL", res.Status)
	}
	if res.ExitCode != 3 {
		t.Errorf("exit code = %d, want 3", res.ExitCode)
	}
}

func TestRunCapturesStdoutAndStderr(t *testing.T) {
	res := New(t.TempDir()).Run(context.Background(), Check{Category: Lint, Command: "echo out; echo err >&2"})

	if res.Status != Pass {
		t.Fatalf("status = %s, want PASS", res.Status)
	}
	if !strings.Contains(res.Stdout, "out") || strings.Contains(res.Stdout, "err") {
		t.Errorf("stdout = %q, want only 'out'", res.Stdout)
	}
	if !strings.Contains(res.Stderr, "err") || strings.Contains(res.Stderr, "out") {
		t.Errorf("stderr = %q, want only 'err'", res.Stderr)
	}
}

func TestRunInvalidWorkingDirectory(t *testing.T) {
	res := New(filepath.Join(t.TempDir(), "missing")).Run(context.Background(), Check{Category: Build, Command: "echo hi"})

	if res.Status != Error {
		t.Fatalf("status = %s, want ERROR", res.Status)
	}
	if res.ExitCode != -1 {
		t.Errorf("exit code = %d, want -1", res.ExitCode)
	}
	if res.Stderr == "" {
		t.Error("expected a diagnostic on stderr")
	}
}

func TestRunContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res := New(t.TempDir()).Run(ctx, Check{Category: Build, Command: "echo hi"})
	if res.Status != Canceled {
		t.Errorf("status = %s, want CANCELED", res.Status)
	}
}

func TestRunTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	res := New(t.TempDir()).Run(ctx, Check{Category: UnitTest, Command: "sleep 5"})
	if res.Status != Canceled {
		t.Errorf("status = %s, want CANCELED", res.Status)
	}
}

func TestRunDurationPopulated(t *testing.T) {
	res := New(t.TempDir()).Run(context.Background(), Check{Category: Build, Command: "true"})
	if res.Status != Pass {
		t.Fatalf("status = %s, want PASS", res.Status)
	}
	if res.Duration < 0 {
		t.Errorf("duration = %v, want non-negative", res.Duration)
	}
}

func TestCommandsChecksOmitsUnconfigured(t *testing.T) {
	checks := Commands{Lint: "echo lint"}.Checks()
	if len(checks) != 1 {
		t.Fatalf("got %d checks, want 1: %+v", len(checks), checks)
	}
	if checks[0].Category != Lint || checks[0].Command != "echo lint" {
		t.Errorf("check = %+v, want LINT echo lint", checks[0])
	}
}

func TestCommandsChecksDeterministicOrder(t *testing.T) {
	checks := Commands{
		Build:           "echo build",
		UnitTest:        "echo unit",
		IntegrationTest: "echo integration",
		Lint:            "echo lint",
		DockerBuild:     "echo docker",
	}.Checks()

	want := []Category{Build, UnitTest, IntegrationTest, Lint, DockerBuild}
	if len(checks) != len(want) {
		t.Fatalf("got %d checks, want %d", len(checks), len(want))
	}
	for i, cat := range want {
		if checks[i].Category != cat {
			t.Errorf("check %d = %s, want %s", i, checks[i].Category, cat)
		}
	}
}

func TestRunAllOrderedSuite(t *testing.T) {
	runner := New(t.TempDir())
	suite := runner.RunAll(context.Background(), []Check{
		{Category: Build, Command: "echo build"},
		{Category: UnitTest, Command: "echo unit"},
		{Category: Lint, Command: "echo lint"},
	})

	if !suite.Passed() {
		t.Fatalf("suite status = %s, want PASS", suite.Status)
	}
	if len(suite.Results) != 3 {
		t.Fatalf("got %d results, want 3", len(suite.Results))
	}
	for i, cat := range []Category{Build, UnitTest, Lint} {
		if suite.Results[i].Category != cat {
			t.Errorf("result %d category = %s, want %s", i, suite.Results[i].Category, cat)
		}
	}
}

func TestRunAllFailsFast(t *testing.T) {
	dir := t.TempDir()
	runner := New(dir)
	suite := runner.RunAll(context.Background(), []Check{
		{Category: Build, Command: "true"},
		{Category: UnitTest, Command: "false"},
		{Category: Lint, Command: "touch lint-ran"},
	})

	if suite.Status != Fail {
		t.Errorf("suite status = %s, want FAIL", suite.Status)
	}
	if len(suite.Results) != 2 {
		t.Fatalf("got %d results, want 2 (lint must not run)", len(suite.Results))
	}
	if suite.Results[1].Category != UnitTest {
		t.Errorf("second result = %s, want UNIT_TEST", suite.Results[1].Category)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "lint-ran")); len(matches) != 0 {
		t.Error("lint command ran despite fail-fast")
	}
}

func TestRunEmptyCommandIsError(t *testing.T) {
	res := New(t.TempDir()).Run(context.Background(), Check{Category: Build, Command: "   "})
	if res.Status == Pass {
		t.Fatal("empty command must not be reported as a passing execution")
	}
	if res.Status != Error {
		t.Errorf("status = %s, want ERROR", res.Status)
	}
}

func TestCategoryNotInjectedIntoCommand(t *testing.T) {
	res := New(t.TempDir()).Run(context.Background(), Check{
		Category: Category("BUILD; echo pwned"),
		Command:  "echo ok",
	})

	if res.Status != Pass {
		t.Fatalf("status = %s, want PASS", res.Status)
	}
	if strings.Contains(res.Stdout, "pwned") {
		t.Errorf("category leaked into the command: %q", res.Stdout)
	}
	if strings.TrimSpace(res.Stdout) != "ok" {
		t.Errorf("stdout = %q, want ok", res.Stdout)
	}
}

func TestEnvironmentOverride(t *testing.T) {
	runner := &Runner{Dir: t.TempDir(), Env: map[string]string{"SOP_TEST_VAR": "bar"}}
	res := runner.Run(context.Background(), Check{Category: Build, Command: "echo $SOP_TEST_VAR"})

	if res.Status != Pass {
		t.Fatalf("status = %s, want PASS", res.Status)
	}
	if strings.TrimSpace(res.Stdout) != "bar" {
		t.Errorf("stdout = %q, want bar", res.Stdout)
	}
}
