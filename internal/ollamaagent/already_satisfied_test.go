package ollamaagent

import (
	"context"
	"encoding/json"
	"path/filepath"
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

// --- SOP-HARD-03: accept verified existing work with zero mutations ---

// TestAlreadySatisfiedRejectsMissingDeliverable: a declared required deliverable that
// is absent rejects the claim even when every criterion is inspected and validated.
func TestAlreadySatisfiedRejectsMissingDeliverable(t *testing.T) {
	dir, req := satisfactionFixture(t)
	req.Capability = agent.Implement
	req.Deliverables = []string{"docs/reports/SOP-HARD-01.md"} // absent from the fixture
	turns := []string{readAdd, readAddTest, validateAdd, satisfiedFinal}
	_, srv := newFakeOllama(t, turns...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 20
	h := New(cfg, dir)
	ev := &mutationEvidence{}
	if _, _, err := h.CompleteWithEvidence(context.Background(), req, ev); err == nil || ev.alreadySatisfied {
		t.Fatalf("missing deliverable accepted: err=%v evidence=%+v", err, ev)
	}
}

// TestAlreadySatisfiedAcceptsAuthorizedValidationWaiver: a required validation command
// that is not executed is accepted only when an authorized waiver names it.
func TestAlreadySatisfiedAcceptsAuthorizedValidationWaiver(t *testing.T) {
	dir, req := satisfactionFixture(t)
	gitInit(t, dir)
	commitAll(t, dir, "existing implementation")
	req.Capability = agent.Implement
	req.ValidationCommands = []string{"go test ./...", "go vet ./..."}
	req.ValidationWaivers = []string{"go vet ./..."} // authorized; unavailable here
	fake, srv := newFakeOllama(t, readAdd, readAddTest, validateAdd, satisfiedFinal)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 20
	native := NewNativeAgent(cfg, dir, nil)
	response, err := native.Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !response.VerifiedAlreadySatisfied {
		t.Fatalf("authorized validation waiver not accepted: %+v", response)
	}
	_ = fake
}

// TestAlreadySatisfiedNormalizedCriterionMatch: a criterion differing only by case and
// whitespace from the evidence is accepted (content match, not byte match).
func TestAlreadySatisfiedNormalizedCriterionMatch(t *testing.T) {
	dir, req := satisfactionFixture(t)
	gitInit(t, dir)
	commitAll(t, dir, "existing implementation")
	req.Capability = agent.Implement
	req.AcceptanceCriteria = []string{"ADD returns the   SUM"} // normalizes to the evidence criterion
	fake, srv := newFakeOllama(t, readAdd, readAddTest, validateAdd, satisfiedFinal)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 20
	native := NewNativeAgent(cfg, dir, nil)
	response, err := native.Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !response.VerifiedAlreadySatisfied {
		t.Fatalf("normalized criterion match not accepted: %+v", response)
	}
	_ = fake
}

// --- SOP-HARD-03: bounded, evidence-only correction ---

// satisfiedFinalNoEvidence carries an ALREADY_SATISFIED claim with no criterion
// evidence, so it is rejected for missing coverage (and nothing else).
const satisfiedFinalNoEvidence = `{"status":"completed","completion":"ALREADY_SATISFIED","changes_expected":false,"evidence":{"acceptance":[]}}`

// TestAlreadySatisfiedEvidenceCorrectionAccepted: an otherwise-sound claim with no
// criterion evidence is corrected once, and the corrected (complete) evidence is
// accepted by the deterministic verifier with no repository mutation.
func TestAlreadySatisfiedEvidenceCorrectionAccepted(t *testing.T) {
	dir, req := satisfactionFixture(t)
	gitInit(t, dir)
	commitAll(t, dir, "existing implementation")
	req.Capability = agent.Implement
	_, srv := newFakeOllama(t, readAdd, readAddTest, validateAdd, satisfiedFinalNoEvidence, satisfiedFinal)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 30
	native := NewNativeAgent(cfg, dir, nil)
	response, err := native.Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !response.VerifiedAlreadySatisfied {
		t.Fatalf("corrected evidence not accepted: %+v", response)
	}
}

// TestAlreadySatisfiedEvidenceCorrectionExhausted: a second incomplete claim is not
// corrected again; the claim is rejected (at most one correction per claim).
func TestAlreadySatisfiedEvidenceCorrectionExhausted(t *testing.T) {
	dir, req := satisfactionFixture(t)
	req.Capability = agent.Implement
	_, srv := newFakeOllama(t, readAdd, readAddTest, validateAdd, satisfiedFinalNoEvidence, satisfiedFinalNoEvidence)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 30
	h := New(cfg, dir)
	ev := &mutationEvidence{}
	if _, _, err := h.CompleteWithEvidence(context.Background(), req, ev); err == nil || ev.alreadySatisfied {
		t.Fatalf("exhausted correction accepted: err=%v evidence=%+v", err, ev)
	}
}

// TestAlreadySatisfiedEvidenceCorrectionRejectsUnreadPath: evidence mapping a criterion
// to a file the agent did not read is not correctable (not a missing-coverage case) and
// is rejected outright.
func TestAlreadySatisfiedEvidenceCorrectionRejectsUnreadPath(t *testing.T) {
	dir, req := satisfactionFixture(t)
	req.Capability = agent.Implement
	// add_test.go is never read, so the satisfiedFinal evidence path is uninspected.
	_, srv := newFakeOllama(t, readAdd, validateAdd, satisfiedFinal, satisfiedFinal)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 30
	h := New(cfg, dir)
	ev := &mutationEvidence{}
	if _, _, err := h.CompleteWithEvidence(context.Background(), req, ev); err == nil || ev.alreadySatisfied {
		t.Fatalf("unread-path evidence accepted: err=%v evidence=%+v", err, ev)
	}
}

// TestAlreadySatisfiedEvidenceCorrectionFromUnreadPath: a claim whose evidence maps a
// criterion to an unread path is still offered the single correction; the corrected
// claim (read-backed) is accepted, and the invalid one is never accepted on its own.
func TestAlreadySatisfiedEvidenceCorrectionFromUnreadPath(t *testing.T) {
	dir, req := satisfactionFixture(t)
	gitInit(t, dir)
	commitAll(t, dir, "existing implementation")
	req.Capability = agent.Implement
	// First claim maps to add_test.go (not read); then the model re-reads and re-claims.
	_, srv := newFakeOllama(t, readAdd, validateAdd,
		`{"status":"completed","completion":"ALREADY_SATISFIED","changes_expected":false,"evidence":{"acceptance":[{"criterion":"Add returns the sum","paths":["add_test.go"]}]}}`,
		readAddTest, satisfiedFinal)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 30
	native := NewNativeAgent(cfg, dir, nil)
	response, err := native.Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !response.VerifiedAlreadySatisfied {
		t.Fatalf("correction from unread path not accepted: %+v", response)
	}
}

// --- SOP-HARD-03: structured rejection diagnostics ---

func satisfProof(baseline string) *satisfactionProof {
	return &satisfactionProof{baseline: baseline, reads: map[string]bool{}, commands: map[string]bool{}, ran: map[string]bool{}, commandText: map[string]string{}}
}

func evidenceFinal(criterion string, paths []any) map[string]any {
	return map[string]any{
		"status":   string(agent.OutcomeCompleted),
		"evidence": map[string]any{"acceptance": []any{map[string]any{"criterion": criterion, "paths": paths}}},
	}
}

func TestVerifySatisfiedDetailedRejectionCodes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package a\n")
	cfg := testConfig("http://127.0.0.1:1")
	h := New(cfg, dir)
	req := agent.Request{Capability: agent.Implement, AcceptanceCriteria: []string{"c1"}, ValidationCommands: []string{"go vet ./..."}}

	canonical := canonicalPath(t, dir, "a.go")
	// readOnly has the evidence path read but the required validation NOT executed.
	readOnly := satisfProof("fp")
	readOnly.reads[canonical] = true

	cases := []struct {
		name  string
		st    *executionState
		p     *satisfactionProof
		final map[string]any
		want  RejectionCode
	}{
		{"repository mutation", &executionState{mutationObserved: true}, satisfProof("fp"), evidenceFinal("c1", []any{"a.go"}), RejectRepositoryMutation},
		{"missing baseline", &executionState{}, satisfProof(""), evidenceFinal("c1", []any{"a.go"}), RejectMissingBaseline},
		{"completion status", &executionState{}, satisfProof("fp"), map[string]any{"status": string(agent.OutcomeFailed)}, RejectCompletionStatus},
		{"malformed evidence", &executionState{}, satisfProof("fp"), map[string]any{"status": string(agent.OutcomeCompleted), "evidence": "not-an-object"}, RejectMalformedEvidence},
		{"missing criterion evidence", &executionState{}, satisfProof("fp"), map[string]any{"status": string(agent.OutcomeCompleted), "evidence": map[string]any{"acceptance": []any{}}}, RejectMissingCriterionEvidence},
		{"unread evidence path", &executionState{}, satisfProof("fp"), evidenceFinal("c1", []any{"missing.go"}), RejectUnreadEvidencePath},
		{"validation not executed", &executionState{}, readOnly, evidenceFinal("c1", []any{"a.go"}), RejectValidationNotExecuted},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ok, rej := h.verifySatisfiedDetailed(context.Background(), req, c.st, c.p, c.final)
			if ok {
				t.Fatalf("accepted, want rejection")
			}
			if rej.Code != c.want {
				t.Fatalf("code = %s, want %s (detail=%s)", rej.Code, c.want, rej.String())
			}
		})
	}
}

