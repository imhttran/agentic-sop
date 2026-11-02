//go:build ragacceptance

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
)

// RAG-EVAL-003 end-to-end failure fixtures.
//
// Each fixture runs SOP's `run` command in-process through runInjectedCLI with
// the real ragclient executing a producer executable. Only the agent is faked.
//
// Producer wire contract (internal/ragclient, schema rag-retrieval/v1): the
// client writes one JSON request line to the producer's stdin; the producer
// prints one JSON response line on stdout:
//
//	{"schema_version":"rag-retrieval/v1","status":"OK"|"ERROR",
//	 "evidence":[{"id","corpus","source","section","text",...}],
//	 "truncated":bool,"diagnostics":{"elapsed_ms":int,"error_code":"..."}}
//
// A non-empty diagnostics.error_code other than OK/EMPTY (or a non-zero exit,
// or unparseable output) makes the retrieval fail with that typed code. An
// evidence item whose corpus differs from the requested corpus is rejected as
// UNAUTHORIZED_SCOPE.

// writeProducer writes a #!/bin/sh producer script in a temp dir with mode
// 0755 and returns its absolute path.
func writeProducer(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "producer.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write producer: %v", err)
	}
	return path
}

// pointProducerAt makes the RAG configuration use the given producer script.
func pointProducerAt(t *testing.T, path string) {
	t.Helper()
	t.Setenv(config.EnvRAGExecutable, path)
}

// newOptionalAgent builds the optional-RAG agent exactly as in
// TestRAGTelemetryOptionalFallback.
func newOptionalAgent() *ragCapturingAgent {
	return &ragCapturingAgent{fakeCapabilityAgent: fakeCapabilityAgent{plan: maxTasksPlanJSON, impl: "changed", review: `{"summary":"clean","findings":[]}`}}
}

// runOptionalTask runs one bounded optional task in dir with agent a.
func runOptionalTask(t *testing.T, dir string, a *ragCapturingAgent) (int, string, string) {
	t.Helper()
	return runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run", "--max-tasks", "1")
}

// assertOptionalFallback asserts the S001 telemetry recorded a BM25-only
// fallback: status failed, reason_code OPTIONAL_FALLBACK, no evidence used.
func assertOptionalFallback(t *testing.T, dir string) {
	t.Helper()
	m := readRAGTelemetry(t, dir, "S001")
	if m["status"] != "failed" || m["reason_code"] != "OPTIONAL_FALLBACK" || m["used_in_context"] != false {
		t.Fatalf("optional-fallback telemetry = %v, want status=failed reason_code=OPTIONAL_FALLBACK used_in_context=false", m)
	}
}

// okResponse prints a rag-retrieval/v1 OK response with the given evidence.
func okResponse(evidence string) string {
	return `read -r req || true
printf '%s\n' '{"schema_version":"rag-retrieval/v1","status":"OK","evidence":[` + evidence + `],"truncated":false,"diagnostics":{"elapsed_ms":1,"error_code":"OK"}}'
`
}

// errorResponse prints a rag-retrieval/v1 ERROR response with the given code.
func errorResponse(code string) string {
	return `read -r req || true
printf '%s\n' '{"schema_version":"rag-retrieval/v1","status":"ERROR","evidence":[],"truncated":false,"diagnostics":{"elapsed_ms":1,"error_code":"` + code + `"}}'
`
}

func TestRAGFixtureColdStart(t *testing.T) {
	ragReqEnv(t)
	pointProducerAt(t, writeProducer(t, okResponse("")))
	dir := threeStageGraph(t)
	a := newOptionalAgent()
	code, _, stderr := runOptionalTask(t, dir, a)
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	m := readRAGTelemetry(t, dir, "S001")
	if m["used_in_context"] != false {
		t.Fatalf("cold-start telemetry = %v, want used_in_context=false", m)
	}
}

func TestRAGFixtureStaleIndex(t *testing.T) {
	ragReqEnv(t)
	pointProducerAt(t, writeProducer(t, errorResponse("STALE_INDEX")))
	dir := threeStageGraph(t)
	a := newOptionalAgent()
	code, _, stderr := runOptionalTask(t, dir, a)
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	assertOptionalFallback(t, dir)
}

