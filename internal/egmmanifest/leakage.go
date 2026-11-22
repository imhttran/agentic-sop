package egmmanifest

import (
	"bytes"
	"errors"
	"os"
	"sort"
)

// Hit is one vocabulary match. It carries no term text, so formatting a Hit cannot
// disclose the vocabulary.
type Hit struct {
	Offset int // byte offset of the match in the scanned data
	Index  int // index of the matching term in the vocabulary
}

// Vocabulary is an operator-supplied set of forbidden terms. The package never hardcodes
// terms and never returns them.
type Vocabulary struct {
	terms []string
}

// NewVocabulary validates and copies terms. An empty vocabulary, or an empty term, is an
// error.
func NewVocabulary(terms []string) (*Vocabulary, error) {
	if len(terms) == 0 {
		return nil, errors.New("egmmanifest: empty leakage vocabulary")
	}
	dedup := make(map[string]bool, len(terms))
	out := make([]string, 0, len(terms))
	for _, t := range terms {
		if t == "" {
			return nil, errors.New("egmmanifest: empty leakage term")
		}
		if dedup[t] {
			continue
		}
		dedup[t] = true
		out = append(out, t)
	}
	return &Vocabulary{terms: out}, nil
}

// Len reports the number of distinct terms without exposing them.
func (v *Vocabulary) Len() int {
	if v == nil {
		return 0
	}
	return len(v.terms)
}

// Scan returns every substring match, sorted by offset then term index. Matching is
// case-sensitive and deterministic. It is a screening aid, not a DLP classifier.
func (v *Vocabulary) Scan(data []byte) []Hit {
	if v == nil {
		return nil
	}
	var hits []Hit
	for i, term := range v.terms {
		b := []byte(term)
		off := 0
		for off <= len(data)-len(b) {
			j := bytes.Index(data[off:], b)
			if j < 0 {
				break
			}
			hits = append(hits, Hit{Offset: off + j, Index: i})
			off += j + 1
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Offset != hits[j].Offset {
			return hits[i].Offset < hits[j].Offset
		}
		return hits[i].Index < hits[j].Index
	})
	return hits
}

// ScanFile reads path and returns its matches. The file's contents are not returned.
func (v *Vocabulary) ScanFile(path string) ([]Hit, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return v.Scan(data), nil
}
