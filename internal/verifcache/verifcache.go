// Package verifcache is the CTX-007 Verification Cache: a correctness-first cache of
// deterministic verification results.
//
// A verification result may be reused only when the identity it was produced under
// matches the current state exactly. The identity combines the repository revision, the
// effective validation inputs, the verification command, the relevant environment and
// toolchain, the command-policy classification, the validation scope, and the verifier's
// own identity and version. A cache hit therefore never weakens verification: any
// difference in any component is a miss, and the verification runs again.
//
// The cache prefers false misses over false hits by construction: it never infers,
// repairs, or approximates a key. Provenance (whether the answer was a hit, the key
// digest, and the source verification) is always available to the caller. Nothing here
// consults a model, a confidence score, or the network; the package is deterministic and
// provider-neutral.
//
// Reuse is fail-closed. Evidence is reusable as PASS only when the identity matches
// exactly AND the stored verdict is VERIFIED: failed, incomplete, partial, hold, or
// untrusted evidence is never reusable as PASS (see Decide and Cache.Reusable).
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
// identity, so a schema change invalidates every entry. It was bumped to /2 when the
// identity gained the effective-input, command-policy, and scope components: entries
// written under /1 are dropped on load rather than reused under the wider key.
const SchemaVersion = "verifcache/2"

// Identity is the deterministic state a cached verification result depends on. Every
// field participates in the digest; two identities are interchangeable only when all
// fields are equal.
//
// The fields are deliberately path-free: none of them records a worktree location, so an
// identity computed in one worktree equals the identity computed in another worktree that
// shares the same revision, effective inputs, command, environment/toolchain, command
// policy, scope, and verifier. That is what makes cross-worktree reuse safe when, and only
// when, the effective inputs are demonstrably identical (see Equivalent).
type Identity struct {
	// Schema is SchemaVersion at the time the result was produced.
	Schema string `json:"schema"`
	// Repository is the repository revision identity (for example RepositoryIdentity):
	// HEAD and the dirty flag for the state the verification observed.
	Repository string `json:"repository"`
	// Inputs is the effective-input identity (for example EffectiveInputIdentity): a
	// digest over the relevant input files (path + content hash) the result actually
	// depends on. An empty value means no effective inputs were declared: a weaker key,
	// never a stronger one.
	Inputs string `json:"inputs"`
	// Command identifies the verification command or suite.
	Command string `json:"command"`
	// Environment identifies the relevant environment and toolchain (for example
	// EnvironmentIdentity, optionally composed with ToolchainIdentity).
	Environment string `json:"environment"`
	// CommandPolicy identifies the command-policy classification of the verification's
	// commands (for example CommandPolicyIdentity), so a policy change that reclassifies a
	// command invalidates the entry.
	CommandPolicy string `json:"command_policy"`
	// Scope is the validation scope identity (for example ScopeIdentity): the set of
	// objective/criterion/check identifiers the evidence covers. Evidence for one scope is
	// never reused for another.
	Scope string `json:"scope"`
	// Verifier identifies the verifier and its version.
	Verifier string `json:"verifier"`
}

// Digest is the stable identity of the verification's inputs. Identical inputs yield
// the same digest; any difference yields a different one.
func (id Identity) Digest() string {
	h := sha256.New()
	fmt.Fprintf(h, "verifcache-identity\nschema=%s\nrepository=%s\ninputs=%s\ncommand=%s\nenvironment=%s\ncommand_policy=%s\nscope=%s\nverifier=%s\n",
		id.Schema, id.Repository, id.Inputs, id.Command, id.Environment, id.CommandPolicy, id.Scope, id.Verifier)
	return hex.EncodeToString(h.Sum(nil))
}

// Equivalent reports whether two identities describe the same verification state. It is a
// pure comparison over every component, and it is worktree-independent: two worktrees may
// reuse each other's evidence exactly when their identities are Equivalent. It never
// infers, repairs, or approximates.
func Equivalent(a, b Identity) bool { return a == b }

// Verdict is the trust classification of stored evidence. Only VerdictVerified may ever be
// reused as PASS.
type Verdict string

