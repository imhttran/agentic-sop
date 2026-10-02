package ollamaagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

const satisfiedFinal = `{"status":"completed","completion":"ALREADY_SATISFIED","changes_expected":true,"repository_mutations":99,"evidence":{"acceptance":[{"criterion":"Add returns the sum","paths":["./add.go","add_test.go"]}],"validation_commands":["fabricated"]}}`
const readAdd = `{"tool":"read_file","args":{"path":"add.go"}}`
const readAddTest = `{"tool":"read_file","args":{"path":"add_test.go"}}`
const validateAdd = `{"tool":"run_command","args":{"command":"go test ./..."}}`

func satisfactionFixture(t *testing.T) (string, agent.Request) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/add\n\ngo 1.22\n")
	writeFile(t, dir, "add.go", "package add\n\nfunc Add(a, b int) int { return a + b }\n")
	writeFile(t, dir, "add_test.go", "package add\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(1, 2) != 3 { t.Fatal(\"wrong sum\") } }\n")
	req := implementRequest()
	req.AcceptanceCriteria = []string{"Add returns the sum"}
	req.ValidationCommands = []string{"go test ./..."}
	return dir, req
}

func TestVerifiedAlreadySatisfied(t *testing.T) {
	for _, cap := range []agent.Capability{agent.Implement, agent.Fix} {
		t.Run(string(cap), func(t *testing.T) {
			dir, req := satisfactionFixture(t)
			// Legitimate partial work is accepted; dirtiness alone supplies no proof.
			gitInit(t, dir)
			commitAll(t, dir, "existing implementation")
			writeFile(t, dir, "unrelated.txt", "pre-existing work")
			req.Capability = cap
			fake, srv := newFakeOllama(t, readAdd, readAddTest, validateAdd, satisfiedFinal)
			cfg := testConfig(srv.URL)
			cfg.MaxToolCalls = 20
			native := NewNativeAgent(cfg, dir, nil)
			response, err := native.Generate(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if !response.VerifiedAlreadySatisfied || response.Outcome == nil || response.Outcome.Status != agent.OutcomeCompleted || response.Outcome.Completion != agent.AlreadySatisfied {
				t.Fatalf("response=%+v content=%s", response, response.Content)
			}
			if response.Outcome.ChangesExpected || len(response.ChangedFiles) != 0 {
				t.Fatalf("fabricated mutation evidence: %+v", response)
			}
			var wire outcomeWire
			if err := json.Unmarshal([]byte(response.Content), &wire); err != nil {
				t.Fatal(err)
			}
			if wire.RepositoryMutations == nil || *wire.RepositoryMutations != 0 || wire.Evidence == nil || len(wire.Evidence.ValidationCommands) != 1 || wire.Evidence.ValidationCommands[0] != "go test ./..." {
				t.Fatalf("completion evidence=%s", response.Content)
			}
			if fake.count() != 4 {
				t.Fatalf("calls=%d", fake.count())
			}
		})
	}
}

func TestAlreadySatisfiedRequiresObjectiveProof(t *testing.T) {
	for _, cap := range []agent.Capability{agent.Implement, agent.Fix} {
		for _, tc := range []struct {
			name  string
			turns []string
			setup func(*testing.T, string, *agent.Request)
		}{
			{name: "claim only"},
			{name: "reads only", turns: []string{readAdd, readAddTest}},
			{name: "no-op write", turns: []string{`{"tool":"write_file","args":{"path":"add.go","content":"package add\n\nfunc Add(a, b int) int { return a + b }\n"}}`}},
			{name: "no-op gofmt", turns: []string{`{"tool":"run_command","args":{"command":"gofmt -w add.go"}}`}},
			{name: "failing validation", turns: []string{readAdd, readAddTest, validateAdd}, setup: func(t *testing.T, dir string, _ *agent.Request) {
				writeFile(t, dir, "add.go", "package add\n\nfunc Add(a, b int) int { return 0 }\n")
			}},
			{name: "failed read", turns: []string{`{"tool":"read_file","args":{"path":"missing.go"}}`, readAddTest, validateAdd}},
			{name: "denied command", turns: []string{readAdd, readAddTest, `{"tool":"run_command","args":{"command":"rm -rf ."}}`}},
			{name: "missing criterion", turns: []string{readAdd, readAddTest, validateAdd}, setup: func(_ *testing.T, _ string, req *agent.Request) {
				req.AcceptanceCriteria = append(req.AcceptanceCriteria, "second criterion")
			}},
			{name: "missing validation", turns: []string{readAdd, readAddTest, validateAdd}, setup: func(_ *testing.T, _ string, req *agent.Request) {
				req.ValidationCommands = append(req.ValidationCommands, "go vet ./...")
			}},
			{name: "no caller contract", turns: []string{readAdd, readAddTest, validateAdd}, setup: func(_ *testing.T, _ string, req *agent.Request) { req.AcceptanceCriteria = nil }},
			{name: "unread evidence path", turns: []string{readAdd, validateAdd}},
			{name: "real mutation cannot claim already satisfied", turns: []string{readAdd, readAddTest, `{"tool":"create_file","args":{"path":"new.txt","content":"new"}}`, validateAdd}},
		} {
			t.Run(string(cap)+"/"+tc.name, func(t *testing.T) {
				dir, req := satisfactionFixture(t)
				req.Capability = cap
				if tc.setup != nil {
					tc.setup(t, dir, &req)
				}
				turns := append(append([]string(nil), tc.turns...), satisfiedFinal)
				_, srv := newFakeOllama(t, turns...)
				cfg := testConfig(srv.URL)
				cfg.MaxToolCalls = 20
				h := New(cfg, dir)
				ev := &mutationEvidence{}
				_, _, err := h.CompleteWithEvidence(context.Background(), req, ev)
				if err == nil || ev.alreadySatisfied {
					t.Fatalf("unverified claim accepted: err=%v evidence=%+v", err, ev)
				}
			})
		}
	}
}

func TestAlreadySatisfiedDoesNotBypassNoProgress(t *testing.T) {
	for _, cap := range []agent.Capability{agent.Implement, agent.Fix} {
		for _, shape := range []string{"narration", "repeated discovery", "no-op writes", "validation without explicit completion"} {
			t.Run(string(cap)+"/"+shape, func(t *testing.T) {
				dir, req := satisfactionFixture(t)
				req.Capability = cap
				var turns []string
				for i := 0; i < 10; i++ {
					turn := stalledNarration
					switch shape {
					case "repeated discovery":
						turn = readAdd
					case "no-op writes":
						turn = `{"tool":"write_file","args":{"path":"add.go","content":"package add\n\nfunc Add(a, b int) int { return a + b }\n"}}`
					case "validation without explicit completion":
						if i == 0 {
							turn = validateAdd
						}
					}
					turns = append(turns, turn)
				}
				fake, srv := newFakeOllama(t, turns...)
				cfg := testConfig(srv.URL)
				cfg.MaxToolCalls = 20
				ev := &mutationEvidence{}
				h := New(cfg, dir)
				_, err := h.ExecuteWithEvidence(context.Background(), req, ev)
				if err == nil || ev.observed || ev.alreadySatisfied || !strings.Contains(err.Error(), "termination=no_progress") || !strings.Contains(err.Error(), string(cap)) {
					t.Fatalf("err=%v evidence=%+v", err, ev)
				}
				if fake.count() > maxNoProgressIterations+1 {
					t.Fatalf("stale budget exceeded: calls=%d", fake.count())
				}
			})
		}
	}
}

func TestAlreadySatisfiedUnavailableSnapshotFailsClosed(t *testing.T) {
	dir, req := satisfactionFixture(t)
	_, srv := newFakeOllama(t, readAdd, readAddTest, validateAdd, satisfiedFinal)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 20
	h := New(cfg, dir)
	h.tools = toolharness.New(dir, toolharness.Config{CommandTimeout: -time.Nanosecond}, h.audit)
	ev := &mutationEvidence{}
	_, err := h.ExecuteWithEvidence(context.Background(), req, ev)
	if err == nil || ev.alreadySatisfied || ev.observed {
		t.Fatalf("err=%v evidence=%+v", err, ev)
	}
}

func TestAlreadySatisfiedAfterFiveInspections(t *testing.T) {
	for _, cap := range []agent.Capability{agent.Implement, agent.Fix} {
		t.Run(string(cap), func(t *testing.T) {
			dir, req := satisfactionFixture(t)
			req.Capability = cap
			writeFile(t, dir, "docs/contract.md", "Add returns the sum.")
			fake, srv := newFakeOllama(t, readAdd, readAddTest,
				`{"tool":"list_files","args":{"path":"."}}`,
				`{"tool":"read_file","args":{"path":"docs/contract.md"}}`,
				`{"tool":"list_files","args":{"path":"docs"}}`, validateAdd, satisfiedFinal)
			cfg := testConfig(srv.URL)
			cfg.MaxToolCalls = 20
			h := New(cfg, dir)
			ev := &mutationEvidence{}
			content, _, err := h.CompleteWithEvidence(context.Background(), req, ev)
			if err != nil || !ev.alreadySatisfied || ev.observed || len(ev.paths) != 0 {
				t.Fatalf("content=%s err=%v evidence=%+v", content, err, ev)
			}
			if !hasEvent(h.TraceRecords(), agent.AlreadySatisfied) || hasEvent(h.TraceRecords(), implementChangeEvent) {
				t.Fatalf("trace=%+v", h.TraceRecords())
			}
			if fake.count() != 7 || len(h.AuditRecords()) != 6 {
				t.Fatalf("calls=%d audit=%+v", fake.count(), h.AuditRecords())
			}
			// The existing implementation nudge fires after validation, but explicitly
			// retains the evidence-backed completion route rather than demanding an edit.
			if !strings.Contains(messageText(fake.request(6)), alreadySatisfiedInstruction) {
				t.Fatal("nudge omitted already-satisfied contract")
			}
		})
	}
}
