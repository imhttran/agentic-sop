package testrunner

import (
	"encoding/json"
	"strings"
)

// parseGoTestExecuted inspects `go test -json` output and reports whether at
// least one TEST actually executed and passed, with no test-level failure and no
// package-only result. It exists because exit code 0 is not proof for the
// PostgreSQL integration category: a run whose tests were all SKIPPED (or that
// only reported package-level `ok` events) also exits 0. The parser is bounded
// (line-oriented) and never executes anything.
func parseGoTestExecuted(stdout string) (bool, string) {
	type event struct {
		Action  string `json:"Action"`
		Test    string `json:"Test"`
		Package string `json:"Package"`
	}
	var passed, failed, skipped, events int
	packagesWithTests := map[string]bool{}
	packagesCompleted := map[string]bool{}
	for _, raw := range strings.Split(stdout, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var e event
		if json.Unmarshal([]byte(line), &e) != nil {
			continue // a malformed/truncated line is not counted as evidence
		}
		events++
		if e.Test == "" {
			// A package-level pass with no test is the package's completion event.
			if e.Action == "pass" && e.Package != "" {
				packagesCompleted[e.Package] = true
			}
			continue // package-level event: not proof a test executed
		}
		if e.Package != "" {
			packagesWithTests[e.Package] = true
		}
		switch e.Action {
		case "pass":
			passed++
		case "fail":
			failed++
		case "skip":
			skipped++
		}
	}
	if events == 0 {
		return false, "no structured go test events"
	}
	if failed > 0 {
		return false, "a test failed"
	}
	if passed == 0 {
		return false, "no test executed and passed (skipped-only or package-only)"
	}
	// Each package that reported test events must also report its completion, so a
	// truncated stream (test events without package completion) is not accepted.
	for pkg := range packagesWithTests {
		if !packagesCompleted[pkg] {
			return false, "missing package completion event (truncated stream)"
		}
	}
	return true, ""
}
