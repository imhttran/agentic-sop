package eval

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/runtrace"
)

// This file is the ORCH-009 orchestration evaluation corpus: one deterministic,
// model-free fixture per enumerated Phase 9 scenario, registered against the
// descriptor that produces the trace it is evaluated against. Nothing here calls
// a live provider, a model, or the network: the composed orchestration graph is a
// pure function of the fixed descriptors below, so the corpus is reproducible
// offline. The fixtures assert only the orchestration graph the trace records; the
// lifecycle outcome is asserted by the single-agent corpus.

// orchFixturesRoot is the deterministic orchestration evaluation corpus.
const orchFixturesRoot = "../../evals/orchestration"

// orchBase is a fixed timestamp so composed orchestration traces are identical
// across runs.
var orchBase = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// orchWorker builds one deterministic, provider-neutral worker lifecycle. The
// provider and model are fakes; no live provider is involved.
func orchWorker(assignment, task, workerID string, accepted bool, reason string) runtrace.WorkerRun {
	return runtrace.WorkerRun{
		AssignmentID:   assignment,
		TaskID:         task,
		WorkerID:       workerID,
		ModelClass:     "MEDIUM",
		RoutingSource:  "policy",
		Provider:       "fake-agent-a",
		Model:          "fake-model-a",
		Started:        orchBase,
		Completed:      orchBase.Add(time.Minute),
		Accepted:       accepted,
		AcceptedReason: reason,
		RejectedReason: reason,
	}
}

// orchScenario is one registry entry: a fixture file plus the descriptor that
// deterministically produces the trace it is evaluated against.
type orchScenario struct {
	name    string
	fixture string
	run     runtrace.OrchestrationRun
	replans []runtrace.ReplanRecord
}

