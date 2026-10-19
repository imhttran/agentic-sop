package dqharness

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/imhttran/agentic-sop/internal/decisionquality"
)

// HarnessRevision identifies the harness semantics that produce a run. It is part of the
// checkpoint binding so a changed harness rejects an incompatible resume.
const HarnessRevision = "rdm004-harness-v1"

// Binding identifies every frozen input and execution condition a checkpoint belongs to.
// A resume is accepted only for an identical binding.
type Binding struct {
	Corpus   string `json:"corpus"`
	Protocol string `json:"protocol"`
	Rubric   string `json:"rubric"`
	Harness  string `json:"harness"`
	Model    string `json:"model"`
	Sampling string `json:"sampling"`
}

// Same reports whether two bindings are identical.
func (b Binding) Same(o Binding) bool { return b == o }

// CheckpointEntry is one completed decision, appended once and never rewritten.
type CheckpointEntry struct {
	Arm    string                     `json:"arm"`
	CaseID string                     `json:"case_id"`
	Repeat int                        `json:"repeat"`
	Result decisionquality.CaseResult `json:"result"`
}

type checkpointLine struct {
	Binding *Binding                    `json:"binding,omitempty"`
	Arm     string                      `json:"arm,omitempty"`
	CaseID  string                      `json:"case_id,omitempty"`
	Repeat  int                         `json:"repeat,omitempty"`
	Result  *decisionquality.CaseResult `json:"result,omitempty"`
}

func checkpointKey(arm, caseID string, repeat int) string {
	return fmt.Sprintf("%s|%s|%d", arm, caseID, repeat)
}

// LoadCheckpoint reads completed decisions for an identical binding. A malformed line, a
// missing binding header, or a different binding is an error: resuming across
// incompatible inputs would mix results from different experiments.
func LoadCheckpoint(path string, want Binding) (map[string]decisionquality.CaseResult, error) {
	out := map[string]decisionquality.CaseResult{}
	if path == "" {
		return out, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	sawBinding := false
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var l checkpointLine
		if err := json.Unmarshal(line, &l); err != nil {
			return nil, fmt.Errorf("dqharness: malformed checkpoint line: %w", err)
		}
		if l.Binding != nil {
			if sawBinding {
				return nil, fmt.Errorf("dqharness: checkpoint has more than one binding header")
			}
			sawBinding = true
			if !l.Binding.Same(want) {
				return nil, fmt.Errorf("dqharness: incompatible checkpoint binding (checkpoint %+v, this run %+v)", *l.Binding, want)
			}
			continue
		}
		if !sawBinding {
			return nil, fmt.Errorf("dqharness: legacy checkpoint has no binding header; refusing to resume")
		}
		if l.Result == nil {
			return nil, fmt.Errorf("dqharness: checkpoint line has no result")
		}
		out[checkpointKey(l.Arm, l.CaseID, l.Repeat)] = *l.Result
	}
	return out, sc.Err()
}

// ensureBinding writes the binding header once for a new checkpoint file.
func ensureBinding(path string, b Binding) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	line, err := json.Marshal(checkpointLine{Binding: &b})
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(line, '\n'), 0o644)
}

func appendCheckpoint(path string, e CheckpointEntry) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

// Coverage is the deterministic retrieval coverage of a corpus under the arm-B retriever.
type Coverage struct {
	Cases        int `json:"cases"`
	WithEvidence int `json:"with_evidence"`
	Without      int `json:"without"`
}

// MeasureCoverage reconstructs arm-B retrieval for every case without any model call.
func MeasureCoverage(corpus decisionquality.Corpus, r Retriever) (Coverage, error) {
	cov := Coverage{Cases: len(corpus.Cases)}
	for _, c := range corpus.Cases {
		got, err := r.Retrieve(c, DefaultEvidenceTokens)
		if err != nil {
			return Coverage{}, err
		}
		if len(got) == 0 {
			cov.Without++
		} else {
			cov.WithEvidence++
		}
	}
	return cov, nil
}

// RunOptions configures one comparison run.
type RunOptions struct {
	Corpus         decisionquality.Corpus
	Rules          decisionquality.ScoringRules
	Config         RunConfig
	Binding        Binding
	Decisions      []string
	DecideA        DecisionFunc
	DecideB        DecisionFunc
	Retriever      Retriever
	Sandbox        Sandbox
	Limits         Limits
	CheckpointPath string
}

// RunOutcome is the result of a (possibly partial) run.
type RunOutcome struct {
	A       decisionquality.RunRecord
	B       decisionquality.RunRecord
	Stopped string
}

