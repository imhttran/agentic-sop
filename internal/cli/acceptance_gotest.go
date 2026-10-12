package cli

import (
	"encoding/json"
	"strings"
)

// This file implements structured evaluation of a `go test` acceptance check from
// its `go test -json` (test2json) event stream, replacing the earlier output-text
// heuristic. A `go test` check is met only when the process exit status is 0 AND at
// least one test-level pass event occurred. Structured events make the distinction
// between a real passing test and a green-but-empty run (all tests skipped, no test
// files, a `-run` filter that matched nothing, or a TestMain that returns without
// calling m.Run) observable, so such runs can never stand in for verification.

// goTestEvent is one decoded `go test -json` event. The fields are the subset of the
// test2json event schema the evaluator needs: Action (run/pass/fail/skip/output),
// Package, Test (the test name; empty for package-level events), Elapsed, and Output.
type goTestEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
}

// goTestJSONMet reports whether a `go test -json` run is met: the exit status must be
// 0 and the event stream must contain at least one test-level pass event
// (Action=="pass" with a non-empty Test name). A package-level pass event with no
// test name (for example a TestMain that returns without calling m.Run) does not
// count, and neither does a run whose only test-level events are skips. Lines that
// are not valid JSON — including go test's trailing `ok`/`FAIL` summary text — are
// ignored, and never by themselves yield met. It returns met together with a reason
// describing the decision.
func goTestJSONMet(exitCode int, output string) (bool, string) {
	if exitCode != 0 {
		return false, "go test exited non-zero: no test result verified"
	}
	if goTestPassEventObserved(output) {
		return true, "at least one test passed"
	}
	return false, "no test executed: no passing test event observed"
}

// goTestPassEventObserved reports whether the `go test -json` output contains at
// least one test-level pass event. Non-JSON lines are skipped.
func goTestPassEventObserved(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev goTestEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			// A non-JSON line (including the trailing `ok`/`FAIL` summary) is not an
			// event; ignore it rather than treating it as a failure.
			continue
		}
		if ev.Action == "pass" && ev.Test != "" {
			return true
		}
	}
	return false
}

// ensureGoTestJSON returns argv with `-json` added to a `go test` invocation that
// does not already request structured output. It never adds `-json` twice and never
// alters an operator-supplied `-count`. Non-`go test` invocations are returned
// unchanged. The flag is inserted immediately after `test` so it applies to every
// package, matching the operator-supplied form.
func ensureGoTestJSON(argv []string) []string {
	if !isGoTestArgv(argv) {
		return argv
	}
	for _, a := range argv[2:] {
		if a == "-json" || a == "--json" || a == "-json=true" || a == "--json=true" {
			return argv
		}
	}
	out := make([]string, 0, len(argv)+1)
	out = append(out, argv[0], argv[1], "-json")
	out = append(out, argv[2:]...)
	return out
}
