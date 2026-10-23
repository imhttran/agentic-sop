package hipcompare

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testGitRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	runTestGit(t, dir, "init")
	runTestGit(t, dir, "config", "user.email", "hipcompare@example.com")
	runTestGit(t, dir, "config", "user.name", "hipcompare test")
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, dir, "add", "base.txt")
	runTestGit(t, dir, "commit", "-m", "base")
	return dir, strings.TrimSpace(runTestGit(t, dir, "rev-parse", "HEAD"))
}

func runTestGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

type testWorktrees struct{ repo string }

func (w testWorktrees) Create(ctx context.Context, path, commit, branch string) error {
	return runGit(ctx, w.repo, "worktree", "add", "-b", branch, path, commit)
}

func (w testWorktrees) Remove(ctx context.Context, path string) error {
	return runGit(ctx, w.repo, "worktree", "remove", "--force", path)
}

type testExecutor struct {
	write  string
	result ExecutorResult
	err    error
}

func (e testExecutor) Execute(_ context.Context, in ExecutorInput) (ExecutorResult, error) {
	if e.err != nil {
		return e.result, e.err
	}
	if e.write != "" {
		if err := os.WriteFile(filepath.Join(in.Worktree, e.write), []byte("changed\n"), 0o644); err != nil {
			return e.result, err
		}
	}
	return e.result, nil
}

func testRunnerConfig(t *testing.T, repo, commit string, exec ArmExecutor, allowed []string) RunnerConfig {
	t.Helper()
	return RunnerConfig{
		Manifest: &TaskManifest{
			ID:                    "TEST-PAIR",
			TaskFile:              "TASK.md",
			VerificationCommands:  []string{"true"},
			AllowedCandidatePaths: allowed,
		},
		Baseline:        &InstructionSet{Name: "baseline"},
		Candidate:       &InstructionSet{Name: "candidate", Files: []InstructionFile{{Path: "prompt.md", Content: "candidate"}}},
		Budget:          Budget{Pairs: 1, HeldOutPairs: 0, MaxExecutions: 2, PerRunTimeSeconds: 5, PerRunTokenLimit: 1000, AutomaticRetries: 0},
		StartingCommit:  commit,
		Worktrees:       testWorktrees{repo: repo},
		WorktreeRoot:    t.TempDir(),
		Exec:            exec,
		Repository:      repo,
		Now:             time.Now,
		PriorExecutions: 0,
	}
}

func TestFingerprintIsDeterministicAndSensitive(t *testing.T) {
	base := FingerprintInputs{
		TaskInputs:           `{"id":"T"}`,
		StartingCommit:       "abc",
		InstructionHashes:    map[string]string{"baseline": "b", "candidate": "c"},
		Executor:             "sop",
		Model:                "model",
		Settings:             "temperature=0",
		ToolPermissions:      "read_file,write_file",
		VerificationCommands: []string{"go test ./..."},
	}
	one := DeriveFingerprint(base)
	two := DeriveFingerprint(base)
	if one.Digest != two.Digest {
		t.Fatalf("fingerprint is not deterministic: %s != %s", one.Digest, two.Digest)
	}

	mutations := map[string]func(*FingerprintInputs){
		"task_inputs":            func(in *FingerprintInputs) { in.TaskInputs = `{"id":"U"}` },
		"starting_commit":        func(in *FingerprintInputs) { in.StartingCommit = "def" },
		"instructions:candidate": func(in *FingerprintInputs) { in.InstructionHashes["candidate"] = "changed" },
		"executor":               func(in *FingerprintInputs) { in.Executor = "other" },
		"model":                  func(in *FingerprintInputs) { in.Model = "other" },
		"settings":               func(in *FingerprintInputs) { in.Settings = "temperature=1" },
		"tool_permissions":       func(in *FingerprintInputs) { in.ToolPermissions = "read_file" },
		"verification_commands":  func(in *FingerprintInputs) { in.VerificationCommands = []string{"true"} },
	}
	for name, mutate := range mutations {
		in := base
		in.InstructionHashes = map[string]string{"baseline": "b", "candidate": "c"}
		mutate(&in)
		if got := DeriveFingerprint(in); got.Digest == one.Digest {
			t.Errorf("changing %s did not change the digest", name)
		}
	}
}

