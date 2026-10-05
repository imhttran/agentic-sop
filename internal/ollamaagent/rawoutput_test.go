package ollamaagent

import (
	"strings"
	"testing"
)

func TestRawOutputRequirementInUserPrompt(t *testing.T) {
	req := implementRequest()
	req.RawOutputEvidence = true
	if p := userPrompt(req); !strings.Contains(p, "RAW command output") || !strings.Contains(p, "does NOT satisfy") {
		t.Errorf("userPrompt missing the raw-output requirement:\n%s", p)
	}
	if strings.Contains(userPrompt(implementRequest()), "RAW command output") {
		t.Error("a task without the requirement must not carry it")
	}
}
