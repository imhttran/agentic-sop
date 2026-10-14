package nimble

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/decision"
)

// goldenPath is the verified real Nimble response. The path is relative to the
// nimble package directory, walking up to the repository root.
const goldenPath = "../../../tests/fixtures/nimble/systemone_response.json"

// TestGoldenSystemOneResponseParses drives the production response parser from
// the real captured Nimble fixture, proving the contract offline.
func TestGoldenSystemOneResponseParses(t *testing.T) {
	data, err := os.ReadFile(filepath.Clean(goldenPath))
	if err != nil {
		t.Fatalf("read golden fixture: %v", err)
	}

	var resp systemOneResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal golden fixture: %v", err)
	}

	got, err := NormalizeSystemOneResponse(resp, exampleRequest())
	if err != nil {
		t.Fatalf("NormalizeSystemOneResponse() error = %v", err)
	}

	if got.Provider != ProviderName || got.Model != "nimble" {
		t.Errorf("Provider/Model = %q/%q", got.Provider, got.Model)
	}
	if len(got.Answers) != 3 {
		t.Fatalf("len(Answers) = %d, want 3", len(got.Answers))
	}

	risk := got.Answers["risk"]
	if risk.Type != decision.AnswerChoice || risk.Choice != "HIGH" {
		t.Errorf("risk = %+v", risk)
	}
	if risk.Confidence == nil || got.Answers["risk"].Probabilities["HIGH"] == 0 {
		t.Errorf("risk probabilities/confidence not preserved: %+v", risk)
	}

	appr := got.Answers["approval_required"]
	if appr.Type != decision.AnswerBoolean || appr.Probability == 0 {
		t.Errorf("approval_required = %+v", appr)
	}

	exec := got.Answers["execution_model"]
	if exec.Type != decision.AnswerChoice || exec.Choice != "LARGE" {
		t.Errorf("execution_model = %+v", exec)
	}

	if got.Usage.InputTokens != 983 || got.Usage.OutputTokens != 4 {
		t.Errorf("Usage = %+v, want 983/4", got.Usage)
	}
}
