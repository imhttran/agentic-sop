// Command rdm-pilot-corpus builds the source-backed planning-only decision-memory
// corpus from recorded SOP run artifacts.
//
// A case is eligible only when the run directory records a task input, a decision
// timestamp, an executable classification, and pre-decision execution/validation
// evidence. Labels come from the recorded disposition. The classification kind and
// reason are never copied into the evidence: doing so would leak the answer.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type evidence struct {
	RecordID       string `json:"record_id"`
	Text           string `json:"text"`
	ContentHash    string `json:"content_hash"`
	ProducedBefore string `json:"produced_before"`
}

type expected struct {
	Kind      string `json:"kind"`
	Decision  string `json:"decision"`
	Rationale string `json:"rationale"`
}

type kase struct {
	ID                       string     `json:"id"`
	Class                    string     `json:"class"`
	NegativeKind             string     `json:"negative_kind,omitempty"`
	TaskContext              string     `json:"task_context"`
	DecisionPoint            string     `json:"decision_point"`
	EvidenceCutoff           string     `json:"evidence_cutoff"`
	PreDecisionEvidence      []evidence `json:"pre_decision_evidence"`
	EventualSolutionIDs      []string   `json:"eventual_solution_ids"`
	NearDuplicateSolutionIDs []string   `json:"near_duplicate_solution_ids"`
	Executable               bool       `json:"executable"`
	Expected                 expected   `json:"expected"`
}

type corpus struct {
	Version     int    `json:"version"`
	Description string `json:"description"`
	Split       string `json:"split"`
	Cases       []kase `json:"cases"`
}

type state struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type classification struct {
	Kind        string `json:"kind"`
	Disposition string `json:"disposition"`
}

type validation struct {
	Results []struct {
		Category string `json:"Category"`
		Command  string `json:"Command"`
		Status   string `json:"Status"`
	} `json:"Results"`
}

type trace struct {
	RepositoryMutations int `json:"repository_mutations"`
	Iterations          []struct {
		Action string `json:"action"`
	} `json:"iterations"`
}

func main() {
	runs := flag.String("runs", ".agent-sdlc/runs", "recorded run directory")
	out := flag.String("out", "evals/decision-memory-pilot", "output directory")
	tuningPercent := flag.Int("tuning-percent", 50, "percentage of task families assigned to tuning")
	flag.Parse()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	entries, err := os.ReadDir(*runs)
	if err != nil {
		fail(err)
	}
	var tuning, heldout []kase
	excluded := map[string]int{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(*runs, e.Name())
		k, reason := buildCase(dir, e.Name())
		if reason != "" {
			excluded[reason]++
			continue
		}
		if inTuning(k.ID, *tuningPercent) {
			tuning = append(tuning, k)
		} else {
			heldout = append(heldout, k)
		}
	}
	sort.Slice(tuning, func(i, j int) bool { return tuning[i].ID < tuning[j].ID })
	sort.Slice(heldout, func(i, j int) bool { return heldout[i].ID < heldout[j].ID })

	write(filepath.Join(*out, "corpus-tuning.json"), corpus{
		Version: 1, Split: "tuning",
		Description: "Source-backed planning decisions (tuning split).",
		Cases:       tuning,
	})
	write(filepath.Join(*out, "corpus-heldout.json"), corpus{
		Version: 1, Split: "heldout",
		Description: "Source-backed planning decisions (held-out split).",
		Cases:       heldout,
	})
	fmt.Printf("tuning=%d heldout=%d\n", len(tuning), len(heldout))
	for _, r := range sortedKeys(excluded) {
		fmt.Printf("excluded %-28s %d\n", r, excluded[r])
	}
}

func buildCase(dir, runID string) (kase, string) {
	clsRaw, err := os.ReadFile(filepath.Join(dir, "classification.json"))
	if err != nil {
		return kase{}, "no classification"
	}
	var cls classification
	if json.Unmarshal(clsRaw, &cls) != nil || cls.Disposition == "" {
		return kase{}, "unreadable classification"
	}
	taskRaw, err := os.ReadFile(filepath.Join(dir, "task.md"))
	if err != nil {
		return kase{}, "no task input"
	}
	stateRaw, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		return kase{}, "no decision timestamp"
	}
	var st state
	if json.Unmarshal(stateRaw, &st) != nil || st.CreatedAt == "" || st.UpdatedAt == "" {
		return kase{}, "no decision timestamp"
	}

	cutoff := st.UpdatedAt
	var ev []evidence
	if trRaw, err := os.ReadFile(filepath.Join(dir, "trace.json")); err == nil {
		var tr trace
		if json.Unmarshal(trRaw, &tr) == nil {
			ev = append(ev, mkEvidence(fmt.Sprintf("execution: repository_mutations=%d iterations=%d", tr.RepositoryMutations, len(tr.Iterations)), cutoff))
			var actions []string
			for i := len(tr.Iterations) - 1; i >= 0 && len(actions) < 5; i-- {
				if a := strings.TrimSpace(tr.Iterations[i].Action); a != "" {
					actions = append(actions, a)
				}
			}
			if len(actions) > 0 {
				ev = append(ev, mkEvidence("recent operations: "+strings.Join(actions, " | "), cutoff))
			}
		}
	}
	if valRaw, err := os.ReadFile(filepath.Join(dir, "validation.json")); err == nil {
		var v validation
		if json.Unmarshal(valRaw, &v) == nil {
			for i, r := range v.Results {
				if i >= 6 {
					break
				}
				ev = append(ev, mkEvidence(fmt.Sprintf("validation %s: %s -> %s", r.Category, r.Command, r.Status), cutoff))
			}
		}
	}
	if len(ev) == 0 {
		return kase{}, "no pre-decision evidence"
	}

	text := string(taskRaw)
	if r := []rune(text); len(r) > 2000 {
		text = string(r[:2000])
	}
	class := "planning"
	if cls.Disposition == "BLOCK" || cls.Disposition == "NEEDS_HUMAN" {
		class = "negative"
	}
	return kase{
		ID:                       "DM-" + runID,
		Class:                    class,
		TaskContext:              strings.TrimSpace(text),
		DecisionPoint:            st.UpdatedAt,
		EvidenceCutoff:           cutoff,
		PreDecisionEvidence:      ev,
		EventualSolutionIDs:      []string{},
		NearDuplicateSolutionIDs: []string{},
		Executable:               false,
		Expected: expected{
			Kind:      "plan",
			Decision:  cls.Disposition,
			Rationale: fmt.Sprintf("recorded SOP disposition %s for run %s", cls.Disposition, runID),
		},
	}, ""
}

func mkEvidence(text, cutoff string) evidence {
	sum := sha256.Sum256([]byte(text))
	return evidence{RecordID: "ev-" + hex.EncodeToString(sum[:6]), Text: text, ContentHash: hex.EncodeToString(sum[:]), ProducedBefore: cutoff}
}

func inTuning(id string, pct int) bool {
	family := strings.SplitN(id, "-", 3)
	key := id
	if len(family) >= 2 {
		key = family[0] + "-" + family[1]
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32()%100) < pct
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func write(path string, c corpus) {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "rdm-pilot-corpus: %v\n", err)
	os.Exit(1)
}
