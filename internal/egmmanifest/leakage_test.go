package egmmanifest_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/egmmanifest"
)

func TestVocabularyPositiveAndNegative(t *testing.T) {
	v, err := egmmanifest.NewVocabulary([]string{syntheticLeak, "SECOND-TERM"})
	if err != nil {
		t.Fatal(err)
	}
	hits := v.Scan([]byte("before " + syntheticLeak + " after"))
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	if hits[0].Offset != len("before ") {
		t.Fatalf("offset = %d", hits[0].Offset)
	}
	if got := v.Scan([]byte("clean text")); len(got) != 0 {
		t.Fatalf("negative scan produced %d hits", len(got))
	}
}

func TestVocabularyDoesNotEchoTerms(t *testing.T) {
	v, _ := egmmanifest.NewVocabulary([]string{syntheticLeak})
	hits := v.Scan([]byte(syntheticLeak))
	rendered := fmt.Sprintf("%+v", hits) + fmt.Sprint(hits)
	if strings.Contains(rendered, syntheticLeak) {
		t.Fatal("a Hit rendered the vocabulary term")
	}
}

func TestVocabularyRejectsEmpty(t *testing.T) {
	if _, err := egmmanifest.NewVocabulary(nil); err == nil {
		t.Fatal("expected empty-vocabulary error")
	}
	if _, err := egmmanifest.NewVocabulary([]string{""}); err == nil {
		t.Fatal("expected empty-term error")
	}
}

func TestScanFile(t *testing.T) {
	p := t.TempDir() + "/leak.txt"
	writeFile(t, p, "a "+syntheticLeak+" b")
	v, _ := egmmanifest.NewVocabulary([]string{syntheticLeak})
	hits, err := v.ScanFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
}
