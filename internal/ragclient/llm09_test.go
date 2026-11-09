package ragclient

import (
	"context"
	"strings"
	"testing"
)

func TestLLM09CrossCorpusCanaryRejected(t *testing.T) {
	allowed := []string{"project-a", "project-b"}
	evA := okEvidence()
	evA.Evidence[0].Corpus = "project-a"
	evA.Evidence[0].Source = "docs/shared.md"
	evA.Evidence[0].Text = "CANARY-A"
	evB := okEvidence()
	evB.Evidence[0].Corpus = "project-b"
	evB.Evidence[0].Source = "docs/shared.md"
	evB.Evidence[0].Text = "CANARY-B"

	mixed := okEvidence()
	mixed.Evidence = append(evA.Evidence, evB.Evidence...)
	c := newTestClient(t, Config{Executable: "/producer", AllowedCorpora: allowed}, &fakeRunner{out: mustJSON(t, mixed)})
	_, err := c.Retrieve(context.Background(), Request{Query: "canary", Filters: Filters{Corpus: "project-a"}})
	if CodeOf(err) != CodeUnauthorizedScope {
		t.Fatalf("code = %s, want %s (%v)", CodeOf(err), CodeUnauthorizedScope, err)
	}

	control := newTestClient(t, Config{Executable: "/producer", AllowedCorpora: allowed}, &fakeRunner{out: mustJSON(t, evA)})
	res, err := control.Retrieve(context.Background(), Request{Query: "canary", Filters: Filters{Corpus: "project-a"}})
	if err != nil {
		t.Fatalf("control retrieval: %v", err)
	}
	if len(res.Evidence) != 1 || !strings.Contains(res.Evidence[0].Text, "CANARY-A") || strings.Contains(res.Evidence[0].Text, "CANARY-B") {
		t.Fatalf("control evidence = %+v, want only CANARY-A", res.Evidence)
	}
}
