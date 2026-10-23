package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/mcp"
)

func toolByName(t *testing.T, tools []mcp.Tool, name string) mcp.Tool {
	t.Helper()
	for _, tl := range tools {
		if tl.Name == name {
			return tl
		}
	}
	t.Fatalf("tool %s not found", name)
	return mcp.Tool{}
}

// TestMCPPlannerSurfaceShape pins the planner-scoped tool set and read-only hints.
func TestMCPPlannerSurfaceShape(t *testing.T) {
	dir := t.TempDir()
	tools := mcpPlannerTools(dir, deps{})
	got := map[string]mcp.Tool{}
	for _, tl := range tools {
		got[tl.Name] = tl
	}
	want := []string{"sop_status", "sop_question_list", "sop_question_get", "sop_answer_submit", "sop_task_result", "sop_decision_list", "sop_decision_submit"}
	if len(tools) != len(want) {
		t.Errorf("planner surface has %d tools, want %d: %v", len(tools), len(want), got)
	}
	for _, n := range want {
		if _, ok := got[n]; !ok {
			t.Errorf("planner surface missing %s", n)
		}
	}
	// Human approval actions (and validate/review) must never be on the planner surface.
	for _, forbidden := range []string{"sop_approve", "sop_decline", "sop_approval", "sop_validate", "sop_review"} {
		if _, ok := got[forbidden]; ok {
			t.Errorf("planner surface must not expose %s", forbidden)
		}
	}
	for _, ro := range []string{"sop_status", "sop_question_list", "sop_question_get", "sop_task_result", "sop_decision_list"} {
		if !got[ro].ReadOnly {
			t.Errorf("%s must be read-only", ro)
		}
	}
	for _, rw := range []string{"sop_answer_submit", "sop_decision_submit"} {
		if got[rw].ReadOnly {
			t.Errorf("%s must not be marked read-only", rw)
		}
	}
}

