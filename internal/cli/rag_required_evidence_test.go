package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	sopctx "github.com/imhttran/agentic-sop/internal/context"
	"github.com/imhttran/agentic-sop/internal/planner"
	runpkg "github.com/imhttran/agentic-sop/internal/run"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/github"
	"github.com/imhttran/agentic-sop/internal/ragclient"
	"github.com/imhttran/agentic-sop/internal/store"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

const requiredEvidencePlanJSON = `{
  "project": "Bounded Run",
  "summary": "x",
  "stages": [
    {"id":"S001","title":"First","objective":"First stage.","dependencies":[],"deliverables":["first"],"acceptance_criteria":["first done"],"required_evidence":["corpus:kb"]}
  ]
}`

func ragReqEnv(t *testing.T) {
	t.Helper()
	t.Setenv(config.EnvRAGEnabled, "true")
	t.Setenv(config.EnvRAGExecutable, trueExecutable())
	t.Setenv(config.EnvRAGCorpus, "kb")
	t.Setenv(config.EnvRAGAllowedCorpora, "kb")
}

func requiredEvidenceGraph(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	writePlanJSON(t, dir, requiredEvidencePlanJSON)
	return dir
}

func runWithRawEvidence(t *testing.T, dir, diff string, a agent.Agent, raw rawEvidenceFunc, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	d := deps{
		getwd:       func() (string, error) { return dir, nil },
		newAgent:    func(string, string, string) (agent.Agent, error) { return a, nil },
		readDiff:    func(context.Context, string) (string, error) { return diff, nil },
		commit:      func(context.Context, string, string) error { return nil },
		newGitHub:   func(string) github.Client { return &fakeGitHub{} },
		rawEvidence: raw,
	}
	code := run(args, &out, &errOut, d)
	return code, out.String(), errOut.String()
}

func newReqAgent() *ragCapturingAgent {
	return &ragCapturingAgent{fakeCapabilityAgent: fakeCapabilityAgent{plan: requiredEvidencePlanJSON, impl: "changed", review: `{"summary":"clean","findings":[]}`}}
}

