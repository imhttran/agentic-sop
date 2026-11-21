package deceleval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func abCorpusPaths() (corpus, freeze string) {
	base := filepath.Join("..", "..", "evals", "decision-model")
	return filepath.Join(base, "disposition-ab-v1.json"), filepath.Join(base, "DISPOSITION-AB.sha256")
}

// TestABCorpusLoadsValidatesAndIsFrozen proves the committed A/B corpus is
// well-formed, has the required 12/36 split with 2/6 cases per governed action,
// and still matches its frozen hash.
func TestABCorpusLoadsValidatesAndIsFrozen(t *testing.T) {
	corpusPath, freezePath := abCorpusPaths()
	c, err := LoadAB(corpusPath)
	if err != nil {
		t.Fatalf("LoadAB: %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := VerifyFreeze(corpusPath, freezePath); err != nil {
		t.Fatalf("VerifyFreeze: %v", err)
	}
	promptFreeze := filepath.Join(filepath.Dir(corpusPath), "DISPOSITION-AB-PROMPT.sha256")
	data, err := os.ReadFile(promptFreeze)
	if err != nil {
		t.Fatalf("read prompt freeze: %v", err)
	}
	if fields := strings.Fields(string(data)); len(fields) == 0 || fields[0] != ABPromptSHA256() {
		t.Fatalf("prompt freeze mismatch: file has %v, code has %s", fields, ABPromptSHA256())
	}
	if n := len(c.Split(ABSplitSmoke)); n != 12 {
		t.Errorf("smoke cases = %d, want 12", n)
	}
	if n := len(c.Split(ABSplitHeldout)); n != 36 {
		t.Errorf("held-out cases = %d, want 36", n)
	}
	smoke := map[string]int{}
	heldout := map[string]int{}
	for _, s := range c.Scenarios {
		switch s.Split {
		case ABSplitSmoke:
			smoke[string(s.Expected.Action)]++
		case ABSplitHeldout:
			heldout[string(s.Expected.Action)]++
		}
	}
	for _, a := range ABChoices {
		if smoke[a] != 2 || heldout[a] != 6 {
			t.Errorf("%s: smoke=%d heldout=%d, want 2/6", a, smoke[a], heldout[a])
		}
	}
}

// TestABRequestKeepsLabelsOut proves both models receive the identical frozen
// question and that the request never carries a case's expected label or
// rationale. The expected-action vocabulary appears only as the closed choice set.
func TestABRequestKeepsLabelsOut(t *testing.T) {
	corpusPath, _ := abCorpusPaths()
	c, err := LoadAB(corpusPath)
	if err != nil {
		t.Fatalf("LoadAB: %v", err)
	}
	spec := ABPromptSpec()
	for _, s := range c.Scenarios {
		req := BuildABRequest(s)
		if req.State != s.Observable {
			t.Fatalf("%s: state changed", s.ID)
		}
		if len(req.Questions) != 1 {
			t.Fatalf("%s: questions = %d, want 1", s.ID, len(req.Questions))
		}
		q := req.Questions[0]
		if q.ID != ABQuestionID || q.Type != "choice" || q.Criteria != spec.Criteria {
			t.Fatalf("%s: unexpected question %+v", s.ID, q)
		}
		if strings.Join(q.Choices, ",") != strings.Join(ABChoices, ",") {
			t.Fatalf("%s: choices = %v, want %v", s.ID, q.Choices, ABChoices)
		}
		data, _ := json.Marshal(req)
		text := string(data)
		if strings.Contains(text, s.Expected.Rationale) {
			t.Fatalf("%s: request leaked the expected rationale", s.ID)
		}
		if strings.Contains(strings.ToLower(text), "expected") {
			t.Fatalf("%s: request carries an expected-field marker", s.ID)
		}
	}
}

func abStrp(v string) *string { return &v }
func abIntp(v int) *int       { return &v }

// TestABSummarizeFixtures verifies the four measured failure modes, the
// per-action breakdown, and batch-wall-time accounting, including an incomplete
// batch.
func TestABSummarizeFixtures(t *testing.T) {
	attempts := []ABAttempt{
		{CaseID: "ok", Model: "m", Concurrency: 1, ExpectedAction: "AUTO_FIX", ActualAction: abStrp("AUTO_FIX"), SchemaValid: true, Correct: true, ElapsedMS: 100},
		{CaseID: "wrong", Model: "m", Concurrency: 1, ExpectedAction: "BLOCK", ActualAction: abStrp("AUTO_FIX"), SchemaValid: true, ElapsedMS: 200},
		{CaseID: "malformed", Model: "m", Concurrency: 1, ExpectedAction: "NEEDS_HUMAN", ErrorCategory: abStrp("malformed"), ElapsedMS: 300},
		{CaseID: "timeout", Model: "m", Concurrency: 1, ExpectedAction: "RETRY", ErrorCategory: abStrp("timeout"), ElapsedMS: 1000},
		{CaseID: "http", Model: "m", Concurrency: 1, ExpectedAction: "CONTINUE", ErrorCategory: abStrp("http_error"), ElapsedMS: 500},
	}
	s := SummarizeAB(attempts, 5, 30*time.Second)
	if !s.Complete || s.Attempted != 5 || s.Cases != 5 {
		t.Fatalf("complete = %v attempted = %d cases = %d, want true/5/5", s.Complete, s.Attempted, s.Cases)
	}
	if s.Correct != 1 || s.SchemaValid != 2 {
		t.Fatalf("correct = %d schema-valid = %d, want 1/2", s.Correct, s.SchemaValid)
	}
	if s.WrongActions != 1 || s.InvalidResponses != 1 || s.Timeouts != 1 || s.OtherErrors != 1 {
		t.Fatalf("wrong=%d invalid=%d timeout=%d other=%d, want 1/1/1/1", s.WrongActions, s.InvalidResponses, s.Timeouts, s.OtherErrors)
	}
	if s.Fallbacks != 0 {
		t.Fatalf("fallbacks = %d, want 0", s.Fallbacks)
	}
	if s.LatencySlowestMS != 1000 || s.SlowestCase != "timeout" {
		t.Fatalf("slowest = %d (%s), want 1000 (timeout)", s.LatencySlowestMS, s.SlowestCase)
	}
	if s.LatencyMedianMS != 300 {
		t.Fatalf("median = %.0f, want 300", s.LatencyMedianMS)
	}
	if s.WallMS != 30000 {
		t.Fatalf("wall = %d, want 30000", s.WallMS)
	}
	if got := s.CorrectPerMinute; got < 1.99 || got > 2.01 {
		t.Fatalf("correct/min = %.4f, want 2.00", got)
	}
	if s.ByAction["BLOCK"].Wrong != 1 || s.ByAction["NEEDS_HUMAN"].Invalid != 1 || s.ByAction["RETRY"].Timeout != 1 {
		t.Fatalf("per-action buckets not populated: %+v", s.ByAction)
	}

	// One case short of the scheduled set is an explicitly incomplete batch.
	short := SummarizeAB(attempts, 6, time.Second)
	if short.Complete {
		t.Fatal("a batch with fewer attempts than cases must not be complete")
	}
}

// TestABConsequentialDisagreements verifies that every expected BLOCK/NEEDS_HUMAN
// miss is reported with the selected action (or its absence), and that
// AUTO_FIX-family misses are not mixed in.
func TestABConsequentialDisagreements(t *testing.T) {
	attempts := []ABAttempt{
		{CaseID: "block-wrong", Model: "m", Concurrency: 1, ExpectedAction: "BLOCK", ActualAction: abStrp("AUTO_FIX"), SchemaValid: true},
		{CaseID: "human-malformed", Model: "m", Concurrency: 1, ExpectedAction: "NEEDS_HUMAN", ErrorCategory: abStrp("malformed")},
		{CaseID: "human-correct", Model: "m", Concurrency: 1, ExpectedAction: "NEEDS_HUMAN", ActualAction: abStrp("NEEDS_HUMAN"), SchemaValid: true, Correct: true},
		{CaseID: "fix-wrong", Model: "m", Concurrency: 1, ExpectedAction: "AUTO_FIX", ActualAction: abStrp("BLOCK"), SchemaValid: true},
		{CaseID: "block-timeout", Model: "m", Concurrency: 1, ExpectedAction: "BLOCK", ErrorCategory: abStrp("timeout")},
	}
	got := ConsequentialDisagreements(attempts)
	if len(got) != 3 {
		t.Fatalf("disagreements = %d, want 3: %+v", len(got), got)
	}
	byID := map[string]ABDisagreement{}
	for _, d := range got {
		byID[d.CaseID] = d
	}
	if d := byID["block-wrong"]; d.SelectedAction == nil || *d.SelectedAction != "AUTO_FIX" {
		t.Fatalf("block-wrong selected = %v, want AUTO_FIX", d.SelectedAction)
	}
	if d := byID["human-malformed"]; d.SelectedAction != nil || d.ErrorCategory == nil || *d.ErrorCategory != "malformed" {
		t.Fatalf("human-malformed = %+v, want no selection and malformed", d)
	}
	if d := byID["block-timeout"]; d.SelectedAction != nil || d.ErrorCategory == nil || *d.ErrorCategory != "timeout" {
		t.Fatalf("block-timeout = %+v, want no selection and timeout", d)
	}
}

// TestABRunBatchRecordsEveryCase proves the pool dispatches every case exactly
// once, preserves the corpus case order in the returned slice, and reports a
// positive wall time at concurrency 1 and 4.
func TestABRunBatchRecordsEveryCase(t *testing.T) {
	cases := []Scenario{
		{ID: "a", Split: ABSplitSmoke, Observable: "x", Expected: Expected{Action: AutoFix}},
		{ID: "b", Split: ABSplitSmoke, Observable: "y", Expected: Expected{Action: Retry}},
		{ID: "c", Split: ABSplitSmoke, Observable: "z", Expected: Expected{Action: Block}},
		{ID: "d", Split: ABSplitSmoke, Observable: "w", Expected: Expected{Action: NeedsHuman}},
	}
	for _, conc := range []int{1, 4} {
		var calls int
		decide := func(ctx context.Context, s Scenario) ABObservation {
			time.Sleep(2 * time.Millisecond)
			return ABObservation{SchemaValid: true, ActualAction: string(s.Expected.Action)}
		}
		attempts, wall := RunABBatch(context.Background(), "m", ABSplitSmoke, cases, conc, 5*time.Second, decide, func(ABAttempt) { calls++ })
		if len(attempts) != len(cases) {
			t.Fatalf("conc=%d: attempts = %d, want %d", conc, len(attempts), len(cases))
		}
		if calls != len(cases) {
			t.Fatalf("conc=%d: callback calls = %d, want %d", conc, calls, len(cases))
		}
		for i, a := range attempts {
			if a.CaseID != cases[i].ID || a.Attempt != 1 {
				t.Fatalf("conc=%d: attempt %d = %s (attempt %d), want %s", conc, i, a.CaseID, a.Attempt, cases[i].ID)
			}
			if !a.Correct {
				t.Fatalf("conc=%d: %s not correct: %+v", conc, a.CaseID, a)
			}
		}
		if wall <= 0 {
			t.Fatalf("conc=%d: wall = %v, want positive", conc, wall)
		}
		sum := SummarizeAB(attempts, len(cases), wall)
		if sum.Correct != len(cases) || !sum.Complete {
			t.Fatalf("conc=%d: summary = %+v", conc, sum)
		}
	}
}

// TestABEndpointDiscovery verifies model-id discovery and the health payload.
func TestABEndpointDiscovery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "uvicorn")
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": []map[string]any{
				{"id": "ModelA", "model_type": "llm", "max_model_len": 32768, "owned_by": "omlx"},
				{"id": "ModelB", "model_type": "vlm", "max_model_len": 1048576, "owned_by": "omlx"},
			}})
		case "/health":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"status": "healthy", "default_model": "ModelA"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	info := DiscoverABEndpoint(context.Background(), srv.URL+"/", 2*time.Second)
	if info.Error != "" {
		t.Fatalf("discover: %s", info.Error)
	}
	if info.Server != "uvicorn" {
		t.Fatalf("server = %q, want uvicorn", info.Server)
	}
	if !info.HasModel("ModelA") || !info.HasModel("ModelB") || info.HasModel("ModelC") {
		t.Fatalf("model discovery = %v", info.ModelIDs())
	}
	if len(info.Health) == 0 || !strings.Contains(string(info.Health), "healthy") {
		t.Fatalf("health = %s", info.Health)
	}
}