func TestResumeRefusesMismatchedFingerprint(t *testing.T) {
	dir := t.TempDir()
	inputs := FingerprintInputs{
		TaskInputs:           "task",
		StartingCommit:       "abc",
		InstructionHashes:    map[string]string{"baseline": "b", "candidate": "c"},
		Executor:             "sop",
		Model:                "model",
		Settings:             "settings",
		ToolPermissions:      "read_file",
		VerificationCommands: []string{"true"},
	}
	want := DeriveFingerprint(inputs)
	cmp := Comparison{Version: SchemaVersion, TaskID: "T", Fingerprint: want, Executions: 2, Completed: true}
	if err := WriteArtifacts(dir, cmp); err != nil {
		t.Fatal(err)
	}
	dec, err := ResumeFingerprint(dir, want)
	if err != nil || !dec.Resumed {
		t.Fatalf("matching fingerprint did not resume: %+v %v", dec, err)
	}
	inputs.Model = "different"
	changed := DeriveFingerprint(inputs)
	dec, err = ResumeFingerprint(dir, changed)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Resumed || dec.DifferingComponent != "model" {
		t.Fatalf("mismatched fingerprint = %+v, want refusal on model", dec)
	}
}

func TestExecutionBudgetRefusesOverCeiling(t *testing.T) {
	b := DefaultBudget()
	if b.ExecutionCeiling() != 8 {
		t.Fatalf("ceiling = %d, want 8", b.ExecutionCeiling())
	}
	if err := enforceExecutionBudget(b, 6, 2); err != nil {
		t.Fatalf("6 prior + 2 planned should fit: %v", err)
	}
	err := enforceExecutionBudget(b, 7, 2)
	if err == nil {
		t.Fatal("7 prior + 2 planned exceeded the ceiling without an error")
	}
	var be *BudgetError
	if !errors.As(err, &be) {
		t.Fatalf("error = %v, want BudgetError", err)
	}
}