func persistedBlockedDetail(t *testing.T, dir, id string) (domain.TaskStatus, domain.BlockedReason, string) {
	t.Helper()
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	task, err := st.Get(id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return task.Status, task.BlockedReason, task.BlockedDetail
}

func TestRequiredEvidenceSatisfiedRuns(t *testing.T) {
	ragReqEnv(t)
	calls := 0
	raw := func(context.Context, *taskfile.Spec, string) ([]ragclient.Evidence, error) {
		calls++
		return []ragclient.Evidence{{Corpus: "kb", Source: "docs/a.md", Section: "S", Text: "e"}}, nil
	}
	dir := requiredEvidenceGraph(t)
	a := newReqAgent()
	code, stdout, stderr := runWithRawEvidence(t, dir, "diff --git a/x b/x\n", a, raw, "run", "--max-tasks", "1")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if calls != 1 {
		t.Fatalf("retrieval calls = %d, want exactly 1", calls)
	}
	if len(a.inputs) == 0 {
		t.Fatal("agent was not dispatched after required evidence was satisfied")
	}
}

func TestRequiredEvidenceMissingBlocks(t *testing.T) {
	ragReqEnv(t)
	calls := 0
	raw := func(context.Context, *taskfile.Spec, string) ([]ragclient.Evidence, error) {
		calls++
		return nil, nil // no evidence
	}
	dir := requiredEvidenceGraph(t)
	a := newReqAgent()
	_, stdout, _ := runWithRawEvidence(t, dir, "diff --git a/x b/x\n", a, raw, "run", "--max-tasks", "1")
	if calls != 1 {
		t.Fatalf("retrieval calls = %d, want 1", calls)
	}
	if len(a.inputs) != 0 {
		t.Fatal("agent ran despite missing required evidence")
	}
	status, reason, detail := persistedBlockedDetail(t, dir, "S001")
	if status != domain.BLOCKED || reason != domain.REQUIRED_EVIDENCE_MISSING {
		t.Fatalf("persisted = %s/%s, want BLOCKED/REQUIRED_EVIDENCE_MISSING", status, reason)
	}
	if !strings.Contains(detail, "corpus:kb") {
		t.Fatalf("BlockedDetail = %q, want the missing identity", detail)
	}
	_ = stdout
}

func TestRequiredEvidenceDisabledBlocksWithoutRetrieval(t *testing.T) {
	t.Setenv(config.EnvRAGEnabled, "false")
	called := false
	raw := func(context.Context, *taskfile.Spec, string) ([]ragclient.Evidence, error) {
		called = true
		return nil, nil
	}
	dir := requiredEvidenceGraph(t)
	a := newReqAgent()
	_, _, _ = runWithRawEvidence(t, dir, "diff --git a/x b/x\n", a, raw, "run", "--max-tasks", "1")
	if called {
		t.Fatal("disabled RAG performed a retrieval")
	}
	if _, reason, _ := persistedBlockedDetail(t, dir, "S001"); reason != domain.REQUIRED_EVIDENCE_MISSING {
		t.Fatalf("reason = %s, want REQUIRED_EVIDENCE_MISSING", reason)
	}
}

func TestRequiredEvidenceRetrievalErrorBlocks(t *testing.T) {
	ragReqEnv(t)
	calls := 0
	raw := func(context.Context, *taskfile.Spec, string) ([]ragclient.Evidence, error) {
		calls++
		return nil, errors.New("producer unavailable")
	}
	dir := requiredEvidenceGraph(t)
	a := newReqAgent()
	_, _, _ = runWithRawEvidence(t, dir, "diff --git a/x b/x\n", a, raw, "run", "--max-tasks", "1")
	if calls != 1 {
		t.Fatalf("retrieval calls = %d, want 1", calls)
	}
	if len(a.inputs) != 0 {
		t.Fatal("agent ran despite a required-evidence retrieval failure")
	}
}

func TestEnforceRequiredEvidenceOnceReturnsAttempt(t *testing.T) {
	ragReqEnv(t)
	saver := &capturingSaver{}
	task := &domain.Task{ID: "T1", Status: domain.PLANNED, RequiredEvidence: []string{"corpus:kb"}}
	d := deps{rawEvidence: func(context.Context, *taskfile.Spec, string) ([]ragclient.Evidence, error) {
		return []ragclient.Evidence{{Corpus: "kb", Source: "docs/a.md", Text: "e"}}, nil
	}}

	blocked, code, attempt := enforceRequiredEvidenceOnce(context.Background(), config.Config{}, saver, task, &taskfile.Spec{ID: "T1"}, d, io.Discard, io.Discard)
	if blocked || code != 0 {
		t.Fatalf("blocked=%v code=%d, want success", blocked, code)
	}
	if !attempt.Attempted || attempt.Corpus != "kb" || len(attempt.Evidence) != 1 {
		t.Fatalf("verified attempt not returned: %+v", attempt)
	}
}

func TestEnforceRequiredEvidenceOnceMissingReturnsZeroAttempt(t *testing.T) {
	ragReqEnv(t)
	saver := &capturingSaver{}
	task := &domain.Task{ID: "T1", Status: domain.PLANNED, RequiredEvidence: []string{"corpus:kb"}}
	d := deps{rawEvidence: func(context.Context, *taskfile.Spec, string) ([]ragclient.Evidence, error) { return nil, nil }}

	blocked, _, attempt := enforceRequiredEvidenceOnce(context.Background(), config.Config{}, saver, task, &taskfile.Spec{ID: "T1"}, d, io.Discard, io.Discard)
	if !blocked {
		t.Fatal("missing required evidence must block")
	}
	if attempt.Attempted {
		t.Fatalf("a blocked task returned an attempt: %+v", attempt)
	}
}

func TestRequiredEvidenceReachesAgentPrompt(t *testing.T) {
	ragReqEnv(t)
	calls := 0
	raw := func(context.Context, *taskfile.Spec, string) ([]ragclient.Evidence, error) {
		calls++
		return []ragclient.Evidence{{Corpus: "kb", Source: "docs/a.md", Section: "S", Text: "verified evidence text"}}, nil
	}
	dir := requiredEvidenceGraph(t)
	a := newReqAgent()
	code, stdout, stderr := runWithRawEvidence(t, dir, "diff --git a/x b/x\n", a, raw, "run", "--max-tasks", "1")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if calls != 1 {
		t.Fatalf("retrieval calls = %d, want exactly 1 (no second retrieval)", calls)
	}
	var found bool
	for _, in := range a.inputs {
		if strings.Contains(in, "UNTRUSTED-RAG-KNOWLEDGE") && strings.Contains(in, "kb/docs/a.md#S") && strings.Contains(in, "verified evidence text") {
			found = true
		}
	}
	if !found {
		t.Fatalf("verified evidence did not reach the agent prompt; captured %d inputs", len(a.inputs))
	}
}

const requiredEvidenceAndPlanJSON = `{
  "project": "Bounded Run",
  "summary": "x",
  "stages": [
    {"id":"S001","title":"First","objective":"First stage.","dependencies":[],"deliverables":["first"],"acceptance_criteria":["first done"],"required_evidence":["corpus:kb","source:docs/"]}
  ]
}`

func requiredEvidenceGraphWith(t *testing.T, planJSON string) string {
	t.Helper()
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	writePlanJSON(t, dir, planJSON)
	return dir
}

func TestRequiredEvidenceAndSemanticsBlocks(t *testing.T) {
	ragReqEnv(t)
	// Corpus is satisfied but the source prefix is not → AND means BLOCK.
	raw := func(context.Context, *taskfile.Spec, string) ([]ragclient.Evidence, error) {
		return []ragclient.Evidence{{Corpus: "kb", Source: "other/x.md", Text: "e"}}, nil
	}
	dir := requiredEvidenceGraphWith(t, requiredEvidenceAndPlanJSON)
	a := &ragCapturingAgent{fakeCapabilityAgent: fakeCapabilityAgent{plan: requiredEvidenceAndPlanJSON, impl: "changed", review: `{"summary":"clean","findings":[]}`}}
	_, _, _ = runWithRawEvidence(t, dir, "diff --git a/x b/x\n", a, raw, "run", "--max-tasks", "1")

	if len(a.inputs) != 0 {
		t.Fatal("agent ran despite an unmet source requirement (AND semantics broken)")
	}
	status, reason, detail := persistedBlockedDetail(t, dir, "S001")
	if status != domain.BLOCKED || reason != domain.REQUIRED_EVIDENCE_MISSING {
		t.Fatalf("persisted = %s/%s, want BLOCKED/REQUIRED_EVIDENCE_MISSING", status, reason)
	}
	if !strings.Contains(detail, "source:docs/") {
		t.Fatalf("BlockedDetail = %q, want the unmet source identity", detail)
	}
}

func TestRequiredEvidenceCrossCorpusNeverInPrompt(t *testing.T) {
	ragReqEnv(t)
	// Retrieval returns evidence from ANOTHER corpus; it must never reach the prompt.
	raw := func(context.Context, *taskfile.Spec, string) ([]ragclient.Evidence, error) {
		return []ragclient.Evidence{{Corpus: "other", Source: "docs/a.md", Section: "S", Text: "secret"}}, nil
	}
	dir := requiredEvidenceGraph(t)
	a := newReqAgent()
	_, _, _ = runWithRawEvidence(t, dir, "diff --git a/x b/x\n", a, raw, "run", "--max-tasks", "1")

	if len(a.inputs) != 0 {
		t.Fatalf("agent ran despite cross-corpus evidence (%d inputs)", len(a.inputs))
	}
	status, reason, _ := persistedBlockedDetail(t, dir, "S001")
	if status != domain.BLOCKED || reason != domain.REQUIRED_EVIDENCE_MISSING {
		t.Fatalf("persisted = %s/%s, want BLOCKED/REQUIRED_EVIDENCE_MISSING", status, reason)
	}
}

func readRAGTelemetry(t *testing.T, dir, id string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, config.DirName, "runs", id, "rag.json"))
	if err != nil {
		t.Fatalf("read rag.json: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("parse rag.json: %v", err)
	}
	return m
}

