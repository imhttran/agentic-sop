package memindex

import (
	"strings"

	"github.com/imhttran/agentic-sop/internal/retrievalboundary"
)

// BuildFTSQuery turns free-form user text into a safe FTS5 MATCH expression. Each token
// is quoted, so punctuation, exact error strings, task IDs and FTS operators (AND, OR,
// NOT, NEAR, *, :, ^, -) cannot break the syntax. Tokens are combined with an explicit OR
// and ranked by bm25; requiring every token (implicit AND) makes a natural-language task
// description match almost nothing, which is the zero-hit defect this replaces.
//
// An empty token set returns ErrInvalidQuery.
//
// The tokenizer is shared with retrievalboundary.Tokenize so the FTS adapter and the
// ranker agree on token boundaries, including non-ASCII letters and digits.
func BuildFTSQuery(text string) (string, error) {
	toks := retrievalboundary.Tokenize(text)
	if len(toks) == 0 {
		return "", &retrievalboundary.InvalidQueryError{Reason: "empty query text"}
	}
	quoted := make([]string, 0, len(toks))
	for _, t := range toks {
		quoted = append(quoted, "\""+strings.ReplaceAll(t, "\"", "\"\"")+"\"")
	}
	return strings.Join(quoted, " OR "), nil
}