// TestABRunBatchStopsDispatchingOnCancel proves an interrupted batch stops
// dispatching cases and reports the interrupted batch as incomplete, so a
// canceled quick run cannot later satisfy the full-run smoke guard.
func TestABRunBatchStopsDispatchingOnCancel(t *testing.T) {
	cases := []Scenario{
		{ID: "a", Split: ABSplitSmoke, Observable: "a", Expected: Expected{Action: AutoFix}},
		{ID: "b", Split: ABSplitSmoke, Observable: "b", Expected: Expected{Action: Continue}},
		{ID: "c", Split: ABSplitSmoke, Observable: "c", Expected: Expected{Action: Retry}},
		{ID: "d", Split: ABSplitSmoke, Observable: "d", Expected: Expected{Action: Block}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	decide := func(_ context.Context, s Scenario) ABObservation {
		if calls.Add(1) == 1 {
			cancel()
		}
		return ABObservation{SchemaValid: true, ActualAction: string(s.Expected.Action)}
	}
	attempts, _ := RunABBatch(ctx, "m", ABSplitSmoke, cases, 1, 5*time.Second, decide, nil)
	if len(attempts) >= len(cases) {
		t.Fatalf("attempts = %d, want fewer than %d after cancellation", len(attempts), len(cases))
	}
	sum := SummarizeAB(attempts, len(cases), time.Second)
	if sum.Complete {
		t.Fatalf("an interrupted batch must not be complete: %+v", sum)
	}
	if sum.Attempted != len(attempts) {
		t.Fatalf("attempted = %d, want %d", sum.Attempted, len(attempts))
	}
}
