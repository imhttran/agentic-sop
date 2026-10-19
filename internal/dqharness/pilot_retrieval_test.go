package dqharness

import (
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/internal/decisionquality"
)

// TestPilotRetrievalCoverage reconstructs arm B's retrieval deterministically from the
// frozen corpus. It makes no model call. A case with zero retrieved items means arm B's
// prompt was identical to arm A's for that case, so any accuracy difference cannot be
// attributed to retrieval.
func TestPilotRetrievalCoverage(t *testing.T) {
	dir := "../../evals/decision-memory-pilot"
	for _, split := range []string{"corpus-heldout.json", "corpus-tuning.json"} {
		corpus, err := decisionquality.LoadCorpus(filepath.Join(dir, split))
		if err != nil {
			t.Fatal(err)
		}
		retriever := KeywordRetriever{}
		withEvidence, without := 0, 0
		for _, c := range corpus.Cases {
			got, err := retriever.Retrieve(c, DefaultEvidenceTokens)
			if err != nil {
				t.Fatalf("%s/%s: %v", split, c.ID, err)
			}
			if len(got) == 0 {
				without++
			} else {
				withEvidence++
			}
		}
		t.Logf("%s: %d cases, %d retrieved evidence, %d retrieved none", split, len(corpus.Cases), withEvidence, without)
	}
}
