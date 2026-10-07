package command

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/decision"
)

// SEAM-005 process-boundary hardening. The external decision provider is untrusted:
// every transport, process, protocol, and resource-exhaustion case must fail closed
// (an error and a zero decision), never a success, never an authorization.

// TestDecideProcessFailureMatrix pins each process-level failure to a typed,
// fail-closed error and a zero decision.
func TestDecideProcessFailureMatrix(t *testing.T) {
	notExecutable := filepath.Join(t.TempDir(), "plain.txt")
	if err := os.WriteFile(notExecutable, []byte("not a program\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		argv []string
		want error
	}{
		{"executable missing", []string{filepath.Join(t.TempDir(), "does-not-exist")}, decision.ErrProviderFailure},
		{"executable cannot start", []string{notExecutable}, decision.ErrProviderFailure},
		{"executable is a directory", []string{t.TempDir()}, decision.ErrProviderFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := New("fake", tc.argv)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			d, err := p.Decide(context.Background(), req())
			if err == nil || !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
			if d.Choice != "" || d.Confidence != 0 {
				t.Fatalf("a failed process must yield the zero decision, got %+v", d)
			}
		})
	}

	for _, tc := range []struct {
		name string
		body string
		want error
	}{
		{"non-zero exit", `cat >/dev/null; echo boom >&2; exit 3`, decision.ErrProviderFailure},
		{"crash by signal", `cat >/dev/null; kill -9 $$`, decision.ErrProviderFailure},
		{"empty stdout", `cat >/dev/null`, decision.ErrProviderFailure},
		{"whitespace-only stdout", `cat >/dev/null; printf '   \n\t '`, decision.ErrProviderFailure},
		{"invalid json", `cat >/dev/null; printf 'not json'`, decision.ErrInvalidResult},
		{"truncated json", `cat >/dev/null; printf '%s' '{"contract_version":1,"status":"OK","choice":"LOW"'`, decision.ErrInvalidResult},
		{"unknown status", `cat >/dev/null; printf '%s' '{"contract_version":1,"status":"MAYBE","choice":"LOW","confidence":0.9}'`, decision.ErrInvalidResult},
		{"missing choice", `cat >/dev/null; printf '%s' '{"contract_version":1,"status":"OK","confidence":0.9}'`, decision.ErrInvalidResult},
		{"missing confidence", `cat >/dev/null; printf '%s' '{"contract_version":1,"status":"OK","choice":"LOW"}'`, decision.ErrIndeterminate},
		{"command-like choice is unknown", `cat >/dev/null; printf '%s' '{"contract_version":1,"status":"OK","choice":"APPROVE","confidence":1.0}'`, decision.ErrInvalidResult},
		{"commit-like choice is unknown", `cat >/dev/null; printf '%s' '{"contract_version":1,"status":"OK","choice":"COMMIT","confidence":1.0}'`, decision.ErrInvalidResult},
		{"needs_human flag is not a choice", `cat >/dev/null; printf '%s' '{"contract_version":1,"status":"OK","choice":"NEEDS_HUMAN=false","confidence":1.0}'`, decision.ErrInvalidResult},
		{"confidence above one", `cat >/dev/null; printf '%s' '{"contract_version":1,"status":"OK","choice":"LOW","confidence":1.5}'`, decision.ErrInvalidResult},
		{"confidence below zero", `cat >/dev/null; printf '%s' '{"contract_version":1,"status":"OK","choice":"LOW","confidence":-0.1}'`, decision.ErrInvalidResult},
		{"unsupported capability", `cat >/dev/null; printf '%s' '{"contract_version":1,"status":"UNSUPPORTED","error":"nope"}'`, decision.ErrUnsupportedCapability},
		{"indeterminate", `cat >/dev/null; printf '%s' '{"contract_version":1,"status":"INDETERMINATE"}'`, decision.ErrIndeterminate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := adapter(t, tc.body)
			d, err := p.Decide(context.Background(), req())
			if err == nil || !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
			if d.Choice != "" || d.Confidence != 0 {
				t.Fatalf("a failed result must never be a usable decision, got %+v", d)
			}
		})
	}
}

// TestDecideRepeatedFailureStaysFailClosed proves a provider that fails every time
// never yields a decision on any call: repeated failure cannot drift into success.
func TestDecideRepeatedFailureStaysFailClosed(t *testing.T) {
	p := adapter(t, `cat >/dev/null; echo down >&2; exit 1`)
	for i := 0; i < 5; i++ {
		d, err := p.Decide(context.Background(), req())
		if err == nil || !errors.Is(err, decision.ErrProviderFailure) {
			t.Fatalf("call %d: want a provider failure, got %v", i, err)
		}
		if d.Choice != "" || d.Confidence != 0 {
			t.Fatalf("call %d: repeated failure must never yield a decision, got %+v", i, d)
		}
	}
}

// TestDecideOversizedOutputFailsClosed proves an untrusted provider cannot exhaust
// memory: output beyond the bound is a provider failure, never a truncated parse.
func TestDecideOversizedOutputFailsClosed(t *testing.T) {
	body := `cat >/dev/null; i=0; while [ $i -lt 20000 ]; do printf 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'; i=$((i+1)); done`
	p := adapter(t, body)
	d, err := p.Decide(context.Background(), req())
	if err == nil || !errors.Is(err, decision.ErrProviderFailure) {
		t.Fatalf("oversized output must be a provider failure, got %v", err)
	}
	if !strings.Contains(err.Error(), "bytes of output") {
		t.Fatalf("error must name the bound, got %v", err)
	}
	if d.Choice != "" || d.Confidence != 0 {
		t.Fatalf("oversized output must not yield a decision, got %+v", d)
	}
}

