// Package ragclient implements the SOP-side client for the versioned,
// retrieval-only RAG protocol (schema "rag-retrieval/v1", see
// docs/specs/RAG-RETRIEVAL-CONTRACT.md). It is a narrowly scoped adapter: it
// executes an operator-configured local executable, sends one bounded JSON
// request, validates the bounded JSON response, and returns raw retrieved
// evidence.
//
// It is independent of SOP governance: it never touches task state, approvals,
// the lifecycle, or the repository, and it does not import any of
// rag-template's internal packages. Retrieval produces untrusted evidence only.
package ragclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SchemaVersion is the only contract version this client speaks. A producer
// speaking any other version is rejected with CodeUnsupportedSchema.
const SchemaVersion = "rag-retrieval/v1"

// Code is the typed failure/success code defined by the contract.
type Code string

const (
	CodeOK                Code = "OK"
	CodeEmpty             Code = "EMPTY"
	CodeTimeout           Code = "TIMEOUT"
	CodeUnavailable       Code = "UNAVAILABLE"
	CodeMalformed         Code = "MALFORMED"
	CodeOversize          Code = "OVERSIZE"
	CodeUnauthorizedScope Code = "UNAUTHORIZED_SCOPE"
	CodeUnsupportedSchema Code = "UNSUPPORTED_SCHEMA"
	CodeInternal          Code = "INTERNAL"
)

// Error is a typed retrieval failure. It carries the contract code so a caller
// can branch on it without matching on text.
type Error struct {
	Code Code
	Msg  string
}

func (e *Error) Error() string { return fmt.Sprintf("ragclient: %s: %s", e.Code, e.Msg) }

// CodeOf returns the typed code of err, or CodeInternal for a non-typed error.
func CodeOf(err error) Code {
	var re *Error
	if errors.As(err, &re) {
		return re.Code
	}
	return CodeInternal
}

func errf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// Filters scope the retrieval. Corpus is required and is the isolation unit.
type Filters struct {
	Corpus       string `json:"corpus"`
	SourcePrefix string `json:"source_prefix,omitempty"`
	Language     string `json:"language,omitempty"`
	IngestedFrom string `json:"ingested_from,omitempty"`
	IngestedTo   string `json:"ingested_to,omitempty"`
}

// Request is the versioned retrieval request.
type Request struct {
	SchemaVersion string  `json:"schema_version"`
	Query         string  `json:"query"`
	TopK          int     `json:"top_k"`
	Filters       Filters `json:"filters"`
	DeadlineMS    int     `json:"deadline_ms"`
}

// Evidence is one retrieved item: raw, attributed, untrusted evidence.
type Evidence struct {
	ID              string  `json:"id"`
	Corpus          string  `json:"corpus"`
	Source          string  `json:"source"`
	DocumentHash    string  `json:"document_hash"`
	Section         string  `json:"section"`
	Page            int     `json:"page"`
	Text            string  `json:"text"`
	RetrievalMethod string  `json:"retrieval_method"`
	Score           float64 `json:"score"`
	Rank            int     `json:"rank"`
}

// Diagnostics carries non-fatal server-side detail.
type Diagnostics struct {
	ElapsedMS int    `json:"elapsed_ms"`
	ErrorCode string `json:"error_code"`
}

// Response is the versioned retrieval response.
type Response struct {
	SchemaVersion string      `json:"schema_version"`
	Status        string      `json:"status"`
	Evidence      []Evidence  `json:"evidence"`
	Truncated     bool        `json:"truncated"`
	Diagnostics   Diagnostics `json:"diagnostics"`
}

// Config configures a Client. All limits are enforced client-side; the producer
// enforces its own independently.
type Config struct {
	// Executable is the absolute path of the retrieval-only producer.
	Executable string
	// AllowedCorpora is the trusted corpus allowlist. Empty forbids retrieval.
	AllowedCorpora []string
	// Timeout bounds one request. Zero defaults to 3s.
	Timeout time.Duration
	// MaxResults caps top_k. Zero defaults to 5.
	MaxResults int
	// MaxQueryBytes caps the query. Zero defaults to 4096.
	MaxQueryBytes int
	// MaxResponseBytes caps the whole response. Zero defaults to 256 KiB.
	MaxResponseBytes int
	// MaxChunkBytes caps one evidence text. Zero defaults to 16 KiB.
	MaxChunkBytes int
}

func (c Config) normalize() Config {
	if c.Timeout <= 0 {
		c.Timeout = 3 * time.Second
	}
	if c.MaxResults <= 0 {
		c.MaxResults = 5
	}
	if c.MaxQueryBytes <= 0 {
		c.MaxQueryBytes = 4096
	}
	if c.MaxResponseBytes <= 0 {
		c.MaxResponseBytes = 256 << 10
	}
	if c.MaxChunkBytes <= 0 {
		c.MaxChunkBytes = 16 << 10
	}
	return c
}

// Runner executes the producer. It is injected so the client is testable
// without a process.
type Runner interface {
	// Run executes executable with stdin and returns stdout/stderr, no shell.
	// It must enforce ctx cancellation and return at most maxOut output bytes.
	Run(ctx context.Context, executable string, stdin []byte, maxOut int) (stdout, stderr []byte, err error)
}

// Client is the SOP-side retrieval client.
type Client struct {
	cfg   Config
	run   Runner
	index map[string]bool
}