// orchScenarios is the fixture registry, one entry per ORCH-009 scenario. Every
// entry is model-free and deterministic: no live provider, no wall clock, no I/O.
func orchScenarios() []orchScenario {
	success := func(run runtrace.OrchestrationRun) runtrace.OrchestrationRun {
		run.RunID = "orch"
		return run
	}
	return []orchScenario{
		{
			name:    "single-worker-success",
			fixture: "single-worker-success.expect.json",
			run: success(runtrace.OrchestrationRun{
				TaskID:      "T1",
				Workers:     []runtrace.WorkerRun{orchWorker("A1", "T1", "W1", true, "worker produced an accepted result")},
				Termination: runtrace.TerminationRun{TaskID: "T1", Reason: "run completed", Timestamp: orchBase.Add(2 * time.Minute)},
			}),
		},
		{
			name:    "parallel-independent-workers",
			fixture: "parallel-independent-workers.expect.json",
			run: success(runtrace.OrchestrationRun{
				TaskID: "T1",
				Workers: []runtrace.WorkerRun{
					orchWorker("A1", "T1", "W1", true, "worker 1 produced an accepted result"),
					orchWorker("A2", "T1", "W2", true, "worker 2 produced an accepted result"),
					orchWorker("A3", "T1", "W3", true, "worker 3 produced an accepted result"),
				},
				Termination: runtrace.TerminationRun{TaskID: "T1", Reason: "run completed", Timestamp: orchBase.Add(2 * time.Minute)},
			}),
		},
		{
			name:    "worker-failure",
			fixture: "worker-failure.expect.json",
			run: success(runtrace.OrchestrationRun{
				TaskID: "T1",
				Workers: []runtrace.WorkerRun{
					orchWorker("A1", "T1", "W1", true, "worker produced an accepted result"),
					orchWorker("A2", "T1", "W2", false, "worker failed and produced no accepted result"),
				},
				Termination: runtrace.TerminationRun{TaskID: "T1", Disposition: "NEEDS_HUMAN", Reason: "one worker failed", Timestamp: orchBase.Add(2 * time.Minute)},
			}),
		},
		{
			name:    "worker-no-progress",
			fixture: "worker-no-progress.expect.json",
			run: success(runtrace.OrchestrationRun{
				TaskID:      "T1",
				Workers:     []runtrace.WorkerRun{orchWorker("A1", "T1", "W1", false, "worker made no progress: repeated activity without a repository mutation")},
				Termination: runtrace.TerminationRun{TaskID: "T1", Disposition: "BLOCK", Reason: "no progress", Timestamp: orchBase.Add(2 * time.Minute)},
			}),
		},
		{
			name:    "worker-conflict",
			fixture: "worker-conflict.expect.json",
			run: success(runtrace.OrchestrationRun{
				TaskID: "T1",
				Workers: []runtrace.WorkerRun{
					orchWorker("A1", "T1", "W1", true, "worker produced an accepted result"),
					orchWorker("A2", "T1", "W2", false, "worker conflict: overlapping write scope"),
				},
				Termination: runtrace.TerminationRun{TaskID: "T1", Disposition: "NEEDS_HUMAN", Reason: "worker conflict", Timestamp: orchBase.Add(2 * time.Minute)},
			}),
		},
		{
			name:    "stale-worker-result",
			fixture: "stale-worker-result.expect.json",
			run: success(runtrace.OrchestrationRun{
				TaskID:      "T1",
				Workers:     []runtrace.WorkerRun{orchWorker("A1", "T1", "W1", false, "stale worker result: repository identity changed")},
				Termination: runtrace.TerminationRun{TaskID: "T1", Disposition: "NEEDS_HUMAN", Reason: "stale result", Timestamp: orchBase.Add(2 * time.Minute)},
			}),
		},
		{
			name:    "integration-failure",
			fixture: "integration-failure.expect.json",
			run: success(runtrace.OrchestrationRun{
				TaskID:  "T1",
				Workers: []runtrace.WorkerRun{orchWorker("A1", "T1", "W1", true, "worker produced an accepted result")},
				Integration: []runtrace.IntegrationRun{{
					TaskID:    "T1",
					Inputs:    []string{"A1"},
					Output:    "IR1",
					Started:   orchBase.Add(time.Minute),
					Completed: orchBase.Add(time.Minute + 30*time.Second),
					Reason:    "integration failed: the integrated tree did not verify",
				}},
				Termination: runtrace.TerminationRun{TaskID: "T1", Disposition: "NEEDS_HUMAN", Reason: "integration failed", Timestamp: orchBase.Add(2 * time.Minute)},
			}),
		},
		{
			name:    "verification-failure",
			fixture: "verification-failure.expect.json",
			run: success(runtrace.OrchestrationRun{
				TaskID:  "T1",
				Workers: []runtrace.WorkerRun{orchWorker("A1", "T1", "W1", true, "worker produced an accepted result")},
				Integration: []runtrace.IntegrationRun{{
					TaskID:    "T1",
					Inputs:    []string{"A1"},
					Output:    "IR1",
					Started:   orchBase.Add(time.Minute),
					Completed: orchBase.Add(time.Minute + 30*time.Second),
					Reason:    "integrated",
				}},
				Verification: []runtrace.VerificationRun{{
					TaskID:    "T1",
					Command:   "go test ./...",
					Status:    "FAIL",
					Timestamp: orchBase.Add(time.Minute + 45*time.Second),
				}},
				Termination: runtrace.TerminationRun{TaskID: "T1", Disposition: "NEEDS_HUMAN", Reason: "verification failed", Timestamp: orchBase.Add(2 * time.Minute)},
			}),
		},
		{
			name:    "bounded-retry",
			fixture: "bounded-retry.expect.json",
			run: success(runtrace.OrchestrationRun{
				TaskID: "T1",
				Workers: []runtrace.WorkerRun{
					orchWorker("A1", "T1", "W1", false, "attempt 1 made no progress; bounded retry scheduled"),
					orchWorker("A2", "T1", "W1", true, "attempt 2 produced an accepted result"),
				},
				Termination: runtrace.TerminationRun{TaskID: "T1", Reason: "run completed after a bounded retry", Timestamp: orchBase.Add(2 * time.Minute)},
			}),
		},
		{
			name:    "bounded-replan",
			fixture: "bounded-replan.expect.json",
			run: success(runtrace.OrchestrationRun{
				TaskID: "T1",
				Workers: []runtrace.WorkerRun{
					orchWorker("A1", "T1", "W1", false, "attempt 1 made no progress; bounded replan scheduled"),
					orchWorker("A2", "T1", "W1", true, "attempt 2 produced an accepted result"),
				},
				Termination: runtrace.TerminationRun{TaskID: "T1", Reason: "run completed after a bounded replan", Timestamp: orchBase.Add(2 * time.Minute)},
			}),
			replans: []runtrace.ReplanRecord{
				{Sequence: 1, Reason: "review failure required a strategy change", FromAttempt: 1, ToAttempt: 2},
			},
		},
		{
			name:    "successful-multi-worker-integration",
			fixture: "successful-multi-worker-integration.expect.json",
			run: success(runtrace.OrchestrationRun{
				TaskID: "T1",
				Workers: []runtrace.WorkerRun{
					orchWorker("A1", "T1", "W1", true, "worker 1 produced an accepted result"),
					orchWorker("A2", "T1", "W2", true, "worker 2 produced an accepted result"),
				},
				Integration: []runtrace.IntegrationRun{{
					TaskID:    "T1",
					Inputs:    []string{"A1", "A2"},
					Output:    "IR1",
					Started:   orchBase.Add(time.Minute),
					Completed: orchBase.Add(time.Minute + 30*time.Second),
					Reason:    "integrated two worker results",
				}},
				Termination: runtrace.TerminationRun{TaskID: "T1", Reason: "run completed", Timestamp: orchBase.Add(2 * time.Minute)},
			}),
		},
	}
}

