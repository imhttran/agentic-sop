package skill

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestDecisionABSkillContract proves the shipped /decision-ab skill is a thin,
// fail-closed client of the repository benchmark: it discovers the served ids,
// runs the smoke split before the held-out split, scores the first production
// attempt, reports consequential BLOCK/NEEDS_HUMAN disagreements, and never
// substitutes a model or changes a default.
func TestDecisionABSkillContract(t *testing.T) {
	body := normalizeSpaces(read(t, filepath.Join(skillsRoot, "decision-ab", "SKILL.md")))
	for _, want := range []string{
		"disable-model-invocation: false",
		"POST /v1/systemone",
		"evals/decision-model/disposition-ab-v1.json",
		"DISPOSITION-AB.sha256",
		"DISPOSITION-AB-PROMPT.sha256",
		"-discover",
		"-mode quick", "-mode full", "-mode all",
		"-concurrency", "-timeout", "-out",
		"no retry", "no fallback",
		"one model at a time",
		"first-pass correct / attempted",
		"schema validity",
		"BLOCK or NEEDS_HUMAN",
		"correct first-pass decisions per minute",
		"baseline only",
		"HTTP 409",
		"PATH",
		"sop status",
		"do not substitute another model",
		"do not change a model default",
		"Run from the SOP checkout",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("decision-ab contract missing %q", want)
		}
	}
	for _, forbidden := range ModelLiterals {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Errorf("decision-ab skill contains a model literal %q", forbidden)
		}
	}
}

// TestDecisionABCommandsAreTheBenchmark proves the skill's shell examples are the
// benchmark command and the SOP status call, not a second runner or a direct
// provider invocation.
func TestDecisionABCommandsAreTheBenchmark(t *testing.T) {
	body := read(t, filepath.Join(skillsRoot, "decision-ab", "SKILL.md"))
	var bench, sopLines int
	for _, line := range bashCommandLines(body) {
		if strings.HasPrefix(line, "go run ./cmd/deceleval-ab ") && !strings.Contains(line, "-endpoint <endpoint>") {
			t.Errorf("benchmark command must carry the discovered endpoint: %q", line)
		}
		switch {
		case strings.HasPrefix(line, "go run ./cmd/deceleval-ab "):
			bench++
		case strings.HasPrefix(line, "sop status"):
			sopLines++
		default:
			t.Errorf("unexpected shell example in decision-ab skill: %q", line)
		}
	}
	if bench == 0 {
		t.Error("decision-ab must show the benchmark command")
	}
	if sopLines == 0 {
		t.Error("decision-ab must show the SOP status preflight")
	}
}