func TestSatisfactionRejectionStringIsSecretFree(t *testing.T) {
	r := SatisfactionRejection{Code: RejectUnreadEvidencePath, Criterion: "c1", Path: "x.go"}
	got := r.String()
	if !strings.Contains(got, "UNREAD_EVIDENCE_PATH") || !strings.Contains(got, "criterion=c1") || !strings.Contains(got, "path=x.go") {
		t.Fatalf("String() = %q", got)
	}
}

func canonicalPath(t *testing.T, dir, name string) string {
	t.Helper()
	c, err := filepath.EvalSymlinks(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("evalsymlinks %s: %v", name, err)
	}
	return c
}

// --- SOP-HARD-03 recovery-path regression: multi-path entry with one unread file ---

// TestVerifySatisfiedUnreadMultiPathMismatch reproduced the HARD-04 blocker: a
// criterion whose evidence entry cites two files where only ONE was inspected is
// rejected by the verifier (UNREAD_EVIDENCE_PATH) while the one-shot correction
// trigger does not offer a correction (it treated the criterion as covered because a
// single cited path had been read). The two paths now agree, so the correction fires.
func TestVerifySatisfiedUnreadMultiPathMismatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package a\n")
	writeFile(t, dir, "b.go", "package a\n")
	cfg := testConfig("http://127.0.0.1:1")
	h := New(cfg, dir)
	req := agent.Request{Capability: agent.Implement, AcceptanceCriteria: []string{"c1"}, ValidationCommands: []string{"go vet ./..."}}

	fp, err := h.tools.RepositoryFingerprint(context.Background())
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	p := satisfProof(fp)
	p.reads[canonicalPath(t, dir, "a.go")] = true // A inspected; B deliberately not.
	id := commandIdentity("go vet ./...")
	p.ran[id] = true
	p.commands[id] = true

	final := evidenceFinal("c1", []any{"a.go", "b.go"})

	// The verifier rejects: every cited path must have been inspected.
	if ok, rej := h.verifySatisfiedDetailed(context.Background(), req, &executionState{}, p, final); ok || rej.Code != RejectUnreadEvidencePath {
		t.Fatalf("verify: ok=%v code=%s, want rejection UNREAD_EVIDENCE_PATH", ok, rej.Code)
	}

	// The correction trigger must offer the single evidence-only correction for the
	// same input: a criterion citing any unread path is uncovered.
	need, offered := h.correctionNeed(context.Background(), req, &executionState{}, p, final)
	if !offered {
		t.Fatalf("the correction was not offered for a criterion with an unread cited path (mismatch not fixed): need=%+v", need)
	}
	if len(need.MissingCriteria) != 1 || need.MissingCriteria[0] != "c1" {
		t.Fatalf("missing criteria = %v, want [c1]", need.MissingCriteria)
	}
	// The trigger never weakens the verifier: the invalid claim is still rejected.
	if ok, _ := h.verifySatisfiedDetailed(context.Background(), req, &executionState{}, p, final); ok {
		t.Fatal("the invalid multi-path claim must never be accepted")
	}
}

