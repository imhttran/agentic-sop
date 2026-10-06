package ollamaagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// Repository no-progress guard tests.
//
// The observed failure (prompt-20261001-191507, P3-006): an IMPLEMENT run
// alternated narration ("planning") with distinct read-only tool calls and never
// mutated, consuming the whole 32-iteration budget. The pre-existing guard only
// caught *consecutive identical* turns, and an alternating shape never repeats a
// fingerprint consecutively.
//
// Mutation remains the only repository progress signal. Successful novel
// inspections may reset the stale streak during bounded initial discovery only.
// Failed inspections, narration, and denials do not earn discovery credit.

// stalledNarration is a plain-prose turn: no tool call and no final object, so the
// loop treats it as narration (planning/reasoning).
const stalledNarration = "I am still considering how to implement this change."

// TestImplementFailedReadsAccumulateNoProgress uses missing files: distinct
// failed reads do not earn discovery credit.
func TestImplementFailedReadsAccumulateNoProgress(t *testing.T) {
	dir := t.TempDir()
	// More than the bound, every read a different file.
	responses := distinctToolCalls(maxNoProgressIterations + 3)
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	var stalled *noProgressError
	if !errors.As(err, &stalled) {
		t.Fatalf("err = %v, want *noProgressError", err)
	}
	msg := err.Error()
	for _, want := range []string{
		"IMPLEMENT_NO_PROGRESS",
		"no repository progress",
		"repository_mutations=0",
		"changed_files=0",
		"termination=no_progress",
		"a retry may succeed",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("err = %q, want it to contain %q", msg, want)
		}
	}
	if got := fake.count(); got > maxNoProgressIterations {
		t.Errorf("chat calls = %d, want <= %d (stop near the bound, never at the ceiling)", got, maxNoProgressIterations)
	}
}

// TestImplementNoProgressStopsStalledRun is the regression for the observed shape:
// narration alternating with distinct read-only calls, zero mutations, for a full
// 32-iteration budget. The run must stop near the no-progress bound instead.
func TestImplementNoProgressStopsStalledRun(t *testing.T) {
	dir := t.TempDir()
	responses := make([]string, 0, maxIterationsImplement*2)
	for i := 0; i < maxIterationsImplement; i++ {
		responses = append(responses, stalledNarration, readToolCall(i))
	}
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	var stalled *noProgressError
	if !errors.As(err, &stalled) {
		t.Fatalf("err = %v, want *noProgressError", err)
	}
	if got := fake.count(); got >= maxIterationsImplement {
		t.Errorf("chat calls = %d, want well under the %d-iteration ceiling", got, maxIterationsImplement)
	}
}

// TestImplementMutationResetsNoProgress proves a successful mutation resets the
// counter: reads either side of a write never accumulate to the bound.
func TestImplementMutationResetsNoProgress(t *testing.T) {
	dir := t.TempDir()
	_, srv := newFakeOllama(t,
		readToolCall(0),
		readToolCall(1),
		`{"tool":"write_file","args":{"path":"out.txt","content":"x"}}`, // reset
		readToolCall(2),
		readToolCall(3),
		`{"status":"completed","summary":"done","changes_expected":true}`,
	)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	content, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	if err != nil {
		t.Fatalf("a run that mutated must not be stopped for no progress: %v", err)
	}
	if !strings.Contains(content, `"status":"completed"`) {
		t.Errorf("content = %q, want the completed outcome", content)
	}
}

// TestImplementRepeatedReadsAreStopped proves repeated reads remain protected: a
// model that reads the same file over and over is stopped by the repetition guard
// (or the no-progress guard), well before the ceiling.
func TestImplementRepeatedReadsAreStopped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.txt", "stable")
	readSame := `{"tool":"read_file","args":{"path":"notes.txt"}}`
	fake, srv := newFakeOllama(t, repeat(readSame, maxIterationsImplement)...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	if err == nil || !strings.Contains(err.Error(), "termination=no_progress") {
		t.Fatalf("err = %v, want a no-progress termination", err)
	}
	if got := fake.count(); got >= maxIterationsImplement {
		t.Errorf("chat calls = %d, want well under the ceiling", got)
	}
}

