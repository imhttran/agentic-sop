package commandprofile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/decision"
)

// This file is the JEV-211 security and failure-path suite. It drives the REAL
// production connector against deterministic fake adapter subprocesses.

func req() decision.Request { return decision.Request{UseCase: "complexity", Subject: "x"} }

func TestAdapterSecurityExitCodes(t *testing.T) {
	cases := []struct {
		mode string
		want error
	}{
		{"invalid", decision.ErrInvalidResult},       // exit 2
		{"unavailable", decision.ErrProviderFailure}, // exit 3
		{"exit1", decision.ErrProviderFailure},       // exit 1
		{"exit7", decision.ErrProviderFailure},       // unknown code
	}
	for _, tc := range cases {
		c := testConnector(t, tc.mode, 10*time.Second)
		if _, err := c.Decide(context.Background(), req()); !errors.Is(err, tc.want) {
			t.Errorf("mode %q: err = %v, want %v", tc.mode, err, tc.want)
		}
	}
}

func TestAdapterSecurityMissingOrMalformed(t *testing.T) {
	for _, mode := range []string{"malformed", "empty", "unknownfield"} {
		c := testConnector(t, mode, 10*time.Second)
		if _, err := c.Decide(context.Background(), req()); err == nil {
			t.Errorf("mode %q: expected a fail-closed error", mode)
		}
	}
}

func TestAdapterSecurityUnsupportedCapability(t *testing.T) {
	// An empty answer set is an unsupported/indeterminate result, never a pass.
	c := testConnector(t, "success", 10*time.Second)
	// Feed a request whose kind has no profile -> Build rejects (unsupported).
	if _, err := c.Decide(context.Background(), decision.Request{UseCase: "no-such-kind"}); !errors.Is(err, decision.ErrUnsupportedCapability) {
		t.Fatalf("unsupported kind: err = %v, want ErrUnsupportedCapability", err)
	}
}

func TestAdapterSecurityProviderMismatchNoFallback(t *testing.T) {
	// A result naming an unrecognized provider is rejected; the connector never
	// falls back to another provider.
	c := testConnector(t, "wrongprovider", 10*time.Second)
	if _, err := c.Decide(context.Background(), req()); !errors.Is(err, decision.ErrInvalidResult) {
		t.Fatalf("provider mismatch: err = %v, want ErrInvalidResult", err)
	}
}

func TestAdapterSecurityAnswerIdentity(t *testing.T) {
	c := testConnector(t, "wrongid", 10*time.Second)
	if _, err := c.Decide(context.Background(), req()); !errors.Is(err, decision.ErrInvalidResult) {
		t.Fatalf("wrong answer id: err = %v, want ErrInvalidResult", err)
	}
}

func TestAdapterSecurityOutputLimit(t *testing.T) {
	c := testConnector(t, "oversize", 10*time.Second)
	if _, err := c.Decide(context.Background(), req()); !errors.Is(err, decision.ErrProviderFailure) {
		t.Fatalf("oversize: err = %v, want ErrProviderFailure", err)
	}
}

func TestAdapterCancellationVersusDeadline(t *testing.T) {
	// Deadline.
	ct := testConnector(t, "sleep", 250*time.Millisecond)
	if _, err := ct.Decide(context.Background(), req()); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("deadline: err = %v, want timed out", err)
	}
	// Cancellation (distinct classification).
	cc := testConnector(t, "sleep", 30*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cc.Decide(ctx, req()); err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("cancel: err = %v, want canceled", err)
	}
}

func TestAdapterTempFileCleanup(t *testing.T) {
	before := countAdapterTempDirs(t)
	c := testConnector(t, "success", 10*time.Second)
	if _, err := c.Decide(context.Background(), req()); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if after := countAdapterTempDirs(t); after != before {
		t.Fatalf("temporary request directories leaked: before=%d after=%d", before, after)
	}
}

func TestAdapterConnectorNoSecrets(t *testing.T) {
	c := testConnector(t, "unavailable", 10*time.Second)
	_, err := c.Decide(context.Background(), req())
	if err == nil {
		t.Fatal("expected an error")
	}
	low := strings.ToLower(err.Error())
	for _, bad := range []string{"api_key", "apikey", "bearer", "password", "secret", "token", "http://", "https://"} {
		if strings.Contains(low, bad) {
			t.Errorf("error text leaks %q: %q", bad, err.Error())
		}
	}
}

func TestAdapterConnectorKillsProcessGroup(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "descendant-marker")
	t.Setenv("ADAPTER_MARKER", marker)
	c := testConnector(t, "grandchild", 300*time.Millisecond)
	if _, err := c.Decide(context.Background(), req()); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want a timeout classification", err)
	}
	time.Sleep(1500 * time.Millisecond) // give a surviving descendant time to write
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("a descendant process survived the process-tree kill")
	}
}

func countAdapterTempDirs(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		t.Fatalf("read temp dir: %v", err)
	}
	n := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "sop-adapter-") {
			n++
		}
	}
	return n
}
