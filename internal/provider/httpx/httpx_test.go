package httpx_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/provider/httpx"
)

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"  http://x:1/  ": "http://x:1",
		"http://x:1":      "http://x:1",
		"":                "",
	}
	for in, want := range cases {
		if got := httpx.NormalizeBaseURL(in); got != want {
			t.Errorf("NormalizeBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewClientTimeout(t *testing.T) {
	if got := httpx.NewClient(0).Timeout; got != httpx.DefaultTimeout {
		t.Errorf("zero timeout = %v, want %v", got, httpx.DefaultTimeout)
	}
	if got := httpx.NewClient(-time.Second).Timeout; got != httpx.DefaultTimeout {
		t.Errorf("negative timeout = %v, want %v", got, httpx.DefaultTimeout)
	}
	if got := httpx.NewClient(3 * time.Second).Timeout; got != 3*time.Second {
		t.Errorf("timeout = %v, want 3s", got)
	}
}

// TestGetReturnsStatusAndBody pins the distinction callers rely on: a non-2xx is
// returned as a status, not raised as an error, so "reached, non-2xx" is
// distinguishable from "unreachable".
func TestGetReturnsStatusAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			w.WriteHeader(http.StatusTeapot)
			_, _ = w.Write([]byte("teapot"))
			return
		}
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	status, body, err := httpx.Get(context.Background(), srv.Client(), srv.URL)
	if err != nil || status != http.StatusOK || string(body) != "hello" {
		t.Fatalf("get = %d %q %v", status, body, err)
	}
	status, body, err = httpx.Get(context.Background(), srv.Client(), srv.URL+"/bad")
	if err != nil || status != http.StatusTeapot || string(body) != "teapot" {
		t.Fatalf("get bad = %d %q %v", status, body, err)
	}
}

func TestGetTransportErrorAndCancellation(t *testing.T) {
	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := closed.URL
	closed.Close()
	if _, _, err := httpx.Get(context.Background(), http.DefaultClient, url); err == nil {
		t.Fatal("an unreachable url must error")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := httpx.Get(ctx, http.DefaultClient, "http://127.0.0.1:1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestGetJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bad":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
		case "/notjson":
			_, _ = w.Write([]byte("not json"))
		default:
			_, _ = w.Write([]byte(`{"value":"x"}`))
		}
	}))
	defer srv.Close()

	var out struct {
		Value string `json:"value"`
	}
	if err := httpx.GetJSON(context.Background(), srv.Client(), srv.URL, &out); err != nil || out.Value != "x" {
		t.Fatalf("GetJSON = %+v %v", out, err)
	}

	err := httpx.GetJSON(context.Background(), srv.Client(), srv.URL+"/bad", &out)
	if err == nil || !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("non-2xx err = %v, want it to name 500 and the detail", err)
	}
	if err := httpx.GetJSON(context.Background(), srv.Client(), srv.URL+"/notjson", &out); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("decode err = %v", err)
	}
	// A nil out decodes nothing and still succeeds on 2xx.
	if err := httpx.GetJSON(context.Background(), srv.Client(), srv.URL, nil); err != nil {
		t.Fatalf("nil out: %v", err)
	}
}

func TestPostJSONSendsBodyAndDecodes(t *testing.T) {
	var gotMethod, gotCT string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	var out struct {
		OK bool `json:"ok"`
	}
	if err := httpx.PostJSON(context.Background(), srv.Client(), srv.URL, map[string]string{"model": "m"}, &out); err != nil || !out.OK {
		t.Fatalf("PostJSON = %+v %v", out, err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q", gotMethod)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type = %q", gotCT)
	}
	var sent map[string]string
	if err := json.Unmarshal(gotBody, &sent); err != nil || sent["model"] != "m" {
		t.Errorf("body = %s (%v)", gotBody, err)
	}
}
