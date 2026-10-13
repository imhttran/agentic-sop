// This file adds the VER-002 portable cross-worktree evidence store and the
// append-only reuse-decision record. It extends the CTX-007 verification cache
// (verifcache.go) rather than replacing it: every identity, digest, equivalence,
// serialization, and reuse decision is defined and computed by the existing
// primitives (Identity.Digest, Equivalent, Decide, Cache.Lookup, Marshal,
// Unmarshal). No second identity or cache implementation is introduced.
//
// Reuse remains correctness-first. The store consults the per-project
// verify-cache.json plus, only when an opt-in shared location is configured, a
// shared cross-worktree store. A hit is returned only for an exact Identity
// equivalence; every failure mode (unreadable file, schema mismatch, or a
// corrupt entry) yields a miss, never a hit. Nothing here consults a model, a
// confidence score, or the network.
package verifcache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SharedEnv is the environment variable naming the opt-in shared cross-worktree
// evidence store. When it is unset or empty the store is disabled and lookup
// behavior is identical to the local-only cache. Setting it to a directory (or a
// verify-cache.json file path) enables cross-worktree reuse for exactly
// equivalent identities.
const SharedEnv = "SOP_VERIFY_CACHE_SHARED"

// SharedStoreFileName is the file name used when a shared location is a
// directory rather than a file path. It mirrors the per-project artifact it
// shadows: verify-cache.json.
const SharedStoreFileName = "verify-cache.json"

// DecisionRecordName is the append-only reuse-decision record written beside a
// run's other artifacts. It follows the existing run-artifact/audit.jsonl
// convention (one JSON object per line) under the run directory.
const DecisionRecordName = "verify-cache-decisions.jsonl"

// Source names which store answered a lookup.
const (
	// SourceLocal is the per-project verify-cache.json cache.
	SourceLocal = "local"
	// SourceShared is the opt-in shared cross-worktree store.
	SourceShared = "shared"
)

// SharedStore is the opt-in, portable, cross-worktree evidence store. It is a
// thin consumer of the existing cache: it reads and writes a verifcache document
// with Marshal/Unmarshal and never re-implements identity or caching.
//
// A zero or disabled SharedStore behaves exactly like an absent store: every
// read is a miss. It is safe to construct even when no shared location is
// configured.
//
// Concurrent writers across worktrees are serialized by an advisory lock file
// beside the store and committed with an atomic rename, so a concurrent store
// can neither lose a previously written entry nor leave the shared document
// truncated/partial. A partial document would still fail closed, but this also
// avoids silently destroying prior evidence.
type SharedStore struct {
	// Path is the resolved shared store file. Empty means disabled.
	Path string
}

// SharedFromEnv resolves the opt-in shared store from the environment. It is
// disabled by default: an unset or blank SOP_VERIFY_CACHE_SHARED yields a
// disabled store (Path empty), so no behavior changes unless an operator opts in.
//
// When the configured location names a directory that already exists, the store
// resolves to that directory's verify-cache.json. A location with an explicit
// trailing separator is always treated as a directory: the store resolves to
// <dir>/verify-cache.json and the directory is created by the first write (this
// removes the silent ambiguity where a not-yet-created directory was treated as a
// file). Any other location is treated as the file path itself.
func SharedFromEnv(getenv func(string) string) SharedStore {
	if getenv == nil {
		getenv = os.Getenv
	}
	loc := strings.TrimSpace(getenv(SharedEnv))
	if loc == "" {
		return SharedStore{}
	}
	// An explicit trailing separator (or an existing directory) means the
	// operator intends a directory holding the shared verify-cache.json.
	if strings.HasSuffix(loc, string(os.PathSeparator)) {
		return SharedStore{Path: filepath.Join(loc, SharedStoreFileName)}
	}
	if info, err := os.Stat(loc); err == nil && info.IsDir() {
		return SharedStore{Path: filepath.Join(loc, SharedStoreFileName)}
	}
	return SharedStore{Path: loc}
}

// Disabled reports whether the shared store is off, in which case lookup is
// local-only.
func (s SharedStore) Disabled() bool { return strings.TrimSpace(s.Path) == "" }

// lockPath is the advisory lock file guarding the shared store's read-modify-write.
func (s SharedStore) lockPath() string { return s.Path + ".lock" }

