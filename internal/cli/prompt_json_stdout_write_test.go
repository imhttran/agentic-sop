package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/github"
)

var errPromptStdoutWrite = errors.New("sentinel stdout write failure")

// failingWriter simulates a final stdout write that cannot deliver the JSON
// document. It records that the write was attempted so the test cannot pass by
// skipping the output entirely.
type failingWriter struct {
	err   error
	calls int
}

func (w *failingWriter) Write(p []byte) (int, error) {
	w.calls++
	return 0, w.err
}

func runCLIWithWriters(t *testing.T, dir string, a agent.Agent, stdout, stderr io.Writer, args ...string) int {
	t.Helper()
	d := deps{
		getwd:    func() (string, error) { return dir, nil },
		newAgent: func(_, _, _, _ string) (agent.Agent, error) { return a, nil },
		readDiff: func(context.Context, string) (string, error) { return "", nil },
		commit:   func(context.Context, string, string) error { return nil },
		newGitHub: func(string) github.Client {
			return &fakeGitHub{}
		},
	}
	return run(args, stdout, stderr, d)
}

// TestPromptJSONStdoutWriteFailureIsSurfaced is the regression: a successful
// read-only prompt whose final --json stdout write fails must exit nonzero and
// report the output-write failure on the separate stderr writer.
func TestPromptJSONStdoutWriteFailureIsSurfaced(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	a := &fakeCapabilityAgent{review: "the review"}
	stdout := &failingWriter{err: errPromptStdoutWrite}
	var stderr bytes.Buffer

	code := runCLIWithWriters(t, dir, a, stdout, &stderr, "prompt", "--capability", "review", "--json", "review this")
	if code == exitOK {
		t.Fatalf("a failed stdout write must exit nonzero")
	}
	if stdout.calls == 0 {
		t.Fatalf("the final stdout write was never attempted")
	}
	msg := stderr.String()
	if !strings.Contains(msg, errPromptStdoutWrite.Error()) {
		t.Errorf("stderr must surface the output-write error; got %q", msg)
	}
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "write") && !strings.Contains(lower, "output") {
		t.Errorf("stderr must identify the output-write failure; got %q", msg)
	}
}

// TestPromptJSONStdoutWriteSuccessControl is the control: a working stdout must
// receive valid success JSON and the command must still exit 0.
func TestPromptJSONStdoutWriteSuccessControl(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	a := &fakeCapabilityAgent{review: "the review"}
	var stdout, stderr bytes.Buffer

	code := runCLIWithWriters(t, dir, a, &stdout, &stderr, "prompt", "--capability", "review", "--json", "review this")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var doc promptResultDoc
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &doc); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if doc.Status != "completed" || doc.Result != "the review" {
		t.Errorf("doc = %+v", doc)
	}
	if doc.ResultPath == "" {
		t.Errorf("result_path is empty")
	}
}
