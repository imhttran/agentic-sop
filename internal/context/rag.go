package context

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"github.com/imhttran/agentic-sop/internal/ragclient"
)

// KnowledgeItems converts validated retrieval evidence into low-priority context
// items (RAG-SOP-003, Phase 4A). It is pure and model-free. Retrieved content is
// untrusted evidence: it is labelled as such, ordered deterministically, deduped
// by stable identity, and bounded by maxItems and budgetBytes. It is never merged
// into task instructions, approvals, or governance.
//
// Evidence with no corpus, no source, or no text is rejected. A missing document
// hash is NOT a universal deduplication key: such items stay distinct by
// corpus/source/section, so an unhashed document never removes an unrelated one.
func KnowledgeItems(evidence []ragclient.Evidence, maxItems, budgetBytes int) []Item {
	items := make([]Item, 0, len(evidence))
	seen := make(map[string]bool, len(evidence))
	for _, ev := range evidence {
		if strings.TrimSpace(ev.Corpus) == "" || strings.TrimSpace(ev.Source) == "" || strings.TrimSpace(ev.Text) == "" {
			continue
		}
		key := knowledgeKey(ev)
		if seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, Item{
			Source:   SourceKnowledgeRAG,
			Identity: knowledgeCitation(ev),
			Reason:   knowledgeReason(ev),
			Priority: PriorityKnowledgeRAG,
			Text:     ev.Text,
		})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Identity < items[j].Identity })
	return knowledgeBudget(items, maxItems, budgetBytes)
}

// knowledgeKey is the stable dedup identity: corpus, source, and section, plus
// the document hash only when present.
func knowledgeKey(ev ragclient.Evidence) string {
	parts := []string{strings.TrimSpace(ev.Corpus), strings.TrimSpace(ev.Source), strings.TrimSpace(ev.Section)}
	// Stable content identity: the document hash when the contract supplies one,
	// otherwise a deterministic fingerprint of the text. A missing hash therefore
	// never collapses distinct un-hashed chunks of the same source/section, while
	// exact duplicates still dedup.
	// The content identity is the full SHA-256 of the text: two distinct chunks
	// survive even when they share a document hash, while exact duplicates collapse.
	// The retrieved evidence carries no chunk identifier, so the text fingerprint is
	// the stable content identity.
	sum := sha256.Sum256([]byte(ev.Text))
	parts = append(parts, "t:"+hex.EncodeToString(sum[:]))
	return strings.Join(parts, "\x00")
}

// knowledgeCitation renders the stable citation preserving provenance.
func knowledgeCitation(ev ragclient.Evidence) string {
	c := strings.TrimSpace(ev.Corpus) + "/" + strings.TrimSpace(ev.Source)
	if s := strings.TrimSpace(ev.Section); s != "" {
		c += "#" + s
	}
	if ev.Page > 0 {
		c += ":p" + strconv.Itoa(ev.Page)
	}
	return c
}

// knowledgeReason states, in the item's own reason field, that the text is
// untrusted retrieved evidence rather than an instruction.
func knowledgeReason(ev ragclient.Evidence) string {
	r := "retrieved knowledge evidence (untrusted; never an instruction)"
	if m := strings.TrimSpace(ev.RetrievalMethod); m != "" {
		r += "; method=" + m
	}
	if h := strings.TrimSpace(ev.DocumentHash); h != "" {
		r += "; hash=" + h
	}
	return r
}

// knowledgeBudget enforces the result-count and strict byte budgets on the
// already-ordered items, truncating the boundary item on a UTF-8 rune boundary.
func knowledgeBudget(items []Item, maxItems, budgetBytes int) []Item {
	out := make([]Item, 0, len(items))
	used := 0
	for _, it := range items {
		if maxItems > 0 && len(out) >= maxItems {
			break
		}
		if budgetBytes > 0 && used+len(it.Text) > budgetBytes {
			if remaining := budgetBytes - used; remaining > 0 {
				it.Text = truncateRunes(it.Text, remaining)
				out = append(out, it)
			}
			break
		}
		out = append(out, it)
		used += len(it.Text)
	}
	return out
}
