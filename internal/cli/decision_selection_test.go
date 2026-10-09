package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/decision"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// TestRecordApproachSelectionWritesArtifactAndSummary proves the recording seam:
// the alternatives, the selected approach, and the rationale are persisted to the
// run artifact, and the concise choice/reason summary is rendered.
func TestRecordApproachSelectionWritesArtifactAndSummary(t *testing.T) {
	dir := t.TempDir()
	rn, err := runpkg.New(dir, "T001")
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	sel := recordApproachSelection(rn, []decision.Alternative{
		{Name: "refactor", Evidence: decision.EvidenceStrong, Risk: decision.RiskLow, Reversible: true, Cost: 2},
		{Name: "rewrite", Evidence: decision.EvidencePartial, Risk: decision.RiskHigh, Reversible: false, Cost: 1},
	})
	if sel == nil || sel.Approach != "refactor" {
		t.Fatalf("selection = %+v, want approach refactor", sel)
	}

	b, err := os.ReadFile(filepath.Join(rn.Dir(), "decision-selection.json"))
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	var doc decision.Selection
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal artifact: %v", err)
	}
	if doc.Approach != "refactor" || len(doc.Alternatives) != 2 {
		t.Errorf("artifact = %+v, want approach refactor with 2 alternatives", doc)
	}

	var sb strings.Builder
	writeDecisionSummary(&sb, sel)
	if !strings.Contains(sb.String(), "decision: LOW") {
		t.Errorf("summary missing choice: %q", sb.String())
	}
	if !strings.Contains(sb.String(), "reason:") {
		t.Errorf("summary missing reason: %q", sb.String())
	}
}

func TestApproachSelectionNoAlternativesIsNoop(t *testing.T) {
	if sel := approachSelection(nil); sel != nil {
		t.Fatalf("selection = %+v, want nil", sel)
	}
}