func TestRAGTelemetryRequiredSuccess(t *testing.T) {
	ragReqEnv(t)
	raw := func(context.Context, *taskfile.Spec, string) ([]ragclient.Evidence, error) {
		return []ragclient.Evidence{{Corpus: "kb", Source: "docs/a.md", Section: "S", Text: "SECRET-EVIDENCE-BODY"}}, nil
	}
	dir := requiredEvidenceGraph(t)
	a := newReqAgent()
	if code, _, se := runWithRawEvidence(t, dir, "diff --git a/x b/x\n", a, raw, "run", "--max-tasks", "1"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, se)
	}
	m := readRAGTelemetry(t, dir, "S001")
	if m["mode"] != "required" || m["status"] != "succeeded" || m["corpus"] != "kb" || m["used_in_context"] != true {
		t.Fatalf("telemetry = %v", m)
	}
	if m["task_id"] != "S001" || m["attempt"].(float64) != 0 {
		t.Fatalf("task/attempt association = %v/%v, want S001/0", m["task_id"], m["attempt"])
	}
	if m["prompt_included"] != true {
		t.Fatalf("prompt_included = %v, want true (selected citation survived the compiler)", m["prompt_included"])
	}
	if m["evidence_count"].(float64) != 1 {
		t.Fatalf("evidence_count = %v", m["evidence_count"])
	}
	// Content exclusion: the evidence body must NOT appear anywhere in the record.
	if s, _ := os.ReadFile(filepath.Join(dir, config.DirName, "runs", "S001", "rag.json")); strings.Contains(string(s), "SECRET-EVIDENCE-BODY") {
		t.Fatal("evidence text leaked into telemetry")
	}
}

