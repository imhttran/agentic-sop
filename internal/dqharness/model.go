package dqharness

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/imhttran/agentic-sop/internal/decisionquality"
	"github.com/imhttran/agentic-sop/internal/retrievalboundary"
)

// ChatFunc is one single-turn model call. It is injectable so tests use fixtures and
// production reuses the configured provider client.
type ChatFunc func(ctx context.Context, model, prompt string) (string, error)

// DecisionSet is the frozen set of decision tokens a case may answer with.
func DecisionSet(c decisionquality.Corpus) []string {
	seen := map[string]bool{}
	var out []string
	for _, cs := range c.Cases {
		d := strings.TrimSpace(cs.Expected.Decision)
		if d != "" && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}

// BuildPrompt renders the deterministic, arm-blind prompt. The only difference between
// arms is the evidence block; everything else is identical, so a score difference cannot
// come from prompt wording.
func BuildPrompt(c decisionquality.Case, retrieved []decisionquality.EvidenceItem, decisions []string) string {
	var b strings.Builder
	b.WriteString("You are answering one historical engineering decision.\n")
	b.WriteString("Use only the task context and the evidence below. Do not assume facts that are absent.\n\n")
	b.WriteString("Task context:\n")
	b.WriteString(strings.TrimSpace(c.TaskContext))
	b.WriteString("\n\nEvidence available before the decision:\n")
	if len(retrieved) == 0 {
		b.WriteString("(none)\n")
	} else {
		for _, ev := range retrieved {
			fmt.Fprintf(&b, "- [%s] %s\n", ev.RecordID, strings.TrimSpace(ev.Text))
		}
	}
	b.WriteString("\nAnswer with exactly one decision token and nothing else. Allowed tokens:\n")
	for _, d := range decisions {
		fmt.Fprintf(&b, "- %s\n", d)
	}
	return b.String()
}

// ParseDecision extracts the first allowed decision token from model output. Output is
// untrusted: an unknown or empty answer is an explicit error, never a default.
func ParseDecision(out string, decisions []string) (string, error) {
	allowed := map[string]string{}
	for _, d := range decisions {
		allowed[strings.ToLower(d)] = d
	}
	for _, field := range strings.FieldsFunc(out, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	}) {
		if d, ok := allowed[strings.ToLower(field)]; ok {
			return d, nil
		}
	}
	return "", fmt.Errorf("dqharness: model output contains no allowed decision token")
}

// ModelDecision adapts a single-turn chat function into a DecisionFunc. It returns the
// tokens used by the prompt and the observed latency. Errors are returned unchanged so
// the runner can classify and record them.
func ModelDecision(chat ChatFunc, model string, decisions []string) DecisionFunc {
	return func(ctx context.Context, c decisionquality.Case, retrieved []decisionquality.EvidenceItem) (string, int, int, error) {
		prompt := BuildPrompt(c, retrieved, decisions)
		start := time.Now()
		out, err := chat(ctx, model, prompt)
		latency := int(time.Since(start).Milliseconds())
		if err != nil {
			return "", retrievalboundary.EstimateTokens(prompt), latency, err
		}
		d, err := ParseDecision(out, decisions)
		if err != nil {
			return "", retrievalboundary.EstimateTokens(prompt) + retrievalboundary.EstimateTokens(out), latency, err
		}
		return d, retrievalboundary.EstimateTokens(prompt) + retrievalboundary.EstimateTokens(out), latency, nil
	}
}
