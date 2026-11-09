package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOWASPLLMDisclosureReportPresent is the deterministic acceptance check for
// the Plan 03 report deliverable. It proves the report exists and is non-empty;
// it does not judge the prose.
func TestOWASPLLMDisclosureReportPresent(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "reports", "owasp-llm", "llm02-disclosure.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		t.Fatalf("%s is empty", path)
	}
}
