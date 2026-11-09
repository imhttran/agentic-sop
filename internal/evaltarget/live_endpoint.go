package evaltarget

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// This file bounds the live inference endpoint. The live path permits exactly one
// endpoint, the fixed local inference URL (SOP_EVAL_LIVE_ENDPOINT). Every other
// host is unrelated egress and is refused. The endpoint is only reachable inside
// the OS isolation boundary, so an unavailable boundary is also a refusal.

// ErrEndpointNotAllowed reports that the configured endpoint is not the one
// allowlisted local inference endpoint.
var ErrEndpointNotAllowed = errors.New("evaltarget: live inference endpoint is not the allowlisted local endpoint")

// EndpointConfig is the bounded live endpoint configuration.
type EndpointConfig struct {
	// URL is the configured endpoint. It must equal LiveEndpointURL.
	URL string
	// Model is the local model name.
	Model string
	// Isolated reports whether the process runs inside the OS isolation boundary.
	Isolated bool
}

// ResolveEndpoint reads the endpoint configuration from the environment and
// records whether the isolation boundary is enforced.
//
// A missing or empty endpoint falls back to LiveEndpointURL so an operator can
// run the smoke path with no extra configuration. A different value is kept so
// AllowEndpoint reports the refusal with its detail.
func ResolveEndpoint(getenv func(string) string) EndpointConfig {
	endpoint := strings.TrimSpace(getenv(LiveEndpointEnv))
	if endpoint == "" {
		endpoint = LiveEndpointURL
	}
	model := strings.TrimSpace(getenv(LiveModelEnv))
	if model == "" {
		model = LiveDefaultModel
	}
	return EndpointConfig{URL: endpoint, Model: model, Isolated: liveIsolationEnforced()}
}

// liveIsolationEnforced reports whether the current process runs inside the OS
// isolation boundary. It returns false when the platform has no supported
// boundary or the boundary was not entered.
func liveIsolationEnforced() bool {
	if !IsolationAvailable() {
		return false
	}
	return RequireIsolation() == nil
}

// AllowEndpoint checks the single-endpoint allowlist. It returns an error for a
// different endpoint, a non-http(s) scheme, a non-loopback host, or a path that is
// not the allowlisted one.
func AllowEndpoint(endpoint string) error {
	want, err := url.Parse(LiveEndpointURL)
	if err != nil {
		return err
	}
	got, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrEndpointNotAllowed, err)
	}
	if got.Scheme != want.Scheme || got.Host != want.Host || got.Path != want.Path {
		return fmt.Errorf("%w: %q is not %q", ErrEndpointNotAllowed, endpoint, LiveEndpointURL)
	}
	if got.RawQuery != want.RawQuery {
		return fmt.Errorf("%w: unexpected query on %q", ErrEndpointNotAllowed, endpoint)
	}
	if !isLoopbackHost(got.Hostname()) {
		return fmt.Errorf("%w: %q is not a loopback host", ErrEndpointNotAllowed, got.Hostname())
	}
	return nil
}

// isLoopbackHost reports whether host is a loopback literal.
func isLoopbackHost(host string) bool {
	switch strings.ToLower(host) {
	case "127.0.0.1", "::1", "localhost":
		return true
	}
	return false
}

// requireLiveIsolation returns an error when the isolation boundary is not
// enforced. A test may set allowUnisolated to exercise the rest of the path.
func requireLiveIsolation(allowUnisolated bool) error {
	if allowUnisolated {
		return nil
	}
	if !IsolationAvailable() {
		return fmt.Errorf("evaltarget: live inference needs OS isolation: %w", ErrIsolationUnavailable)
	}
	return RequireIsolation()
}

// LocalDecider asks the one fixed local inference endpoint for one model output.
// It sends exactly one request, refuses every endpoint but the allowlisted local
// URL, and reads at most maxBytes of the response body.
type LocalDecider struct {
	// Endpoint is the configured endpoint. It must equal LiveEndpointURL.
	Endpoint string
	// Model is the local model name sent in the request.
	Model string
	// Prompt is the one prompt the model receives.
	Prompt string
	// MaxOutputBytes bounds the response body read. A zero value uses
	// LiveMaxOutputBytes.
	MaxOutputBytes int
	// Client is the HTTP client used for the single local request.
	Client *http.Client
}

// ollamaRequest is the minimal local generation request body.
type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
	Format string `json:"format,omitempty"`
}

// ollamaResponse is the minimal local generation response body.
type ollamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// Decide issues the single bounded local inference request and returns the raw
// model output. It refuses a non-allowlisted endpoint before the request and never
// follows a redirect off the loopback endpoint.
func (d LocalDecider) Decide(ctx context.Context) (string, error) {
	if err := AllowEndpoint(d.Endpoint); err != nil {
		return "", err
	}
	max := d.MaxOutputBytes
	if max <= 0 {
		max = LiveMaxOutputBytes
	}
	model := strings.TrimSpace(d.Model)
	if model == "" {
		model = LiveDefaultModel
	}
	body, err := json.Marshal(ollamaRequest{Model: model, Prompt: d.Prompt, Stream: false, Format: "json"})
	if err != nil {
		return "", err
	}
	client := d.Client
	if client == nil {
		client = &http.Client{
			Timeout:       LiveMaxProcessLifetime,
			CheckRedirect: noRedirect,
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, LiveEndpointURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("evaltarget: local inference request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("evaltarget: local inference returned status %d", resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, int64(max)+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("evaltarget: read local inference response: %w", err)
	}
	if len(raw) > max {
		return "", fmt.Errorf("evaltarget: local inference output exceeds the %d byte bound", max)
	}
	var parsed ollamaResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("evaltarget: malformed local inference response: %w", err)
	}
	return parsed.Response, nil
}

// noRedirect is the live client's redirect policy: a redirect is never followed,
// so a model endpoint cannot bounce the request to an unallowlisted destination.
func noRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
