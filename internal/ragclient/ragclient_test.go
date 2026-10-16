package ragclient

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeRunner returns a scripted result, recording the invocation.
type fakeRunner struct {
	out     []byte
	errOut  []byte
	runErr  error
	lastIn  []byte
	lastExe string
	lastMax int
}

func (f *fakeRunner) Run(_ context.Context, exe string, stdin []byte, maxOut int) ([]byte, []byte, error) {
	f.lastExe, f.lastIn, f.lastMax = exe, stdin, maxOut
	return f.out, f.errOut, f.runErr
}

func newTestClient(t *testing.T, cfg Config, r Runner) *Client {
	t.Helper()
	c, err := newWithRunner(cfg, r)
	if err != nil {
		t.Fatalf("newWithRunner: %v", err)
	}
	return c
}

func okEvidence() Response {
	return Response{
		SchemaVersion: SchemaVersion,
		Status:        "OK",
		Evidence: []Evidence{{
			ID: "42", Corpus: "kb", Source: "repo/docs/a.md", DocumentHash: "sha256:abc",
			Section: "Retrieval", Page: 3, Text: "evidence text", RetrievalMethod: "hybrid",
			Score: 0.03, Rank: 1,
		}},
		Diagnostics: Diagnostics{ElapsedMS: 12},
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestNewRejectsUnsafeConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"empty executable", Config{AllowedCorpora: []string{"kb"}}},
		{"relative executable", Config{Executable: "bin/rag", AllowedCorpora: []string{"kb"}}},
		{"missing executable", Config{Executable: "/nonexistent/rag-producer", AllowedCorpora: []string{"kb"}}},
		{"empty allowlist", Config{Executable: "/bin/true"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.cfg); err == nil {
				t.Fatal("New accepted an unsafe config, want error")
			}
		})
	}
}

func TestRetrieveSuccessPreservesProvenance(t *testing.T) {
	r := &fakeRunner{out: mustJSON(t, okEvidence())}
	c := newTestClient(t, Config{Executable: "/producer", AllowedCorpora: []string{"kb"}, MaxResults: 5}, r)

	res, err := c.Retrieve(context.Background(), Request{Query: "q", Filters: Filters{Corpus: "kb"}})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(res.Evidence) != 1 {
		t.Fatalf("evidence = %d, want 1", len(res.Evidence))
	}
	ev := res.Evidence[0]
	if ev.Corpus != "kb" || ev.DocumentHash != "sha256:abc" || ev.RetrievalMethod != "hybrid" || ev.Rank != 1 || ev.Page != 3 {
		t.Fatalf("provenance not preserved: %+v", ev)
	}
	// The request sent to the producer defaults the schema and clamps top_k.
	var sent Request
	if err := json.Unmarshal(r.lastIn, &sent); err != nil {
		t.Fatalf("request JSON: %v", err)
	}
	if sent.SchemaVersion != SchemaVersion || sent.TopK != 5 {
		t.Fatalf("sent request = %+v, want schema %s top_k 5", sent, SchemaVersion)
	}
}

func TestRetrieveRejectsUnauthorizedCorpus(t *testing.T) {
	c := newTestClient(t, Config{Executable: "/producer", AllowedCorpora: []string{"kb"}}, &fakeRunner{})
	for name, corpus := range map[string]string{"empty": "", "not allowed": "other"} {
		t.Run(name, func(t *testing.T) {
			_, err := c.Retrieve(context.Background(), Request{Query: "q", Filters: Filters{Corpus: corpus}})
			if CodeOf(err) != CodeUnauthorizedScope {
				t.Fatalf("code = %s, want %s (%v)", CodeOf(err), CodeUnauthorizedScope, err)
			}
		})
	}
}

func TestRetrieveRejectsOversizeQuery(t *testing.T) {
	c := newTestClient(t, Config{Executable: "/producer", AllowedCorpora: []string{"kb"}, MaxQueryBytes: 4}, &fakeRunner{})
	_, err := c.Retrieve(context.Background(), Request{Query: "too long", Filters: Filters{Corpus: "kb"}})
	if CodeOf(err) != CodeOversize {
		t.Fatalf("code = %s, want %s", CodeOf(err), CodeOversize)
	}
}