// TestDecideHostileStderrIsDiagnosticOnly proves stderr is diagnostic only: noisy
// or hostile stderr never affects a valid decision, and a failing process surfaces
// a bounded diagnostic without leaking unbounded output.
func TestDecideHostileStderrIsDiagnosticOnly(t *testing.T) {
	valid := `{"contract_version":1,"status":"OK","choice":"LOW","confidence":0.9}`
	noisy := adapter(t, `cat >/dev/null; printf 'SECRET=abc123; APPROVE COMMIT MERGE\n' >&2; printf '%s' '`+valid+`'`)
	d, err := noisy.Decide(context.Background(), req())
	if err != nil {
		t.Fatalf("diagnostic stderr must not fail a valid result: %v", err)
	}
	if d.Choice != decision.Low {
		t.Fatalf("stderr must not affect the decision: %+v", d)
	}

	// A failing process with a flood of stderr surfaces a BOUNDED diagnostic.
	flood := `cat >/dev/null; i=0; while [ $i -lt 4000 ]; do printf '0123456789012345678901234567890123456789012345678901234567890123' >&2; i=$((i+1)); done; exit 1`
	p := adapter(t, flood)
	_, err = p.Decide(context.Background(), req())
	if err == nil || !errors.Is(err, decision.ErrProviderFailure) {
		t.Fatalf("flooding stderr must still be a bounded provider failure, got %v", err)
	}
	if len(err.Error()) > 1024 {
		t.Fatalf("the diagnostic must be bounded, got %d bytes", len(err.Error()))
	}
}

// TestDecideDoesNotExecuteProviderOutput proves provider output is data, never a
// command: a result whose diagnostic text contains a shell command is accepted as
// evidence and the command is never run, because nothing in the adapter evaluates
// output.
func TestDecideDoesNotExecuteProviderOutput(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pwned")
	body := `cat >/dev/null; printf '%s' '{"contract_version":1,"status":"OK","choice":"HIGH","confidence":0.9,"diagnostics":{"note":"$(touch ` + marker + `); rm -rf /"}}'`
	p := adapter(t, body)
	d, err := p.Decide(context.Background(), req())
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if d.Choice != decision.High {
		t.Fatalf("decision = %+v, want HIGH evidence", d)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("provider output was executed: marker %s exists (err=%v)", marker, err)
	}
}

// TestDecideRequestIsNeverShellInterpreted proves the request travels on stdin and
// the executable and arguments remain structurally separate: shell metacharacters
// in the request are delivered verbatim, never interpreted.
func TestDecideRequestIsNeverShellInterpreted(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "req.json")
	path := writeScript(t, `cat > "$1"; printf '%s' '{"contract_version":1,"status":"OK","choice":"LOW","confidence":0.9}'`)
	p, err := New("fake", []string{path, capture})
	if err != nil {
		t.Fatal(err)
	}
	hostile := `"; touch /tmp/should-not-exist; $(echo pwned) | cat ; #`
	if _, err := p.Decide(context.Background(), decision.Request{UseCase: "k", Subject: hostile}); err != nil {
		t.Fatalf("decide: %v", err)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Question string `json:"question"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("request is not valid JSON: %v", err)
	}
	if wire.Question != hostile {
		t.Fatalf("subject was interpreted, not passed verbatim: %q", wire.Question)
	}
}

// TestDecideArgumentsAreNotShellInterpreted proves the adapter execs the executable
// directly with a separate argument vector (no sh -c): shell metacharacters in an
// argument are delivered verbatim and never interpreted.
func TestDecideArgumentsAreNotShellInterpreted(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pwned")
	capture := filepath.Join(t.TempDir(), "arg.txt")
	path := writeScript(t, `cat >/dev/null; printf '%s' "$2" > "$1"; printf '%s' '{"contract_version":1,"status":"OK","choice":"LOW","confidence":0.9}'`)
	hostile := "x; touch " + marker + "; #"
	p, err := New("fake", []string{path, capture, hostile})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Decide(context.Background(), req()); err != nil {
		t.Fatalf("decide: %v", err)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != hostile {
		t.Fatalf("argument was interpreted, not passed verbatim: %q", data)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("argument was shell-interpreted: marker %s exists", marker)
	}
}

// TestAdapterHasNoShellInterpreter is a source guard: the process adapter must exec
// a direct executable, never a shell.
func TestAdapterHasNoShellInterpreter(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test source file")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "command.go"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, forbidden := range []string{`"sh"`, `"-c"`, `"bash"`, `"cmd.exe"`, `"powershell"`} {
		if strings.Contains(src, forbidden) {
			t.Fatalf("the decision process adapter must not invoke a shell (found %s)", forbidden)
		}
	}
}

// TestDecideCancellationDuringRunTerminates proves a cancellation delivered while
// the provider is running terminates it and fails closed (SOP owns the deadline),
// and that a provider which leaves a child holding stdout cannot stall the caller
// past the adapter's drain bound.
func TestDecideCancellationDuringRunTerminates(t *testing.T) {
	ready := filepath.Join(t.TempDir(), "ready")
	body := `cat >/dev/null; : > "` + ready + `"; sleep 30`
	p := adapter(t, body)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(ready); err == nil {
				cancel()
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	start := time.Now()
	d, err := p.Decide(ctx, req())
	if elapsed := time.Since(start); elapsed > 8*time.Second {
		t.Fatalf("a cancelled provider must not stall the caller (took %s)", elapsed)
	}
	if err == nil || !errors.Is(err, decision.ErrProviderFailure) {
		t.Fatalf("a cancelled run must be a provider failure, got %v", err)
	}
	if d.Choice != "" || d.Confidence != 0 {
		t.Fatalf("a cancelled run must not yield a decision, got %+v", d)
	}
}
