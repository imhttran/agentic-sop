// Package rageval loads and validates the frozen RAG evaluation dataset.
//
// The dataset lives in evals/rag and has two parts: questions.json (the
// labelled questions) and corpus/MANIFEST.sha256 (sha256sum-style lines that
// pin every corpus document). Load refuses the dataset if a corpus hash does
// not match, if a question is missing a required field, if a question uses an
// unknown category, or if a category count falls below its H-1 minimum.
package rageval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Category is a known question category.
type Category string

// The known categories. The set is fixed by docs/specs/RAG-EVALUATION.md §1.
const (
	CategoryCode                 Category = "code"
	CategoryArchitectureHistory  Category = "architecture_history"
	CategoryDesignDecision       Category = "design_decision"
	CategoryFailureInvestigation Category = "failure_investigation"
	CategoryMultiHop             Category = "multi_hop"
	CategoryNoAnswer             Category = "no_answer"
	CategoryInjection            Category = "injection"
)

// Categories lists the known categories in a stable order.
var Categories = []Category{
	CategoryCode,
	CategoryArchitectureHistory,
	CategoryDesignDecision,
	CategoryFailureInvestigation,
	CategoryMultiHop,
	CategoryNoAnswer,
	CategoryInjection,
}

// KnownCategory reports whether c belongs to the known category set.
func KnownCategory(c Category) bool {
	for _, k := range Categories {
		if k == c {
			return true
		}
	}
	return false
}

// H1Minimums holds the H-1 per-category minimum question counts.
var H1Minimums = map[Category]int{
	CategoryCode:                 8,
	CategoryArchitectureHistory:  8,
	CategoryDesignDecision:       8,
	CategoryFailureInvestigation: 8,
	CategoryMultiHop:             10,
	CategoryNoAnswer:             4,
	CategoryInjection:            4,
}

// Typed error classes. Wrap with fmt.Errorf("...: %w", err) so callers can use
// errors.Is to identify the failure class.
var (
	ErrUnreadable        = errors.New("rageval: unreadable dataset file")
	ErrInvalidJSON       = errors.New("rageval: invalid JSON")
	ErrMissingField      = errors.New("rageval: missing or malformed question field")
	ErrUnknownCategory   = errors.New("rageval: unknown category")
	ErrHashMismatch      = errors.New("rageval: corpus hash mismatch")
	ErrMissingCorpusFile = errors.New("rageval: missing corpus file")
	ErrMalformedManifest = errors.New("rageval: malformed manifest line")
	ErrUnderMinimum      = errors.New("rageval: category count below H-1 minimum")
)

// Expected is one labelled evidence reference for a question.
type Expected struct {
	Source   string   `json:"source"`
	Section  string   `json:"section"`
	Evidence []string `json:"evidence"`
}

// Question is one labelled evaluation question.
type Question struct {
	ID              string     `json:"id"`
	Category        Category   `json:"category"`
	Question        string     `json:"question"`
	PermittedCorpus string     `json:"permitted_corpus"`
	ExpectedFacts   []string   `json:"expected_facts"`
	Expected        []Expected `json:"expected"`

	// Optional, category-specific fields.
	AbsentTerms []string `json:"absent_terms,omitempty"`
	// InjectedText is the exact string an injection question tries to place
	// into the assembled context. The runner uses it to detect placement.
	InjectedText     string `json:"injected_text,omitempty"`
	Injected         *bool  `json:"injected,omitempty"`
	ExpectedBehavior string `json:"expected_behavior,omitempty"`
	RewordedFrom     string `json:"reworded_from,omitempty"`
}

// CategoryCounts is a per-category question count.
type CategoryCounts map[Category]int

// ManifestEntry is one verified corpus file.
type ManifestEntry struct {
	Path   string
	SHA256 string
}

// Dataset is a loaded, validated evaluation dataset.
type Dataset struct {
	Root      string
	Questions []Question
	Counts    CategoryCounts
	Manifest  []ManifestEntry
	// Corpus names the corpus the loaded documents belong to.
	Corpus string
}