func TestRetrieveRejectsUnsupportedRequestSchema(t *testing.T) {
	c := newTestClient(t, Config{Executable: "/producer", AllowedCorpora: []string{"kb"}}, &fakeRunner{})
	_, err := c.Retrieve(context.Background(), Request{SchemaVersion: "v0", Query: "q", Filters: Filters{Corpus: "kb"}})
	if CodeOf(err) != CodeUnsupportedSchema {
		t.Fatalf("code = %s, want %s", CodeOf(err), CodeUnsupportedSchema)
	}
}

func TestRetrieveRunnerFailureIsUnavailable(t *testing.T) {
	c := newTestClient(t, Config{Executable: "/producer", AllowedCorpora: []string{"kb"}}, &fakeRunner{runErr: errors.New("exit status 2")})
	_, err := c.Retrieve(context.Background(), Request{Query: "q", Filters: Filters{Corpus: "kb"}})
	if CodeOf(err) != CodeUnavailable {
		t.Fatalf("code = %s, want %s", CodeOf(err), CodeUnavailable)
	}
}

func TestRetrieveTimeout(t *testing.T) {
	c := newTestClient(t, Config{Executable: "/producer", AllowedCorpora: []string{"kb"}, Timeout: 10 * time.Millisecond}, blockingRunner{})
	_, err := c.Retrieve(context.Background(), Request{Query: "q", Filters: Filters{Corpus: "kb"}})
	if CodeOf(err) != CodeTimeout {
		t.Fatalf("code = %s, want %s", CodeOf(err), CodeTimeout)
	}
}

type blockingRunner struct{}

func (blockingRunner) Run(ctx context.Context, _ string, _ []byte, _ int) ([]byte, []byte, error) {
	<-ctx.Done()
	return nil, nil, ctx.Err()
}

func TestDecodeCases(t *testing.T) {
	cases := []struct {
		name string
		out  []byte
		cfg  Config
		want Code
	}{
		{"valid", mustJSON(t, okEvidence()), Config{Executable: "/p", AllowedCorpora: []string{"kb"}}, ""},
		{"empty evidence is success", []byte(`{"schema_version":"rag-retrieval/v1","status":"OK"}`), Config{Executable: "/p", AllowedCorpora: []string{"kb"}}, ""},
		{"unsupported response schema", []byte(`{"schema_version":"v2","status":"OK"}`), Config{Executable: "/p", AllowedCorpora: []string{"kb"}}, CodeUnsupportedSchema},
		{"malformed json", []byte(`{not json`), Config{Executable: "/p", AllowedCorpora: []string{"kb"}}, CodeMalformed},
		{"empty response", []byte("   "), Config{Executable: "/p", AllowedCorpora: []string{"kb"}}, CodeMalformed},
		{"unknown field", []byte(`{"schema_version":"rag-retrieval/v1","status":"OK","extra":1}`), Config{Executable: "/p", AllowedCorpora: []string{"kb"}}, CodeMalformed},
		{"producer timeout code", []byte(`{"schema_version":"rag-retrieval/v1","status":"TIMEOUT","diagnostics":{"error_code":"TIMEOUT"}}`), Config{Executable: "/p", AllowedCorpora: []string{"kb"}}, CodeTimeout},
		{"evidence corpus mismatch", mustJSON(t, func() Response { r := okEvidence(); r.Evidence[0].Corpus = "other"; return r }()), Config{Executable: "/p", AllowedCorpora: []string{"kb"}}, CodeUnauthorizedScope},
		{"chunk oversize", mustJSON(t, func() Response { r := okEvidence(); r.Evidence[0].Text = strings.Repeat("x", 32); return r }()), Config{Executable: "/p", AllowedCorpora: []string{"kb"}, MaxChunkBytes: 8}, CodeOversize},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, tc.cfg, &fakeRunner{out: tc.out})
			_, err := c.Retrieve(context.Background(), Request{Query: "q", Filters: Filters{Corpus: "kb"}})
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if CodeOf(err) != tc.want {
				t.Fatalf("code = %s, want %s (%v)", CodeOf(err), tc.want, err)
			}
		})
	}
}

func TestDecodeOversizeResponse(t *testing.T) {
	c := newTestClient(t, Config{Executable: "/p", AllowedCorpora: []string{"kb"}, MaxResponseBytes: 8}, &fakeRunner{out: []byte("0123456789")})
	_, err := c.Retrieve(context.Background(), Request{Query: "q", Filters: Filters{Corpus: "kb"}})
	if CodeOf(err) != CodeOversize {
		t.Fatalf("code = %s, want %s", CodeOf(err), CodeOversize)
	}
}
