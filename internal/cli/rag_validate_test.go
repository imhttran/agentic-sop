package cli

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/ragclient"
)

func ev(corpus, source string) ragclient.Evidence {
	return ragclient.Evidence{Corpus: corpus, Source: source, Text: "evidence"}
}

func TestValidateRequiredEvidence(t *testing.T) {
	kb := []ragclient.Evidence{ev("kb", "docs/adr.md")}
	cases := []struct {
		name     string
		required []string
		corpus   string
		evidence []ragclient.Evidence
		want     int
	}{
		{"all satisfied", []string{"corpus:kb", "source:docs/"}, "kb", kb, 0},
		{"missing corpus", []string{"corpus:kb"}, "kb", nil, 1},
		{"missing source", []string{"source:absent/"}, "kb", kb, 1},
		{"unauthorized corpus", []string{"corpus:other"}, "kb", kb, 1},
		{"cross-corpus evidence", []string{"corpus:kb"}, "kb", []ragclient.Evidence{ev("other", "x")}, 1},
		{"and semantics one unmet", []string{"corpus:kb", "source:missing/"}, "kb", kb, 1},
		{"empty evidence", []string{"corpus:kb", "source:docs/"}, "kb", []ragclient.Evidence{}, 2},
		{"malformed requirement", []string{"bogus:x"}, "kb", kb, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			missing, err := validateRequiredEvidence(tc.required, tc.corpus, tc.evidence)
			if tc.want == 0 {
				if err != nil || len(missing) != 0 {
					t.Fatalf("got missing=%v err=%v, want satisfied", missing, err)
				}
				return
			}
			if err == nil {
				t.Fatal("want an error for an unmet requirement")
			}
			if len(missing) != tc.want {
				t.Fatalf("missing = %v, want %d", missing, tc.want)
			}
		})
	}
}
