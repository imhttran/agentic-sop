package decisionquality

import (
	"fmt"
	"sort"
	"strings"
)

// InsufficientCoverage reports why the held-out split cannot support a conclusion under
// the pilot's declared minimum coverage. Coverage is measured on the recorded DISPOSITION
// labels (CONTINUE, AUTO_FIX, RETRY, BLOCK, NEEDS_HUMAN), not on the planning/negative
// classes: the frozen gate requires at least three cases per evaluated label. It never
// enlarges the sample.
func InsufficientCoverage(c Corpus) string {
	if len(c.Cases) < 20 {
		return fmt.Sprintf("held-out corpus has %d cases, need at least 20", len(c.Cases))
	}
	labels := map[string]int{}
	for _, cs := range c.Cases {
		d := strings.TrimSpace(cs.Expected.Decision)
		if d != "" {
			labels[d]++
		}
	}
	if len(labels) < 2 {
		return fmt.Sprintf("held-out corpus covers %d disposition labels, need at least 2", len(labels))
	}
	var names []string
	for name := range labels {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if labels[name] < 3 {
			return fmt.Sprintf("disposition label %s has %d cases, need at least 3", name, labels[name])
		}
	}
	return ""
}

// PilotDecision is the planning-only pilot's decision. It never generalizes to executable
// recovery: the pilot measures classification accuracy only.
func PilotDecision(g Gate, insufficient string) string {
	if insufficient != "" {
		return "INSUFFICIENT_EVIDENCE"
	}
	if g.Pass {
		return "GO"
	}
	return "NO-GO"
}

// ValidatePilot checks the planning-only decision-memory corpus contract. It is separate
// from Validate, which encodes the original RDM-004 10/10/10 experiment and is preserved
// unchanged.
//
// Pilot rules: classes are planning and negative only; every case carries task input, a
// decision timestamp, pre-decision evidence, and a recorded expected decision; the held-out
// split must reach the declared minimum case count and class coverage.
func ValidatePilot(c Corpus) error {
	if c.Version != 1 {
		return fmt.Errorf("decisionquality: unsupported corpus version %d", c.Version)
	}
	if c.Split != "heldout" && c.Split != "tuning" {
		return fmt.Errorf("decisionquality: split must be heldout or tuning")
	}
	seen := map[string]bool{}
	counts := map[string]int{}
	for i, cs := range c.Cases {
		if strings.TrimSpace(cs.ID) == "" {
			return fmt.Errorf("decisionquality: case %d has no id", i)
		}
		if seen[cs.ID] {
			return fmt.Errorf("decisionquality: duplicate case id %q", cs.ID)
		}
		seen[cs.ID] = true
		counts[cs.Class]++
		switch cs.Class {
		case ClassPlanning, ClassNegative:
		default:
			return fmt.Errorf("decisionquality: pilot case %q: class must be planning or negative", cs.ID)
		}
		if strings.TrimSpace(cs.TaskContext) == "" {
			return fmt.Errorf("decisionquality: case %q: task_context is required", cs.ID)
		}
		if strings.TrimSpace(cs.DecisionPoint) == "" || strings.TrimSpace(cs.EvidenceCutoff) == "" {
			return fmt.Errorf("decisionquality: case %q: decision_point and evidence_cutoff are required", cs.ID)
		}
		if cs.EvidenceCutoff > cs.DecisionPoint {
			return fmt.Errorf("decisionquality: case %q: evidence_cutoff after decision_point", cs.ID)
		}
		if len(cs.PreDecisionEvidence) == 0 {
			return fmt.Errorf("decisionquality: case %q: no pre-decision evidence", cs.ID)
		}
		for _, ev := range cs.PreDecisionEvidence {
			if strings.TrimSpace(ev.RecordID) == "" || strings.TrimSpace(ev.Text) == "" {
				return fmt.Errorf("decisionquality: case %q: incomplete evidence item", cs.ID)
			}
			if ev.ProducedBefore > cs.EvidenceCutoff {
				return fmt.Errorf("decisionquality: case %q: evidence %q after cutoff", cs.ID, ev.RecordID)
			}
		}
		if strings.TrimSpace(cs.Expected.Decision) == "" || strings.TrimSpace(cs.Expected.Rationale) == "" {
			return fmt.Errorf("decisionquality: case %q: expected decision and rationale are required", cs.ID)
		}
		if cs.Expected.Kind != "plan" {
			return fmt.Errorf("decisionquality: pilot case %q: expected kind must be plan", cs.ID)
		}
	}
	if c.Split == "tuning" {
		if len(c.Cases) < 2 {
			return fmt.Errorf("decisionquality: tuning corpus has %d cases, need at least 2", len(c.Cases))
		}
		return nil
	}
	if len(c.Cases) < 20 {
		return fmt.Errorf("decisionquality: held-out corpus has %d cases, need at least 20", len(c.Cases))
	}
	if len(counts) < 2 {
		return fmt.Errorf("decisionquality: held-out corpus covers %d classes, need at least 2", len(counts))
	}
	for class, n := range counts {
		if n < 3 {
			return fmt.Errorf("decisionquality: held-out class %q has %d cases, need at least 3", class, n)
		}
	}
	return nil
}