// New validates the configuration and returns a Client. It fails closed: a
// missing executable, a relative path, or an empty corpus allowlist is an
// error, so an unconfigured client cannot retrieve.
func New(cfg Config) (*Client, error) {
	cfg = cfg.normalize()
	if strings.TrimSpace(cfg.Executable) == "" {
		return nil, errors.New("ragclient: executable is required")
	}
	if !filepath.IsAbs(cfg.Executable) {
		return nil, fmt.Errorf("ragclient: executable must be an absolute path, got %q", cfg.Executable)
	}
	st, err := os.Stat(cfg.Executable)
	if err != nil {
		return nil, fmt.Errorf("ragclient: executable %q: %w", cfg.Executable, err)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("ragclient: executable %q is a directory", cfg.Executable)
	}
	idx, err := corpusIndex(cfg.AllowedCorpora)
	if err != nil {
		return nil, err
	}
	return &Client{cfg: cfg, run: &execRunner{}, index: idx}, nil
}

// newWithRunner is the test constructor (it skips the filesystem stat so tests
// do not need a real executable).
func newWithRunner(cfg Config, run Runner) (*Client, error) {
	cfg = cfg.normalize()
	idx, err := corpusIndex(cfg.AllowedCorpora)
	if err != nil {
		return nil, err
	}
	return &Client{cfg: cfg, run: run, index: idx}, nil
}

func corpusIndex(corpora []string) (map[string]bool, error) {
	if len(corpora) == 0 {
		return nil, errors.New("ragclient: an empty corpus allowlist forbids retrieval")
	}
	idx := make(map[string]bool, len(corpora))
	for _, c := range corpora {
		c = strings.TrimSpace(c)
		if c == "" {
			return nil, errors.New("ragclient: corpus allowlist contains an empty entry")
		}
		idx[c] = true
	}
	return idx, nil
}

// Allowed reports whether corpus is in the trusted allowlist.
func (c *Client) Allowed(corpus string) bool { return c.index[strings.TrimSpace(corpus)] }

// Retrieve performs one bounded, retrieval-only request and returns validated
// evidence. Every failure is a typed *Error. An empty result is a success with
// no evidence (CodeEmpty is reserved for a producer that reports it). It never
// mutates SOP state.
func (c *Client) Retrieve(ctx context.Context, req Request) (Response, error) {
	if req.SchemaVersion == "" {
		req.SchemaVersion = SchemaVersion
	}
	if req.SchemaVersion != SchemaVersion {
		return Response{}, errf(CodeUnsupportedSchema, "request schema_version %q is not supported", req.SchemaVersion)
	}
	if strings.TrimSpace(req.Query) == "" {
		return Response{}, errf(CodeMalformed, "query is empty")
	}
	if len(req.Query) > c.cfg.MaxQueryBytes {
		return Response{}, errf(CodeOversize, "query %d bytes exceeds limit %d", len(req.Query), c.cfg.MaxQueryBytes)
	}
	corpus := strings.TrimSpace(req.Filters.Corpus)
	if corpus == "" {
		return Response{}, errf(CodeUnauthorizedScope, "corpus is required")
	}
	if !c.Allowed(corpus) {
		return Response{}, errf(CodeUnauthorizedScope, "corpus %q is not in the trusted allowlist", corpus)
	}
	req.TopK = clampTopK(req.TopK, c.cfg.MaxResults)

	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	in, err := json.Marshal(req)
	if err != nil {
		return Response{}, errf(CodeInternal, "marshal request: %v", err)
	}
	out, _, err := c.run.Run(ctx, c.cfg.Executable, append(in, '\n'), c.cfg.MaxResponseBytes)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return Response{}, errf(CodeTimeout, "producer exceeded %s", c.cfg.Timeout)
		}
		return Response{}, errf(CodeUnavailable, "producer failed: %v", err)
	}
	if len(out) >= c.cfg.MaxResponseBytes {
		return Response{}, errf(CodeOversize, "response exceeds %d bytes", c.cfg.MaxResponseBytes)
	}
	return c.decode(out, corpus)
}

// decode parses and validates a producer response.
func (c *Client) decode(raw []byte, corpus string) (Response, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return Response{}, errf(CodeMalformed, "empty response")
	}
	var res Response
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&res); err != nil {
		return Response{}, errf(CodeMalformed, "invalid response JSON: %v", err)
	}
	if res.SchemaVersion != SchemaVersion {
		return Response{}, errf(CodeUnsupportedSchema, "response schema_version %q is not supported", res.SchemaVersion)
	}
	if code := strings.TrimSpace(res.Diagnostics.ErrorCode); code != "" && Code(code) != CodeOK && Code(code) != CodeEmpty {
		return Response{}, errf(Code(code), "producer reported %s", code)
	}
	for i := range res.Evidence {
		ev := &res.Evidence[i]
		if strings.TrimSpace(ev.Corpus) != corpus {
			return Response{}, errf(CodeUnauthorizedScope, "evidence %d corpus %q is not the requested corpus", i, ev.Corpus)
		}
		if len(ev.Text) > c.cfg.MaxChunkBytes {
			return Response{}, errf(CodeOversize, "evidence %d text %d bytes exceeds %d", i, len(ev.Text), c.cfg.MaxChunkBytes)
		}
	}
	if res.Status == "" {
		res.Status = "OK"
	}
	// An empty result is a valid success, not a failure.
	return res, nil
}

func clampTopK(k, max int) int {
	if k <= 0 {
		k = max
	}
	if k > max {
		k = max
	}
	return k
}
