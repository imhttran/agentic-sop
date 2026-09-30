// Package httpx provides the small, bounded HTTP helpers the provider adapters
// share: a GET that returns a status and a size-capped body, and a GET that
// decodes a JSON body. Keeping them here avoids three near-identical copies in
// the ollama, llama.cpp, and MLX adapters.
//
// Every call is read-only. Nothing in this package mutates a request payload or
// any SOP state.
package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultTimeout bounds a single probe when the caller configures none.
const DefaultTimeout = 10 * time.Second

// maxBodyBytes caps how much of a response is read, so a misbehaving server
// cannot exhaust memory.
const maxBodyBytes = 4 << 20 // 4 MiB

// NewClient returns an HTTP client with the given timeout, falling back to
// DefaultTimeout when it is non-positive.
func NewClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &http.Client{Timeout: timeout}
}

// Get performs a GET and returns the HTTP status and a size-capped body. A
// transport error (including context cancellation) is returned as-is so callers
// can distinguish "could not reach" from "reached, non-2xx".
func Get(ctx context.Context, client *http.Client, url string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return 0, nil, ctxErr
		}
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read response: %w", err)
	}
	return resp.StatusCode, body, nil
}

// GetJSON performs a GET and decodes a 2xx JSON body into out. A non-2xx status
// becomes a bounded error; a transport error is returned wrapped.
func GetJSON(ctx context.Context, client *http.Client, url string, out any) error {
	return doJSON(ctx, client, http.MethodGet, url, nil, out)
}

// PostJSON performs a POST of a JSON body and decodes a 2xx JSON response into
// out. It is used for APIs that require a body (for example Ollama's /api/show).
func PostJSON(ctx context.Context, client *http.Client, url string, in, out any) error {
	payload, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	return doJSON(ctx, client, http.MethodPost, url, payload, out)
}

func doJSON(ctx context.Context, client *http.Client, method, url string, payload []byte, out any) error {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := strings.TrimSpace(string(data))
		if len(detail) > 256 {
			detail = detail[:256]
		}
		if detail != "" {
			return fmt.Errorf("http %d: %s", resp.StatusCode, detail)
		}
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// NormalizeBaseURL trims whitespace and trailing slashes from a base URL.
func NormalizeBaseURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}
