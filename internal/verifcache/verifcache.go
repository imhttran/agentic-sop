// Package verifcache is the CTX-007 Verification Cache: a correctness-first cache of
// deterministic verification results.
//
// A verification result may be reused only when the identity it was produced under
// matches the current state exactly. The identity combines the repository state, the
// verification command, the relevant environment, and the verifier's own identity and
// version. A cache hit therefore never weakens verification: any difference in
// repository state, command, environment, or verifier version is a miss, and the
// verification runs again.
//
// The cache prefers false misses over false hits by construction: it never infers,
// repairs, or approximates a key. Provenance (whether the answer was a hit, the key
// digest, and the source verification) is always available to the caller.
package verifcache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// SchemaVersion identifies the cache's key and storage schema. It is part of every
// identity, so a schema change invalidates every entry.
const SchemaVersion = "verifcache/1"

// Identity is the deterministic state a cached verification result depends on. Every
// field participates in the digest; two identities are interchangeable only when all
// fields are equal.
type Identity struct {
	// Schema is SchemaVersion at the time the result was produced.
	Schema string `json:"schema"`
	// Repository is the repository identity (for example RepositoryIdentity): HEAD,
	// the dirty flag, and a content digest of the tree the verification observed.
	Repository string `json:"repository"`
	// Command identifies the verification command or suite.
	Command string `json:"command"`
	// Environment identifies the relevant environment and configuration.
	Environment string `json:"environment"`
	// Verifier identifies the verifier and its version.
	Verifier string `json:"verifier"`
}

// Digest is the stable identity of the verification's inputs. Identical inputs yield
// the same digest; any difference yields a different one.
func (id Identity) Digest() string {
	h := sha256.New()
	fmt.Fprintf(h, "verifcache-identity\nschema=%s\nrepository=%s\ncommand=%s\nenvironment=%s\nverifier=%s\n",
		id.Schema, id.Repository, id.Command, id.Environment, id.Verifier)
	return hex.EncodeToString(h.Sum(nil))
}

// Result is one verification outcome as a verifier reported it.
type Result struct {
	Passed bool   `json:"passed"`
	Output string `json:"output,omitempty"`
	// Source names the verification that produced the result, for provenance.
	Source string `json:"source"`
}

// Entry is a stored cache row: the identity it belongs to, its digest, and the result
// with provenance.
type Entry struct {
	Identity Identity `json:"identity"`
	Digest   string   `json:"digest"`
	Passed   bool     `json:"passed"`
	Output   string   `json:"output,omitempty"`
	Source   string   `json:"source"`
}

// Lookup is the outcome of a cache read. Hit is true only for an exact identity match.
type Lookup struct {
	Hit   bool  `json:"hit"`
	Entry Entry `json:"entry"`
}

// Cache is an in-memory verification cache. It stores at most one entry per identity
// digest. The zero Cache is empty and unusable; construct one with New.
type Cache struct {
	entries map[string]Entry
}

// New returns an empty cache.
func New() *Cache { return &Cache{entries: map[string]Entry{}} }

// Lookup returns the entry stored for id, or a zero Lookup when no entry matches the
// identity exactly. A Lookup with Hit false is not a negative verification result; it
// means the verification must run.
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

// Store records a verification result under id and returns the stored entry. Storing
// again for the same identity replaces the prior result.
func (c *Cache) Store(id Identity, res Result) Entry {
	if c == nil {
		return Entry{}
	}
	if c.entries == nil {
		c.entries = map[string]Entry{}
	}
	e := Entry{
		Identity: id,
		Digest:   id.Digest(),
		Passed:   res.Passed,
		Output:   res.Output,
		Source:   res.Source,
	}
	c.entries[e.Digest] = e
	return e
}

// Run returns the cached result for id when it is an exact match for the current
// state. Otherwise it runs verify, stores the result under id, and returns it with
// Hit false. It never reuses a result whose identity differs in any field, so a
// stale-state hit cannot occur.
func (c *Cache) Run(id Identity, verify func() Result) (Result, bool) {
	if hit := c.Lookup(id); hit.Hit {
		return Result{Passed: hit.Entry.Passed, Output: hit.Entry.Output, Source: hit.Entry.Source}, true
	}
	res := verify()
	c.Store(id, res)
	return res, false
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

// Marshal serializes the cache deterministically: entries are ordered by digest, so
// an unchanged cache always marshals to identical bytes.
func (c *Cache) Marshal() ([]byte, error) {
	doc := struct {
		Schema  string  `json:"schema"`
		Entries []Entry `json:"entries"`
	}{Schema: SchemaVersion, Entries: c.Entries()}
	return json.MarshalIndent(doc, "", "  ")
}

// Unmarshal loads a cache written by Marshal. A cache whose schema does not match the
// current SchemaVersion loads empty rather than reusing results from an incompatible
// schema.
func Unmarshal(data []byte) (*Cache, error) {
	var doc struct {
		Schema  string  `json:"schema"`
		Entries []Entry `json:"entries"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("verifcache: %w", err)
	}
	c := New()
	if doc.Schema != SchemaVersion {
		return c, nil
	}
	for _, e := range doc.Entries {
		// Trust the stored digest only when it is self-consistent; otherwise recompute,
		// so a corrupted file cannot fabricate a hit for a different identity.
		if e.Digest == "" || e.Digest != e.Identity.Digest() {
			e.Digest = e.Identity.Digest()
		}
		c.entries[e.Digest] = e
	}
	return c, nil
}

// RepositoryIdentity is a deterministic repository identity for a verification: HEAD,
// the dirty flag, and a content digest of the tree the verification observed. A
// modified working tree, a moved HEAD, or a changed content digest each produce a
// different identity, so a result from another state cannot be reused.
func RepositoryIdentity(head string, dirty bool, contentDigest string) string {
	h := sha256.New()
	fmt.Fprintf(h, "repository-identity/1\nhead=%s\ndirty=%t\ncontent=%s\n", strings.TrimSpace(head), dirty, strings.TrimSpace(contentDigest))
	return hex.EncodeToString(h.Sum(nil))
}

// CommandIdentity is a deterministic identity for a set of verification commands. The
// set is deduplicated and sorted, so the identity depends on which commands run, not
// on the order a caller happened to list them.
func CommandIdentity(commands []string) string {
	uniq := map[string]bool{}
	var list []string
	for _, c := range commands {
		c = strings.TrimSpace(c)
		if c == "" || uniq[c] {
			continue
		}
		uniq[c] = true
		list = append(list, c)
	}
	sort.Strings(list)
	return hashLines("command-identity/1", list)
}

// EnvironmentIdentity is a deterministic identity for the relevant environment. Keys
// are sorted so the identity does not depend on map iteration order.
func EnvironmentIdentity(env map[string]string) string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("environment-identity/1\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\n", k, env[k])
	}
	return hashString(b.String())
}

func hashLines(header string, lines []string) string {
	var b strings.Builder
	b.WriteString(header)
	b.WriteByte('\n')
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return hashString(b.String())
}

func hashString(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