// TestImplementNarrationIsNotProgress proves a model's narrative is never evidence
// of progress, even when it claims it implemented the change: narration increments
// the counter like any other non-mutating turn.
func TestImplementNarrationIsNotProgress(t *testing.T) {
	dir := t.TempDir()
	claim := "I have implemented the change and updated all the required files."
	_, srv := newFakeOllama(t,
		claim, readToolCall(0),
		claim, readToolCall(1),
		claim,
	)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	var stalled *noProgressError
	if !errors.As(err, &stalled) {
		t.Fatalf("err = %v, want *noProgressError", err)
	}
	if !strings.Contains(err.Error(), "repository_mutations=0") {
		t.Errorf("err = %q, want it to record zero repository mutations", err)
	}
}

// TestImplementDeniedMutationIsNotProgress proves a denied/failed mutation attempt
// does not count as progress: it increments the counter like any other non-mutating
// turn, so the run still stops for no progress.
func TestImplementDeniedMutationIsNotProgress(t *testing.T) {
	dir := t.TempDir()
	_, srv := newFakeOllama(t,
		`{"tool":"write_file","args":{"path":"../escape.txt","content":"x"}}`, // refused: escapes the repo
		readToolCall(0),
		readToolCall(1),
		readToolCall(2),
		readToolCall(3),
	)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	var stalled *noProgressError
	if !errors.As(err, &stalled) {
		t.Fatalf("err = %v, want *noProgressError (a denied write is not progress)", err)
	}
	if !strings.Contains(err.Error(), "repository_mutations=0") {
		t.Errorf("err = %q, want zero repository mutations", err)
	}
}

// TestImplementNoProgressGuardIsSeparateFromCeiling proves the guard is a distinct,
// smaller bound: the iteration ceilings are unchanged (final safety ceilings), and
// the no-progress bound is well below them.
func TestImplementNoProgressGuardIsSeparateFromCeiling(t *testing.T) {
	if got := PolicyFor(agent.Implement).MaxIterations; got != maxIterationsImplement {
		t.Fatalf("IMPLEMENT MaxIterations = %d, want %d", got, maxIterationsImplement)
	}
	if got := PolicyFor(agent.Fix).MaxIterations; got != maxIterationsFix {
		t.Fatalf("FIX MaxIterations = %d, want %d", got, maxIterationsFix)
	}
	if maxIterationsImplement != 32 || maxIterationsFix != 24 {
		t.Fatalf("the iteration ceilings changed: implement=%d fix=%d, want 32/24", maxIterationsImplement, maxIterationsFix)
	}
	if maxNoProgressIterations <= 0 || maxNoProgressIterations >= maxIterationsFix {
		t.Fatalf("maxNoProgressIterations = %d, want a small positive bound below the ceilings", maxNoProgressIterations)
	}
}

// The observed five-operation discovery sequence must leave room for a real
// mutation in both capabilities; discovery itself never supplies mutation evidence.
func TestPhasedDiscoveryThenMutation(t *testing.T) {
	for _, req := range []agent.Request{implementRequest(), fixRequest()} {
		t.Run(string(req.Capability), func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "internal/sopclient/boundary.go", "package sopclient")
			writeFile(t, dir, "internal/sopclient/boundary_test.go", "package sopclient")
			writeFile(t, dir, "docs/history/CTRL001/notes.md", "notes")
			fake, srv := newFakeOllama(t,
				`{"tool":"list_files","args":{"path":"internal/sopclient"}}`,
				`{"tool":"read_file","args":{"path":"internal/sopclient/boundary.go"}}`,
				`{"tool":"read_file","args":{"path":"internal/sopclient/boundary_test.go"}}`,
				`{"tool":"list_files","args":{"path":"docs/history"}}`,
				`{"tool":"list_files","args":{"path":"docs/history/CTRL001"}}`,
				`{"tool":"write_file","args":{"path":"out.txt","content":"implemented"}}`,
				`{"status":"completed","summary":"done","changes_expected":true}`,
			)
			cfg := testConfig(srv.URL)
			cfg.MaxToolCalls = 100
			h := New(cfg, dir)
			ev := &mutationEvidence{}
			content, err := h.ExecuteWithEvidence(context.Background(), req, ev)
			if err != nil || !strings.Contains(content, `"status":"completed"`) {
				t.Fatalf("content=%q err=%v, want completion after five inspections", content, err)
			}
			if fake.count() != 7 || !ev.observed || strings.Join(ev.mutationPaths(), ",") != "out.txt" {
				t.Errorf("calls=%d evidence=%+v, want seven turns and only out.txt mutation", fake.count(), ev)
			}
			if got := countPhase(h.TraceRecords(), "DISCOVER"); got != 5 {
				t.Errorf("discovery turns=%d, want 5", got)
			}
			if got := countPhase(h.TraceRecords(), "CHANGE"); got != 2 {
				t.Errorf("CHANGE turns=%d, want mutation and final response", got)
			}
		})
	}
}

