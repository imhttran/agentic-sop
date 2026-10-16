package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/ragclient"
)

type fakeRetrievalClient struct {
	resp ragclient.Response
	err  error
	last ragclient.Request
}

func (f *fakeRetrievalClient) Retrieve(_ context.Context, req ragclient.Request) (ragclient.Response, error) {
	f.last = req
	return f.resp, f.err
}

func okResponse(corpus, source string) ragclient.Response {
	return ragclient.Response{
		SchemaVersion: ragclient.SchemaVersion,
		Status:        "OK",
		Evidence: []ragclient.Evidence{
			{ID: "1", Corpus: corpus, Source: source, Section: "s", Text: "evidence", RetrievalMethod: "hybrid", Rank: 1},
		},
	}
}

func verifyTask(evidence ...string) *domain.Task {
	return &domain.Task{ID: "T1", Title: "Add widget", Objective: "add the widget", RequiredEvidence: evidence}
}

func TestRAGVerifierNoRequirementIsNoop(t *testing.T) {
	v := &ragEvidenceVerifier{client: &fakeRetrievalClient{}, corpus: "kb"}
	missing, err := v.VerifyRequiredEvidence(context.Background(), verifyTask())
	if err != nil || len(missing) != 0 {
		t.Fatalf("no-op expected: %v %v", missing, err)
	}
}

func TestRAGVerifierSatisfiedUsesDeterministicQuery(t *testing.T) {
	c := &fakeRetrievalClient{resp: okResponse("kb", "imsop/docs/a.md")}
	v := &ragEvidenceVerifier{client: c, corpus: "kb"}

	missing, err := v.VerifyRequiredEvidence(context.Background(), verifyTask("corpus:kb", "source:imsop/docs/"))
	if err != nil || len(missing) != 0 {
		t.Fatalf("expected satisfied: %v %v", missing, err)
	}
	if c.last.Filters.Corpus != "kb" {
		t.Fatalf("request corpus = %q, want kb", c.last.Filters.Corpus)
	}
	if c.last.Query != "Add widget add the widget" {
		t.Fatalf("query = %q, want deterministic title+objective", c.last.Query)
	}
}

func TestRAGVerifierFailures(t *testing.T) {
	t.Run("empty results", func(t *testing.T) {
		c := &fakeRetrievalClient{resp: ragclient.Response{SchemaVersion: ragclient.SchemaVersion, Status: "OK"}}
		v := &ragEvidenceVerifier{client: c, corpus: "kb"}
		missing, err := v.VerifyRequiredEvidence(context.Background(), verifyTask("corpus:kb"))
		if err == nil || len(missing) != 1 {
			t.Fatalf("expected missing: %v %v", missing, err)
		}
	})
	t.Run("missing source prefix", func(t *testing.T) {
		c := &fakeRetrievalClient{resp: okResponse("kb", "other/a.md")}
		v := &ragEvidenceVerifier{client: c, corpus: "kb"}
		missing, err := v.VerifyRequiredEvidence(context.Background(), verifyTask("source:docs/"))
		if err == nil || len(missing) != 1 {
			t.Fatalf("expected missing source: %v %v", missing, err)
		}
	})
	t.Run("other corpus identity fails closed", func(t *testing.T) {
		c := &fakeRetrievalClient{resp: okResponse("kb", "a.md")}
		v := &ragEvidenceVerifier{client: c, corpus: "kb"}
		missing, err := v.VerifyRequiredEvidence(context.Background(), verifyTask("corpus:other"))
		if err == nil || len(missing) != 1 {
			t.Fatalf("expected fail closed: %v %v", missing, err)
		}
	})
	t.Run("and semantics keeps only the unmet identity", func(t *testing.T) {
		c := &fakeRetrievalClient{resp: okResponse("kb", "other/a.md")}
		v := &ragEvidenceVerifier{client: c, corpus: "kb"}
		missing, err := v.VerifyRequiredEvidence(context.Background(), verifyTask("corpus:kb", "source:docs/"))
		if err == nil || len(missing) != 1 || missing[0] != "source:docs/" {
			t.Fatalf("AND semantics: %v %v", missing, err)
		}
	})
	t.Run("producer unavailable", func(t *testing.T) {
		c := &fakeRetrievalClient{err: errors.New("UNAVAILABLE")}
		v := &ragEvidenceVerifier{client: c, corpus: "kb"}
		missing, err := v.VerifyRequiredEvidence(context.Background(), verifyTask("corpus:kb"))
		if err == nil || len(missing) != 1 {
			t.Fatalf("expected unavailable: %v %v", missing, err)
		}
	})
}

func TestNewRAGEvidenceVerifierDisabledIsNil(t *testing.T) {
	v, err := newRAGEvidenceVerifier(config.Config{})
	if err != nil || v != nil {
		t.Fatalf("disabled RAG must yield a nil verifier: %v %v", v, err)
	}
}
