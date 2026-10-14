package nimble

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"
)

// Transport performs a native SystemOne decision call plus availability checks
// against an Ollama-compatible backend.
//
// Keeping this an interface lets the provider and its translation be tested
// without a running Ollama instance (for example with httptest or a fake).
type Transport interface {
	// SystemOne performs POST {baseURL}/v1/systemone.
	SystemOne(ctx context.Context, req systemOneRequest) (systemOneResponse, error)
	// ListModels reports the models the backend currently serves. It is used by
	// Available and must not trigger an inference request.
	ListModels(ctx context.Context) ([]string, error)
}

// HTTPTransport is the production Transport backed by Go's standard HTTP client.
type HTTPTransport struct {
	baseURL string
	client  *http.Client
}

// NewHTTPTransport builds an HTTPTransport rooted at baseURL. A non-positive
// timeout falls back to DefaultTimeout.
func NewHTTPTransport(baseURL string, timeout time.Duration) *HTTPTransport {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &HTTPTransport{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		client:  &http.Client{Timeout: timeout},
	}
}

// SystemOne performs POST {baseURL}/v1/systemone and returns the raw response.
//
// The transport is responsible only for HTTP, JSON encoding/decoding, timeouts,
// HTTP status handling, and context cancellation. It contains no SOP policy.
func (t *HTTPTransport) SystemOne(ctx context.Context, req systemOneRequest) (systemOneResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return systemOneResponse{}, fmt.Errorf("nimble: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return systemOneResponse{}, fmt.Errorf("nimble: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return systemOneResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return systemOneResponse{}, &httpStatusError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(msg))}
	}

	var out systemOneResponse
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&out); err != nil {
		return systemOneResponse{}, &malformedResponseError{Err: fmt.Errorf("decode systemone response: %w", err)}
	}
	return out, nil
}

// modelList is the subset of Ollama's model listing response the adapter relies
// on. Ollama's GET /api/tags returns a list of tags with a "name" per model;
// names may include a tag suffix (for example "nimble:latest").
type modelList struct {
	Models []struct {
		Name  string `json:"name"`
		Model string `json:"model"`
	} `json:"models"`
}

// ListModels performs GET {baseURL}/api/tags and returns the reported model
// names. It never issues an inference request.
func (t *HTTPTransport) ListModels(ctx context.Context) ([]string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, t.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("nimble: build request: %w", err)
	}

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &httpStatusError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(msg))}
	}

	var list modelList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, &malformedResponseError{Err: fmt.Errorf("decode model list: %w", err)}
	}

	names := make([]string, 0, len(list.Models))
	for _, m := range list.Models {
		name := strings.TrimSpace(m.Name)
		if name == "" {
			name = strings.TrimSpace(m.Model)
		}
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// httpStatusError wraps a non-200 HTTP status from the backend.
type httpStatusError struct {
	StatusCode int
	Body       string
}

func (e *httpStatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("nimble: backend status %d", e.StatusCode)
	}
	return fmt.Sprintf("nimble: backend status %d: %s", e.StatusCode, e.Body)
}

// malformedResponseError marks a response the adapter could not decode.
type malformedResponseError struct{ Err error }

func (e *malformedResponseError) Error() string {
	return "nimble: malformed response: " + e.Err.Error()
}
func (e *malformedResponseError) Unwrap() error { return e.Err }

// isMalformedResponse reports whether err represents an undecodable response.
func isMalformedResponse(err error) bool {
	var mre *malformedResponseError
	return errors.As(err, &mre)
}

// isUnavailable reports whether err is a transport-level connectivity failure
// that should be normalized to decision.ErrUnavailable.
func isUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EHOSTUNREACH) || errors.Is(err, syscall.ENETUNREACH) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return true
		}
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	return false
}