func TestPhasedDiscoveryStalls(t *testing.T) {
	for _, req := range []agent.Request{implementRequest(), fixRequest()} {
		t.Run(string(req.Capability), func(t *testing.T) {
			type stallCase struct {
				name      string
				responses []string
				wantCalls int
			}
			cases := []stallCase{
				{"repeated", repeat(`{"tool":"read_file","args":{"path":"pkg/f0.go"}}`, 10), 4},
				{"alternating", []string{readToolCall(0), readToolCall(1), readToolCall(0), readToolCall(1), readToolCall(0), readToolCall(1), readToolCall(0)}, 7},
				{"failed", distinctToolCalls(5), 5},
				{"narration", []string{stalledNarration, readToolCall(0), stalledNarration, readToolCall(1), stalledNarration}, 5},
			}
			denied, empty := []string{}, []string{}
			for i := 0; i < 5; i++ {
				denied = append(denied, fmt.Sprintf(`{"tool":"read_file","args":{"path":"%s.agent-sdlc/state.db"}}`, strings.Repeat("./", i)))
				empty = append(empty, fmt.Sprintf(`{"tool":"search_files","args":{"path":"pkg","pattern":"missing-%d"}}`, i))
			}
			cases = append(cases,
				stallCase{"denied", denied, 5},
				stallCase{"empty search", empty, 5},
			)
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					dir := t.TempDir()
					if tc.name != "failed" && tc.name != "narration" {
						seedToolFiles(t, dir, "pkg", 2)
					}
					fake, srv := newFakeOllama(t, tc.responses...)
					cfg := testConfig(srv.URL)
					cfg.MaxToolCalls = 100
					h := New(cfg, dir)
					ev := &mutationEvidence{}
					_, err := h.ExecuteWithEvidence(context.Background(), req, ev)
					if err == nil || !strings.Contains(err.Error(), "termination=no_progress") {
						t.Fatalf("err=%v, want no_progress", err)
					}
					if tc.name != "repeated" && !strings.Contains(err.Error(), string(req.Capability)+"_NO_PROGRESS") {
						t.Errorf("err=%v, want capability-specific stale diagnostic", err)
					}
					if got := fake.count(); got != tc.wantCalls {
						t.Errorf("model turns=%d, want %d", got, tc.wantCalls)
					}
					if ev.observed || len(ev.mutationPaths()) != 0 || hasEvent(h.TraceRecords(), implementChangeEvent) {
						t.Errorf("non-mutating run supplied mutation evidence: %+v", ev)
					}
					if tc.name == "denied" {
						for _, r := range h.AuditRecords() {
							if r.Action != "deny" {
								t.Errorf("audit=%+v, want denied", r)
							}
						}
					}
				})
			}
		})
	}
}

func TestPhasedNovelDiscoveryIsBounded(t *testing.T) {
	for _, req := range []agent.Request{implementRequest(), fixRequest()} {
		for _, narrate := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/narration=%t", req.Capability, narrate), func(t *testing.T) {
				dir := t.TempDir()
				seedToolFiles(t, dir, "pkg", maxIterationsImplement)
				responses := []string{}
				for i := 1; i <= maxIterationsImplement; i++ {
					if narrate && i <= implementNowAfter && i%2 == 1 {
						responses = append(responses, stalledNarration)
					} else {
						responses = append(responses, readToolCall(i-1))
					}
				}
				fake, srv := newFakeOllama(t, responses...)
				cfg := testConfig(srv.URL)
				cfg.MaxToolCalls = 100
				h := New(cfg, dir)
				ev := &mutationEvidence{}
				_, err := h.ExecuteWithEvidence(context.Background(), req, ev)
				var stalled *noProgressError
				if !errors.As(err, &stalled) {
					t.Fatalf("err=%v, want resumable noProgressError", err)
				}
				if got := fake.count(); got != implementNowAfter+maxNoProgressIterations {
					t.Errorf("model turns=%d, want %d (window counts narration too)", got, implementNowAfter+maxNoProgressIterations)
				}
				if ev.observed || len(ev.mutationPaths()) != 0 || hasEvent(h.TraceRecords(), implementChangeEvent) {
					t.Errorf("discovery supplied mutation evidence: %+v", ev)
				}
				if !narrate && !hasEvent(h.TraceRecords(), implementContinueEvent) {
					t.Error("productive discovery must reach existing implementation-now guidance")
				}
				for _, want := range []string{string(req.Capability) + "_NO_PROGRESS", "repository_mutations=0", "continuation checkpoint", "termination=no_progress"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("err=%v, want %q", err, want)
					}
				}
			})
		}
	}
}

