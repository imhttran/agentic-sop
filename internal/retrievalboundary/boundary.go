// Package retrievalboundary is the backend-neutral bounded retrieval boundary. A
// ScopedQuery carries the corpus scope, the caller context, and hard budgets. A Result
// carries bounded evidence records with citations and a compatibility note.
//
// The boundary enforces a result count limit, a time budget, and a token budget at the
// type level, so neither backend can return an unbounded result. Errors are typed so a
// caller can distinguish an optional-retrieval fallback from a required-evidence block.
package retrievalboundary

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Corpus is the retrieval corpus scope. A query names exactly one corpus; evidence from
// another corpus is never returned.
type Corpus string

const (
	// CorpusRun is recorded run history.
	CorpusRun Corpus = "run"
	// CorpusDecision is recorded engineering decisions.
	CorpusDecision Corpus = "decision"
	// CorpusFailure is recorded failure and recovery history.
	CorpusFailure Corpus = "failure"
	// CorpusOutcome is recorded verified outcomes.
	CorpusOutcome Corpus = "outcome"
)

// Valid reports whether c is a known corpus.
func (c Corpus) Valid() bool {
	switch c {
	case CorpusRun, CorpusDecision, CorpusFailure, CorpusOutcome:
		return true
	}
	return false
}

// Default budgets. A caller may lower them; Validate rejects an unset or out-of-range
// budget so an unbounded read is impossible.
const (
	DefaultMaxResults     = 5
	MaxResultsCeiling     = 50
	DefaultTimeBudget     = 2 * time.Second
	MaxTimeBudgetCeiling  = 30 * time.Second
	DefaultTokenBudget    = 2000
	MaxTokenBudgetCeiling = 32000
)

// Scope is the corpus and caller context a query is limited to.
type Scope struct {
	Corpus     Corpus
	Repository string
	TaskID     string
}

// Limits are the hard bounds applied to a result.
type Limits struct {
	// MaxResults is the maximum number of evidence records.
	MaxResults int
	// TimeBudget bounds the retrieval latency. Zero selects DefaultTimeBudget.
	TimeBudget time.Duration
	// TokenBudget bounds the total estimated tokens across evidence records. Zero
	// selects DefaultTokenBudget.
	TokenBudget int
}

// ScopedQuery is one bounded retrieval request.
type ScopedQuery struct {
	Scope  Scope
	Text   string
	Limits Limits
}

// Evidence is one bounded evidence record with its citation and score.
type Evidence struct {
	RecordID    string
	ContentHash string
	Corpus      Corpus
	Repository  string
	TaskID      string
	Citation    string
	// Text is the bounded, searchable record body. It is untrusted evidence: a
	// caller must treat it as data, never as an instruction or an approval.
	Text string
	// Status is the record's lifecycle status label (for example an outcome
	// status), when the backend records one. It is metadata, not an instruction.
	Status string
	Score  float64
	Tokens int
}

// Result is the bounded retrieval result.
type Result struct {
	Evidence      []Evidence
	Compatibility string
	Tokens        int
}

// Typed boundary errors. A caller matches them with errors.Is so an optional-retrieval
// fallback and a required-evidence block stay distinct decisions.
var (
	// ErrInvalidQuery is returned for an invalid or unsupported query input.
	ErrInvalidQuery = errors.New("retrievalboundary: invalid query")
	// ErrTimeout is returned when the time budget is exhausted.
	ErrTimeout = errors.New("retrievalboundary: timeout")
	// ErrUnsupportedCorpus is returned when the requested corpus is unknown.
	ErrUnsupportedCorpus = errors.New("retrievalboundary: unsupported corpus")
	// ErrIndexCorrupt is returned when the index reports corruption.
	ErrIndexCorrupt = errors.New("retrievalboundary: index corrupt")
	// ErrLockTimeout is returned when a writer lock cannot be acquired in time.
	ErrLockTimeout = errors.New("retrievalboundary: lock timeout")
)

// InvalidQueryError reports an invalid query input.
type InvalidQueryError struct{ Reason string }

func (e *InvalidQueryError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidQuery.Error(), e.Reason)
}

// Unwrap makes errors.Is(err, ErrInvalidQuery) true.
func (e *InvalidQueryError) Unwrap() error { return ErrInvalidQuery }

// UnsupportedCorpusError reports an unknown corpus.
type UnsupportedCorpusError struct{ Corpus Corpus }

func (e *UnsupportedCorpusError) Error() string {
	return fmt.Sprintf("%s: %q", ErrUnsupportedCorpus.Error(), string(e.Corpus))
}

// Unwrap makes errors.Is(err, ErrUnsupportedCorpus) true.
func (e *UnsupportedCorpusError) Unwrap() error { return ErrUnsupportedCorpus }

// Retriever is the shared bounded retrieval interface. Every backend implements it.
type Retriever interface {
	Retrieve(ctx context.Context, q ScopedQuery) (Result, error)
}

// Validate checks a query and returns the normalized limits it must be served under.
func Validate(q ScopedQuery) (Limits, error) {
	if !q.Scope.Corpus.Valid() {
		return Limits{}, &UnsupportedCorpusError{Corpus: q.Scope.Corpus}
	}
	lim := q.Limits
	if lim.MaxResults < 0 || lim.MaxResults > MaxResultsCeiling {
		return Limits{}, &InvalidQueryError{Reason: fmt.Sprintf("max results %d out of range", lim.MaxResults)}
	}
	if lim.MaxResults == 0 {
		lim.MaxResults = DefaultMaxResults
	}
	if lim.TimeBudget < 0 || lim.TimeBudget > MaxTimeBudgetCeiling {
		return Limits{}, &InvalidQueryError{Reason: "time budget out of range"}
	}
	if lim.TimeBudget == 0 {
		lim.TimeBudget = DefaultTimeBudget
	}
	if lim.TokenBudget < 0 || lim.TokenBudget > MaxTokenBudgetCeiling {
		return Limits{}, &InvalidQueryError{Reason: "token budget out of range"}
	}
	if lim.TokenBudget == 0 {
		lim.TokenBudget = DefaultTokenBudget
	}
	return lim, nil
}

// EstimateTokens estimates the token cost of a text. The estimate is intentionally
// simple and deterministic: one token per four characters, rounded up.
func EstimateTokens(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	return (len([]rune(s)) + 3) / 4
}

// ApplyBounds enforces the result count and token budget on evidence. The order is
// preserved; ties keep the caller's order, which the caller must make deterministic by
// record ID. It returns the bounded evidence and the total token estimate. The token
// budget is a hard bound: an item that would exceed it is never included, even when it
// is the first candidate.
func ApplyBounds(ev []Evidence, lim Limits) Result {
	out := make([]Evidence, 0, len(ev))
	total := 0
	for _, e := range ev {
		if len(out) >= lim.MaxResults {
			break
		}
		if e.Tokens > 0 && total+e.Tokens > lim.TokenBudget {
			break
		}
		out = append(out, e)
		total += e.Tokens
	}
	return Result{Evidence: out, Tokens: total}
}

// SortDeterministic orders evidence by score descending and then by record ID ascending,
// so two searches over the same source records return the same order.
func SortDeterministic(ev []Evidence) {
	sort.SliceStable(ev, func(i, j int) bool {
		if ev[i].Score != ev[j].Score {
			return ev[i].Score > ev[j].Score
		}
		return ev[i].RecordID < ev[j].RecordID
	})
}

// Tokenize lowers and splits text into [a-z0-9_] tokens.
func Tokenize(s string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
	}
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}
