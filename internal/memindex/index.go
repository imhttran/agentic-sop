// Package memindex is the separate, rebuildable local SQLite memory index for RDM-002B.
// It stores structured metadata in a normal table and the searchable text in an FTS5
// virtual table, in a database file that is distinct from the SOP execution-state
// database. Search applies corpus scope and eligibility filters in SQL before ranking
// and limiting, parameterizes every query, and quotes each FTS term so user text can
// never break the FTS syntax.
package memindex

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	sqlite "modernc.org/sqlite"

	"github.com/imhttran/agentic-sop/internal/retrievalboundary"
)

// SchemaVersion is the on-disk layout version. Open applies a forward migration to this
// version and rejects a stored version it does not know.
const SchemaVersion = 2

// busyTimeoutMS bounds a writer's wait for the SQLite lock. A wait longer than this is a
// lock timeout reported as ErrLockTimeout rather than a hang.
const busyTimeoutMS = 1000

// Record is one indexed memory record. The identity is RecordID (stable); the content
// hash makes an upsert idempotent.
type Record struct {
	RecordID    string
	ContentHash string
	Corpus      retrievalboundary.Corpus
	Repository  string
	TaskID      string
	Status      string
	Lifecycle   string
	Eligible    bool
	Citation    string
	// Text is the searchable body indexed by FTS5.
	Text string
}

// Status is the index health snapshot the status command reports.
type Status struct {
	SchemaVersion int       `json:"schema_version"`
	Records       int       `json:"records"`
	Eligible      int       `json:"eligible"`
	Ineligible    int       `json:"ineligible"`
	LastRebuild   time.Time `json:"last_rebuild"`
}

// Index is a handle to the separate memory-index database file.
type Index struct {
	db   *sql.DB
	path string
}

