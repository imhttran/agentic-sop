package cli

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/ragclient"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// trueBin returns the absolute path of the platform's `true` executable
// (/bin/true on Linux, /usr/bin/true on macOS), used as a stand-in producer.
func trueBin(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("true")
	if err != nil {
		t.Skipf("no true executable on PATH: %v", err)
	}
	return p
}

func ragEnabledEnv(t *testing.T) {
	t.Helper()
	t.Setenv(config.EnvRAGEnabled, "true")
	t.Setenv(config.EnvRAGExecutable, trueBin(t))
	t.Setenv(config.EnvRAGCorpus, "kb")
	t.Setenv(config.EnvRAGAllowedCorpora, "kb")
}

func TestRetrieveAttemptEvidenceInjected(t *testing.T) {
	ragEnabledEnv(t)
	want := []ragclient.Evidence{{Corpus: "kb", Source: "docs/a.md", Text: "e"}}
	d := deps{rawEvidence: func(context.Context, *taskfile.Spec, string) ([]ragclient.Evidence, error) { return want, nil }}

	got := retrieveAttemptEvidence(context.Background(), config.Config{}, &taskfile.Spec{ID: "T1", Title: "t"}, d)
	if !got.Attempted || got.Err != nil || got.Corpus != "kb" || len(got.Evidence) != 1 {
		t.Fatalf("attempt = %+v", got)
	}
}

func TestRetrieveAttemptEvidenceError(t *testing.T) {
	ragEnabledEnv(t)
	d := deps{rawEvidence: func(context.Context, *taskfile.Spec, string) ([]ragclient.Evidence, error) {
		return nil, errors.New("producer unavailable")
	}}
	got := retrieveAttemptEvidence(context.Background(), config.Config{}, &taskfile.Spec{ID: "T1"}, d)
	if !got.Attempted || got.Err == nil {
		t.Fatalf("attempt = %+v, want attempted with error", got)
	}
}

func TestRetrieveAttemptEvidenceDisabledDoesNotRetrieve(t *testing.T) {
	t.Setenv(config.EnvRAGEnabled, "false")
	called := false
	d := deps{rawEvidence: func(context.Context, *taskfile.Spec, string) ([]ragclient.Evidence, error) {
		called = true
		return nil, nil
	}}
	got := retrieveAttemptEvidence(context.Background(), config.Config{}, &taskfile.Spec{ID: "T1"}, d)
	if got.Attempted || called {
		t.Fatalf("disabled RAG retrieved: attempt=%+v called=%v", got, called)
	}
}

func TestRetrieveAttemptEvidenceUnauthorizedCorpus(t *testing.T) {
	t.Setenv(config.EnvRAGEnabled, "true")
	t.Setenv(config.EnvRAGExecutable, trueBin(t))
	t.Setenv(config.EnvRAGCorpus, "kb")
	t.Setenv(config.EnvRAGAllowedCorpora, "other") // corpus not in the allowlist
	called := false
	d := deps{rawEvidence: func(context.Context, *taskfile.Spec, string) ([]ragclient.Evidence, error) {
		called = true
		return nil, nil
	}}
	got := retrieveAttemptEvidence(context.Background(), config.Config{}, &taskfile.Spec{ID: "T1"}, d)
	if got.Attempted || called {
		t.Fatalf("unauthorized corpus retrieved: attempt=%+v called=%v", got, called)
	}
}

func TestRetrieveAttemptEvidenceCancellation(t *testing.T) {
	ragEnabledEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	d := deps{rawEvidence: func(ctx context.Context, _ *taskfile.Spec, _ string) ([]ragclient.Evidence, error) {
		cancel()
		return nil, ctx.Err()
	}}
	got := retrieveAttemptEvidence(ctx, config.Config{}, &taskfile.Spec{ID: "T1"}, d)
	if !got.Attempted || !errors.Is(got.Err, context.Canceled) {
		t.Fatalf("attempt = %+v, want attempted with context.Canceled", got)
	}
}
