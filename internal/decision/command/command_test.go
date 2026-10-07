package command

import (
	"context"
	"encoding/json"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/decision"
)

// writeScript writes an executable POSIX fixture and returns its path.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX script fixture")
	}
	path := filepath.Join(t.TempDir(), "adapter.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// adapter builds a process Adapter from a fixture body.
func adapter(t *testing.T, body string) decision.Provider {
	t.Helper()
	p, err := New("fake", []string{writeScript(t, body)})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

func req() decision.Request { return decision.Request{UseCase: "implementation-risk", Subject: "q"} }

func TestDecideValidResult(t *testing.T) {
	p := adapter(t, `cat >/dev/null; printf '%s' '{"contract_version":1,"status":"OK","kind":"implementation-risk","choice":"MEDIUM","confidence":0.8,"diagnostics":{"reason":"moderate"}}'`)
	d, err := p.Decide(context.Background(), req())
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if d.Choice != decision.Medium || d.Confidence != 0.8 || d.Metadata["reason"] != "moderate" {
		t.Fatalf("decision=%+v", d)
	}
	if err := decision.Validate(d); err != nil {
		t.Fatalf("valid result failed validation: %v", err)
	}
}

func TestDecideRequestDTOShape(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "req.json")
	// The script receives the capture path as $1; the adapter keeps the
	// executable and arguments structurally separate.
	path := writeScript(t, `cat > "$1"; printf '%s' '{"contract_version":1,"status":"OK","kind":"implementation-risk","choice":"LOW","confidence":0.9}'`)
	p, err := New("fake", []string{path, capture})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Decide(context.Background(), decision.Request{
		UseCase: "implementation-risk", Subject: "add caching",
		Signals: map[string]float64{"files": 2}, Choices: []decision.Choice{decision.Low, decision.Medium},
	}); err != nil {
		t.Fatalf("decide: %v", err)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		ContractVersion int                `json:"contract_version"`
		Kind            string             `json:"kind"`
		Question        string             `json:"question"`
		Signals         map[string]float64 `json:"signals"`
		Choices         []string           `json:"choices"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("request DTO is not valid JSON: %v (%s)", err, data)
	}
	if wire.ContractVersion != decision.ContractVersion || wire.Kind != "implementation-risk" ||
		wire.Question != "add caching" || wire.Signals["files"] != 2 || len(wire.Choices) != 2 {
		t.Fatalf("request DTO shape wrong: %+v", wire)
	}
}

func TestDecideFailuresFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want error
	}{
		{"unsupported capability", `cat >/dev/null; printf '%s' '{"contract_version":1,"status":"UNSUPPORTED","error":"nope"}'`, decision.ErrUnsupportedCapability},
		{"provider error", `cat >/dev/null; printf '%s' '{"contract_version":1,"status":"ERROR","error":"boom"}'`, decision.ErrProviderResult},
		{"indeterminate", `cat >/dev/null; printf '%s' '{"contract_version":1,"status":"INDETERMINATE"}'`, decision.ErrIndeterminate},
		{"unknown choice", `cat >/dev/null; printf '%s' '{"contract_version":1,"status":"OK","choice":"APPROVE","confidence":0.9}'`, decision.ErrInvalidResult},
		{"invalid confidence", `cat >/dev/null; printf '%s' '{"contract_version":1,"status":"OK","choice":"LOW","confidence":1.5}'`, decision.ErrInvalidResult},
		{"missing confidence", `cat >/dev/null; printf '%s' '{"contract_version":1,"status":"OK","choice":"LOW"}'`, decision.ErrIndeterminate},
		{"malformed output", `cat >/dev/null; printf 'not json'`, decision.ErrInvalidResult},
		{"empty output", `cat >/dev/null`, decision.ErrProviderFailure},
		{"non-zero exit", `cat >/dev/null; echo boom >&2; exit 3`, decision.ErrProviderFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := adapter(t, tc.body)
			d, err := p.Decide(context.Background(), req())
			if err == nil {
				t.Fatalf("a %s result must fail closed, got decision %+v", tc.name, d)
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
			if d.Choice != "" || d.Confidence != 0 {
				t.Fatalf("a failed decision must be the zero value, got %+v", d)
			}
		})
	}
}

func TestDecideTimeoutFailsClosed(t *testing.T) {
	p := adapter(t, `cat >/dev/null; sleep 5`)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	d, err := p.Decide(ctx, req())
	if err == nil || !errors.Is(err, decision.ErrProviderFailure) {
		t.Fatalf("timeout must be a provider failure, got %v", err)
	}
	if d.Choice != "" || d.Confidence != 0 {
		t.Fatalf("timeout must not yield a decision: %+v", d)
	}
}

func TestDecideCancellationFailsClosed(t *testing.T) {
	p := adapter(t, `cat >/dev/null; sleep 5`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d, err := p.Decide(ctx, req())
	if err == nil || !errors.Is(err, decision.ErrProviderFailure) {
		t.Fatalf("cancellation must be a provider failure, got %v", err)
	}
	if d.Choice != "" || d.Confidence != 0 {
		t.Fatalf("cancellation must not yield a decision: %+v", d)
	}
}

func TestNewRejectsEmptyNameOrExecutable(t *testing.T) {
	if _, err := New("", []string{"/bin/true"}); err == nil {
		t.Error("empty name must be rejected")
	}
	if _, err := New("fake", nil); err == nil {
		t.Error("empty executable must be rejected")
	}
}

// TestCommandPackageStaysNeutral proves the adapter has no provider- or
// policy-implementation coupling: it imports neither a provider implementation,
// nor execution-model routing, nor the lifecycle/approval/autonomy packages,
// and it names no specific provider.
func TestCommandPackageStaysNeutral(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test source file")
	}
	dir := filepath.Dir(file)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{
		"github.com/imhttran/agentic-sop/internal/provider",
		"github.com/imhttran/agentic-sop/internal/ollamaagent",
		"github.com/imhttran/agentic-sop/internal/model",
		"github.com/imhttran/agentic-sop/internal/autonomy",
		"github.com/imhttran/agentic-sop/internal/run",
		"github.com/imhttran/agentic-sop/internal/approval",
		"github.com/imhttran/agentic-sop/internal/domain",
	}
	providerNames := []string{"clef", "ollama", "omlx", "systemone", "nimble", "laya", "julia", "jev"}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		for _, spec := range f.Imports {
			p, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			for _, bad := range forbidden {
				if p == bad || strings.HasPrefix(p, bad+"/") {
					t.Errorf("%s imports %q (%s); the decision adapter must stay provider- and policy-neutral", e.Name(), p, bad)
				}
			}
			lower := strings.ToLower(p)
			for _, name := range providerNames {
				if strings.Contains(lower, name) {
					t.Errorf("%s imports %q (provider-specific %q); SOP must stay provider-neutral", e.Name(), p, name)
				}
			}
		}
	}
}
