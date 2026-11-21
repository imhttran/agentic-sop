package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/internal/deceleval"
)

func TestExpandModes(t *testing.T) {
	cases := map[string]int{"quick": 1, "full": 1, "all": 2}
	for mode, want := range cases {
		got, err := expandModes(mode)
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if len(got) != want {
			t.Fatalf("%s: %d modes, want %d", mode, len(got), want)
		}
	}
	if _, err := expandModes("nope"); err == nil {
		t.Fatal("unknown mode must fail")
	}
}

func TestParseConcurrency(t *testing.T) {
	got, err := parseConcurrency("1,4")
	if err != nil || len(got) != 2 || got[0] != 1 || got[1] != 4 {
		t.Fatalf("got %v, %v", got, err)
	}
	for _, bad := range []string{"", "0", "-1", "x", "1,y"} {
		if _, err := parseConcurrency(bad); err == nil {
			t.Fatalf("%q must fail", bad)
		}
	}
}

// TestRequireSmokeSummary verifies the full run cannot start until a completed
// quick run exists for every requested model that was not already recorded
// unavailable. This is the "confirm the harness works first" gate.
func TestRequireSmokeSummary(t *testing.T) {
	dir := t.TempDir()
	requested := []string{"ModelA"}

	if err := requireSmokeSummary(dir, requested); err == nil {
		t.Fatal("full mode must fail without a quick summary")
	}

	s := deceleval.ABRunSummary{
		ModelsRequested: requested,
		Batches: []deceleval.ABBatchSummary{
			{Model: "ModelA", Split: deceleval.ABSplitSmoke, Attempted: 12, Complete: true},
		},
	}
	write := func(v deceleval.ABRunSummary) {
		data, _ := json.Marshal(v)
		if err := os.WriteFile(filepath.Join(dir, "summary-quick.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(s)
	if err := requireSmokeSummary(dir, requested); err != nil {
		t.Fatalf("completed smoke must pass: %v", err)
	}

	s.Batches[0].Complete = false
	write(s)
	if err := requireSmokeSummary(dir, requested); err == nil {
		t.Fatal("an incomplete smoke batch must not authorize the full run")
	}

	// A model already recorded unavailable is allowed to proceed to the full
	// run, where it is re-evaluated and reported rather than silently replaced.
	s.Batches = nil
	s.ModelsUnavailable = requested
	write(s)
	if err := requireSmokeSummary(dir, requested); err != nil {
		t.Fatalf("unavailable model must not block the full run: %v", err)
	}
}

func TestWarmupFailure(t *testing.T) {
	cases := []struct {
		cat  string
		want bool
	}{
		{"", false},
		{"malformed", false}, // the model served a response
		{"timeout", true},
		{"http_error", true},
		{"unavailable", true},
		{"provider_error", true},
	}
	for _, c := range cases {
		err := warmupFailure(deceleval.ABObservation{ErrorCategory: c.cat, ErrorDetail: "detail"})
		if (err != nil) != c.want {
			t.Fatalf("category %q: got err=%v, want failure=%v", c.cat, err, c.want)
		}
	}
}