// Load reads and validates the dataset rooted at dir, then fills InjectedText
// for every injection question from its planted evidence file.
func Load(dir string) (*Dataset, error) {
	qs, err := loadQuestions(filepath.Join(dir, "questions.json"))
	if err != nil {
		return nil, err
	}
	manifest, err := loadManifest(filepath.Join(dir, "corpus", "MANIFEST.sha256"))
	if err != nil {
		return nil, err
	}
	corpusRoot := filepath.Join(dir, "corpus")
	if err := verifyCorpus(corpusRoot, manifest); err != nil {
		return nil, err
	}
	counts := countQuestions(qs)
	if err := checkMinimums(counts); err != nil {
		return nil, err
	}
	ds := &Dataset{Root: dir, Questions: qs, Counts: counts, Manifest: manifest}
	if err := fillInjectedText(ds); err != nil {
		return nil, err
	}
	return ds, nil
}

// InjectionSourcePrefix is the path segment that marks a planted injection
// corpus document. An injection question's planted entry is the expected entry
// whose source lies under evals/rag/injection/.
const InjectionSourcePrefix = "evals/rag/injection/"

// isInjectionSource reports whether source names a planted injection file under
// evals/rag/injection/. The source may carry a leading repository prefix (for
// example imhttran/agentic-sop/evals/rag/injection/...), so the segment is
// matched at the start or after a slash. A path such as docs/injection/note.md
// does not match, because it lacks the evals/rag/ prefix.
func isInjectionSource(source string) bool {
	if strings.HasPrefix(source, InjectionSourcePrefix) {
		return true
	}
	return strings.Contains(source, "/"+InjectionSourcePrefix)
}

// injectionEvidencePath returns the corpus-relative path of a planted injection
// file. A source that carries a repository prefix is reduced to the part from
// evals/rag/injection/ onward so that filepath.Join(CorpusRoot, path) resolves
// the planted file inside the corpus directory.
func injectionEvidencePath(source string) string {
	if i := strings.Index(source, InjectionSourcePrefix); i >= 0 {
		return source[i:]
	}
	return source
}

// fillInjectedText sets Question.InjectedText for each injection question. The
// planted entry is the first expected entry whose source is under
// evals/rag/injection/. InjectedText is that entry's evidence phrase, which is
// the exact text the question tries to place into the assembled context. It is
// not the whole planted file. The phrase must appear verbatim in the planted
// file; this loader fails if it does not.
func fillInjectedText(ds *Dataset) error {
	root := CorpusRoot(ds)
	for i := range ds.Questions {
		q := &ds.Questions[i]
		if q.Category != CategoryInjection {
			continue
		}
		var planted string
		var phrase string
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
			continue
		}
		if phrase == "" {
			return fmt.Errorf("%w: injection question %s: planted entry %s has no evidence phrase", ErrMissingField, q.ID, planted)
		}
		rel := filepath.FromSlash(planted)
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil && os.IsNotExist(err) {
			// The corpus may mirror only the evals/rag/injection/ segment of a
			// repo-prefixed source; fall back to that shortened path.
			raw, err = os.ReadFile(filepath.Join(root, filepath.FromSlash(injectionEvidencePath(planted))))
		}
		if err != nil {
			return fmt.Errorf("%w: %s: %v", ErrUnreadable, planted, err)
		}
		if !strings.Contains(string(raw), phrase) {
			return fmt.Errorf("%w: injection question %s: evidence phrase does not appear verbatim in %s", ErrMissingField, q.ID, planted)
		}
		q.InjectedText = phrase
	}
	return nil
}

func loadQuestions(path string) ([]Question, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrUnreadable, path, err)
	}
	var rawList []json.RawMessage
	if err := json.Unmarshal(raw, &rawList); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidJSON, path, err)
	}
	if len(rawList) == 0 {
		return nil, fmt.Errorf("%w: %s: no questions", ErrMissingField, path)
	}
	seen := map[string]bool{}
	qs := make([]Question, 0, len(rawList))
	for i, r := range rawList {
		missing := missingRequiredFields(r)
		if len(missing) > 0 {
			return nil, fmt.Errorf("%w: question #%d in %s: missing %s", ErrMissingField, i, path, strings.Join(missing, ", "))
		}
		var q Question
		if err := json.Unmarshal(r, &q); err != nil {
			return nil, fmt.Errorf("%w: question #%d in %s: %v", ErrInvalidJSON, i, path, err)
		}
		if err := validateQuestion(&q); err != nil {
			return nil, err
		}
		if seen[q.ID] {
			return nil, fmt.Errorf("%w: question id %q: duplicate id", ErrMissingField, q.ID)
		}
		seen[q.ID] = true
		qs = append(qs, q)
	}
	return qs, nil
}

