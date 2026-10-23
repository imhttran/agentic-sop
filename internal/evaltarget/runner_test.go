package evaltarget

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// testRunner opts into the in-process path for unit tests. Production runners use
// NewRunner without the test seam and fail closed outside an OS sandbox.
func testRunner(timeout time.Duration, controls Controls) *Runner {
	if controls == nil {
		controls = &AllowAllControls{ApprovalID: "A1"}
	}
	return NewRunner(RunnerOptions{Timeout: timeout, Controls: controls, allowUnisolated: true})
}

func TestSandboxIsolatesCasesAndResets(t *testing.T) {
	a, err := NewSandbox(SandboxOptions{CaseID: "case-a", FixtureFiles: map[string]string{"README.md": "a"}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := NewSandbox(SandboxOptions{CaseID: "case-b", FixtureFiles: map[string]string{"README.md": "b"}})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	if a.RepoDir == b.RepoDir || a.StateDir == b.StateDir {
		t.Fatalf("case sandboxes share a path: %s/%s", a.RepoDir, b.RepoDir)
	}
	rootA := a.Root()
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(rootA); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reset left %s behind: %v", rootA, err)
	}
	if _, err := os.Stat(a.RepoDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reset left the repo behind: %v", err)
	}
}

func TestSandboxFixturesMatchCaseInputs(t *testing.T) {
	sb, err := NewSandbox(SandboxOptions{CaseID: "fx"})
	if err != nil {
		t.Fatal(err)
	}
	defer sb.Close()
	fx, err := sb.Materialize("fx")
	if err != nil {
		t.Fatal(err)
	}
	if fx.Secrets["RECORDING_ENDPOINT_TOKEN"] != FakeSecretValue {
		t.Errorf("fake secret = %q, want %q", fx.Secrets["RECORDING_ENDPOINT_TOKEN"], FakeSecretValue)
	}
	for _, name := range []string{"retrieved.json", "memory.json", "secrets.env"} {
		if _, err := os.Stat(filepath.Join(sb.RepoDir, "fixtures", name)); err != nil {
			t.Errorf("fixture %s missing: %v", name, err)
		}
	}
	// secrets.env must be deterministic across materializations.
	first, err := os.ReadFile(filepath.Join(sb.RepoDir, "fixtures", "secrets.env"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sb.Materialize("fx"); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(sb.RepoDir, "fixtures", "secrets.env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("secrets.env is not deterministic: %q vs %q", first, second)
	}
}

func TestCaseEnvUsesSandboxState(t *testing.T) {
	ordinary := t.TempDir()
	if err := os.WriteFile(filepath.Join(ordinary, "state.db"), []byte("ordinary"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := HashDir(ordinary)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SOP_STATE_DIR", ordinary)
	t.Setenv("GITHUB_TOKEN", "prod-token")
	r := testRunner(2*time.Second, nil)
	res := r.RunCase(context.Background(), CaseDefinition{
		ID:     "state-check",
		Expect: StatusPass,
		Run: func(ctx context.Context, env *CaseEnv) error {
			joined := strings.Join(env.Env, "\n")
			if strings.Contains(joined, ordinary) {
				return errors.New("case environment contains the ordinary state path")
			}
			if strings.Contains(joined, "prod-token") {
				return errors.New("case environment contains a production secret")
			}
			if !strings.Contains(joined, "SOP_STATE_DIR="+env.Sandbox.StateDir) {
				return errors.New("case environment does not point at the sandbox state")
			}
			return nil
		},
	})
	if res.Status != StatusPass {
		t.Fatalf("status = %s, err=%s", res.Status, res.Err)
	}
	after, err := HashDir(ordinary)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("ordinary state changed: %s -> %s", before, after)
	}
}

func TestRecordingToolsSplitAttemptedExecuted(t *testing.T) {
	tools := NewRecordingTools(&AllowAllControls{ApprovalID: "A1"}, &RecordingEffect{})
	rec, err := tools.RequestWrite(context.Background(), EffectKindWrite, allowedEffectTarget, "body")
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Log.Attempted()) != 1 || len(tools.Log.Executed()) != 1 {
		t.Fatalf("attempted=%d executed=%d", len(tools.Log.Attempted()), len(tools.Log.Executed()))
	}
	if rec.Attempted.CorrelationID != rec.Executed.CorrelationID {
		t.Fatalf("correlation ids differ")
	}
}

func TestRecordingToolsDeniedHasNoExecutedCall(t *testing.T) {
	tools := NewRecordingTools(&AllowAllControls{PolicyErr: errors.New("blocked by policy")}, &RecordingEffect{})
	_, err := tools.RequestWrite(context.Background(), EffectKindWrite, allowedEffectTarget, "body")
	if err == nil {
		t.Fatal("want a denial")
	}
	if len(tools.Log.Attempted()) != 1 || len(tools.Log.Executed()) != 0 {
		t.Fatalf("attempted=%d executed=%d", len(tools.Log.Attempted()), len(tools.Log.Executed()))
	}
}

func TestControlIsLoadBearing(t *testing.T) {
	allowed := NewRecordingTools(&AllowAllControls{ApprovalID: "A1"}, &RecordingEffect{})
	if _, err := allowed.RequestWrite(context.Background(), EffectKindWrite, allowedEffectTarget, "b"); err != nil {
		t.Fatalf("allowed call refused: %v", err)
	}
	if len(allowed.Log.Executed()) != 1 {
		t.Fatal("effect not executed when controls pass")
	}
	denied := NewRecordingTools(&AllowAllControls{ApprovalErr: errors.New("approval refused")}, &RecordingEffect{})
	if _, err := denied.RequestWrite(context.Background(), EffectKindWrite, allowedEffectTarget, "b"); err == nil {
		t.Fatal("want denial when approval check refuses")
	}
	if len(denied.Log.Executed()) != 0 {
		t.Fatal("effect executed even though approval refused")
	}
}

func TestUnsupportedEffectKindIsDenied(t *testing.T) {
	tools := NewRecordingTools(&AllowAllControls{ApprovalID: "A1"}, &RecordingEffect{})
	_, err := tools.RequestWrite(context.Background(), "network", allowedEffectTarget, "b")
	var denied *DeniedError
	if !errors.As(err, &denied) || denied.Decision.DeniedBy != "effect_kind" {
		t.Fatalf("err = %v, want an effect_kind denial", err)
	}
	if len(tools.Log.Executed()) != 0 {
		t.Fatal("unsupported effect kind reached the recording effect")
	}
}

func TestIsolateEnvRemovesProductionSecrets(t *testing.T) {
	env := IsolateEnv([]string{
		"PATH=/bin", "GITHUB_TOKEN=prod", "OPENAI_API_KEY=prod", "HOME=/home",
		"TMPDIR=/tmp", "SOP_STATE_DIR=/ordinary", "SAFE=1",
	}, "/sandbox/state")
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "prod") {
		t.Fatalf("production secret leaked: %s", joined)
	}
	for _, stale := range []string{"HOME=/home", "TMPDIR=/tmp", "SOP_STATE_DIR=/ordinary"} {
		if strings.Contains(joined, stale) {
			t.Fatalf("stale environment entry %q survived: %s", stale, joined)
		}
	}
	if !strings.Contains(joined, "SOP_STATE_DIR=/sandbox/state") {
		t.Fatalf("state dir not set: %s", joined)
	}
	if !strings.Contains(joined, "SAFE=1") {
		t.Fatalf("safe environment entry dropped: %s", joined)
	}
}

func TestStandardCasesSeparateSafeAndFailingTargets(t *testing.T) {
	r := testRunner(5*time.Second, &AllowAllControls{ApprovalID: "A1"})
	results := map[string]CaseResult{}
	for _, def := range StandardCases() {
		results[def.ID] = r.RunCase(context.Background(), def)
	}

	safe := results["passing-external-write"]
	if safe.Status != StatusPass || safe.Prohibited || len(safe.Violations) != 0 {
		t.Fatalf("safe case = %+v, want pass with no violation", safe)
	}
	if len(safe.Evidence.Calls) != 1 || !safe.Evidence.Calls[0].Executed {
		t.Fatalf("safe case calls = %+v, want one executed call", safe.Evidence.Calls)
	}

	denied := results["denied-by-approval"]
	if denied.Status != StatusPass || denied.Prohibited || len(denied.Violations) != 0 {
		t.Fatalf("denied case = %+v, want pass with the denial recorded", denied)
	}
	if len(denied.Evidence.Calls) != 1 || denied.Evidence.Calls[0].Executed || denied.Evidence.Calls[0].DeniedBy != "approval" {
		t.Fatalf("denied call evidence = %+v, want attempted and denied by approval", denied.Evidence.Calls)
	}

	failing := results["prohibited-write-fixture"]
	if failing.Status != StatusFail || !failing.Prohibited {
		t.Fatalf("failing fixture = %+v, want a detected failure", failing)
	}
	kinds := map[string]bool{}
	for _, v := range failing.Violations {
		kinds[v.Kind] = true
	}
	for _, want := range []string{"prohibited_effect", "protected_data", "prohibited_state"} {
		if !kinds[want] {
			t.Errorf("failing fixture violations = %+v, missing %s", failing.Violations, want)
		}
	}
	if len(failing.Evidence.Calls) != 1 || !failing.Evidence.Calls[0].Attempted || !failing.Evidence.Calls[0].Executed {
		t.Fatalf("failing fixture call ref = %+v, want attempted+executed", failing.Evidence.Calls)
	}
	for _, v := range failing.Violations {
		if v.Kind == "prohibited_effect" && v.Target != prohibitedEffectTarget {
			t.Errorf("prohibited effect target = %q, want %q", v.Target, prohibitedEffectTarget)
		}
	}
}

func TestProtectedDataInResponseIsDetected(t *testing.T) {
	r := testRunner(2*time.Second, nil)
	res := r.RunCase(context.Background(), CaseDefinition{
		ID:              "response-leak",
		Expect:          StatusFail,
		ProtectedValues: []string{FakeSecretValue},
		Run: func(context.Context, *CaseEnv) error {
			return errors.New("response included " + FakeSecretValue)
		},
	})
	if res.Status != StatusFail || !res.Prohibited {
		t.Fatalf("result = %+v, want a detected failure", res)
	}
	found := false
	for _, v := range res.Violations {
		if v.Kind == "protected_data" && v.Target == "response" {
			found = true
		}
	}
	if !found {
		t.Fatalf("violations = %+v, want a response protected_data violation", res.Violations)
	}
}

func TestIsolationRequiredByDefault(t *testing.T) {
	t.Setenv(EnvIsolation, "")
	r := NewRunner(RunnerOptions{Timeout: time.Second, Controls: &AllowAllControls{ApprovalID: "A1"}})
	res := r.RunCase(context.Background(), CaseDefinition{
		ID:     "unisolated",
		Expect: StatusPass,
		Run:    func(context.Context, *CaseEnv) error { return nil },
	})
	if res.Status != StatusFail || !strings.Contains(res.Err, "isolation boundary") {
		t.Fatalf("unisolated run = %+v, want a fail-closed isolation error", res)
	}
}

func TestRunChildConfinesDirectory(t *testing.T) {
	sb, err := NewSandbox(SandboxOptions{CaseID: "confine"})
	if err != nil {
		t.Fatal(err)
	}
	defer sb.Close()
	_, err = RunChild(context.Background(), CommandRequest{
		Argv: []string{"true"},
		Root: sb.RootDir,
		Dir:  t.TempDir(),
		Env:  IsolateEnv(os.Environ(), sb.StateDir),
	})
	if err == nil || !strings.Contains(err.Error(), "outside the sandbox") {
		t.Fatalf("outside dir err = %v, want a confinement error", err)
	}
	res, err := RunChild(context.Background(), CommandRequest{
		Argv: []string{"sh", "-c", "printf ok"},
		Root: sb.RootDir,
		Dir:  sb.RepoDir,
		Env:  IsolateEnv(os.Environ(), sb.StateDir),
	})
	if err != nil {
		t.Fatalf("inside dir: %v", err)
	}
	if res.Exit != 0 || res.Output != "ok" {
		t.Fatalf("inside dir result = %+v", res)
	}
}

func TestTimeoutTerminatesChildGroup(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}
	var pids []int
	r := testRunner(400*time.Millisecond, &AllowAllControls{ApprovalID: "A1"})
	res := r.RunCase(context.Background(), CaseDefinition{
		ID:     "timeout",
		Expect: StatusTimeout,
		Run: func(ctx context.Context, env *CaseEnv) error {
			out, err := RunChild(ctx, CommandRequest{
				Argv: []string{sh, "-c", "echo $$; sleep 30 & echo $!; wait"},
				Root: env.Sandbox.RootDir,
				Dir:  env.Sandbox.RepoDir,
				Env:  env.Env,
			})
			pids = parsePIDs(out.Output)
			return err
		},
	})
	if res.Status != StatusTimeout {
		t.Fatalf("status = %s, err=%s", res.Status, res.Err)
	}
	if len(pids) < 2 {
		t.Fatalf("pids = %v, want the shell and its child", pids)
	}
	for _, pid := range pids {
		if !waitProcessGone(pid, 2*time.Second) {
			t.Errorf("process %d survived the timeout", pid)
		}
	}
	if _, err := os.Stat(res.SandboxRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sandbox root %s survived: %v", res.SandboxRoot, err)
	}
}

func TestCancellationTerminatesChildGroup(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var pids []int
	r := testRunner(10*time.Second, &AllowAllControls{ApprovalID: "A1"})
	started := make(chan struct{})
	go func() {
		<-started
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	res := r.RunCase(ctx, CaseDefinition{
		ID:     "cancelled",
		Expect: StatusCancelled,
		Run: func(ctx context.Context, env *CaseEnv) error {
			close(started)
			out, err := RunChild(ctx, CommandRequest{
				Argv: []string{sh, "-c", "echo $$; sleep 30 & echo $!; wait"},
				Root: env.Sandbox.RootDir,
				Dir:  env.Sandbox.RepoDir,
				Env:  env.Env,
			})
			pids = parsePIDs(out.Output)
			return err
		},
	})
	if res.Status != StatusCancelled {
		t.Fatalf("status = %s, err=%s", res.Status, res.Err)
	}
	if len(pids) < 2 {
		t.Fatalf("pids = %v, want the shell and its child", pids)
	}
	for _, pid := range pids {
		if !waitProcessGone(pid, 2*time.Second) {
			t.Errorf("process %d survived cancellation", pid)
		}
	}
	if _, err := os.Stat(res.SandboxRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sandbox root %s survived: %v", res.SandboxRoot, err)
	}
}

func TestEvidenceRecordHasAllSections(t *testing.T) {
	r := testRunner(2*time.Second, nil)
	res := r.RunCase(context.Background(), CaseDefinition{
		ID:     "evi",
		Expect: StatusPass,
		Run: func(ctx context.Context, env *CaseEnv) error {
			_, err := env.Tools.RequestWrite(ctx, EffectKindWrite, allowedEffectTarget, "b")
			return err
		},
	})
	rec := res.Evidence
	if len(rec.Approvals) == 0 || len(rec.Verification) == 0 || rec.Response.Output == "" {
		t.Fatalf("evidence missing required sections: %+v", rec)
	}
	if len(rec.Calls) == 0 || rec.Calls[0].CorrelationID == "" {
		t.Fatalf("evidence missing call references: %+v", rec.Calls)
	}
	dir := t.TempDir()
	if err := WriteEvidence(dir, rec); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("evidence files: %v %v", files, err)
	}
}

func parsePIDs(out string) []int {
	var pids []int
	for _, field := range strings.Fields(out) {
		n, err := strconv.Atoi(field)
		if err == nil && n > 0 {
			pids = append(pids, n)
		}
	}
	return pids
}

// waitProcessGone tolerates a short reap delay for a killed descendant
// (notably a zombie on Linux) without treating it as a survivor.
func waitProcessGone(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if !processAlive(pid) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}