// TestOrchestrationFixtureRegistry evaluates every registered ORCH-009 fixture
// against the deterministic trace its descriptor produces. The pipeline is the
// same core evaluation path as the single-agent corpus; only the fixture and the
// descriptor differ, so a PASS proves the evaluation works without a provider.
func TestOrchestrationFixtureRegistry(t *testing.T) {
	for _, sc := range orchScenarios() {
		t.Run(sc.name, func(t *testing.T) {
			f, err := LoadFixture(filepath.Join(orchFixturesRoot, sc.fixture))
			if err != nil {
				t.Fatalf("load fixture %s: %v", sc.fixture, err)
			}
			tr := runtrace.Build(runtrace.Inputs{
				RunID:               sc.run.RunID,
				OrchestrationEvents: runtrace.ComposeOrchestrationEvents(sc.run),
				Replans:             sc.replans,
			})
			res := Evaluate(tr, f)
			if !res.Passed {
				for _, d := range res.Diagnostics {
					t.Errorf("%s", d.Message)
				}
				t.Fatalf("evaluation %q FAILED", f.Name)
			}
		})
	}
}

// TestOrchestrationFixturesAreRegistered keeps the registry and the corpus from
// drifting: every fixture file is registered, and every entry has a file, so a
// new scenario cannot be added without being evaluated.
func TestOrchestrationFixturesAreRegistered(t *testing.T) {
	registered := map[string]bool{}
	for _, sc := range orchScenarios() {
		if registered[sc.fixture] {
			t.Errorf("fixture %s registered twice", sc.fixture)
		}
		registered[sc.fixture] = true
	}
	entries, err := os.ReadDir(orchFixturesRoot)
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	var onDisk []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".expect.json") {
			onDisk = append(onDisk, e.Name())
		}
	}
	sort.Strings(onDisk)
	for _, name := range onDisk {
		if !registered[name] {
			t.Errorf("fixture %s exists on disk but is not registered", name)
		}
	}
	if len(onDisk) != len(registered) {
		t.Errorf("corpus has %d fixtures, registry has %d", len(onDisk), len(registered))
	}
}

// TestEvaluateOrchestrationMismatchFails proves an orchestration expectation is
// evaluated and a mismatch is a deterministic diagnostic.
func TestEvaluateOrchestrationMismatchFails(t *testing.T) {
	run := runtrace.OrchestrationRun{
		RunID:   "m",
		TaskID:  "T1",
		Workers: []runtrace.WorkerRun{orchWorker("A1", "T1", "W1", true, "ok")},
	}
	tr := runtrace.Build(runtrace.Inputs{OrchestrationEvents: runtrace.ComposeOrchestrationEvents(run)})
	f := Fixture{Name: "m", Expected: Expectation{Orchestration: &OrchestrationExpectation{
		Kinds:          map[string]*CountAssertion{"result_rejected": {Equal: intptr(1)}},
		Assignments:    &CountAssertion{Equal: intptr(9)},
		ReasonContains: []string{"no progress"},
	}}}
	got := Evaluate(tr, f)
	if got.Passed {
		t.Fatal("expected FAIL")
	}
	fields := diagFields(got)
	for _, want := range []string{"orchestration.kinds.result_rejected", "orchestration.assignments", "orchestration.reason_contains"} {
		if !contains(fields, want) {
			t.Errorf("missing diagnostic %s in %v", want, fields)
		}
	}
}

// TestEvaluateOrchestrationSequenceInvariant proves the ordering invariant is
// asserted: a duplicated sequence number is neither total nor unique.
func TestEvaluateOrchestrationSequenceInvariant(t *testing.T) {
	dup := []runtrace.OrchestrationEvent{
		{Sequence: 1, Kind: runtrace.EventWorkerStarted},
		{Sequence: 1, Kind: runtrace.EventWorkerCompleted},
	}
	tr := runtrace.Build(runtrace.Inputs{OrchestrationEvents: dup})
	f := Fixture{Name: "seq", Expected: Expectation{Orchestration: &OrchestrationExpectation{
		Sequence: &SequenceExpectation{Total: boolptr(true), Unique: boolptr(true)},
	}}}
	got := Evaluate(tr, f)
	if got.Passed {
		t.Fatal("expected FAIL for a non-total, non-unique sequence")
	}
	fields := diagFields(got)
	if !contains(fields, "orchestration.sequence.total") || !contains(fields, "orchestration.sequence.unique") {
		t.Errorf("fields = %v", fields)
	}
}

// TestParseFixtureRejectsUnboundedOrchestrationCount proves an orchestration
// count with no bound is an explicit parse error, matching the other counts.
func TestParseFixtureRejectsUnboundedOrchestrationCount(t *testing.T) {
	if _, err := ParseFixture([]byte(`{"name":"x","expected":{"orchestration":{"assignments":{}}}}`)); err == nil {
		t.Fatal("an unbounded orchestration count must be an error")
	}
	if _, err := ParseFixture([]byte(`{"name":"x","expected":{"orchestration":{"kinds":{"worker_started":{}}}}}`)); err == nil {
		t.Fatal("an unbounded orchestration kind count must be an error")
	}
}
