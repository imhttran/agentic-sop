package clef

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/decision"
)

// TestBuildChatRequestRedactsUserMessage proves that the state and question text in
// the chat user message are scrubbed before the transport sends them.
func TestBuildChatRequestRedactsUserMessage(t *testing.T) {
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
	out, err := BuildChatRequest("m", req)
	if err != nil {
		t.Fatal(err)
	}
	user := out.Messages[len(out.Messages)-1].Content
	if strings.Contains(user, token) || strings.Contains(user, pair) {
		t.Fatalf("secret reached the chat user message: %s", user)
	}
}