func validateQuestion(q *Question) error {
	id := q.ID
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%w: question at id %q: id", ErrMissingField, id)
	}
	if strings.TrimSpace(string(q.Category)) == "" {
		return fmt.Errorf("%w: question %s: category", ErrMissingField, id)
	}
	if !KnownCategory(q.Category) {
		return fmt.Errorf("%w: question %s: category %q", ErrUnknownCategory, id, q.Category)
	}
	if strings.TrimSpace(q.Question) == "" {
		return fmt.Errorf("%w: question %s: question", ErrMissingField, id)
	}
	if strings.TrimSpace(q.PermittedCorpus) == "" {
		return fmt.Errorf("%w: question %s: permitted_corpus", ErrMissingField, id)
	}
	return nil
}

func missingRequiredFields(raw json.RawMessage) []string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	required := []string{"id", "category", "question", "permitted_corpus", "expected_facts", "expected"}
	var missing []string
	for _, k := range required {
		if _, ok := obj[k]; !ok {
			missing = append(missing, k)
		}
	}
	return missing
}

func loadManifest(path string) ([]ManifestEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrUnreadable, path, err)
	}
	var entries []ManifestEntry
	lines := strings.Split(string(raw), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		sum, rel, ok := parseManifestLine(trimmed)
		if !ok {
			return nil, fmt.Errorf("%w: %s:%d: %q", ErrMalformedManifest, path, i+1, line)
		}
		entries = append(entries, ManifestEntry{Path: rel, SHA256: sum})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%w: %s: no entries", ErrMalformedManifest, path)
	}
	return entries, nil
}

func parseManifestLine(line string) (sum, rel string, ok bool) {
	idx := strings.Index(line, "  ")
	if idx < 0 {
		return "", "", false
	}
	sum = line[:idx]
	rel = line[idx+2:]
	if len(sum) != 64 || len(rel) == 0 {
		return "", "", false
	}
	if _, err := hex.DecodeString(sum); err != nil {
		return "", "", false
	}
	return sum, rel, true
}

func verifyCorpus(root string, entries []ManifestEntry) error {
	for _, e := range entries {
		full := filepath.Join(root, filepath.FromSlash(e.Path))
		f, err := os.Open(full)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("%w: %s", ErrMissingCorpusFile, e.Path)
			}
			return fmt.Errorf("%w: %s: %v", ErrUnreadable, e.Path, err)
		}
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			return fmt.Errorf("%w: %s: %v", ErrUnreadable, e.Path, err)
		}
		f.Close()
		got := hex.EncodeToString(h.Sum(nil))
		if !strings.EqualFold(got, e.SHA256) {
			return fmt.Errorf("%w: %s: want %s, got %s", ErrHashMismatch, e.Path, e.SHA256, got)
		}
	}
	return nil
}

func countQuestions(qs []Question) CategoryCounts {
	counts := make(CategoryCounts, len(Categories))
	for _, c := range Categories {
		counts[c] = 0
	}
	for _, q := range qs {
		counts[q.Category]++
	}
	return counts
}

func checkMinimums(counts CategoryCounts) error {
	cats := make([]string, 0, len(H1Minimums))
	for c := range H1Minimums {
		cats = append(cats, string(c))
	}
	sort.Strings(cats)
	for _, cs := range cats {
		c := Category(cs)
		min := H1Minimums[c]
		if counts[c] < min {
			return fmt.Errorf("%w: category %q: have %d, need at least %d", ErrUnderMinimum, c, counts[c], min)
		}
	}
	return nil
}
