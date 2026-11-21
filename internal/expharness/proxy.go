package expharness

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Record is one request the proxy saw from SOP.
type Record struct {
	Seq           int       `json:"seq"`
	Path          string    `json:"path"`
	ModelSent     string    `json:"model_sent"`
	ReqSHA256     string    `json:"req_sha256"`
	Start         time.Time `json:"start"`
	DurationMS    int64     `json:"duration_ms"`
	Status        int       `json:"status"`
	Refused       bool      `json:"refused,omitempty"` // a ceiling stopped it; nothing was forwarded
	ModelReturned string    `json:"model_returned,omitempty"`
	RespSHA256    string    `json:"resp_sha256,omitempty"`
	InputTokens   int64     `json:"input_tokens"`
	OutputTokens  int64     `json:"output_tokens"`
	HasTools      bool      `json:"has_tools"`
}

// Proxy is a recording reverse proxy placed between one SOP run and its provider
// endpoint. Use one Proxy per run so its records are that run's provider traffic.
// It never rewrites a request or response body. Streaming responses are not
// supported (SOP sends stream:false).
type Proxy struct {
	upstream  string
	maxCalls  int
	maxTokens int64
	client    *http.Client

	mu     sync.Mutex
	recs   []Record
	tokens int64
}

// NewProxy forwards to upstream (a base URL). Once maxCalls requests were forwarded,
// every further request is refused with 429 and not forwarded: a hard call ceiling.
// maxTokens is a STOP THRESHOLD, not a hard cost bound: it is compared with usage of
// responses already received, so one response (and each request already in flight) can
// overshoot it; the worst case is maxTokens plus the output of the in-flight requests.
// SOP exposes no generation limit to set per request. Non-positive ceilings refuse
// everything, so a ceiling must be set deliberately.
func NewProxy(upstream string, maxCalls int, maxTokens int64) *Proxy {
	return &Proxy{upstream: strings.TrimRight(upstream, "/"), maxCalls: maxCalls, maxTokens: maxTokens,
		client: &http.Client{Timeout: 10 * time.Minute}}
}

// Records returns a copy of the records so far.
func (p *Proxy) Records() []Record {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Record(nil), p.recs...)
}

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	var req struct {
		Model string `json:"model"`
		Tools []any  `json:"tools"`
	}
	_ = json.Unmarshal(body, &req)
	rec := Record{Path: r.URL.Path, ModelSent: req.Model, ReqSHA256: sha(body), Start: time.Now().UTC(), HasTools: len(req.Tools) > 0}

	p.mu.Lock()
	forwarded := 0
	for _, x := range p.recs {
		if !x.Refused {
			forwarded++
		}
	}
	rec.Seq = len(p.recs) + 1
	if forwarded >= p.maxCalls || p.tokens >= p.maxTokens {
		rec.Refused, rec.Status = true, http.StatusTooManyRequests
		p.recs = append(p.recs, rec)
		p.mu.Unlock()
		http.Error(w, "experiment ceiling reached", http.StatusTooManyRequests)
		return
	}
	p.recs = append(p.recs, rec) // reserve the slot so concurrent requests count
	idx := len(p.recs) - 1
	p.mu.Unlock()

	resp, rb, err := p.forward(r, body)
	status := http.StatusBadGateway
	if err == nil {
		status = resp.StatusCode
	}
	var got struct {
		Model           string `json:"model"`
		PromptEval      int64  `json:"prompt_eval_count"`
		Eval            int64  `json:"eval_count"`
		OpenAIUsageJSON struct {
			Prompt     int64 `json:"prompt_tokens"`
			Completion int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	_ = json.Unmarshal(rb, &got)
	rec.Status, rec.ModelReturned, rec.RespSHA256 = status, got.Model, sha(rb)
	rec.InputTokens = got.PromptEval + got.OpenAIUsageJSON.Prompt
	rec.OutputTokens = got.Eval + got.OpenAIUsageJSON.Completion
	rec.DurationMS = time.Since(rec.Start).Milliseconds()

	p.mu.Lock()
	p.recs[idx] = rec
	p.tokens += rec.InputTokens + rec.OutputTokens
	p.mu.Unlock()

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(status)
	_, _ = w.Write(rb)
}

// forward sends the request upstream and reads the whole response; a failed read is an
// error, never a silently truncated body.
func (p *Proxy) forward(r *http.Request, body []byte) (*http.Response, []byte, error) {
	// Only the path and query are taken from the caller; scheme and host always come
	// from the configured upstream, so a crafted request target cannot redirect it.
	u, err := url.Parse(p.upstream)
	if err != nil {
		return nil, nil, err
	}
	if !strings.HasPrefix(r.URL.Path, "/") || r.URL.Opaque != "" {
		return nil, nil, fmt.Errorf("refusing request target %q", r.URL.String())
	}
	u.Path = strings.TrimRight(u.Path, "/") + r.URL.Path
	u.RawQuery = r.URL.RawQuery
	out, err := http.NewRequestWithContext(r.Context(), r.Method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	out.Header = r.Header.Clone()
	resp, err := p.client.Do(out)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	rb, err := io.ReadAll(resp.Body)
	return resp, rb, err
}
