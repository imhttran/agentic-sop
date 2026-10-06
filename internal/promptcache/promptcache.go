// Package promptcache is the CTX-008 Prompt Result Cache: a conservative cache of
// read-only model responses.
//
// It caches only the output of capabilities that cannot mutate the repository. A
// mutation-producing capability (IMPLEMENT, FIX) is never cached: a cached response
// must not stand in for repository work. Even for a read-only capability, a cache hit
// is only a replay of model output; it does not constitute task success and does not
// stand in for SOP's downstream validation, verification, or review, which still run.
//
// The cache key combines every input that can materially change the result: the
// compiled prompt identity, the model/provider identity, the relevant model
// parameters, the context identity, and the compiler/schema version. Any difference is
// a miss. The cache prefers false misses over unsafe hits and never infers, repairs, or
// approximates a key.
package promptcache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// SchemaVersion identifies the cache's key and storage schema. It is part of every
// identity, so a schema change invalidates every entry.
const SchemaVersion = "promptcache/1"

// Identity is the deterministic set of inputs a cached model result depends on. Every
// field participates in the digest.
type Identity struct {
	// Schema is SchemaVersion at the time the result was produced.
	Schema string `json:"schema"`
	// Capability is the requested capability. Only non-mutating capabilities are
	// cacheable.
	Capability string `json:"capability"`
	// Prompt is the compiled prompt identity (for example prompt.Compiled.Digest).
	Prompt string `json:"prompt"`
	// Context is the context identity the prompt was compiled from.
	Context string `json:"context"`
	// Model identifies the provider and model.
	Model string `json:"model"`
	// Parameters identifies the relevant model parameters.
	Parameters string `json:"parameters"`
	// Compiler identifies the prompt compiler and its version.
	Compiler string `json:"compiler"`
}

// Digest is the stable identity of the request's inputs.
func (id Identity) Digest() string {
	h := sha256.New()
	fmt.Fprintf(h, "promptcache-identity\nschema=%s\ncapability=%s\nprompt=%s\ncontext=%s\nmodel=%s\nparameters=%s\ncompiler=%s\n",
		id.Schema, id.Capability, id.Prompt, id.Context, id.Model, id.Parameters, id.Compiler)
	return hex.EncodeToString(h.Sum(nil))
}

// Entry is a stored cache row: the identity it belongs to, its digest, the cached
// content, and provenance.
type Entry struct {
	Identity Identity `json:"identity"`
	Digest   string   `json:"digest"`
	Content  string   `json:"content"`
	// Source names what produced the cached content, for provenance.
	Source string `json:"source"`
}

// Lookup is the outcome of a cache read.
type Lookup struct {
	Hit   bool  `json:"hit"`
	Entry Entry `json:"entry"`
}

// Cache is an in-memory prompt-result cache. Construct one with New.
type Cache struct {
	entries map[string]Entry
}

// New returns an empty cache.
func New() *Cache { return &Cache{entries: map[string]Entry{}} }

// Eligible reports whether a capability's results may be cached. It is false for any
// mutation-producing capability, so repository work is never replaced by a cached
// response.
func Eligible(capability agent.Capability) bool {
	return !agent.IsRepositoryMutation(capability)
}

// Lookup returns the entry stored for id, or a zero Lookup when no entry matches the
// identity exactly.
func (c *Cache) Lookup(id Identity) Lookup {
	if c == nil {
		return Lookup{}
	}
	digest := id.Digest()
	e, ok := c.entries[digest]
	if !ok || e.Identity != id {
		return Lookup{}
	}
	return Lookup{Hit: true, Entry: e}
}

// Store records content under id. It refuses to cache a mutation-producing capability,
// so a cached response can never stand in for repository work. Storing again for the
// same identity replaces the prior content.
func (c *Cache) Store(id Identity, content, source string) (Entry, error) {
	if !Eligible(agent.Capability(id.Capability)) {
		return Entry{}, fmt.Errorf("promptcache: capability %q mutates the repository and must not be cached", id.Capability)
	}
	if c == nil {
		return Entry{}, nil
	}
	if c.entries == nil {
		c.entries = map[string]Entry{}
	}
	e := Entry{Identity: id, Digest: id.Digest(), Content: content, Source: source}
	c.entries[e.Digest] = e
	return e, nil
}

// Run returns cached content for id when the identity matches exactly. Otherwise it
// calls generate and, for an eligible capability, stores the result. A mutation-
// producing capability is never consulted or stored: generate always runs. The bool
// reports whether the content was a cache hit; a hit is a replay, not task success.
func (c *Cache) Run(id Identity, generate func() (string, error)) (string, bool, error) {
	if !Eligible(agent.Capability(id.Capability)) {
		content, err := generate()
		return content, false, err
	}
	if hit := c.Lookup(id); hit.Hit {
		return hit.Entry.Content, true, nil
	}
	content, err := generate()
	if err != nil {
		return "", false, err
	}
	if _, err := c.Store(id, content, "agent"); err != nil {
		return "", false, err
	}
	return content, false, nil
}

// Len is the number of stored entries.
func (c *Cache) Len() int {
	if c == nil {
		return 0
	}
	return len(c.entries)
}

// Entries returns the stored entries sorted by digest, so serialization is
// deterministic regardless of insertion order.
func (c *Cache) Entries() []Entry {
	if c == nil {
		return nil
	}
	out := make([]Entry, 0, len(c.entries))
	for _, e := range c.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Digest < out[j].Digest })
	return out
}

// Marshal serializes the cache deterministically: entries are ordered by digest, so an
// unchanged cache always marshals to identical bytes.
func (c *Cache) Marshal() ([]byte, error) {
	doc := struct {
		Schema  string  `json:"schema"`
		Entries []Entry `json:"entries"`
	}{Schema: SchemaVersion, Entries: c.Entries()}
	return json.MarshalIndent(doc, "", "  ")
}

// Unmarshal loads a cache written by Marshal. A cache whose schema does not match the
// current SchemaVersion loads empty, and a stored digest that is not self-consistent is
// recomputed, so a corrupt or incompatible file cannot fabricate a hit.
func Unmarshal(data []byte) (*Cache, error) {
	var doc struct {
		Schema  string  `json:"schema"`
		Entries []Entry `json:"entries"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("promptcache: %w", err)
	}
	c := New()
	if doc.Schema != SchemaVersion {
		return c, nil
	}
	for _, e := range doc.Entries {
		if e.Digest == "" || e.Digest != e.Identity.Digest() {
			e.Digest = e.Identity.Digest()
		}
		// Never load an entry that could not have been stored: a mutation-producing
		// capability must not be reintroduced through a crafted file.
		if !Eligible(agent.Capability(e.Identity.Capability)) {
			continue
		}
		c.entries[e.Digest] = e
	}
	return c, nil
}
