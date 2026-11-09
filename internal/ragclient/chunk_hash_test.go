package ragclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func chunkHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// GAP-5 (PROPOSAL-RAG-EVIDENCE-HASH.md option A): an optional chunk_hash is
// recomputed from the exact text bytes; a mismatch rejects the whole response,
// a malformed value is distinguished from a mismatch, and an absent value is
// accepted as unverified. document_hash alone is never verified.
func TestChunkHash(t *testing.T) {
	const text = "evidence text"
	cases := []struct {
		name, text, hash string
		code             Code // "" means accepted
		verified         bool
	}{
		{"correct", text, chunkHash(text), "", true},
		{"absent is unverified", text, "", "", false},
		{"one-byte mutation", text + "!", chunkHash(text), CodeEvidenceHashMismatch, false},
		{"trailing newline", text + "\n", chunkHash(text), CodeEvidenceHashMismatch, false},
		{"NFD vs NFC", "café", chunkHash("café"), CodeEvidenceHashMismatch, false},
		{"no prefix", text, strings.TrimPrefix(chunkHash(text), "sha256:"), CodeMalformed, false},
		{"uppercase hex", text, "sha256:" + strings.ToUpper(strings.TrimPrefix(chunkHash(text), "sha256:")), CodeMalformed, false},
		{"short", text, "sha256:abc", CodeMalformed, false},
		{"too long", text, chunkHash(text) + "00", CodeMalformed, false},
		{"not hex", text, "sha256:" + strings.Repeat("z", 64), CodeMalformed, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := okEvidence()
			resp.Evidence[0].Text, resp.Evidence[0].ChunkHash = tc.text, tc.hash
			c := newTestClient(t, Config{Executable: "/producer", AllowedCorpora: []string{"kb"}}, &fakeRunner{out: mustJSON(t, resp)})
			got, err := c.Retrieve(context.Background(), Request{Query: "q", Filters: Filters{Corpus: "kb"}})
			if tc.code != "" {
				if CodeOf(err) != tc.code {
					t.Fatalf("err = %v, want code %s", err, tc.code)
				}
				return
			}
			if err != nil {
				t.Fatalf("Retrieve: %v", err)
			}
			if v := got.Evidence[0].ChunkVerified(); v != tc.verified {
				t.Fatalf("ChunkVerified = %v, want %v", v, tc.verified)
			}
		})
	}
}