func TestRAGTelemetryDisabledSkipped(t *testing.T) {
	t.Setenv(config.EnvRAGEnabled, "false")
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	writePlanJSON(t, dir, maxTasksPlanJSON)
	a := &ragCapturingAgent{fakeCapabilityAgent: fakeCapabilityAgent{plan: maxTasksPlanJSON, impl: "changed", review: `{"summary":"clean","findings":[]}`}}
	if code, _, se := runWithRAG(t, dir, "diff --git a/x b/x\n", a, nil, "run", "--max-tasks", "1"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, se)
	}
	m := readRAGTelemetry(t, dir, "S001")
	if m["status"] != "skipped" || m["reason_code"] != "RAG_DISABLED" || m["used_in_context"] != false {
		t.Fatalf("telemetry = %v", m)
	}
}

func TestRAGPreRunFailureReasonCode(t *testing.T) {
	ragReqEnv(t)
	raw := func(context.Context, *taskfile.Spec, string) ([]ragclient.Evidence, error) { return nil, nil }
	dir := requiredEvidenceGraph(t)
	a := newReqAgent()
	_, stdout, _ := runWithRawEvidence(t, dir, "diff --git a/x b/x\n", a, raw, "run", "--max-tasks", "1")
	if !strings.Contains(stdout, "[reason_code=REQUIRED_EVIDENCE_MISSING]") {
		t.Fatalf("BLOCKED stdout missing the bounded reason code: %q", stdout)
	}
}

