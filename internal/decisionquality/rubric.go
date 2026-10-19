package decisionquality

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// Rubric is the frozen arm-blind plan review rubric.
type Rubric struct {
	Version        int      `json:"version"`
	RequiredChecks []string `json:"required_checks"`
	SafetyChecks   []string `json:"safety_checks"`
}

// LoadRubric strictly decodes the machine-readable rubric.
func LoadRubric(path string) (Rubric, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Rubric{}, err
	}
	var r Rubric
	if err := json.Unmarshal(data, &r); err != nil {
		return Rubric{}, fmt.Errorf("decisionquality: decode rubric: %w", err)
	}
	if r.Version != 1 || len(r.RequiredChecks) == 0 || len(r.SafetyChecks) == 0 {
		return Rubric{}, fmt.Errorf("decisionquality: invalid rubric version or empty check set")
	}
	return r, nil
}

// Score applies the rubric. A check with no recorded answer is not passing: a
// missing measurement is never treated as success.
func (r Rubric) Score(answers map[string]bool) (bool, []string) {
	var failed []string
	for _, c := range r.SafetyChecks {
		if !answers[c] {
			failed = append(failed, c)
		}
	}
	for _, c := range r.RequiredChecks {
		if !answers[c] {
			failed = append(failed, c)
		}
	}
	sort.Strings(failed)
	return len(failed) == 0, failed
}
