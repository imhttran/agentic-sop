package security

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOWASPLLMReportsPresent is the deterministic acceptance check for the LLM04
// report deliverables. It proves the reports exist and are non-empty, and that
// the plan inventory references this workstream. It does not judge the report's
// prose; SOP's review and the operator do that.
func TestOWASPLLMReportsPresent(t *testing.T) {
	for _, rel := range []string{
		filepath.Join("docs", "reports", "owasp-llm", "llm04-supply-chain.md"),
		filepath.Join("docs", "reports", "owasp-llm", "README.md"),
	} {
		data, err := os.ReadFile(filepath.Join("..", "..", rel))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		if strings.TrimSpace(string(data)) == "" {
			t.Fatalf("%s is empty", rel)
		}
	}
	index, err := os.ReadFile(filepath.Join("..", "..", "docs", "plans", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "PLAN-04-SOP-LLM04-Supply-Chain.md") {
		t.Fatal("docs/plans/README.md does not reference PLAN-04-SOP-LLM04-Supply-Chain.md")
	}
}