func TestRAGFixtureEmbeddingProviderUnavailable(t *testing.T) {
	ragReqEnv(t)
	// UNAVAILABLE stands in for the embedding provider (Ollama) being down.
	pointProducerAt(t, writeProducer(t, errorResponse("UNAVAILABLE")))
	dir := threeStageGraph(t)
	a := newOptionalAgent()
	code, _, stderr := runOptionalTask(t, dir, a)
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	assertOptionalFallback(t, dir)
}

func TestRAGFixturePostgresUnavailable(t *testing.T) {
	ragReqEnv(t)
	// The producer really runs psql against an unreachable address and
	// propagates psql's exit status; nothing listens on 127.0.0.1:1.
	pointProducerAt(t, writeProducer(t, `read -r req || true
psql "postgres://rag:rag@127.0.0.1:1/rag?sslmode=disable" -c "select 1" >/dev/null 2>&1
exit $?
`))
	dir := threeStageGraph(t)
	a := newOptionalAgent()
	code, _, stderr := runOptionalTask(t, dir, a)
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	assertOptionalFallback(t, dir)
}

func TestRAGFixtureMalformedProducerResponse(t *testing.T) {
	ragReqEnv(t)
	pointProducerAt(t, writeProducer(t, `read -r req || true
printf '%s\n' 'not json'
`))
	dir := threeStageGraph(t)
	a := newOptionalAgent()
	code, _, stderr := runOptionalTask(t, dir, a)
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	assertOptionalFallback(t, dir)
}

func TestRAGFixtureCrossRepositoryScope(t *testing.T) {
	ragReqEnv(t)
	// One evidence item from corpus "other", which is not in the allowlist:
	// the client rejects it as UNAUTHORIZED_SCOPE and no such item may reach
	// the agent's prompt.
	pointProducerAt(t, writeProducer(t, okResponse(`{"id":"e1","corpus":"other","source":"docs/secret.md","section":"S","text":"CROSS-CORPUS-SECRET","retrieval_method":"bm25","score":1,"rank":1}`)))
	dir := threeStageGraph(t)
	a := newOptionalAgent()
	code, _, stderr := runOptionalTask(t, dir, a)
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	assertOptionalFallback(t, dir)
	for _, in := range a.inputs {
		if strings.Contains(in, "other") || strings.Contains(in, "CROSS-CORPUS-SECRET") {
			t.Fatalf("cross-corpus evidence reached the agent prompt: %q", in)
		}
	}
}

func TestRAGFixtureRequiredEvidenceBlocks(t *testing.T) {
	ragReqEnv(t)
	pointProducerAt(t, writeProducer(t, errorResponse("UNAVAILABLE")))
	dir := requiredEvidenceGraph(t)
	a := newReqAgent()
	_, stdout, _ := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run", "--max-tasks", "1")
	if len(a.inputs) != 0 {
		t.Fatalf("agent was invoked %d time(s) despite the required-evidence failure", len(a.inputs))
	}
	if !strings.Contains(stdout, "REQUIRED_EVIDENCE_MISSING") {
		t.Fatalf("stdout = %q, want REQUIRED_EVIDENCE_MISSING", stdout)
	}
}

func TestRAGFixtureResumedRun(t *testing.T) {
	ragReqEnv(t)
	pointProducerAt(t, writeProducer(t, errorResponse("STALE_INDEX")))
	dir := threeStageGraph(t)
	a := newOptionalAgent()
	code, _, stderr := runOptionalTask(t, dir, a)
	if code != exitOK {
		t.Fatalf("first run: code=%d stderr=%s", code, stderr)
	}
	assertOptionalFallback(t, dir)
	code, _, stderr = runOptionalTask(t, dir, a)
	if code != exitOK {
		t.Fatalf("second run: code=%d stderr=%s", code, stderr)
	}
	assertOptionalFallback(t, dir)
}
