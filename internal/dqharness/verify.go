package dqharness

import (
	"fmt"

	"github.com/imhttran/agentic-sop/internal/decisionquality"
)

// VerifyRun checks that a run is a complete, matched, frozen comparison. It is
// deterministic and never interprets the result. Any gap is an explicit error.
func VerifyRun(corpus decisionquality.Corpus, rules decisionquality.ScoringRules, a, b decisionquality.RunRecord) error {
	if a.Arm != decisionquality.ArmA || b.Arm != decisionquality.ArmB {
		return fmt.Errorf("dqharness: arms are %q/%q, want A/B", a.Arm, b.Arm)
	}
	for _, f := range []struct {
		name string
		av   any
		bv   any
	}{
		{"model", a.Model, b.Model},
		{"model version", a.ModelVersion, b.ModelVersion},
		{"prompt template", a.PromptTemplate, b.PromptTemplate},
		{"prompt hash", a.PromptHash, b.PromptHash},
		{"sampling", a.Sampling, b.Sampling},
		{"temperature", a.TempC, b.TempC},
		{"top_p", a.TopP, b.TopP},
		{"seed", a.Seed, b.Seed},
		{"evidence token budget", a.EvidenceTokens, b.EvidenceTokens},
	} {
		if f.av != f.bv {
			return fmt.Errorf("dqharness: matched condition %q differs between arms (%v vs %v)", f.name, f.av, f.bv)
		}
	}
	if a.Model == "" {
		return fmt.Errorf("dqharness: model identity is not recorded")
	}

	want := map[string]map[int]bool{}
	for _, c := range corpus.Cases {
		want[c.ID] = map[int]bool{}
		for r := 1; r <= rules.RepeatsPerCase; r++ {
			want[c.ID][r] = true
		}
	}
	check := func(rec decisionquality.RunRecord) error {
		seen := map[string]map[int]bool{}
		for _, cr := range rec.Results {
			reps, ok := want[cr.CaseID]
			if !ok {
				return fmt.Errorf("dqharness: arm %s returned unknown case %q", rec.Arm, cr.CaseID)
			}
			if cr.Repeat < 1 || cr.Repeat > rules.RepeatsPerCase {
				return fmt.Errorf("dqharness: arm %s case %s repeat %d outside 1..%d", rec.Arm, cr.CaseID, cr.Repeat, rules.RepeatsPerCase)
			}
			if seen[cr.CaseID] == nil {
				seen[cr.CaseID] = map[int]bool{}
			}
			if seen[cr.CaseID][cr.Repeat] {
				return fmt.Errorf("dqharness: arm %s case %s repeat %d recorded twice", rec.Arm, cr.CaseID, cr.Repeat)
			}
			seen[cr.CaseID][cr.Repeat] = true
			if cr.NotEvaluable && cr.Reason == "" {
				return fmt.Errorf("dqharness: arm %s case %s repeat %d is not evaluable with no reason", rec.Arm, cr.CaseID, cr.Repeat)
			}
			_ = reps
		}
		for id, reps := range want {
			for r := range reps {
				if seen[id] == nil || !seen[id][r] {
					return fmt.Errorf("dqharness: arm %s is missing case %s repeat %d (resume or re-run; never impute)", rec.Arm, id, r)
				}
			}
		}
		return nil
	}
	if err := check(a); err != nil {
		return err
	}
	if err := check(b); err != nil {
		return err
	}
	return nil
}

// VerifyFrozen confirms the protocol and corpus freeze sidecars match their files.
func VerifyFrozen(root string) error {
	for _, name := range []string{"PROTOCOL.sha256", "CORPUS.sha256"} {
		if err := decisionquality.VerifyFreeze(root, joinRoot(root, name)); err != nil {
			return err
		}
	}
	return nil
}

func joinRoot(root, rel string) string {
	if root == "" {
		return rel
	}
	return root + "/" + rel
}
