package julia

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunnerFuncName(t *testing.T) {
	if got := (RunnerFunc{}).Name(); got != ProviderName {
		t.Errorf("Name() = %q, want %q", got, ProviderName)
	}
	if got := (RunnerFunc{NameValue: "custom"}).Name(); got != "custom" {
		t.Errorf("Name() = %q, want custom", got)
	}
}

func TestRunnerFuncAvailableAndRun(t *testing.T) {
	rf := RunnerFunc{
		AvailableValue: true,
		RunFunc: func(_ context.Context, in Inputs) (Outputs, error) {
			return Outputs{Model: in.State}, nil
		},
	}
	if !rf.Available(context.Background()) {
		t.Error("Available() = false, want true")
	}
	out, err := rf.Run(context.Background(), Inputs{State: "julia"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if out.Model != "julia" {
		t.Errorf("Model = %q, want julia", out.Model)
	}
}

func TestRunnerFuncNilRunFunc(t *testing.T) {
	rf := RunnerFunc{}
	if _, err := rf.Run(context.Background(), Inputs{}); err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
}

func TestCommandRunnerUnavailableWhenUnset(t *testing.T) {
	r := &CommandRunner{}
	if r.Available(context.Background()) {
		t.Error("Available() = true, want false when unset")
	}
	_, err := r.Run(context.Background(), Inputs{})
	if !errors.Is(err, errRunnerUnavailable) {
		t.Fatalf("error = %v, want errRunnerUnavailable", err)
	}
}

// TestCommandRunnerRoundTrip exercises the whole CommandRunner path with a real
// but trivial local command, which consumes stdin and emits valid Outputs JSON.
// No ONNX, Julia, or network is involved.
func TestCommandRunnerRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	r := &CommandRunner{
		Command: []string{"sh", "-c", `cat >/dev/null; printf '%s' '{"model":"julia","answers":{"risk":{"choice":"HIGH"}}}'`},
		Timeout: 5 * time.Second,
	}
	if !r.Available(context.Background()) {
		t.Fatal("Available() = false, want true")
	}
	in := Inputs{
		State: "julia state",
		Questions: []QuestionInput{
			{ID: "risk", Type: "choice", QType: juliaQTypeChoice, Text: "Assess risk.", Options: []string{"LOW", "HIGH"}},
		},
	}
	out, err := r.Run(context.Background(), in)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if out.Model != "julia" {
		t.Errorf("Model = %q, want julia", out.Model)
	}
	if out.Answers["risk"].Choice != "HIGH" {
		t.Errorf("risk = %+v", out.Answers["risk"])
	}
}

// TestCommandRunnerForwardsModelPath asserts JULIA_MODEL_PATH reaches the child
// environment.
func TestCommandRunnerForwardsModelPath(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	r := &CommandRunner{
		Command:   []string{"sh", "-c", `cat >/dev/null; printf '{"model":"%s","answers":{}}' "$JULIA_MODEL_PATH"`},
		ModelPath: "/models/julia.onnx",
		Timeout:   5 * time.Second,
	}
	out, err := r.Run(context.Background(), Inputs{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if out.Model != "/models/julia.onnx" {
		t.Errorf("Model = %q, want /models/julia.onnx", out.Model)
	}
}

func TestCommandRunnerFailingCommand(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	r := &CommandRunner{Command: []string{"sh", "-c", "exit 1"}, Timeout: 5 * time.Second}
	_, err := r.Run(context.Background(), Inputs{})
	if !errors.Is(err, errRunnerUnavailable) {
		t.Fatalf("error = %v, want errRunnerUnavailable", err)
	}
	if !isRunnerUnavailable(err) {
		t.Error("isRunnerUnavailable() = false, want true")
	}
}

func TestCommandRunnerGarbageOutput(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	r := &CommandRunner{Command: []string{"sh", "-c", "cat >/dev/null; printf 'not json'"}, Timeout: 5 * time.Second}
	_, err := r.Run(context.Background(), Inputs{})
	if !errors.Is(err, errRunnerMalformed) {
		t.Fatalf("error = %v, want errRunnerMalformed", err)
	}
	if !isRunnerMalformed(err) {
		t.Error("isRunnerMalformed() = false, want true")
	}
}

// TestCommandRunnerTimeout asserts Run returns promptly once the configured
// timeout elapses. It invokes a single process directly (no shell wrapper) so
// the context kill terminates exactly the process being waited on.
func TestCommandRunnerTimeout(t *testing.T) {
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep not available")
	}
	r := &CommandRunner{Command: []string{sleep, "5"}, Timeout: 100 * time.Millisecond}
	start := time.Now()
	_, runErr := r.Run(context.Background(), Inputs{})
	if !errors.Is(runErr, errRunnerUnavailable) {
		t.Fatalf("error = %v, want errRunnerUnavailable", runErr)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("Run() took %s, want prompt timeout", elapsed)
	}
}

// TestCommandRunnerHelperErrorClassification asserts the optional helper error
// envelope maps onto the right runner sentinel: "malformed" -> errRunnerMalformed,
// "failure" -> a generic error (adapter: ErrProviderFailure), and an unclassified
// non-zero exit -> errRunnerUnavailable.
func TestCommandRunnerHelperErrorClassification(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	tests := map[string]struct {
		script          string
		wantUnavailable bool
		wantMalformed   bool
		wantGeneric     bool
	}{
		"malformed envelope": {
			script:        `cat >/dev/null; printf '{"error":{"kind":"malformed","message":"bad logits"}}'; exit 1`,
			wantMalformed: true,
		},
		"failure envelope": {
			script:      `cat >/dev/null; printf '{"error":{"kind":"failure","message":"onnx boom"}}'; exit 1`,
			wantGeneric: true,
		},
		"unclassified exit": {
			script:          `cat >/dev/null; exit 1`,
			wantUnavailable: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := &CommandRunner{Command: []string{"sh", "-c", tt.script}, Timeout: 5 * time.Second}
			_, err := r.Run(context.Background(), Inputs{})
			if err == nil {
				t.Fatal("Run() error = nil, want error")
			}
			if got := errors.Is(err, errRunnerUnavailable); got != tt.wantUnavailable {
				t.Errorf("errRunnerUnavailable = %v, want %v (err=%v)", got, tt.wantUnavailable, err)
			}
			if got := errors.Is(err, errRunnerMalformed); got != tt.wantMalformed {
				t.Errorf("errRunnerMalformed = %v, want %v (err=%v)", got, tt.wantMalformed, err)
			}
			if tt.wantGeneric && (errors.Is(err, errRunnerUnavailable) || errors.Is(err, errRunnerMalformed)) {
				t.Errorf("err = %v, want a generic (unclassified) failure", err)
			}
		})
	}
}

// TestCommandRunnerAvailableChecks asserts Available verifies the program, the
// helper script, and the model path without ever running inference.
func TestCommandRunnerAvailableChecks(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}
	ctx := context.Background()

	if (&CommandRunner{Command: []string{"definitely-not-a-real-binary-xyz"}}).Available(ctx) {
		t.Error("Available() = true for an unresolvable program")
	}

	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")
	if (&CommandRunner{Command: []string{sh}, HelperPath: missing}).Available(ctx) {
		t.Error("Available() = true when the helper script is missing")
	}

	helper := filepath.Join(dir, "infer.py")
	if err := os.WriteFile(helper, []byte("# helper\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if (&CommandRunner{Command: []string{sh, helper}, HelperPath: helper}).Available(ctx) {
		t.Error("Available() = true when the model path is unset for the helper")
	}

	model := filepath.Join(dir, "julia.onnx")
	if err := os.WriteFile(model, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !(&CommandRunner{Command: []string{sh, helper}, HelperPath: helper, ModelPath: model}).Available(ctx) {
		t.Error("Available() = false when the helper and model both exist")
	}
}

// TestCommandRunnerIntegration is opt-in: it only runs when
// JULIA_INTEGRATION_TEST=1 and JULIA_INFERENCE_CMD is set. The default suite
// never reaches a live runtime.
func TestCommandRunnerIntegration(t *testing.T) {
	if os.Getenv("JULIA_INTEGRATION_TEST") != "1" {
		t.Skip("set JULIA_INTEGRATION_TEST=1 and JULIA_INFERENCE_CMD to run")
	}
	cmd := splitCommand(os.Getenv("JULIA_INFERENCE_CMD"))
	if len(cmd) == 0 {
		t.Skip("JULIA_INFERENCE_CMD is not set")
	}
	r := &CommandRunner{Command: cmd, ModelPath: os.Getenv("JULIA_MODEL_PATH"), Timeout: 30 * time.Second}
	if !r.Available(context.Background()) {
		t.Fatal("Available() = false, want true")
	}
	in := Inputs{State: "deployment risk", Questions: []QuestionInput{{ID: "risk", Type: "choice", QType: juliaQTypeChoice, Text: "Assess risk.", Options: []string{"LOW", "MEDIUM", "HIGH"}}}}
	out, err := r.Run(context.Background(), in)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if strings.TrimSpace(out.Model) == "" {
		t.Log("runner returned no model identifier")
	}
}
