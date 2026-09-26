package review

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestOCRProviderParsesFindings(t *testing.T) {
	command := `printf '%s' '{"summary":"s","findings":[{"severity":"HIGH","title":"bug","file":"x.go","line":2}]}'`
	report, err := NewOCRProvider(command).Review(context.Background(), Request{Task: "T1"})
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}
	if len(report.Findings) != 1 || report.Findings[0].Severity != High {
		t.Fatalf("findings = %+v", report.Findings)
	}
	if !report.Blocking(High) {
		t.Error("HIGH finding should block at HIGH threshold")
	}
}

func TestOCRProviderRejectsMalformedOutput(t *testing.T) {
	if _, err := NewOCRProvider(`printf '%s' 'not json'`).Review(context.Background(), Request{}); err == nil {
		t.Fatal("expected error for malformed external output")
	}
}

func TestOCRProviderCommandFailure(t *testing.T) {
	_, err := NewOCRProvider("echo boom >&2; exit 3").Review(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected error for a failing command")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %q, want stderr diagnostics", err)
	}
}

func TestOCRProviderCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewOCRProvider("sleep 5").Review(ctx, Request{}); err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestProviderFromEnv(t *testing.T) {
	t.Setenv(EnvReviewCommand, "")
	if _, err := ProviderFromEnv(); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("err = %v, want ErrNotConfigured", err)
	}

	t.Setenv(EnvReviewCommand, `printf '%s' '{"summary":"","findings":[]}'`)
	provider, err := ProviderFromEnv()
	if err != nil {
		t.Fatalf("ProviderFromEnv failed: %v", err)
	}
	report, err := provider.Review(context.Background(), Request{})
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}
	if report.Blocking(Info) {
		t.Error("empty findings should not block")
	}
}

func TestOCRProviderSendsRequestOnStdin(t *testing.T) {
	// The command only emits a report when stdin contains the request's task
	// value, proving the request travels through stdin.
	command := `grep -q hello-task && printf '%s' '{"summary":"ok","findings":[]}'`
	report, err := NewOCRProvider(command).Review(context.Background(), Request{Task: "hello-task"})
	if err != nil {
		t.Fatalf("expected success when the request is on stdin: %v", err)
	}
	if report.Blocking(Info) {
		t.Error("expected no findings")
	}
}
