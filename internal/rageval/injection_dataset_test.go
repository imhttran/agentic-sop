package rageval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommittedInjectionQuestionsHaveInjectedText proves every committed
// injection question loads with a non-empty InjectedText that appears verbatim
// in its planted file.
func TestCommittedInjectionQuestionsHaveInjectedText(t *testing.T) {
	dir := filepath.Join("..", "..", "evals", "rag")
	ds, err := Load(dir)
	if err != nil {
		t.Fatalf("load committed dataset: %v", err)
	}
	root := CorpusRoot(ds)
	count := 0
	for _, q := range ds.Questions {
		if q.Category != CategoryInjection {
			continue
		}
		count++
		if strings.TrimSpace(q.InjectedText) == "" {
			t.Fatalf("injection question %s: InjectedText is empty", q.ID)
		}
		var planted, phrase string
		for _, e := range q.Expected {
			if isInjectionSource(e.Source) {
				planted = e.Source
				if len(e.Evidence) > 0 {
					phrase = e.Evidence[0]
				}
				break
			}
		}
		if planted == "" {
			t.Fatalf("injection question %s: no expected evidence under injection/", q.ID)
		}
		if phrase == "" {
			t.Fatalf("injection question %s: planted entry %s has no evidence phrase", q.ID, planted)
		}
		if q.InjectedText != phrase {
			t.Fatalf("injection question %s: InjectedText = %q, want evidence phrase %q", q.ID, q.InjectedText, phrase)
		}
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(planted)))
		if err != nil {
			t.Fatalf("injection question %s: read planted file %s: %v", q.ID, planted, err)
		}
		if !strings.Contains(string(raw), q.InjectedText) {
			t.Fatalf("injection question %s: InjectedText does not appear verbatim in %s", q.ID, planted)
		}
	}
	if count == 0 {
		t.Fatal("committed dataset holds no injection questions")
	}
}