func TestRAGTelemetryOptionalSuccess(t *testing.T) {
	ragReqEnv(t)
	rag := func(context.Context, *taskfile.Spec) ([]sopctx.Item, error) {
		return []sopctx.Item{{Source: sopctx.SourceKnowledgeRAG, Identity: "kb/docs/a.md#S", Priority: sopctx.PriorityKnowledgeRAG, Text: "opt evidence"}}, nil
	}
	dir := threeStageGraph(t)
	a := &ragCapturingAgent{fakeCapabilityAgent: fakeCapabilityAgent{plan: maxTasksPlanJSON, impl: "changed", review: `{"summary":"clean","findings":[]}`}}
	if code, _, se := runWithRAG(t, dir, "diff --git a/x b/x\n", a, rag, "run", "--max-tasks", "1"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, se)
	}
	m := readRAGTelemetry(t, dir, "S001")
	if m["mode"] != "optional" || m["status"] != "succeeded" || m["used_in_context"] != true || m["task_id"] != "S001" {
		t.Fatalf("optional-success telemetry = %v", m)
	}
}

func TestRAGTelemetryOptionalFallback(t *testing.T) {
	ragReqEnv(t)
	rag := func(context.Context, *taskfile.Spec) ([]sopctx.Item, error) {
		return nil, errors.New("producer unavailable")
	}
	dir := threeStageGraph(t)
	a := &ragCapturingAgent{fakeCapabilityAgent: fakeCapabilityAgent{plan: maxTasksPlanJSON, impl: "changed", review: `{"summary":"clean","findings":[]}`}}
	if code, _, se := runWithRAG(t, dir, "diff --git a/x b/x\n", a, rag, "run", "--max-tasks", "1"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, se)
	}
	m := readRAGTelemetry(t, dir, "S001")
	if m["status"] != "failed" || m["reason_code"] != "OPTIONAL_FALLBACK" || m["used_in_context"] != false {
		t.Fatalf("optional-fallback telemetry = %v", m)
	}
}

// TestRAGConcurrentTasksIsolated drives two RAG-enabled tasks concurrently in
// isolated project dirs and proves there is no cross-task evidence or telemetry
// leakage. Fixtures are built on the main goroutine (so t.Fatalf is never called
// from a non-test goroutine); only run(...) executes concurrently.
func TestRAGConcurrentTasksIsolated(t *testing.T) {
	ragReqEnv(t)

	mkFixture := func(tag string) (string, *ragCapturingAgent, ragKnowledgeFunc) {
		dir := t.TempDir()
		initProject(t, dir)
		writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
		writePlanJSON(t, dir, maxTasksPlanJSON)
		a := &ragCapturingAgent{fakeCapabilityAgent: fakeCapabilityAgent{plan: maxTasksPlanJSON, impl: "changed", review: `{"summary":"clean","findings":[]}`}}
		rag := func(context.Context, *taskfile.Spec) ([]sopctx.Item, error) {
			return []sopctx.Item{{Source: sopctx.SourceKnowledgeRAG, Identity: "kb/" + tag, Priority: sopctx.PriorityKnowledgeRAG, Text: tag}}, nil
		}
		return dir, a, rag
	}

	dirA, aA, ragA := mkFixture("EVID-A")
	dirB, aB, ragB := mkFixture("EVID-B")

	type outcome struct {
		code int
		err  string
	}
	runOne := func(dir string, a *ragCapturingAgent, rag ragKnowledgeFunc, out chan<- outcome) {
		code, _, se := runWithRAG(t, dir, "diff --git a/x b/x\n", a, rag, "run", "--max-tasks", "1")
		out <- outcome{code: code, err: se}
	}
	outA, outB := make(chan outcome, 1), make(chan outcome, 1)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); runOne(dirA, aA, ragA, outA) }()
	go func() { defer wg.Done(); runOne(dirB, aB, ragB, outB) }()
	wg.Wait()
	oa, ob := <-outA, <-outB
	if oa.code != exitOK || ob.code != exitOK {
		t.Fatalf("codes = %d/%d (errA=%s errB=%s)", oa.code, ob.code, oa.err, ob.err)
	}

	joinedA := strings.Join(aA.inputs, "\n---\n")
	joinedB := strings.Join(aB.inputs, "\n---\n")
	if !strings.Contains(joinedA, "EVID-A") || strings.Contains(joinedA, "EVID-B") {
		t.Fatal("task A's prompts leaked task B's evidence (or missed its own)")
	}
	if !strings.Contains(joinedB, "EVID-B") || strings.Contains(joinedB, "EVID-A") {
		t.Fatal("task B's prompts leaked task A's evidence (or missed its own)")
	}

	mA := readRAGTelemetry(t, dirA, "S001")
	mB := readRAGTelemetry(t, dirB, "S001")
	if mA["task_id"] != "S001" || mB["task_id"] != "S001" {
		t.Fatalf("telemetry task association: %v / %v", mA["task_id"], mB["task_id"])
	}
	if mA["used_in_context"] != true || mB["used_in_context"] != true {
		t.Fatalf("used_in_context: %v / %v", mA["used_in_context"], mB["used_in_context"])
	}
}

