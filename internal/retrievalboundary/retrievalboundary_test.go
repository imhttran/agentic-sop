package retrievalboundary

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/retrieval"
)

func TestValidateRejectsUnboundedOrInvalidLimits(t *testing.T) {
	ok := ScopedQuery{Scope: Scope{Corpus: CorpusOutcome}, Text: "x"}
	if lim, err := Validate(ok); err != nil || lim.MaxResults != DefaultMaxResults {
		t.Fatalf("defaults = %+v, %v", lim, err)
	}
	for name, q := range map[string]ScopedQuery{
		"unknown corpus":         {Scope: Scope{Corpus: Corpus("bogus")}},
		"too many results":       {Scope: Scope{Corpus: CorpusOutcome}, Limits: Limits{MaxResults: MaxResultsCeiling + 1}},
		"negative results":       {Scope: Scope{Corpus: CorpusOutcome}, Limits: Limits{MaxResults: -1}},
		"time budget too large":  {Scope: Scope{Corpus: CorpusOutcome}, Limits: Limits{TimeBudget: MaxTimeBudgetCeiling + time.Second}},
		"token budget too large": {Scope: Scope{Corpus: CorpusOutcome}, Limits: Limits{TokenBudget: MaxTokenBudgetCeiling + 1}},
	} {
		if _, err := Validate(q); err == nil {
			t.Errorf("%s: Validate accepted an invalid query", name)
		}
	}
}

func TestApplyBoundsEnforcesCountAndTokens(t *testing.T) {
	ev := []Evidence{
		{RecordID: "a", Tokens: 10},
		{RecordID: "b", Tokens: 10},
		{RecordID: "c", Tokens: 10},
	}
	res := ApplyBounds(ev, Limits{MaxResults: 2, TokenBudget: 100})
	if len(res.Evidence) != 2 || res.Tokens != 20 {
		t.Fatalf("count bound = %+v", res)
	}
	res = ApplyBounds(ev, Limits{MaxResults: 10, TokenBudget: 15})
	if len(res.Evidence) != 1 || res.Tokens != 10 {
		t.Fatalf("token bound = %+v", res)
	}
}

func TestApplyBoundsRejectsItemOverBudget(t *testing.T) {
	// A hard token budget is never exceeded, even by the first candidate.
	res := ApplyBounds([]Evidence{{RecordID: "a", Tokens: 50}, {RecordID: "b", Tokens: 1}}, Limits{MaxResults: 10, TokenBudget: 20})
	if len(res.Evidence) != 0 || res.Tokens != 0 {
		t.Fatalf("over-budget first item was included: %+v", res)
	}
}

func TestSortDeterministicBreaksTiesByRecordID(t *testing.T) {
	ev := []Evidence{
		{RecordID: "c", Score: 1},
		{RecordID: "a", Score: 2},
		{RecordID: "b", Score: 1},
	}
	SortDeterministic(ev)
	got := []string{ev[0].RecordID, ev[1].RecordID, ev[2].RecordID}
	want := []string{"a", "b", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestErrorsAreTyped(t *testing.T) {
	cases := []error{
		&InvalidQueryError{Reason: "x"},
		&UnsupportedCorpusError{Corpus: "bogus"},
		ErrTimeout,
		ErrIndexCorrupt,
		ErrLockTimeout,
	}
	want := []error{ErrInvalidQuery, ErrUnsupportedCorpus, ErrTimeout, ErrIndexCorrupt, ErrLockTimeout}
	for i, err := range cases {
		if !errors.Is(err, want[i]) {
			t.Errorf("%v does not match %v", err, want[i])
		}
	}
}

func TestTokenEstimateIsBoundedAndDeterministic(t *testing.T) {
	if got := EstimateTokens(""); got != 0 {
		t.Errorf("empty = %d", got)
	}
	if a, b := EstimateTokens("abcdefgh"), EstimateTokens("abcdefgh"); a != b {
		t.Errorf("not deterministic: %d vs %d", a, b)
	}
	if got := Tokenize("RDM-002B: failure_signature (empty)"); len(got) < 4 {
		t.Errorf("tokens = %v", got)
	}
}

// fakeRetriever proves the interface is satisfiable by a non-SQLite backend.
type fakeRetriever struct{}

func (fakeRetriever) Retrieve(context.Context, ScopedQuery) (Result, error) {
	return Result{Evidence: []Evidence{{RecordID: "x"}}}, nil
}

func TestRetrieverInterfaceIsBackendNeutral(t *testing.T) {
	var r Retriever = fakeRetriever{}
	res, err := r.Retrieve(context.Background(), ScopedQuery{Scope: Scope{Corpus: CorpusOutcome}})
	if err != nil || len(res.Evidence) != 1 {
		t.Fatalf("Retrieve = %+v, %v", res, err)
	}
}

func TestRankerAdapterRejectsScopeItCannotHonor(t *testing.T) {
	a := &RankerAdapter{Index: retrieval.New(nil), Corpus: CorpusOutcome}
	for name, scope := range map[string]Scope{
		"repository": {Corpus: CorpusOutcome, Repository: "agentic-sop"},
		"task":       {Corpus: CorpusOutcome, TaskID: "RDM-003"},
	} {
		_, err := a.Retrieve(context.Background(), ScopedQuery{Scope: scope, Text: "failure"})
		if !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("%s: err = %v, want an InvalidQueryError so evidence from another scope is never returned", name, err)
		}
	}
}