// multiPathUnreadClaim cites two files while only add.go has been read: it is the
// multi-path form of an incomplete claim that the single correction is meant to fix.
const multiPathUnreadClaim = `{"status":"completed","completion":"ALREADY_SATISFIED","changes_expected":false,"evidence":{"acceptance":[{"criterion":"Add returns the sum","paths":["./add.go","add_test.go"]}]}}`

// TestAlreadySatisfiedEvidenceCorrectionMultiPathAccepted: a claim whose evidence cites
// two files while only one was read now triggers the single correction; once the agent
// reads the second file, the corrected claim is accepted (zero mutations).
func TestAlreadySatisfiedEvidenceCorrectionMultiPathAccepted(t *testing.T) {
	dir, req := satisfactionFixture(t)
	gitInit(t, dir)
	commitAll(t, dir, "existing implementation")
	req.Capability = agent.Implement
	_, srv := newFakeOllama(t, readAdd, validateAdd, multiPathUnreadClaim, readAddTest, satisfiedFinal)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 30
	native := NewNativeAgent(cfg, dir, nil)
	response, err := native.Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !response.VerifiedAlreadySatisfied {
		t.Fatalf("multi-path correction not accepted: %+v", response)
	}
}

// TestAlreadySatisfiedEvidenceCorrectionMultiPathExhausted: the single correction is
// consumed at most once; a second multi-path-incomplete claim is rejected outright.
func TestAlreadySatisfiedEvidenceCorrectionMultiPathExhausted(t *testing.T) {
	dir, req := satisfactionFixture(t)
	req.Capability = agent.Implement
	_, srv := newFakeOllama(t, readAdd, validateAdd, multiPathUnreadClaim, multiPathUnreadClaim)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 30
	h := New(cfg, dir)
	ev := &mutationEvidence{}
	if _, _, err := h.CompleteWithEvidence(context.Background(), req, ev); err == nil || ev.alreadySatisfied {
		t.Fatalf("exhausted multi-path correction accepted: err=%v evidence=%+v", err, ev)
	}
}