func countInput(a *ragCapturingAgent, marker string) bool {
	return strings.Contains(strings.Join(a.inputs, "\n"), marker)
}

func TestRAGConcurrentRequiredAndOptional(t *testing.T) {
	ragReqEnv(t)
	rDir := requiredEvidenceGraph(t)
	rAgent := newReqAgent()
	raw := func(context.Context, *taskfile.Spec, string) ([]ragclient.Evidence, error) {
		return []ragclient.Evidence{{Corpus: "kb", Source: "docs/req.md", Section: "S", Text: "REQ-EVID"}}, nil
	}
	oDir := threeStageGraph(t)
	oAgent := &ragCapturingAgent{fakeCapabilityAgent: fakeCapabilityAgent{plan: maxTasksPlanJSON, impl: "changed", review: `{"summary":"clean","findings":[]}`}}
	rag := func(context.Context, *taskfile.Spec) ([]sopctx.Item, error) {
		return []sopctx.Item{{Source: sopctx.SourceKnowledgeRAG, Identity: "kb/docs/opt.md#S", Priority: sopctx.PriorityKnowledgeRAG, Text: "OPT-EVID"}}, nil
	}
	ch := make(chan int, 2)
	go func() {
		c, _, _ := runWithRawEvidence(t, rDir, "diff --git a/x b/x\n", rAgent, raw, "run", "--max-tasks", "1")
		ch <- c
	}()
	go func() {
		c, _, _ := runWithRAG(t, oDir, "diff --git a/x b/x\n", oAgent, rag, "run", "--max-tasks", "1")
		ch <- c
	}()
	c1, c2 := <-ch, <-ch
	if c1 != exitOK || c2 != exitOK {
		t.Fatalf("codes = %d/%d", c1, c2)
	}
	if !countInput(rAgent, "REQ-EVID") || countInput(rAgent, "OPT-EVID") {
		t.Fatal("required task evidence wrong")
	}
	if !countInput(oAgent, "OPT-EVID") || countInput(oAgent, "REQ-EVID") {
		t.Fatal("optional task evidence wrong")
	}
	if m := readRAGTelemetry(t, rDir, "S001"); m["mode"] != "required" || m["used_in_context"] != true {
		t.Fatalf("required telemetry = %v", m)
	}
	if m := readRAGTelemetry(t, oDir, "S001"); m["mode"] != "optional" || m["used_in_context"] != true {
		t.Fatalf("optional telemetry = %v", m)
	}
}

