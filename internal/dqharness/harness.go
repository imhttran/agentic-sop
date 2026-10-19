// Package dqharness runs the RDM-004 A/B comparison. It executes one case three
// times per arm, strips arm identity before plan review, records model version
// and sampling settings, and records matched evidence-token budgets.
//
// It is tooling only. It calls no network service by default and never reads SOP
// state. Replayable recovery tasks run through the injected Sandbox; a recovery
// task with no sandbox is recorded as not evaluable.
package dqharness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/imhttran/agentic-sop/internal/decisionquality"
)

// DecisionFunc answers one case under one arm. identity carries the arm-blind
// inputs; retrieved is empty for arm A and holds arm B evidence for arm B.
// The function returns the arm's decision token, its token use, and its latency.
type DecisionFunc func(ctx context.Context, c decisionquality.Case, retrieved []decisionquality.EvidenceItem) (decision string, tokens, latencyMS int, err error)

// Sandbox replays an executable recovery on a copy of the repository. It returns
// true only when the recovery is verified.
type Sandbox interface {
	Replay(ctx context.Context, c decisionquality.Case, recovery string) (bool, error)
}

// Retriever returns the evidence a case may see under the retrieval arm. It must
// only return items present in c.PreDecisionEvidence and it must respect the
// evidence-token budget.
type Retriever interface {
	Retrieve(c decisionquality.Case, budget int) ([]decisionquality.EvidenceItem, error)
	Mode() string
}

// RunConfig freezes the matched execution conditions shared by every arm.
type RunConfig struct {
	Model          string
	ModelVersion   string
	PromptTemplate string
	PromptHash     string
	Temperature    float64
	TopP           float64
	Seed           int64
	EvidenceTokens int
}

// Harness runs the comparison.
type Harness struct {
	Config    RunConfig
	Rules     decisionquality.ScoringRules
	Decide    DecisionFunc
	Retriever Retriever
	Sandbox   Sandbox
}

// RunArm runs every case three times under the named arm.
func (h Harness) RunArm(ctx context.Context, arm string, corpus decisionquality.Corpus) (decisionquality.RunRecord, error) {
	rec := decisionquality.RunRecord{
		Arm: arm, Model: h.Config.Model, ModelVersion: h.Config.ModelVersion,
		PromptTemplate: h.Config.PromptTemplate, PromptHash: h.Config.PromptHash,
		TempC: h.Config.Temperature, TopP: h.Config.TopP, Seed: h.Config.Seed,
		EvidenceTokens: h.Config.EvidenceTokens,
		Sampling:       samplingLabel(h.Config),
		RetrievalMode:  "none",
	}
	if arm == decisionquality.ArmB {
		if h.Retriever == nil {
			return decisionquality.RunRecord{}, fmt.Errorf("dqharness: arm B needs a retriever")
		}
		rec.RetrievalMode = h.Retriever.Mode()
	}
	for _, c := range corpus.Cases {
		for repeat := 1; repeat <= h.Rules.RepeatsPerCase; repeat++ {
			res := decisionquality.CaseResult{Arm: arm, CaseID: c.ID, Repeat: repeat}
			var retrieved []decisionquality.EvidenceItem
			if arm == decisionquality.ArmB {
				got, err := h.Retriever.Retrieve(c, h.Config.EvidenceTokens)
				if err != nil {
					return decisionquality.RunRecord{}, fmt.Errorf("dqharness: retrieve %s: %w", c.ID, err)
				}
				if err := assertNoExcluded(c, got); err != nil {
					res.ApprovalBypass = false
					return decisionquality.RunRecord{}, err
				}
				retrieved = got
			}
			if c.Executable && h.Sandbox == nil {
				res.NotEvaluable = true
				res.Reason = "no sandboxed replay available for executable task"
				rec.Results = append(rec.Results, res)
				continue
			}
			d, tokens, latency, err := h.Decide(ctx, c, retrieved)
			if err != nil {
				res.NotEvaluable = true
				res.Reason = err.Error()
				rec.Results = append(rec.Results, res)
				continue
			}
			res.Tokens = tokens
			res.LatencyMS = latency
			res.Predicted = d
			res.Expected = c.Expected.Decision
			if c.Executable {
				ok, err := h.Sandbox.Replay(ctx, c, d)
				if err != nil {
					res.NotEvaluable = true
					res.Reason = err.Error()
					rec.Results = append(rec.Results, res)
					continue
				}
				res.Correct = ok
			} else {
				res.Correct = d == c.Expected.Decision
			}
			rec.Results = append(rec.Results, res)
		}
	}
	return rec, nil
}

// assertNoExcluded fails when a retriever exposes a solution record or evidence
// produced after the cutoff.
func assertNoExcluded(c decisionquality.Case, got []decisionquality.EvidenceItem) error {
	excluded := map[string]bool{}
	for _, id := range c.EventualSolutionIDs {
		excluded[id] = true
	}
	for _, id := range c.NearDuplicateSolutionIDs {
		excluded[id] = true
	}
	allowed := map[string]bool{}
	for _, ev := range c.PreDecisionEvidence {
		allowed[ev.RecordID] = true
	}
	for _, ev := range got {
		if excluded[ev.RecordID] {
			return fmt.Errorf("dqharness: case %s: retriever exposed excluded solution %q", c.ID, ev.RecordID)
		}
		if !allowed[ev.RecordID] {
			return fmt.Errorf("dqharness: case %s: retriever returned evidence %q outside the pre-decision set", c.ID, ev.RecordID)
		}
		if ev.ProducedBefore > c.EvidenceCutoff {
			return fmt.Errorf("dqharness: case %s: retriever returned evidence %q after cutoff", c.ID, ev.RecordID)
		}
	}
	return nil
}

func samplingLabel(c RunConfig) string {
	return fmt.Sprintf("temperature=%.2f top_p=%.2f seed=%d", c.Temperature, c.TopP, c.Seed)
}

// SamplingLabel renders the sampling configuration for a checkpoint binding. It is the
// exported form of the label recorded on each run.
func SamplingLabel(c RunConfig) string { return samplingLabel(c) }

// HashBytes returns the sha256 hex of data. It lets callers freeze artifacts.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
