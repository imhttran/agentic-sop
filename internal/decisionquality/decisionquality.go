// Package decisionquality holds the RDM-004 decision-quality evaluation contract:
// the frozen held-out and tuning corpora, the pre-decision evidence rules, the
// freeze hashes, and the scoring rules frozen before the final run.
//
// It is tooling only. It defines no production policy, calls no model, and never
// reads SOP state. Expected outcomes are authored from pre-decision evidence only.
package decisionquality

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Case classes required by RDM-004.
const (
	ClassPlanning = "planning"
	ClassRecovery = "recovery"
	ClassNegative = "negative"
)

// Negative kinds covered by the negative class.
var negativeKinds = map[string]bool{
	"irrelevant_evidence": true,
	"obsolete_fix":        true,
	"missing_evidence":    true,
	"policy_manipulation": true,
}

// EvidenceItem is one historical record available before the decision point.
type EvidenceItem struct {
	RecordID    string `json:"record_id"`
	Text        string `json:"text"`
	ContentHash string `json:"content_hash"`
	// ProducedBefore is the record timestamp; it must be <= the case cutoff.
	ProducedBefore string `json:"produced_before"`
}

// Case is one held-out (or tuning) historical case.
type Case struct {
	ID    string `json:"id"`
	Class string `json:"class"`
	// NegativeKind is set only for negative cases.
	NegativeKind string `json:"negative_kind,omitempty"`
	// TaskContext is the current task context (arm A input).
	TaskContext string `json:"task_context"`
	// DecisionPoint is the ISO date/time of the historical decision.
	DecisionPoint string `json:"decision_point"`
	// EvidenceCutoff is the latest time evidence may be drawn from.
	EvidenceCutoff string `json:"evidence_cutoff"`
	// PreDecisionEvidence is the only historical evidence the case may expose.
	PreDecisionEvidence []EvidenceItem `json:"pre_decision_evidence"`
	// EventualSolutionIDs name records produced after the decision point that the
	// case must never expose (its own eventual solution).
	EventualSolutionIDs []string `json:"eventual_solution_ids"`
	// NearDuplicateSolutionIDs name near-duplicate solution records excluded.
	NearDuplicateSolutionIDs []string `json:"near_duplicate_solution_ids"`
	// Executable marks tasks that need sandboxed replay; PlanOnly marks plans
	// scored by the arm-blind rubric.
	Executable bool `json:"executable"`
	// Expected is the frozen expected outcome (the correct plan or recovery).
	Expected ExpectedOutcome `json:"expected"`
}

// ExpectedOutcome is the frozen label for a case.
type ExpectedOutcome struct {
	// Kind is "plan" or "recovery".
	Kind string `json:"kind"`
	// Decision is the expected decision token (e.g. PROCEED, RETRY, NEEDS_HUMAN).
	Decision string `json:"decision"`
	// Rationale justifies the label from pre-decision evidence only.
	Rationale string `json:"rationale"`
}

// Corpus is one frozen case set (held-out or tuning).
type Corpus struct {
	Version     int    `json:"version"`
	Description string `json:"description"`
	// Split is "heldout" or "tuning". The final run uses heldout only.
	Split string `json:"split"`
	Cases []Case `json:"cases"`
}

// LoadCorpus strictly decodes a corpus file. Unknown keys are rejected.
func LoadCorpus(path string) (*Corpus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	var c Corpus
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("decisionquality: decode corpus: %w", err)
	}
	return &c, nil
}

// Counts reports per-class counts.
func (c Corpus) Counts() map[string]int {
	out := map[string]int{}
	for _, cs := range c.Cases {
		out[cs.Class]++
	}
	return out
}

// Validate reports the first corpus contract violation.
func (c Corpus) Validate() error {
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
		case ClassPlanning, ClassRecovery, ClassNegative:
		default:
			return fmt.Errorf("decisionquality: case %q: unknown class %q", cs.ID, cs.Class)
		}
		if cs.Class == ClassNegative && !negativeKinds[cs.NegativeKind] {
			return fmt.Errorf("decisionquality: case %q: negative_kind %q is not irrelevant_evidence|obsolete_fix|missing_evidence|policy_manipulation", cs.ID, cs.NegativeKind)
		}
		if cs.Class != ClassNegative && cs.NegativeKind != "" {
			return fmt.Errorf("decisionquality: case %q: negative_kind set on non-negative case", cs.ID)
		}
		if strings.TrimSpace(cs.TaskContext) == "" {
			return fmt.Errorf("decisionquality: case %q: task_context is required", cs.ID)
		}
		if strings.TrimSpace(cs.DecisionPoint) == "" || strings.TrimSpace(cs.EvidenceCutoff) == "" {
			return fmt.Errorf("decisionquality: case %q: decision_point and evidence_cutoff are required", cs.ID)
		}
		if cs.EvidenceCutoff > cs.DecisionPoint {
			return fmt.Errorf("decisionquality: case %q: evidence_cutoff %q after decision_point %q", cs.ID, cs.EvidenceCutoff, cs.DecisionPoint)
		}
		if strings.TrimSpace(cs.Expected.Decision) == "" || strings.TrimSpace(cs.Expected.Rationale) == "" {
			return fmt.Errorf("decisionquality: case %q: expected decision and rationale are required", cs.ID)
		}
		if cs.Expected.Kind != "plan" && cs.Expected.Kind != "recovery" {
			return fmt.Errorf("decisionquality: case %q: expected kind must be plan or recovery", cs.ID)
		}
		excluded := map[string]bool{}
		for _, id := range cs.EventualSolutionIDs {
			excluded[id] = true
		}
		for _, id := range cs.NearDuplicateSolutionIDs {
			excluded[id] = true
		}
		for _, ev := range cs.PreDecisionEvidence {
			if strings.TrimSpace(ev.RecordID) == "" {
				return fmt.Errorf("decisionquality: case %q: evidence item has no record_id", cs.ID)
			}
			if ev.ProducedBefore > cs.EvidenceCutoff {
				return fmt.Errorf("decisionquality: case %q: evidence %q produced %q after cutoff %q", cs.ID, ev.RecordID, ev.ProducedBefore, cs.EvidenceCutoff)
			}
			if excluded[ev.RecordID] {
				return fmt.Errorf("decisionquality: case %q: exposed excluded solution record %q", cs.ID, ev.RecordID)
			}
		}
	}
	if c.Split == "heldout" {
		if len(c.Cases) < 30 {
			return fmt.Errorf("decisionquality: held-out corpus has %d cases, need at least 30", len(c.Cases))
		}
		for _, class := range []string{ClassPlanning, ClassRecovery, ClassNegative} {
			if counts[class] < 10 {
				return fmt.Errorf("decisionquality: held-out class %q has %d cases, need at least 10", class, counts[class])
			}
		}
	}
	return nil
}

// FreezeOf returns the sha256 of a file's bytes.
func FreezeOf(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// VerifyFreeze checks every `<path>  <sha256>` entry of a freeze file against the
// current bytes of that path (paths resolve relative to root).
func VerifyFreeze(root, freezePath string) error {
	data, err := os.ReadFile(freezePath)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return fmt.Errorf("decisionquality: malformed freeze line %q", line)
		}
		want, rel := fields[0], fields[1]
		got, err := FreezeOf(joinRoot(root, rel))
		if err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("decisionquality: freeze mismatch for %s: file is %s, frozen %s", rel, got, want)
		}
	}
	return nil
}
