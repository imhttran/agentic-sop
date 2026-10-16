//go:build ragacceptance

// This file is the SOP acceptance test for RAG-SOP-002. It is compiled ONLY
// under the `ragacceptance` build tag, so the ordinary repository run
// (`go test ./...`, without the tag) does not include it and stays fast.
//
// The RAG-SOP-002 acceptance check runs it explicitly:
//
//	go test -tags ragacceptance ./internal/ragclient/...
//
// It MUST execute and MUST FAIL when the retrieval producer binary, the
// PostgreSQL corpus, or the environment is unavailable. It never skips: a
// missing producer, database, or corpus is a failure, so a hollow pass is
// impossible.
package ragclient

import (
	"context"
	"os"
	"testing"
	"time"
)

// requireEnv fails (never skips) when a required environment variable is unset.
func requireEnv(t *testing.T, name string) string {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		t.Fatalf("%s is not set: the RAG-SOP-002 acceptance check requires a live producer and PostgreSQL corpus (this must fail, not skip)", name)
	}
	return v
}

// TestAcceptanceProducerContract runs the real rag-template retrieval producer
// through this client and verifies the rag-retrieval/v1 contract and corpus
// isolation end to end. It fails closed on any missing dependency.
func TestAcceptanceProducerContract(t *testing.T) {
	exe := requireEnv(t, "RAG_PRODUCER")
	requireEnv(t, "DATABASE_URL")

	if st, err := os.Stat(exe); err != nil {
		t.Fatalf("RAG_PRODUCER %q is not executable: %v", exe, err)
	} else if st.IsDir() {
		t.Fatalf("RAG_PRODUCER %q is a directory", exe)
	}

	c, err := New(Config{Executable: exe, AllowedCorpora: []string{"kb"}, Timeout: 60 * time.Second, MaxResults: 5})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	res, err := c.Retrieve(context.Background(), Request{Query: "retrieval fallback", TopK: 5, Filters: Filters{Corpus: "kb"}})
	if err != nil {
		t.Fatalf("Retrieve(kb): %v", err)
	}
	if len(res.Evidence) == 0 {
		t.Fatal("no evidence returned: the producer, database, or corpus fixture is unavailable (must fail, not pass)")
	}
	for _, ev := range res.Evidence {
		if ev.Corpus != "kb" {
			t.Fatalf("cross-corpus leak: corpus=%q", ev.Corpus)
		}
		if ev.Source == "" || ev.Section == "" || ev.DocumentHash == "" {
			t.Fatalf("incomplete provenance: %+v", ev)
		}
		if ev.RetrievalMethod == "" || ev.Rank <= 0 || ev.Text == "" {
			t.Fatalf("incomplete evidence: %+v", ev)
		}
	}

	// Client-side rejection: a corpus outside the client allowlist never reaches
	// the producer.
	if _, err := c.Retrieve(context.Background(), Request{Query: "x", Filters: Filters{Corpus: "nope"}}); CodeOf(err) != CodeUnauthorizedScope {
		t.Fatalf("client allowlist: code=%s, want %s", CodeOf(err), CodeUnauthorizedScope)
	}

	// Producer-side rejection: the client allowlist includes "other" but the
	// producer's own allowlist does not, so the producer must reject it.
	c2, err := New(Config{Executable: exe, AllowedCorpora: []string{"kb", "other"}, Timeout: 60 * time.Second, MaxResults: 5})
	if err != nil {
		t.Fatalf("New(c2): %v", err)
	}
	if _, err := c2.Retrieve(context.Background(), Request{Query: "x", Filters: Filters{Corpus: "other"}}); CodeOf(err) != CodeUnauthorizedScope {
		t.Fatalf("producer allowlist: code=%s, want %s", CodeOf(err), CodeUnauthorizedScope)
	}
}
