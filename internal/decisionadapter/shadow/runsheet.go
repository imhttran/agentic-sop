package shadow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/imhttran/agentic-sop/decision"
)

// RunSheet is the offline input describing a shadow evaluation to replay: the
// comparison sources (each with an optional per-source comparability gate) and
// the cases, where every case carries the primary decision, an optional reference
// outcome, and the recorded observations to replay. ProduceEvidence never calls a
// provider and never opens a network connection: it replays recorded observations
// through the existing harness, so its output is deterministic.
type RunSheet struct {
	Sources []RunSheetSource "json:\"sources\""
	Cases   []RunSheetCase   "json:\"cases\""
}

// RunSheetSource declares a comparison source and the case IDs that are not
// semantically comparable for it.
type RunSheetSource struct {
	Name                 string   "json:\"name\""
	NonComparableCaseIDs []string "json:\"non_comparable_case_ids,omitempty\""
}

// RunSheetCase is one primary case and the recorded observations to replay.
type RunSheetCase struct {
	ID           string                "json:\"id\""
	Question     decision.Question     "json:\"question\""
	Primary      string                "json:\"primary\""
	Reference    *Reference            "json:\"reference,omitempty\""
	Observations []RunSheetObservation "json:\"observations\""
}

// RunSheetObservation is one recorded source observation. Outcome records the
// containment outcome when no value was obtained: "" (a value was obtained),
// "timeout", "malformed", or "error".
type RunSheetObservation struct {
	Source     string   "json:\"source\""
	Choice     string   "json:\"choice,omitempty\""
	Confidence *float64 "json:\"confidence,omitempty\""
	LatencyMS  int64    "json:\"latency_ms,omitempty\""
	Outcome    string   "json:\"outcome,omitempty\""
}

// ProduceEvidence replays a run-sheet through the shadow harness and returns the
// deterministic evidence snapshot. It validates the sheet (unique sources and
// case IDs, declared observation sources, well-formed questions) and fails closed
// rather than emitting partial evidence.
func ProduceEvidence(rs RunSheet) (Evidence, error) {
	declared := make(map[string]bool, len(rs.Sources))
	nonComparable := make(map[string]map[string]bool, len(rs.Sources))
	for _, s := range rs.Sources {
		if s.Name == "" {
			return Evidence{}, errors.New("shadow: run-sheet source name is required")
		}
		if declared[s.Name] {
			return Evidence{}, fmt.Errorf("shadow: duplicate source %q", s.Name)
		}
		declared[s.Name] = true
		set := make(map[string]bool, len(s.NonComparableCaseIDs))
		for _, id := range s.NonComparableCaseIDs {
			set[id] = true
		}
		nonComparable[s.Name] = set
	}

	rec := NewRecorder()
	seen := make(map[string]bool, len(rs.Cases))
	for _, cs := range rs.Cases {
		if cs.ID == "" {
			return Evidence{}, errors.New("shadow: run-sheet case id is required")
		}
		if seen[cs.ID] {
			return Evidence{}, fmt.Errorf("shadow: duplicate case id %q", cs.ID)
		}
		seen[cs.ID] = true
		if err := cs.Question.Validate(); err != nil {
			return Evidence{}, fmt.Errorf("shadow: case %q: %w", cs.ID, err)
		}
		c := Case{ID: cs.ID, Question: cs.Question, Reference: cs.Reference}
		for _, ob := range cs.Observations {
			if !declared[ob.Source] {
				return Evidence{}, fmt.Errorf("shadow: case %q: observation source %q is not declared", cs.ID, ob.Source)
			}
			comparable := !nonComparable[ob.Source][cs.ID]
			rec.RecordSupplied(
				c, ob.Source, cs.Primary, comparable,
				Observation{Choice: ob.Choice, Confidence: ob.Confidence},
				time.Duration(ob.LatencyMS)*time.Millisecond,
				outcomeError(ob.Outcome),
			)
		}
	}
	return rec.Evidence(), nil
}

// outcomeError maps a recorded containment outcome to the error the harness
// classifiers consume. An empty outcome is a value that was obtained.
func outcomeError(outcome string) error {
	switch outcome {
	case "", "ok", "value":
		return nil
	case "timeout":
		return context.DeadlineExceeded
	case "malformed":
		return decision.ErrMalformedResponse
	default:
		return errors.New("shadow source error")
	}
}
