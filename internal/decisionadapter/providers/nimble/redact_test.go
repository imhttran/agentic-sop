package nimble

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/decision"
)

// TestBuildSystemOneRequestRedactsModelText proves that the state and per-question
// instructions sent to the hosted service are scrubbed. Choices are not scrubbed,
// because they are the answer keys the model must echo back.
func TestBuildSystemOneRequestRedactsModelText(t *testing.T) {
	const (
		token = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
		pair  = "api_key=sk-canary-5521"
	)
	req := decision.DecisionRequest{
		State: "deploy with " + token,
		Questions: []decision.Question{{
			ID:           "q1",
			Type:         decision.QuestionChoice,
			Criteria:     "risk",
			Choices:      []string{"LOW", "HIGH"},
			Instructions: "check " + pair,
		}},
	}
	out, err := BuildSystemOneRequest("m", req)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	sent := string(data)
	if strings.Contains(sent, token) || strings.Contains(sent, pair) {
		t.Fatalf("secret reached the SystemOne body: %s", sent)
	}
	if !strings.Contains(sent, `"LOW"`) || !strings.Contains(sent, `"HIGH"`) {
		t.Fatalf("choices were altered: %s", sent)
	}
}
