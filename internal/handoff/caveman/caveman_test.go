package caveman

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/handoff"
)

type fakeRunner struct {
	commands []string
	inputs   []string
	out      string
	err      error
}

func (r *fakeRunner) Run(_ context.Context, command, input string) (string, error) {
	r.commands = append(r.commands, command)
	r.inputs = append(r.inputs, input)
	return r.out, r.err
}

func TestName(t *testing.T) {
	if got := New("h", "c", &fakeRunner{}).Name(); got != "Caveman" {
		t.Errorf("Name() = %q, want Caveman", got)
	}
}

func TestCheck(t *testing.T) {
	if err := New("", "compress", &fakeRunner{}).Check(context.Background()); err == nil {
		t.Error("expected error when the health command is not configured")
	}
	if err := New("health", "compress", nil).Check(context.Background()); err == nil {
		t.Error("expected error when no runner is configured")
	}
	if err := New("health", "compress", &fakeRunner{err: errors.New("not ready")}).Check(context.Background()); err == nil {
		t.Error("expected error when the health command fails")
	}

	runner := &fakeRunner{}
	if err := New("health --version", "compress", runner).Check(context.Background()); err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if len(runner.commands) != 1 || runner.commands[0] != "health --version" {
		t.Errorf("commands = %v, want [health --version]", runner.commands)
	}
	if runner.inputs[0] != "" {
		t.Errorf("health check should send no input, got %q", runner.inputs[0])
	}
}

func TestCompressParsesJSONResponse(t *testing.T) {
	runner := &fakeRunner{out: `{"content":"small","references":[{"kind":"CI_LOG","locator":"run/1"}]}`}
	input := handoff.ContextBundle{
		Capsule:   handoff.Capsule{TaskID: "T1"},
		Artifacts: []handoff.Artifact{{Kind: handoff.ArtifactCILog, Content: "huge log"}},
	}

	got, err := New("health", "compress", runner).Compress(context.Background(), input)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}
	if got.Content != "small" || len(got.References) != 1 {
		t.Errorf("got %+v, want parsed JSON", got)
	}

	// The bundle is sent as JSON on stdin.
	var sent handoff.ContextBundle
	if err := json.Unmarshal([]byte(runner.inputs[0]), &sent); err != nil {
		t.Fatalf("input was not a JSON bundle: %v", err)
	}
	if sent.Capsule.TaskID != "T1" {
		t.Errorf("bundle capsule = %+v, want T1", sent.Capsule)
	}
}

func TestCompressAcceptsPlainText(t *testing.T) {
	runner := &fakeRunner{out: "  just text  \n"}
	got, err := New("health", "compress", runner).Compress(context.Background(), handoff.ContextBundle{})
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}
	if got.Content != "just text" {
		t.Errorf("content = %q, want trimmed plain text", got.Content)
	}
}

func TestCompressErrors(t *testing.T) {
	if _, err := New("health", "", &fakeRunner{}).Compress(context.Background(), handoff.ContextBundle{}); err == nil {
		t.Error("expected error when the compress command is not configured")
	}
	if _, err := New("health", "compress", nil).Compress(context.Background(), handoff.ContextBundle{}); err == nil {
		t.Error("expected error when no runner is configured")
	}
	if _, err := New("health", "compress", &fakeRunner{err: errors.New("boom")}).Compress(context.Background(), handoff.ContextBundle{}); err == nil {
		t.Error("expected runner error to propagate")
	}
}

func TestShellRunnerEchoesInput(t *testing.T) {
	out, err := ShellRunner{}.Run(context.Background(), "cat", "hello")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if strings.TrimSpace(out) != "hello" {
		t.Errorf("output = %q, want hello", out)
	}
}

func TestShellRunnerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (ShellRunner{}).Run(ctx, "cat", "hello"); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