const (
	// VerdictVerified: the evidence is an independently verified PASS.
	VerdictVerified Verdict = "VERIFIED"
	// VerdictPartial: safe work was done, but a required criterion is unmet or unverifiable.
	VerdictPartial Verdict = "PARTIAL"
	// VerdictHold: an authority or safety boundary was crossed.
	VerdictHold Verdict = "HOLD"
	// VerdictFailed: the verification ran and failed.
	VerdictFailed Verdict = "FAILED"
	// VerdictIncomplete: the verification did not complete.
	VerdictIncomplete Verdict = "INCOMPLETE"
	// VerdictUntrusted: the evidence is not trusted (missing, foreign, or unverifiable).
	VerdictUntrusted Verdict = "UNTRUSTED"
)

// Result is one verification outcome as a verifier reported it.
type Result struct {
	Passed bool   `json:"passed"`
	Output string `json:"output,omitempty"`
	// Source names the verification that produced the result, for provenance.
	Source string `json:"source"`
	// Verdict is the trust classification. The zero value ("") is not VERIFIED, so a result
	// stored without an explicit verdict is never reusable as PASS.
	Verdict Verdict `json:"verdict,omitempty"`
}

// Entry is a stored cache row: the identity it belongs to, its digest, and the result
// with provenance and verdict.
type Entry struct {
	Identity Identity `json:"identity"`
	Digest   string   `json:"digest"`
	Passed   bool     `json:"passed"`
	Output   string   `json:"output,omitempty"`
	Source   string   `json:"source"`
	Verdict  Verdict  `json:"verdict,omitempty"`
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
		Verdict:  res.Verdict,
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
		return Result{Passed: hit.Entry.Passed, Output: hit.Entry.Output, Source: hit.Entry.Source, Verdict: hit.Entry.Verdict}, true
	}
	res := verify()
	c.Store(id, res)
	return res, false
}