func TestDiscoveryIdentityAndCheckpointIndependence(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pkg/a.go", "package pkg")
	if err := os.Symlink(filepath.Join(dir, "pkg/a.go"), filepath.Join(dir, "alias.go")); err != nil {
		t.Fatal(err)
	}
	st := newExecutionState()
	st.recordInspected("pkg/a.go") // attempted checkpoint entry is not success evidence
	if st.observeDiscovery(1, dir, "read_file", map[string]any{"path": "pkg/a.go"}, "", errors.New("failed")) || len(st.discoverySeen) != 0 {
		t.Fatal("failed inspection earned discovery credit")
	}
	st.consecutiveNoProgress = 4
	for i, path := range []string{"pkg/./a.go", "./pkg/a.go", filepath.Join(dir, "pkg/a.go"), "alias.go"} {
		discovered := st.observeDiscovery(i+1, dir, "read_file", map[string]any{"path": path}, "package pkg", nil)
		if i == 0 && (st.stalled(false, discovered, maxNoProgressIterations) || st.consecutiveNoProgress != 0) {
			t.Error("first successful discovery must reset a stale streak")
		}
		if discovered != (i == 0) {
			t.Errorf("path=%q credit=%t, want first canonical identity only", path, discovered)
		}
	}
	if !st.observeDiscovery(5, dir, "list_files", map[string]any{"path": "pkg"}, "a.go", nil) {
		t.Error("listing and reading are distinct inspection operations")
	}
	for i, query := range []string{"package", "pkg", " package "} {
		credited := st.observeDiscovery(6+i, dir, "search_files", map[string]any{"path": "./pkg", "pattern": query}, "pkg/a.go:1: package pkg", nil)
		if credited != (i < 2) {
			t.Errorf("query=%q credit=%t, want normalized distinct queries only", query, credited)
		}
	}
	if st.mutationObserved || st.repositoryMutations != 0 {
		t.Fatal("discovery became mutation")
	}
	fresh := newExecutionState()
	if !fresh.observeDiscovery(1, dir, "read_file", map[string]any{"path": "pkg/a.go"}, "package pkg", nil) {
		t.Error("discovery credit must be invocation-scoped")
	}
	st.consecutiveNoProgress = 4
	if st.observeDiscovery(9, dir, "read_file", map[string]any{"path": "pkg/a.go"}, "", errors.New("failed")) || len(st.discoverySeen) != 4 {
		t.Error("failed discovery changed successful inspection evidence")
	}
	st.observeMutation()
	if st.consecutiveNoProgress != 0 || !st.mutationObserved || st.repositoryMutations != 1 {
		t.Errorf("mutation failed to reset progress: %+v", st)
	}
}

// TestDiscoveryIdentityCreditsNonMutatingCommands proves a first-seen successful
// non-mutating command earns bounded discovery, while repeats, mutating commands,
// failed commands, and past-window commands do not. Discovery never becomes
// mutation evidence.
func TestDiscoveryIdentityCreditsNonMutatingCommands(t *testing.T) {
	dir := t.TempDir()
	st := newExecutionState()
	cmd := func(c string) map[string]any { return map[string]any{"command": c} }
	ok := "exit 0\nok"

	if !st.observeDiscovery(1, dir, "run_command", cmd("go test ./..."), ok, nil) {
		t.Error("a first-seen successful non-mutating command must earn discovery")
	}
	if st.observeDiscovery(2, dir, "run_command", cmd("go test ./..."), ok, nil) {
		t.Error("an identical command must not earn discovery twice")
	}
	if !st.observeDiscovery(3, dir, "run_command", cmd("go vet ./..."), ok, nil) {
		t.Error("a distinct successful command must earn discovery")
	}
	if st.observeDiscovery(implementNowAfter+1, dir, "run_command", cmd("go build ./..."), ok, nil) {
		t.Error("commands past the discovery window must not earn credit")
	}
	if st.observeDiscovery(4, dir, "run_command", cmd("gofmt -w a.go"), ok, nil) {
		t.Error("a mutating command is mutation, not discovery")
	}
	if st.observeDiscovery(5, dir, "run_command", cmd("go test ./broken"), "exit 1", errors.New("exit 1")) {
		t.Error("a failed command is not discovery")
	}
	if st.mutationObserved || st.repositoryMutations != 0 {
		t.Fatalf("discovery became mutation: %+v", st)
	}
}