// TestMCPPlannerServerSurface drives the real server: list/get/submit work, and the
// forbidden approval tools are rejected; the submit records ANSWERED only and leaves
// the task lifecycle and approval state unchanged.
func TestMCPPlannerServerSurface(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	writePlanMeta(t, dir, "plan-x", "abc")
	q := seedQuestion(t, dir, "T001", "plan-x@abc")

	input := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize"}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"sop_question_list","arguments":{}}}
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"sop_question_get","arguments":{"question_id":%q}}}
{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"sop_answer_submit","arguments":{"question_id":%q,"body":"sort","answered_by":"planner","idempotency_key":"k1","expected_revision":1}}}
{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"sop_approve","arguments":{"taskId":"T001"}}}
{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"sop_decline","arguments":{"taskId":"T001"}}}
`, q.ID, q.ID)

	resps := serveMCP(t, mcpPlannerTools(dir, deps{}), input)
	if len(resps) != 7 {
		t.Fatalf("got %d responses, want 7", len(resps))
	}

	// tools/list: exactly the planner tools.
	list := resps[1]["result"].(map[string]any)["tools"].([]any)
	if len(list) != 7 {
		t.Fatalf("tools/list = %d tools, want 7", len(list))
	}
	seen := map[string]bool{}
	for _, raw := range list {
		seen[raw.(map[string]any)["name"].(string)] = true
	}
	for _, n := range []string{"sop_status", "sop_question_list", "sop_question_get", "sop_answer_submit", "sop_task_result", "sop_decision_list", "sop_decision_submit"} {
		if !seen[n] {
			t.Errorf("tools/list missing %s", n)
		}
	}

	// list shows the pending question.
	if text := toolResultText(t, resps[2]); !strings.Contains(text, q.ID) {
		t.Errorf("question_list text = %q, want the seeded id", text)
	}
	// get returns the question (still PENDING at this point).
	if text := toolResultText(t, resps[3]); !strings.Contains(text, "PENDING") {
		t.Errorf("question_get text = %q, want PENDING", text)
	}
	// submit is accepted.
	if text := toolResultText(t, resps[4]); !strings.Contains(text, "ACCEPTED") {
		t.Errorf("answer_submit text = %q, want ACCEPTED", text)
	}
	// forbidden tools are rejected by tools/call.
	for i, name := range []string{"sop_approve", "sop_decline"} {
		resp := resps[5+i]
		if _, isResult := resp["result"]; isResult {
			t.Fatalf("%s returned a result, want a JSON-RPC error", name)
		}
		rpcErr, ok := resp["error"].(map[string]any)
		if !ok || rpcErr["code"].(float64) != -32602 {
			t.Fatalf("%s error = %v, want -32602", name, resp["error"])
		}
	}

	// The submit advanced only the question; the task and approvals are unchanged.
	if _, out, _ := runCLI(t, dir, "question", q.ID); !strings.Contains(out, "ANSWERED") {
		t.Errorf("question not ANSWERED after submit: %q", out)
	}
	if _, out, _ := runCLI(t, dir, "status"); !strings.Contains(out, "T001") || !strings.Contains(out, "PLANNED") {
		t.Errorf("task changed after submit: %q", out)
	}
	if _, out, _ := runCLI(t, dir, "approvals"); !strings.Contains(out, "no pending approvals") {
		t.Errorf("approvals changed after submit: %q", out)
	}
}

// TestMCPPlannerStatusOnlyMutuallyExclusive pins the flag contract.
func TestMCPPlannerStatusOnlyMutuallyExclusive(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runMCP([]string{"--planner", "--status-only"}, &out, &errOut, deps{}); code != exitUsage {
		t.Errorf("code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut.String(), "mutually exclusive") {
		t.Errorf("stderr = %q, want a mutual-exclusion message", errOut.String())
	}
}

// TestMCPPlannerServerHasNoInitializationGate pins the server side of the observed
// tunnel-client 409 (mcp_initialization_required, SPIKE-005 §6). The planner server
// keeps no session state: a tools/call is answered without a preceding initialize,
// and parallel read sessions (one question-get and one status each, some without an
// initialize) all succeed and leave the question untouched. The 409 therefore
// originates in tunnel-client, not here.
func TestMCPPlannerServerHasNoInitializationGate(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	writePlanMeta(t, dir, "plan-x", "abc")
	q := seedQuestion(t, dir, "T001", "plan-x@abc")

	get := fmt.Sprintf(`{"jsonrpc":"2.0","id":%%d,"method":"tools/call","params":{"name":"sop_question_get","arguments":{"question_id":%q}}}`, q.ID)
	status := `{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"sop_status","arguments":{}}}`
	handshake := `{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"

	const sessions = 8
	var wg sync.WaitGroup
	errs := make(chan string, sessions*4)
	for i := 0; i < sessions; i++ {
		withInit := i%2 == 0
		wg.Add(1)
		go func() {
			defer wg.Done()
			input := fmt.Sprintf(get, 2) + "\n" + fmt.Sprintf(status, 3) + "\n"
			if withInit {
				input = handshake + input
			}
			var out bytes.Buffer
			srv := mcp.NewServer(Version, mcpPlannerTools(dir, deps{})...)
			if err := srv.Serve(context.Background(), strings.NewReader(input), &out); err != nil {
				errs <- fmt.Sprintf("serve: %v", err)
				return
			}
			dec := json.NewDecoder(&out)
			for dec.More() {
				var m map[string]any
				if err := dec.Decode(&m); err != nil {
					errs <- fmt.Sprintf("decode: %v", err)
					return
				}
				if m["error"] != nil {
					errs <- fmt.Sprintf("protocol error (init=%v): %v", withInit, m["error"])
				}
				if res, ok := m["result"].(map[string]any); ok && res["isError"] == true {
					errs <- fmt.Sprintf("tool error (init=%v): %v", withInit, res)
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}

	if _, out, _ := runCLI(t, dir, "question", q.ID); !strings.Contains(out, "PENDING") {
		t.Errorf("parallel reads changed the question: %q", out)
	}
}