// Reusable returns the stored entry for id only when it is an exact identity match and its
// verdict is VERIFIED. It is the fail-closed read path: a miss, an identity mismatch, or any
// non-VERIFIED verdict returns (Entry{}, false). Prefer it over Lookup when the caller
// intends to reuse evidence as PASS.
func (c *Cache) Reusable(id Identity) (Entry, bool) {
	hit := c.Lookup(id)
	if !hit.Hit {
		return Entry{}, false
	}
	if Decide(hit.Entry, id).Reusable {
		return hit.Entry, true
	}
	return Entry{}, false
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

// Decision is the outcome of deciding whether prior evidence may be reused. The zero
// value is "not reusable", so an uninitialized Decision fails closed.
type Decision struct {
	Reusable bool   `json:"reusable"`
	Reason   string `json:"reason"`
}

// Decide reports whether prior evidence may be reused for the current state. It is pure
// and fail-closed: reuse requires an exact identity match AND a VERIFIED verdict. Missing
// evidence, an identity mismatch, or any non-VERIFIED verdict (failed, incomplete, partial,
// hold, untrusted, or unrecorded) reuses nothing, and the caller must (re)run verification.
func Decide(pri Entry, current Identity) Decision {
	if pri.Digest == "" && pri.Identity == (Identity{}) {
		return Decision{Reusable: false, Reason: "no prior evidence"}
	}
	if !Equivalent(pri.Identity, current) {
		return Decision{Reusable: false, Reason: "identity mismatch: prior evidence is for a different state"}
	}
	if pri.Verdict != VerdictVerified {
		return Decision{Reusable: false, Reason: "prior evidence is not VERIFIED: " + verdictNote(pri.Verdict)}
	}
	return Decision{Reusable: true, Reason: "exact identity match and VERIFIED evidence"}
}

// verdictNote renders a verdict for a decision reason, treating an empty verdict as
// untrusted so the message never implies reuse is safe for unclassified evidence.
func verdictNote(v Verdict) string {
	if strings.TrimSpace(string(v)) == "" {
		return "no verdict recorded (treated as untrusted)"
	}
	return string(v)
}

// Input is one relevant validation input: a repository-relative path and a content hash.
// The hash is caller-supplied so this package stays free of I/O and filesystem policy.
type Input struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

// EffectiveInputIdentity is a deterministic identity for the effective validation inputs.
// Entries are trimmed, deduplicated, and sorted by (path, hash), so the identity depends on
// which inputs exist and their content, not on enumeration order. A blank path is ignored.
// Two worktrees with identical effective inputs produce the same identity, which is what
// permits cross-worktree reuse when equivalence is demonstrable.
func EffectiveInputIdentity(inputs []Input) string {
	seen := map[string]bool{}
	list := make([]string, 0, len(inputs))
	for _, in := range inputs {
		p := strings.TrimSpace(in.Path)
		if p == "" {
			continue
		}
		key := p + "\x00" + strings.TrimSpace(in.Hash)
		if seen[key] {
			continue
		}
		seen[key] = true
		list = append(list, key)
	}
	sort.Strings(list)
	return hashLines("effective-input-identity/1", list)
}

// EffectiveInputDigest is the effective-input digest over the relevant input set (declared
// inputs, spec, and acceptance checks). It is a deterministic, sorted path:content-hash
// digest: entries are trimmed, deduplicated, and sorted by (path, hash), and blank paths are
// ignored, so the digest is order- and duplicate-independent and content-sensitive.
//
// The content hashes are caller-supplied, which keeps this package free of I/O: the caller
// hashes current file contents (including uncommitted changes) and passes the pairs in. The
// digest is consumed via RepositoryIdentity(head, dirty, contentDigest) so a changed relevant
// input is a cache miss. It delegates to EffectiveInputIdentity so the two names cannot drift.
func EffectiveInputDigest(files []Input) string { return EffectiveInputIdentity(files) }

// ScopeIdentity is a deterministic identity for a validation scope: the set of
// objective/criterion/check identifiers the evidence covers. Identifiers are trimmed,
// deduplicated, and sorted, so the identity is order-independent and evidence for one scope
// can never be reused for another.
func ScopeIdentity(ids []string) string { return setIdentity("scope-identity/1", ids) }

// CommandPolicyIdentity is a deterministic identity for the command-policy classification
// of a verification's commands. The classifications are caller-supplied strings (from
// internal/commandpolicy: SAFE, REQUIRES_APPROVAL, DENIED), so this package stays
// classification-agnostic and free of a policy dependency. A policy change that
// reclassifies any command changes this identity and invalidates the evidence.
func CommandPolicyIdentity(classes []string) string {
	return setIdentity("command-policy-identity/1", classes)
}

// ToolchainIdentity is a deterministic identity for the toolchain facts a verification
// depends on (for example the Go version and GOOS/GOARCH). It is meant to be composed into
// the Identity Environment component (for example via EnvironmentIdentity) so a changed
// toolchain invalidates the entry.
func ToolchainIdentity(goVersion, goos, goarch string) string {
	return hashLines("toolchain-identity/1", []string{
		"go=" + strings.TrimSpace(goVersion),
		"goos=" + strings.TrimSpace(goos),
		"goarch=" + strings.TrimSpace(goarch),
	})
}

// EnvironmentInputs is the structured set of environment facts a verification result
// depends on: PATH, the Go toolchain version, GOOS/GOARCH, and the command-policy
// classification of the check argv. The classifications are caller-supplied strings, so
// this package stays classification-agnostic (internal/commandpolicy is not imported).
type EnvironmentInputs struct {
	// PATH is the process PATH relevant to the verification.
	PATH string
	// GoVersion is the Go toolchain version (for example "go1.22.0").
	GoVersion string
	// GOOS is the target operating system (for example "linux").
	GOOS string
	// GOARCH is the target architecture (for example "amd64").
	GOARCH string
	// CommandPolicy is the command-policy classification of the check argv (for example
	// SAFE, REQUIRES_APPROVAL, DENIED). It is a classification only: no command runs.
	CommandPolicy []string
}

// EnvironmentDigest is a deterministic identity for the structured environment a
// verification depends on. It composes PATH, the Go toolchain version and GOOS/GOARCH (via
// ToolchainIdentity), and the command-policy classification of the check argv (via
// CommandPolicyIdentity). It is pure, model-free, order-independent, and classification-only:
// it performs no execution and invokes no command. It is consumed as the Identity.Environment
// component so a changed PATH, toolchain, or policy classification invalidates the entry.
func EnvironmentDigest(in EnvironmentInputs) string {
	return EnvironmentIdentity(map[string]string{
		"path":      strings.TrimSpace(in.PATH),
		"toolchain": ToolchainIdentity(in.GoVersion, in.GOOS, in.GOARCH),
		"policy":    CommandPolicyIdentity(in.CommandPolicy),
	})
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
	return setIdentity("command-identity/1", commands)
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

// setIdentity trims, deduplicates, and sorts a set of values, then hashes them under
// header. It is the shared shape behind CommandIdentity, ScopeIdentity, and
// CommandPolicyIdentity, so a blank value or a duplicate never perturbs the identity.
func setIdentity(header string, vals []string) string {
	seen := map[string]bool{}
	list := make([]string, 0, len(vals))
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		list = append(list, v)
	}
	sort.Strings(list)
	return hashLines(header, list)
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
