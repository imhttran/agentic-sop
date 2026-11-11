package agent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/redact"
)

// TestRedactedBeforeChatTransports proves that model-bound text is scrubbed before
// each chat transport sends it. A known-format token and a key=value pair are
// removed with or without C1; an inventory value is removed only when C1 is enabled,
// which is the positive control that the policy is really wired through.
func TestRedactedBeforeChatTransports(t *testing.T) {
	const (
		token     = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
		pair      = "api_key=sk-canary-5521"
		inventory = "inventory-canary-7741"
	)
	task := "use " + token + " with " + pair + " and " + inventory

	transports := []struct {
		name  string
		path  string
		reply string
		build func(baseURL string) (Agent, error)
	}{
		{
			name:  "openai_compatible",
			path:  "/v1/chat/completions",
			reply: `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`,
			build: func(u string) (Agent, error) {
				return NewOpenAICompatible(ProviderOpenAICompatible, u, "m", "", 5*time.Second)
			},
		},
		{
			name:  "ollama",
			path:  "/api/chat",
			reply: `{"message":{"role":"assistant","content":"ok"},"done_reason":"stop","prompt_eval_count":1,"eval_count":1}`,
			build: func(u string) (Agent, error) {
				return NewOllama(u, "m", 5*time.Second)
			},
		},
	}

	for _, tr := range transports {
		for _, enabled := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/c1=%t", tr.name, enabled), func(t *testing.T) {
				var body []byte
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != tr.path {
						http.NotFound(w, r)
						return
					}
					body, _ = io.ReadAll(r.Body)
					_, _ = w.Write([]byte(tr.reply))
				}))
				defer srv.Close()

				a, err := tr.build(srv.URL)
				if err != nil {
					t.Fatal(err)
				}
				policy := redact.Policy{Enabled: enabled, Values: []string{inventory}}
				agent := NewChecked(NewRedacted(a, policy))
				if _, err := agent.Generate(context.Background(), Request{Capability: Plan, Task: task}); err != nil {
					t.Fatalf("generate: %v", err)
				}

				sent := string(body)
				if strings.Contains(sent, token) || strings.Contains(sent, pair) {
					t.Errorf("baseline secret reached the model body: %s", sent)
				}
				if got := strings.Contains(sent, inventory); got != !enabled {
					t.Errorf("inventory value present=%t with c1=%t; want present=%t", got, enabled, !enabled)
				}
			})
		}
	}
}

// TestRenderPromptSendsOnlyRedactedFields guards the field list Redacted scrubs. The
// chat transports render only Task, Input, and OutputRequirements. If a later change
// renders another Request field, this fails so that field gets redaction too.
// ValidationCommands, Deliverables, and the other caller-owned lists are deliberately
// not scrubbed, since changing them would change the verification contract.
func TestRenderPromptSendsOnlyRedactedFields(t *testing.T) {
	const canary = "canary-field-4418"
	req := Request{
		Capability:         Plan,
		Task:               "task",
		Input:              "input",
		OutputRequirements: "reqs",
		AcceptanceCriteria: []string{canary},
		ValidationCommands: []string{canary},
		Deliverables:       []string{canary},
		ValidationWaivers:  []string{canary},
	}
	if got := renderPrompt(req); strings.Contains(got, canary) {
		t.Fatalf("renderPrompt sends a field Redacted does not scrub:\n%s", got)
	}
}