func TestRunRecordsCandidateTaskEdit(t *testing.T) {
	repo, commit := testGitRepo(t)
	cfg := testRunnerConfig(t, repo, commit, testExecutor{
		write:  "bad.txt",
		result: ExecutorResult{Completed: true, AcceptancePassed: true},
	}, []string{"prompt.md"})
	res, err := runArm(context.Background(), cfg, cfg.WorktreeRoot, DeriveFingerprint(FingerprintInputs{}), ArmCandidate, cfg.Candidate, true, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.Completed {
		t.Fatalf("result = %+v, want a completed candidate arm", res)
	}
	if len(res.ChangedPaths) != 1 || res.ChangedPaths[0] != "bad.txt" {
		t.Fatalf("changed paths = %v, want [bad.txt]", res.ChangedPaths)
	}
}

func TestRunAcceptsCandidateEditInsideAllowedPaths(t *testing.T) {
	repo, commit := testGitRepo(t)
	cfg := testRunnerConfig(t, repo, commit, testExecutor{
		write:  "prompt.md",
		result: ExecutorResult{Completed: true, AcceptancePassed: true},
	}, []string{"prompt.md"})
	res, err := runArm(context.Background(), cfg, cfg.WorktreeRoot, DeriveFingerprint(FingerprintInputs{}), ArmCandidate, cfg.Candidate, true, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.Completed || !res.AcceptancePassed {
		t.Fatalf("result = %+v, want a completed accepted arm", res)
	}
	if len(res.ChangedPaths) != 1 || res.ChangedPaths[0] != "prompt.md" {
		t.Fatalf("changed paths = %v, want [prompt.md]", res.ChangedPaths)
	}
}

func TestRunRecordsBaselineTaskEdit(t *testing.T) {
	repo, commit := testGitRepo(t)
	cfg := testRunnerConfig(t, repo, commit, testExecutor{
		write:  "prompt.md",
		result: ExecutorResult{Completed: true, AcceptancePassed: true},
	}, []string{"prompt.md"})
	res, err := runArm(context.Background(), cfg, cfg.WorktreeRoot, DeriveFingerprint(FingerprintInputs{}), ArmBaseline, cfg.Baseline, false, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.Completed {
		t.Fatalf("result = %+v, want a completed baseline arm", res)
	}
	if len(res.ChangedPaths) != 1 || res.ChangedPaths[0] != "prompt.md" {
		t.Fatalf("changed paths = %v, want [prompt.md]", res.ChangedPaths)
	}
}

func TestRunRecordsTimeoutAsLimitFailure(t *testing.T) {
	repo, commit := testGitRepo(t)
	cfg := testRunnerConfig(t, repo, commit, testExecutor{
		err:    context.DeadlineExceeded,
		result: ExecutorResult{Failures: []string{"timed out"}},
	}, []string{"prompt.md"})
	res, err := runArm(context.Background(), cfg, cfg.WorktreeRoot, DeriveFingerprint(FingerprintInputs{}), ArmCandidate, cfg.Candidate, true, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.LimitFailure || res.Completed {
		t.Fatalf("result = %+v, want a timed-out limit failure", res)
	}
}

func TestDeterministicFixtureWritesBothReportsWithoutModelCalls(t *testing.T) {
	dir := t.TempDir()
	cmp := Comparison{
		Version:        SchemaVersion,
		TaskID:         "FIXTURE",
		StartingCommit: "0123456789abcdef",
		Fingerprint:    DeriveFingerprint(FingerprintInputs{TaskInputs: "fixture", StartingCommit: "0123456789abcdef"}),
		Arms: []ArmResult{
			{Arm: ArmBaseline, Completed: true, AcceptancePassed: true, ElapsedMillis: 12, TokensUsed: 100},
			{Arm: ArmCandidate, Completed: true, AcceptancePassed: true, ElapsedMillis: 10, TokensUsed: 90},
		},
		Budget:     DefaultBudget(),
		Executions: 2,
		Completed:  true,
	}
	if err := WriteArtifacts(dir, cmp); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ResultsFileName))
	if err != nil {
		t.Fatal(err)
	}
	var got Comparison
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode results: %v", err)
	}
	if !got.Completed || got.Executions != 2 || len(got.Arms) != 2 {
		t.Fatalf("results = %+v", got)
	}
	md, err := os.ReadFile(filepath.Join(dir, ComparisonMarkdownFileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# HIP-003 comparison: FIXTURE", "| Completed |", "| Acceptance |"} {
		if !strings.Contains(string(md), want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestRedactionRemovesCredentialValues(t *testing.T) {
	t.Setenv("SOP_TOKEN", "super-secret-value")
	got := redactText("token=abc SOP_TOKEN=super-secret-value")
	if strings.Contains(got, "super-secret-value") {
		t.Fatalf("redaction left the secret: %q", got)
	}
}

func TestCommandExecutorAppliesAllowedInstructionOnly(t *testing.T) {
	worktree := t.TempDir()
	exec := commandExecutor{}
	res, err := exec.Execute(context.Background(), ExecutorInput{
		Manifest:             &TaskManifest{ID: "TEST", TaskFile: "TASK.md"},
		Worktree:             worktree,
		AllowedPaths:         []string{"prompts/sop.md"},
		VerificationCommands: []string{"true"},
		Instructions:         InstructionSet{Name: "candidate", Files: []InstructionFile{{Path: "prompts/sop.md", Content: "candidate text"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Completed || !res.AcceptancePassed {
		t.Fatalf("result = %+v, want completed and accepted", res)
	}
	data, err := os.ReadFile(filepath.Join(worktree, "prompts", "sop.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "candidate text" {
		t.Fatalf("instruction content = %q", data)
	}
}

func TestCommandExecutorRejectsDisallowedInstruction(t *testing.T) {
	worktree := t.TempDir()
	_, err := (commandExecutor{}).Execute(context.Background(), ExecutorInput{
		Manifest:             &TaskManifest{ID: "TEST", TaskFile: "TASK.md"},
		Worktree:             worktree,
		AllowedPaths:         []string{"prompts/sop.md"},
		VerificationCommands: []string{"true"},
		Instructions:         InstructionSet{Name: "candidate", Files: []InstructionFile{{Path: "internal/cli/cli.go", Content: "tampered"}}},
	})
	var de *DisallowedEditError
	if !errors.As(err, &de) {
		t.Fatalf("error = %v, want DisallowedEditError", err)
	}
}