// lock acquires an exclusive advisory lock for the shared store, creating the
// containing directory and the lock file when needed. It returns an unlock
// function. Locking is best-effort: on platforms where an advisory lock cannot be
// taken it degrades to no lock (still atomic via rename) rather than failing the
// caller. The lock is process/OS-scoped; the atomic rename below is the primary
// guarantee, the lock only prevents lost updates across concurrent writers.
func (s SharedStore) lock() (func(), error) {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return func() {}, err
	}
	f, err := os.OpenFile(s.lockPath(), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		// A lock file we cannot create must not block a fail-closed store; fall
		// back to a lock-free (still atomic-rename) write.
		return func() {}, nil
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		return func() {}, nil
	}
	return func() {
		_ = unlockFile(f)
		_ = f.Close()
	}, nil
}

// load reads the shared store. It is fail-closed: an unreadable file, a schema
// mismatch, or a corrupt entry each yields an empty cache with ok false rather
// than a hit. The caller therefore records a miss.
func (s SharedStore) load() (*Cache, bool) {
	if s.Disabled() {
		return New(), false
	}
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return New(), false
	}
	cache, err := Unmarshal(data)
	if err != nil {
		// A corrupt/unparseable store must never produce a hit.
		return New(), false
	}
	return cache, true
}

// Lookup consults the local cache first and, when the shared store is enabled,
// the shared store. It returns the first exact identity match and the source
// that answered it. A miss in both stores returns a zero Lookup.
//
// Cross-worktree reuse occurs only on exact identity equivalence: both caches
// are consulted with the existing Cache.Lookup, which requires a full Identity
// match. Diverged revisions, inputs, commands, environment/toolchain, policy,
// scope, or verifier therefore miss.
//
// Lookup is fail-closed for the shared store: if it is unreadable, schema-
// mismatched, or corrupt, the shared store contributes no entries and the
// lookup falls back to the local result (which may itself be a miss).
func (s SharedStore) Lookup(local *Cache, id Identity) (Lookup, string) {
	if hit := lookupOK(local, id); hit.Hit {
		return hit, SourceLocal
	}
	if s.Disabled() {
		return Lookup{}, SourceLocal
	}
	shared, _ := s.load()
	if hit := lookupOK(shared, id); hit.Hit {
		return hit, SourceShared
	}
	return Lookup{}, SourceShared
}

// Store records a result in the local cache and, when the shared store is
// enabled, persists the same entry to the shared store so another worktree can
// find it. It reuses Marshal/Unmarshal for serialization. A shared store that
// cannot be read is treated as empty (fail closed) rather than propagated as a
// hit; a write failure is returned so the caller can decide.
//
// The shared read-modify-write is serialized by an advisory lock and committed
// with an atomic rename, so concurrent writers do not lose each other's entries
// and the shared document is never left truncated or partial.
func (s SharedStore) Store(local *Cache, id Identity, res Result) (Entry, error) {
	entry := local.Store(id, res)
	if s.Disabled() {
		return entry, nil
	}
	unlock, err := s.lock()
	if err != nil {
		return entry, err
	}
	defer unlock()

	shared, _ := s.load()
	shared.Store(id, res)
	data, err := shared.Marshal()
	if err != nil {
		return entry, err
	}
	return entry, atomicWrite(s.Path, append(data, '\n'))
}

// atomicWrite writes data to path atomically: it writes a temp file in the same
// directory, fsyncs it, and renames it over the destination (an atomic replace on
// POSIX filesystems). A concurrent reader therefore never observes a truncated or
// partial document, and a concurrent writer never loses entries written under the
// advisory lock.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// lookupOK is a nil-safe Cache.Lookup, so a nil local cache is a miss rather
// than a panic.
func lookupOK(c *Cache, id Identity) Lookup {
	if c == nil {
		return Lookup{}
	}
	return c.Lookup(id)
}