func TestRAGConcurrentRequiredFailureIsolated(t *testing.T) {
	ragReqEnv(t)
	// Task 1: required evidence missing -> BLOCKED.
	fDir := requiredEvidenceGraph(t)
	fAgent := newReqAgent()
	missing := func(context.Context, *taskfile.Spec, string) ([]ragclient.Evidence, error) { return nil, nil }
	// Task 2: unrelated optional task must still complete.
	oDir := threeStageGraph(t)
	oAgent := &ragCapturingAgent{fakeCapabilityAgent: fakeCapabilityAgent{plan: maxTasksPlanJSON, impl: "changed", review: `{"summary":"clean","findings":[]}`}}
	ch := make(chan int, 2)
	go func() {
		c, _, _ := runWithRawEvidence(t, fDir, "diff --git a/x b/x\n", fAgent, missing, "run", "--max-tasks", "1")
		ch <- c
	}()
	go func() {
		c, _, _ := runWithRAG(t, oDir, "diff --git a/x b/x\n", oAgent, nil, "run", "--max-tasks", "1")
		ch <- c
	}()
	<-ch
	<-ch
	if s, reason, _ := persistedBlockedDetail(t, fDir, "S001"); s != domain.BLOCKED || reason != domain.REQUIRED_EVIDENCE_MISSING {
		t.Fatalf("required task = %s/%s, want BLOCKED/REQUIRED_EVIDENCE_MISSING", s, reason)
	}
	if len(fAgent.inputs) != 0 {
		t.Fatal("required-failure task dispatched an agent")
	}
	if len(oAgent.inputs) == 0 {
		t.Fatal("an unrelated optional task was blocked by another task's required-evidence failure")
	}
}

func TestRAGConcurrentDisabledNoRetrieval(t *testing.T) {
	t.Setenv(config.EnvRAGEnabled, "false")
	var calls int64
	mk := func() (string, *ragCapturingAgent) {
		dir := threeStageGraph(t)
		a := &ragCapturingAgent{fakeCapabilityAgent: fakeCapabilityAgent{plan: maxTasksPlanJSON, impl: "changed", review: `{"summary":"clean","findings":[]}`}}
		return dir, a
	}
	d1, a1 := mk()
	d2, a2 := mk()
	rag := func(context.Context, *taskfile.Spec) ([]sopctx.Item, error) {
		atomic.AddInt64(&calls, 1)
		return nil, nil
	}
	ch := make(chan int, 2)
	go func() {
		c, _, _ := runWithRAG(t, d1, "diff --git a/x b/x\n", a1, rag, "run", "--max-tasks", "1")
		ch <- c
	}()
	go func() {
		c, _, _ := runWithRAG(t, d2, "diff --git a/x b/x\n", a2, rag, "run", "--max-tasks", "1")
		ch <- c
	}()
	<-ch
	<-ch
	if n := atomic.LoadInt64(&calls); n != 0 {
		t.Fatalf("disabled RAG invoked the retriever %d times during concurrent execution", n)
	}
	if m := readRAGTelemetry(t, d1, "S001"); m["status"] != "skipped" || m["reason_code"] != "RAG_DISABLED" {
		t.Fatalf("telemetry = %v", m)
	}
}

func TestRAGTelemetryAttemptUsesCurrentTaskAttempt(t *testing.T) {
	ragReqEnv(t)
	dir := t.TempDir()
	if _, err := runpkg.New(dir, "S001"); err != nil {
		t.Fatalf("runpkg.New: %v", err)
	}
	// attemptNumber mirrors task.Attempt; here it is 2 (a later attempt), not 0.
	if _, _ = implementContext(context.Background(), &taskfile.Spec{ID: "S001", Title: "t"}, &planner.Plan{}, mustRun(t, dir, "S001"), config.Config{}, nil, "", deps{attemptNumber: 2}); false {
	}
	m := readRAGTelemetry(t, dir, "S001")
	if m["attempt"].(float64) != 2 {
		t.Fatalf("attempt = %v, want 2 (current task.Attempt)", m["attempt"])
	}
}

func mustRun(t *testing.T, dir, id string) *runpkg.Run {
	t.Helper()
	rn, err := runpkg.New(dir, id)
	if err != nil {
		t.Fatalf("runpkg.New: %v", err)
	}
	return rn
}