// RunComparison runs both arms with checkpoint/resume. A completed decision is never
// repeated: it is loaded from the checkpoint and replayed into the record. A limit stop
// returns a partial outcome with Stopped set and no error.
func RunComparison(ctx context.Context, opts RunOptions) (RunOutcome, error) {
	if opts.DecideA == nil || opts.DecideB == nil {
		return RunOutcome{}, errors.New("dqharness: both arm decision functions are required")
	}
	if opts.Rules.RepeatsPerCase <= 0 {
		return RunOutcome{}, errors.New("dqharness: repeats_per_case must be positive")
	}
	if err := ensureBinding(opts.CheckpointPath, opts.Binding); err != nil {
		return RunOutcome{}, err
	}
	done, err := LoadCheckpoint(opts.CheckpointPath, opts.Binding)
	if err != nil {
		return RunOutcome{}, err
	}
	usagePath := ""
	if opts.CheckpointPath != "" {
		usagePath = filepath.Join(filepath.Dir(opts.CheckpointPath), "usage.json")
	}
	usage, err := LoadUsage(usagePath, opts.Binding)
	if err != nil {
		return RunOutcome{}, err
	}
	usage.Binding = opts.Binding
	lim := NewLimiterWithUsage(opts.Limits, usage)
	save := func(inFlight bool) error { return SaveUsage(usagePath, lim.Snapshot(inFlight)) }
	if err := save(false); err != nil {
		return RunOutcome{}, err
	}
	base := decisionquality.RunRecord{
		Model: opts.Config.Model, ModelVersion: opts.Config.ModelVersion,
		PromptTemplate: opts.Config.PromptTemplate, PromptHash: opts.Config.PromptHash,
		TempC: opts.Config.Temperature, TopP: opts.Config.TopP, Seed: opts.Config.Seed,
		Sampling: samplingLabel(opts.Config), EvidenceTokens: opts.Config.EvidenceTokens,
	}
	out := RunOutcome{}
	out.A = base
	out.A.Arm = decisionquality.ArmA
	out.B = base
	out.B.Arm = decisionquality.ArmB
	out.A.RetrievalMode = "none"
	if opts.Retriever != nil {
		out.B.RetrievalMode = opts.Retriever.Mode()
	} else {
		out.B.RetrievalMode = "none"
	}

	for _, arm := range []string{decisionquality.ArmA, decisionquality.ArmB} {
		decide := opts.DecideA
		rec := &out.A
		if arm == decisionquality.ArmB {
			decide = opts.DecideB
			rec = &out.B
		}
		for _, c := range opts.Corpus.Cases {
			for repeat := 1; repeat <= opts.Rules.RepeatsPerCase; repeat++ {
				key := checkpointKey(arm, c.ID, repeat)
				if prev, ok := done[key]; ok {
					rec.Results = append(rec.Results, prev)
					continue
				}
				if err := lim.CanDecision(); err != nil {
					out.Stopped = err.Error()
					return out, nil
				}
				res := decisionquality.CaseResult{Arm: arm, CaseID: c.ID, Repeat: repeat}

				var retrieved []decisionquality.EvidenceItem
				if arm == decisionquality.ArmB && opts.Retriever != nil {
					got, rerr := opts.Retriever.Retrieve(c, DefaultEvidenceTokens)
					if rerr != nil {
						res.NotEvaluable = true
						res.Reason = rerr.Error()
						rec.Results = append(rec.Results, res)
						continue
					}
					if aerr := assertNoExcluded(c, got); aerr != nil {
						return RunOutcome{}, aerr
					}
					retrieved = got
				}
				if c.Executable && opts.Sandbox == nil {
					res.NotEvaluable = true
					res.Reason = "no sandboxed replay available for executable task"
					rec.Results = append(rec.Results, res)
					continue
				}
				lim.BeginRequest()
				if err := save(true); err != nil {
					return RunOutcome{}, err
				}
				d, tokens, latency, derr := decide(ctx, c, retrieved)
				lim.EndRequest(tokens)
				if err := save(false); err != nil {
					return RunOutcome{}, err
				}
				for derr != nil {
					if rerr := lim.CanRetry(); rerr != nil {
						break
					}
					lim.RecordRetry()
					_ = save(false)
					lim.BeginRequest()
					if err := save(true); err != nil {
						return RunOutcome{}, err
					}
					d, tokens, latency, derr = decide(ctx, c, retrieved)
					lim.EndRequest(tokens)
					if err := save(false); err != nil {
						return RunOutcome{}, err
					}
				}
				lim.RecordDecision()
				if err := save(false); err != nil {
					return RunOutcome{}, err
				}
				res.Tokens, res.LatencyMS = tokens, latency
				res.Predicted = d
				res.Expected = c.Expected.Decision
				if derr != nil {
					res.NotEvaluable = true
					res.Reason = derr.Error()
					rec.Results = append(rec.Results, res)
					_ = appendCheckpoint(opts.CheckpointPath, CheckpointEntry{arm, c.ID, repeat, res})
					continue
				}
				if c.Executable {
					ok, serr := opts.Sandbox.Replay(ctx, c, d)
					if serr != nil {
						res.NotEvaluable = true
						res.Reason = serr.Error()
						rec.Results = append(rec.Results, res)
						_ = appendCheckpoint(opts.CheckpointPath, CheckpointEntry{arm, c.ID, repeat, res})
						continue
					}
					res.Correct = ok
				} else {
					res.Correct = d == c.Expected.Decision
				}
				rec.Results = append(rec.Results, res)
				if err := appendCheckpoint(opts.CheckpointPath, CheckpointEntry{arm, c.ID, repeat, res}); err != nil {
					return RunOutcome{}, err
				}
			}
		}
	}
	return out, nil
}