// DecisionRecord is one append-only reuse decision. It is written as a single
// JSON object per line (JSONL), matching the existing audit.jsonl convention,
// and it is inspectable: it names the hit/miss outcome, the key digest, the
// scope, and the source run that produced or answered the evidence.
type DecisionRecord struct {
	// Time is the RFC3339 UTC timestamp of the decision.
	Time string `json:"time"`
	// Hit is true only when an exact identity match was found. A miss is not a
	// negative verification result; it means verification must run.
	Hit bool `json:"hit"`
	// KeyDigest is the digest of the current identity consulted (Identity.Digest).
	KeyDigest string `json:"key_digest"`
	// Scope is the validation scope identity of the current state (Identity.Scope).
	Scope string `json:"scope"`
	// Source is the store that answered the lookup: "local" or "shared".
	Source string `json:"source"`
	// SourceRun names the run whose decision this is (for example "S001"); it is
	// the provenance of the reuse decision itself.
	SourceRun string `json:"source_run"`
	// PriorRun names the run that produced the reused evidence, when known.
	PriorRun string `json:"prior_run,omitempty"`
	// Reason is the human-readable decision reason (from Decide on a miss).
	Reason string `json:"reason,omitempty"`
}

// NewDecisionRecord builds a decision record for a lookup of id answered by the
// store source, attributed to sourceRun. The digest and scope are taken from the
// identity itself, so the record never diverges from the key that was consulted.
func NewDecisionRecord(id Identity, hit bool, source, sourceRun string, pri Entry, reason string) DecisionRecord {
	priorRun := ""
	if hit {
		priorRun = strings.TrimSpace(pri.Source)
	}
	return DecisionRecord{
		Time:      time.Now().UTC().Format(time.RFC3339),
		Hit:       hit,
		KeyDigest: id.Digest(),
		Scope:     id.Scope,
		Source:    source,
		SourceRun: sourceRun,
		PriorRun:  priorRun,
		Reason:    reason,
	}
}

// AppendDecision appends one reuse-decision record to path as a JSON line,
// creating the containing directory when needed. The file is opened for append
// (O_APPEND|O_CREATE|O_WRONLY), so concurrent writers do not overwrite earlier
// decisions and prior records are never rewritten: the record is append-only.
//
// Write, Sync, and Close errors are all reported, so a failed flush on Close is
// not silently dropped. The record write is not best-effort: a caller that
// requires an audit trail must not ignore a non-nil error.
func AppendDecision(path string, rec DecisionRecord) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("verifcache: decision record path is empty")
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("verifcache: marshal decision: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	var errs []error
	if _, err := f.Write(append(data, '\n')); err != nil {
		errs = append(errs, err)
	}
	if err := f.Sync(); err != nil {
		errs = append(errs, err)
	}
	if err := f.Close(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// DecisionPath returns the run-relative path of a run's reuse-decision record,
// under the existing runs/<id>/ layout. The caller passes the project directory
// and the run id; the record sits beside the run's other artifacts.
func DecisionPath(projectDir, runID string) string {
	return filepath.Join(projectDir, ".agent-sdlc", "runs", runID, DecisionRecordName)
}

// LookupRecorded performs a shared-store lookup and always appends exactly one
// append-only decision record for the decision, so a caller cannot bypass the
// audit trail for a reuse decision. It is the recording entry point: prefer it
// over Lookup when a decision record must exist for every lookup.
//
// If the decision record cannot be appended the lookup still returns its result,
// but the append error is surfaced alongside it; callers that require the audit
// trail treat a non-nil error as fatal, and callers that do not can still inspect
// the hit/miss.
func (s SharedStore) LookupRecorded(local *Cache, id Identity, decisionPath, sourceRun string) (Lookup, string, error) {
	hit, source := s.Lookup(local, id)
	rec := NewDecisionRecord(id, hit.Hit, source, sourceRun, hit.Entry, "")
	if !hit.Hit {
		rec.Reason = Decide(Entry{}, id).Reason
	}
	return hit, source, AppendDecision(decisionPath, rec)
}

// StoreRecorded records a result (local and, when enabled, shared) and appends
// exactly one append-only decision record describing the reuse decision that
// preceded it, so the store decision is inspectable too. It does not run a
// lookup: callers that also want the pre-store decision should record the lookup
// with LookupRecorded first. The append error is returned alongside the entry.
func (s SharedStore) StoreRecorded(local *Cache, id Identity, res Result, decisionPath, sourceRun string) (Entry, error) {
	entry, err := s.Store(local, id, res)
	if err != nil {
		return entry, err
	}
	rec := NewDecisionRecord(id, false, SourceLocal, sourceRun, entry, "stored: no reusable prior evidence")
	return entry, AppendDecision(decisionPath, rec)
}
