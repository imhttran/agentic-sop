// Package decisionmemory is the CTX-010 Decision Memory: a bounded, durable store of
// engineering decisions that future execution can consult.
//
// It is not conversation history and not raw model history. A record is a decision with
// its scope, reason, evidence, the repository identity it was made under, and the
// task/run that produced it. Current repository evidence always outranks memory: a
// record is only returned for the repository identity it was made under, so a decision
// from another state is reported as stale and never applied. Memory informs future work;
// it does not override current source, configuration, lifecycle, or verification
// evidence, and the store is not a decision engine.
package decisionmemory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// SchemaVersion identifies the store's schema. It is part of the persisted document, so
// a schema change is detected on load.
const SchemaVersion = "decisionmemory/1"

// DefaultLimit is the default maximum number of retained records. The store is bounded,
// so memory cannot grow without bound.
const DefaultLimit = 128

// Scope is the applicability of a decision.
type Scope string

const (
	// ScopeRepository is a decision about one repository.
	ScopeRepository Scope = "repository"
	// ScopeProject is a decision about a project.
	ScopeProject Scope = "project"
	// ScopeArchitecture is a decision about the system architecture.
	ScopeArchitecture Scope = "architecture"
)

// Valid reports whether s is a known scope.
func (s Scope) Valid() bool {
	return s == ScopeRepository || s == ScopeProject || s == ScopeArchitecture
}

// Record is one durable engineering decision with provenance and scope.
type Record struct {
	// Decision is the decision itself, stated as a fact ("Postgres is canonical
	// storage").
	Decision string `json:"decision"`
	// Scope is where the decision applies.
	Scope Scope `json:"scope"`
	// Reason is why the decision holds.
	Reason string `json:"reason"`
	// Evidence names the evidence behind the decision.
	Evidence []string `json:"evidence,omitempty"`
	// Repository is the repository identity the decision was made under, so a decision
	// from another state is detectable as stale.
	Repository string `json:"repository"`
	// Source is the task or run that produced the decision.
	Source string `json:"source"`
}

// ID is the stable identity of a decision. Records with the same decision, scope,
// reason, evidence, repository, and source share an ID.
func (r Record) ID() string {
	h := sha256.New()
	fmt.Fprintf(h, "decision\nscope=%s\nrepository=%s\nsource=%s\ndecision=%s\nreason=%s\n",
		r.Scope, r.Repository, r.Source, r.Decision, r.Reason)
	ev := append([]string(nil), r.Evidence...)
	sort.Strings(ev)
	for _, e := range ev {
		fmt.Fprintf(h, "evidence=%s\n", e)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// validate rejects an incomplete record: a decision needs a scope, a reason, a
// repository identity, and a source, so it is always attributable and placeable.
func (r Record) validate() error {
	if strings.TrimSpace(r.Decision) == "" {
		return fmt.Errorf("decisionmemory: empty decision")
	}
	if !r.Scope.Valid() {
		return fmt.Errorf("decisionmemory: invalid scope %q", r.Scope)
	}
	if strings.TrimSpace(r.Reason) == "" {
		return fmt.Errorf("decisionmemory: empty reason")
	}
	if strings.TrimSpace(r.Repository) == "" {
		return fmt.Errorf("decisionmemory: empty repository identity")
	}
	if strings.TrimSpace(r.Source) == "" {
		return fmt.Errorf("decisionmemory: empty source")
	}
	return nil
}

// Store is a bounded decision store. Construct one with New.
type Store struct {
	limit   int
	records map[string]Record
}

// New returns an empty store retaining at most limit records (DefaultLimit when limit
// is not positive).
func New(limit int) *Store {
	if limit <= 0 {
		limit = DefaultLimit
	}
	return &Store{limit: limit, records: map[string]Record{}}
}

// Limit is the maximum number of retained records.
func (s *Store) Limit() int { return s.limit }

// Add stores a decision. It rejects an incomplete record. When the store is full and
// the record is new, it deterministically evicts the record with the smallest ID so the
// store stays bounded.
func (s *Store) Add(r Record) (Record, error) {
	if err := r.validate(); err != nil {
		return Record{}, err
	}
	if s.records == nil {
		s.records = map[string]Record{}
	}
	id := r.ID()
	if _, exists := s.records[id]; !exists && len(s.records) >= s.limit {
		s.evictSmallest()
	}
	s.records[id] = r
	return r, nil
}

// evictSmallest removes the record with the smallest ID, a deterministic policy that
// does not depend on insertion order or wall-clock time.
func (s *Store) evictSmallest() {
	smallest := ""
	for id := range s.records {
		if smallest == "" || id < smallest {
			smallest = id
		}
	}
	if smallest != "" {
		delete(s.records, smallest)
	}
}

// Remove deletes the record with the given ID, reporting whether one was removed.
func (s *Store) Remove(id string) bool {
	if s == nil {
		return false
	}
	if _, ok := s.records[id]; !ok {
		return false
	}
	delete(s.records, id)
	return true
}

// Len is the number of retained records.
func (s *Store) Len() int {
	if s == nil {
		return 0
	}
	return len(s.records)
}

// All returns every record sorted by ID, so listing is deterministic.
func (s *Store) All() []Record {
	if s == nil {
		return nil
	}
	out := make([]Record, 0, len(s.records))
	for _, r := range s.records {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// Applicable returns the records in scope that were made under the given repository
// identity, sorted by ID. Records from another identity are excluded: memory from a
// different repository state never overrides current evidence.
func (s *Store) Applicable(scope Scope, repository string) []Record {
	var out []Record
	for _, r := range s.All() {
		if r.Scope == scope && r.Repository == repository {
			out = append(out, r)
		}
	}
	return out
}

// Stale returns the records in scope made under a different repository identity than
// the current one, sorted by ID, so a caller can report or invalidate them.
func (s *Store) Stale(scope Scope, repository string) []Record {
	var out []Record
	for _, r := range s.All() {
		if r.Scope == scope && r.Repository != repository {
			out = append(out, r)
		}
	}
	return out
}

// Marshal serializes the store deterministically: records are ordered by ID, so an
// unchanged store always marshals to identical bytes.
func (s *Store) Marshal() ([]byte, error) {
	doc := struct {
		Schema  string   `json:"schema"`
		Limit   int      `json:"limit"`
		Records []Record `json:"records"`
	}{Schema: SchemaVersion, Limit: s.limit, Records: s.All()}
	return json.MarshalIndent(doc, "", "  ")
}

// Unmarshal loads a store written by Marshal. A store whose schema does not match the
// current SchemaVersion loads empty rather than applying decisions from an incompatible
// schema.
func Unmarshal(data []byte) (*Store, error) {
	var doc struct {
		Schema  string   `json:"schema"`
		Limit   int      `json:"limit"`
		Records []Record `json:"records"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("decisionmemory: %w", err)
	}
	s := New(doc.Limit)
	if doc.Schema != SchemaVersion {
		return s, nil
	}
	for _, r := range doc.Records {
		if r.validate() != nil {
			continue
		}
		s.records[r.ID()] = r
	}
	return s, nil
}