// Open opens (and creates if absent) the memory-index database at path. The path must be
// a file; it is intentionally a separate file from the execution-state database.
//
// The connection pool allows several concurrent readers. SQLite serializes writers with
// its own file lock; a busy_timeout turns lock contention into an explicit
// ErrLockTimeout instead of a hang, and WAL mode lets readers proceed while a writer
// holds the write lock.
func Open(path string) (*Index, error) {
	if strings.TrimSpace(path) == "" {
		return nil, &retrievalboundary.InvalidQueryError{Reason: "empty index path"}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("memindex: open: %w", err)
	}
	if _, err := db.Exec(`PRAGMA busy_timeout = ` + fmt.Sprint(busyTimeoutMS) + `;`); err != nil {
		db.Close()
		return nil, wrapSQL(err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode = WAL;`); err != nil {
		db.Close()
		return nil, wrapSQL(err)
	}
	ix := &Index{db: db, path: path}
	if err := ix.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return ix, nil
}

// Path returns the database file path.
func (ix *Index) Path() string { return ix.path }

// Close closes the database.
func (ix *Index) Close() error { return ix.db.Close() }

// migrate creates the schema if absent and records the schema version. It is idempotent:
// every statement uses IF NOT EXISTS and the version row is inserted only when absent, so
// a healthy database opened twice never errors.
func (ix *Index) migrate() error {
	var cur string
	err := ix.db.QueryRow(`SELECT v FROM schema_meta WHERE k='schema_version'`).Scan(&cur)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		// A missing schema_meta is a fresh (or pre-version) database; any other error
		// is resolved by the CREATE statements below, which fail closed if the file is
		// not a usable database.
		cur = ""
	}
	if cur != fmt.Sprint(SchemaVersion) {
		// The memory index is disposable and derived from authoritative outcome files,
		// so a layout change rebuilds it instead of migrating rows in place.
		for _, s := range []string{`DROP TABLE IF EXISTS records_fts;`, `DROP TABLE IF EXISTS records;`, `DROP TABLE IF EXISTS schema_meta;`} {
			if _, err := ix.db.Exec(s); err != nil {
				return wrapSQL(err)
			}
		}
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_meta (k TEXT PRIMARY KEY, v TEXT NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS records (
			record_id TEXT PRIMARY KEY,
			content_hash TEXT NOT NULL,
			corpus TEXT NOT NULL,
			repository TEXT NOT NULL DEFAULT '',
			task_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			lifecycle TEXT NOT NULL DEFAULT '',
			eligible INTEGER NOT NULL DEFAULT 0,
			citation TEXT NOT NULL DEFAULT '',
			indexed_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS records_scope ON records (corpus, eligible);`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS records_fts USING fts5(record_id UNINDEXED, body);`,
	}
	for _, s := range stmts {
		if _, err := ix.db.Exec(s); err != nil {
			return wrapSQL(err)
		}
	}
	if cur != fmt.Sprint(SchemaVersion) {
		if _, err := ix.db.Exec(`INSERT INTO schema_meta(k,v) VALUES('schema_version', ?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, fmt.Sprint(SchemaVersion)); err != nil {
			return wrapSQL(err)
		}
	}
	return nil
}

// Upsert inserts or updates one record by stable RecordID. When the stored content hash
// equals r.ContentHash the write is a no-op, so repeated ingestion is idempotent.
func (ix *Index) Upsert(ctx context.Context, r Record) error {
	if strings.TrimSpace(r.RecordID) == "" {
		return &retrievalboundary.InvalidQueryError{Reason: "empty record id"}
	}
	if !r.Corpus.Valid() {
		return &retrievalboundary.UnsupportedCorpusError{Corpus: r.Corpus}
	}
	var existing string
	err := ix.db.QueryRowContext(ctx, `SELECT content_hash FROM records WHERE record_id=?`, r.RecordID).Scan(&existing)
	if err == nil && existing == r.ContentHash {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return wrapSQL(err)
	}

	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return wrapSQL(err)
	}
	defer tx.Rollback()
	el := 0
	if r.Eligible {
		el = 1
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO records(record_id,content_hash,corpus,repository,task_id,status,lifecycle,eligible,citation,indexed_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(record_id) DO UPDATE SET
		  content_hash=excluded.content_hash, corpus=excluded.corpus, repository=excluded.repository,
		  task_id=excluded.task_id, status=excluded.status, lifecycle=excluded.lifecycle,
		  eligible=excluded.eligible, citation=excluded.citation, indexed_at=excluded.indexed_at`,
		r.RecordID, r.ContentHash, string(r.Corpus), r.Repository, r.TaskID, r.Status, r.Lifecycle, el, r.Citation, nowRFC()); err != nil {
		return wrapSQL(err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM records_fts WHERE record_id=?`, r.RecordID); err != nil {
		return wrapSQL(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO records_fts(record_id, body) VALUES(?,?)`, r.RecordID, r.Text); err != nil {
		return wrapSQL(err)
	}
	return tx.Commit()
}

// Search runs one scoped, bounded search. Corpus scope and eligibility are applied in SQL
// before ranking and limiting. Empty text is an InvalidQueryError; unmatched text is an
// empty Result.
func (ix *Index) Search(ctx context.Context, q retrievalboundary.ScopedQuery) (retrievalboundary.Result, error) {
	lim, err := retrievalboundary.Validate(q)
	if err != nil {
		return retrievalboundary.Result{}, err
	}
	match, err := BuildFTSQuery(q.Text)
	if err != nil {
		return retrievalboundary.Result{}, err
	}

	dctx, cancel := context.WithTimeout(ctx, lim.TimeBudget)
	defer cancel()

	// bm25(records_fts) is smaller-is-better; sort ascending then by record_id ascending
	// for a deterministic tie-break. Corpus and eligibility filters are applied first.
	const base = `
		SELECT r.record_id, r.content_hash, r.corpus, r.repository, r.task_id, r.citation,
		       r.status, records_fts.body, bm25(records_fts) AS score
		FROM records_fts JOIN records r ON r.record_id = records_fts.record_id
		WHERE records_fts MATCH ? AND r.corpus = ? AND r.eligible = 1`

	args := []any{match, string(q.Scope.Corpus)}
	query := base
	if q.Scope.Repository != "" {
		query += ` AND r.repository = ?`
		args = append(args, q.Scope.Repository)
	}
	if q.Scope.TaskID != "" {
		query += ` AND r.task_id = ?`
		args = append(args, q.Scope.TaskID)
	}
	query += ` ORDER BY score ASC, r.record_id ASC LIMIT ?`
	args = append(args, lim.MaxResults)

	rows, err := ix.db.QueryContext(dctx, query, args...)
	if err != nil {
		if dctx.Err() != nil {
			return retrievalboundary.Result{}, fmt.Errorf("%w: %v", retrievalboundary.ErrTimeout, dctx.Err())
		}
		return retrievalboundary.Result{}, wrapSQL(err)
	}
	defer rows.Close()

	ev := make([]retrievalboundary.Evidence, 0, lim.MaxResults)
	for rows.Next() {
		var e retrievalboundary.Evidence
		var corpus string
		var score float64
		if err := rows.Scan(&e.RecordID, &e.ContentHash, &corpus, &e.Repository, &e.TaskID, &e.Citation, &e.Status, &e.Text, &score); err != nil {
			return retrievalboundary.Result{}, wrapSQL(err)
		}
		e.Corpus = retrievalboundary.Corpus(corpus)
		// FTS5 bm25 is negative (more negative = better); negate for a higher-is-better score.
		e.Score = -score
		e.Tokens = retrievalboundary.EstimateTokens(e.Text)
		ev = append(ev, e)
	}
	if err := rows.Err(); err != nil {
		if dctx.Err() != nil {
			return retrievalboundary.Result{}, fmt.Errorf("%w: %v", retrievalboundary.ErrTimeout, dctx.Err())
		}
		return retrievalboundary.Result{}, wrapSQL(err)
	}
	// Relevance floor: OR matching alone admits records that share only one common
	// word. Keep a record only when it shares at least two distinct query terms (or the
	// only term of a one-term query), so unrelated evidence is rejected rather than
	// merely ranked lower.
	queryTerms := retrievalboundary.Tokenize(q.Text)
	minShared := 1
	if len(queryTerms) >= 2 {
		minShared = 2
	}
	kept := ev[:0]
	for _, e := range ev {
		if sharedTerms(e.Text, queryTerms) >= minShared {
			kept = append(kept, e)
		}
	}
	res := retrievalboundary.ApplyBounds(kept, lim)
	res.Compatibility = "sqlite fts5: bm25 ascending, record id ascending"
	return res, nil
}

// Rebuild truncates the index and reseeds it from records. It returns indexed, skipped
// (ineligible) and excluded (zero; the caller reports source-side exclusions) counts.
func (ix *Index) Rebuild(ctx context.Context, records []Record) (indexed, skipped, excluded int, err error) {
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, 0, wrapSQL(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM records_fts`); err != nil {
		return 0, 0, 0, wrapSQL(err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM records`); err != nil {
		return 0, 0, 0, wrapSQL(err)
	}
	for _, r := range records {
		if !r.Eligible || strings.TrimSpace(r.RecordID) == "" {
			skipped++
			continue
		}
		el := 1
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO records(record_id,content_hash,corpus,repository,task_id,status,lifecycle,eligible,citation,indexed_at)
			VALUES(?,?,?,?,?,?,?,?,?,?)`,
			r.RecordID, r.ContentHash, string(r.Corpus), r.Repository, r.TaskID, r.Status, r.Lifecycle, el, r.Citation, nowRFC()); err != nil {
			return 0, 0, 0, wrapSQL(err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO records_fts(record_id, body) VALUES(?,?)`, r.RecordID, r.Text); err != nil {
			return 0, 0, 0, wrapSQL(err)
		}
		indexed++
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_meta(k,v) VALUES('last_rebuild',?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, nowRFC()); err != nil {
		return 0, 0, 0, wrapSQL(err)
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, 0, wrapSQL(err)
	}
	return indexed, skipped, excluded, nil
}

// Status returns the index health snapshot.
func (ix *Index) Status(ctx context.Context) (Status, error) {
	st := Status{SchemaVersion: SchemaVersion}
	if err := ix.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM records`).Scan(&st.Records); err != nil {
		return st, wrapSQL(err)
	}
	if err := ix.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM records WHERE eligible=1`).Scan(&st.Eligible); err != nil {
		return st, wrapSQL(err)
	}
	st.Ineligible = st.Records - st.Eligible
	var lr string
	err := ix.db.QueryRowContext(ctx, `SELECT v FROM schema_meta WHERE k='last_rebuild'`).Scan(&lr)
	if err == nil {
		if t, perr := time.Parse(time.RFC3339Nano, lr); perr == nil {
			st.LastRebuild = t
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return st, wrapSQL(err)
	}
	return st, nil
}

// HashRows returns record_id -> content_hash for drift comparison.
func (ix *Index) HashRows(ctx context.Context) (map[string]string, error) {
	rows, err := ix.db.QueryContext(ctx, `SELECT record_id, content_hash FROM records`)
	if err != nil {
		return nil, wrapSQL(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, h string
		if err := rows.Scan(&id, &h); err != nil {
			return nil, wrapSQL(err)
		}
		out[id] = h
	}
	return out, rows.Err()
}

// sharedTerms counts distinct query terms present in the record text.
func sharedTerms(text string, query []string) int {
	set := make(map[string]bool, len(query))
	for _, t := range retrievalboundary.Tokenize(text) {
		set[t] = true
	}
	seen := map[string]bool{}
	n := 0
	for _, t := range query {
		if set[t] && !seen[t] {
			seen[t] = true
			n++
		}
	}
	return n
}

func nowRFC() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// SQLite primary result codes used to classify a driver error without string matching.
const (
	sqliteBusy    = 5
	sqliteLocked  = 6
	sqliteCorrupt = 11
	sqliteNotADB  = 26
)

// wrapSQL maps a raw driver error to a typed boundary error: a corrupt database is
// ErrIndexCorrupt, a lock/busy condition is ErrLockTimeout, and any other driver error is
// returned wrapped. Classification prefers the driver's numeric result code and falls back
// to message matching only for errors that do not expose one.
func wrapSQL(err error) error {
	if err == nil {
		return nil
	}
	var se *sqlite.Error
	if errors.As(err, &se) {
		switch se.Code() {
		case sqliteBusy, sqliteLocked:
			return fmt.Errorf("%w: %v", retrievalboundary.ErrLockTimeout, err)
		case sqliteCorrupt, sqliteNotADB:
			return fmt.Errorf("%w: %v", retrievalboundary.ErrIndexCorrupt, err)
		}
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "locked") || strings.Contains(msg, "busy"):
		return fmt.Errorf("%w: %v", retrievalboundary.ErrLockTimeout, err)
	case strings.Contains(msg, "malformed") || strings.Contains(msg, "corrupt") || strings.Contains(msg, "not a database"):
		return fmt.Errorf("%w: %v", retrievalboundary.ErrIndexCorrupt, err)
	default:
		return fmt.Errorf("memindex: %w", err)
	}
}
