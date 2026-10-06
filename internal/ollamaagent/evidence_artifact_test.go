package ollamaagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// TestEvidenceArtifactMutationCompletes is the regression for the closure
// evidence-artifact workflow: an IMPLEMENT/FIX invocation that performs FIVE
// distinct successful repository inspections and THEN writes its one authorized
// evidence artifact must complete as a normal CHANGE. Before the repository
// no-progress guard credited novel inspections, a run that read five files before
// writing was falsely stopped as a stalled no-progress run, so the artifact was
// never produced.
//
// The immutable Go fixture written into the temp repository is objective,
// independently-run artifact validation (a real go test), not model testimony:
// it asserts the report exists, is nonempty, and carries the required labels.
// Everything runs in t.TempDir against the scripted fake model; no real project,
// provider, or SOP state is touched.
func TestEvidenceArtifactMutationCompletes(t *testing.T) {
	// reportContent is the exact evidence artifact the invocation is authorized
	// to write. It carries every label the immutable Go test requires.
	const reportContent = "# Evidence Artifact\n" +
		"\n" +
		"## agentic-sop\n\nagentic-sop baseline section.\n" +
		"\n" +
		"## sop-controller\n\nsop-controller baseline section.\n" +
		"\n" +
		"## Toolchain/runtime\n\nGo version: go1.22\nSOP binary: /usr/local/bin/sop\n" +
		"\n" +
		"## Model configuration\n\nSMALL: small-model\nMEDIUM: medium-model\nLARGE: large-model\n"

	// immutableGoTest asserts the report is present, nonempty, and labeled. It is
	// written once and must be byte-identical after the invocation: the model may
	// not weaken its own acceptance test.
	const immutableGoTest = `package evidence

import (
	"os"
	"strings"
	"testing"
)

func TestBaselineReport(t *testing.T) {
	data, err := os.ReadFile("docs/reports/baseline.md")
	if err != nil {
		t.Fatalf("read baseline report: %v", err)
	}
	body := string(data)
	if strings.TrimSpace(body) == "" {
		t.Fatal("baseline report is empty")
	}
	for _, label := range []string{"agentic-sop", "sop-controller", "Go version", "SOP binary", "SMALL", "MEDIUM", "LARGE"} {
		if !strings.Contains(body, label) {
			t.Errorf("baseline report missing label %q", label)
		}
	}
}
`

	// inspectionPaths are five DIFFERENT inspectable files seeded before the run.
	inspectionPaths := []string{
		"docs/inspection-0.md",
		"docs/inspection-1.md",
		"docs/inspection-2.md",
		"docs/inspection-3.md",
		"docs/inspection-4.md",
	}
	const userOwnedPath = "notes.txt"
	const reportPath = "docs/reports/baseline.md"

	// successfulToolResult is the exact prefix toolResultMessage renders for a
	// successful run_command: the harness's exit-0 status line plus the fixture's
	// real Go test output. A request echo never contains this prefix, so asserting
	// it proves the tool actually executed successfully rather than merely being
	// requested.
	const successfulToolResult = "Tool \"run_command\" result:\nexit 0\n"

	for _, capability := range []agent.Capability{agent.Implement, agent.Fix} {
		t.Run(string(capability), func(t *testing.T) {
			dir := t.TempDir()

			// The immutable Go fixture: a module is sufficient with just go.mod and a
			// _test.go file.
			writeFile(t, dir, "go.mod", "module example.com/evidence\n\ngo 1.22\n")
			writeFile(t, dir, "evidence_test.go", immutableGoTest)

			// Five distinct successful inspections plus a user-owned file whose
			// preservation is not the task's business.
			for i, p := range inspectionPaths {
				writeFile(t, dir, p, "inspection file "+string(rune('0'+i))+"\n")
			}
			writeFile(t, dir, userOwnedPath, "user-owned work that must not change\n")

			// Snapshot everything the invocation must leave untouched.
			type snapshot struct {
				path string
				data []byte
			}
			var before []snapshot
			for _, p := range append([]string{"go.mod", "evidence_test.go", userOwnedPath}, inspectionPaths...) {
				data, err := os.ReadFile(filepath.Join(dir, p))
				if err != nil {
					t.Fatalf("snapshot %s: %v", p, err)
				}
				before = append(before, snapshot{path: p, data: data})
			}

			// Eight model turns: five read_file inspections, one write_file for the
			// one authorized report, one run_command go test ./..., one completed
			// outcome. Tool arguments are JSON-encoded rather than string-escaped.
			var responses []string
			for _, p := range inspectionPaths {
				call, err := json.Marshal(map[string]any{"tool": "read_file", "args": map[string]any{"path": p}})
				if err != nil {
					t.Fatal(err)
				}
				responses = append(responses, string(call))
			}
			writeCall, err := json.Marshal(map[string]any{"tool": "write_file", "args": map[string]any{"path": reportPath, "content": reportContent}})
			if err != nil {
				t.Fatal(err)
			}
			responses = append(responses, string(writeCall))
			testCall, err := json.Marshal(map[string]any{"tool": "run_command", "args": map[string]any{"command": "go test ./..."}})
			if err != nil {
				t.Fatal(err)
			}
			responses = append(responses, string(testCall))
			responses = append(responses, `{"status":"completed","summary":"wrote the evidence artifact","changes_expected":true}`)

			fake, srv := newFakeOllama(t, responses...)
			cfg := testConfig(srv.URL)
			cfg.MaxToolCalls = 100

			req := implementRequest()
			req.Capability = capability
			req.Task = "Write ONLY docs/reports/baseline.md as the required evidence artifact. Do not modify any other repository file."
			req.AcceptanceCriteria = []string{"docs/reports/baseline.md exists, is nonempty, and contains agentic-sop, sop-controller, Go version, SOP binary, SMALL, MEDIUM and LARGE."}
			req.ValidationCommands = []string{"go test ./..."}

			h := New(cfg, dir)
			ev := &mutationEvidence{}
			content, _, err := h.CompleteWithEvidence(context.Background(), req, ev)
			if err != nil {
				t.Fatalf("CompleteWithEvidence: %v", err)
			}

			// The invocation completed as a normal structured outcome, grounded in
			// observed mutation evidence — not stopped as a stalled run.
			if !strings.Contains(content, `"status":"completed"`) {
				t.Errorf("content = %q, want a completed outcome", content)
			}
			if !strings.Contains(content, `"changes_expected":true`) {
				t.Errorf("content = %q, want changes_expected=true", content)
			}
			if !ev.observed {
				t.Error("no mutation evidence observed: five novel inspections followed by the artifact write did not earn CHANGE")
			}
			if paths := ev.mutationPaths(); len(paths) != 1 || paths[0] != reportPath {
				t.Errorf("mutationPaths = %v, want exactly [%s]", paths, reportPath)
			}
			records := h.TraceRecords()
			if !hasEvent(records, implementChangeEvent) {
				t.Errorf("trace does not record the CHANGE transition: %+v", records)
			}
			for _, r := range records {
				if r.Termination == terminationNoProgress {
					t.Errorf("run was falsely terminated as no_progress: %+v", r)
				}
			}

			// The report content is exact.
			got, err := os.ReadFile(filepath.Join(dir, reportPath))
			if err != nil {
				t.Fatalf("read report: %v", err)
			}
			if string(got) != reportContent {
				t.Errorf("report content = %q, want the exact authorized content", got)
			}

			// Every immutable/user-owned file is byte-identical.
			for _, s := range before {
				now, err := os.ReadFile(filepath.Join(dir, s.path))
				if err != nil {
					t.Fatalf("re-read %s: %v", s.path, err)
				}
				if string(now) != string(s.data) {
					t.Errorf("%s changed during the invocation: %q -> %q", s.path, s.data, now)
				}
			}

			// Exactly eight model turns were consumed.
			if fake.count() != 8 {
				t.Errorf("model turns = %d, want 8 (five reads, one write, one validation, one outcome)", fake.count())
			}

			// The validation command really executed and its successful output was
			// fed back to the model. The conversation after the run_command turn (the
			// final request) must carry the real successful tool-result prefix AND the
			// fixture's actual Go test output — a request echo alone (which mentions
			// "go test" or "ok" in the prompt) cannot satisfy this.
			last := messageText(fake.request(fake.count() - 1))
			if !strings.Contains(last, successfulToolResult) {
				t.Errorf("the post-validation conversation is missing the successful tool result prefix %q:\n%s", successfulToolResult, last)
			}
			if !strings.Contains(last, "ok") || !strings.Contains(last, "example.com/evidence") {
				t.Errorf("the post-validation conversation does not carry the successful fixture output (ok + example.com/evidence):\n%s", last)
			}
			if strings.Contains(last, "FAIL") {
				t.Errorf("the validation command did not succeed:\n%s", last)
			}

			// The validation command was actually executed (audited), not merely
			// requested: only an ALLOWED run_command that completed OK counts. A
			// denied or errored call is never credited as execution.
			executedTest := false
			for _, r := range h.AuditRecords() {
				if r.Tool == toolharness.ToolRunCommand &&
					strings.Contains(r.Request, "go test") &&
					r.Action == toolharness.ActionAllow &&
					r.Outcome == toolharness.OutcomeOK {
					executedTest = true
				}
			}
			if !executedTest {
				t.Errorf("the validation command was not executed successfully: audit=%+v", h.AuditRecords())
			}
		})
	}
}
