package clef

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/decision"
)

// TestDecideOnceRecordsFirstAttemptOnly verifies the evaluation helper sends one
// production-shaped SystemOne request, normalizes the first response, and
// classifies the four failure modes the benchmark must distinguish: a
// valid-but-wrong choice, malformed output, an HTTP/model-load error, and a
// timeout. It also counts requests so a hidden retry would fail the test.
func TestDecideOnceRecordsFirstAttemptOnly(t *testing.T) {
	req := decision.DecisionRequest{
		State: "a deterministic unit test failed",
		Questions: []decision.Question{{
			ID:       "failure-disposition",
			Type:     decision.QuestionChoice,
			Criteria: "classify the failure",
			Choices:  []string{"AUTO_FIX", "BLOCK"},
		}},
	}

	cases := []struct {
		name       string
		status     int
		body       string
		wantValid  bool
		wantCat    string
		wantChoice string
	}{
		{
			name:       "valid but wrong action",
			status:     http.StatusOK,
			body:       `{"model":"fixture","answers":{"failure-disposition":{"type":"choice","choice":"BLOCK","confidence":0.9}},"usage":{"input_tokens":5,"output_tokens":0}}`,
			wantValid:  true,
			wantChoice: "BLOCK",
		},
		{
			name:      "malformed body",
			status:    http.StatusOK,
			body:      `this is not the decision schema`,
			wantValid: false,
			wantCat:   "malformed",
		},
		{
			name:      "missing answer",
			status:    http.StatusOK,
			body:      `{"model":"fixture","answers":{}}`,
			wantValid: false,
			wantCat:   "malformed",
		},
		{
			name:      "model load conflict",
			status:    http.StatusConflict,
			body:      `{"error":{"message":"VLM load failed"}}`,
			wantValid: false,
			wantCat:   "http_error",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/v1/systemone" {
					t.Errorf("path = %q, want /v1/systemone", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()

			out, err := DecideOnce(context.Background(), srv.URL, "fixture", 2*time.Second, req)
			if requests != 1 {
				t.Fatalf("requests = %d, want exactly 1 (no retry)", requests)
			}
			if out.SchemaValid != tc.wantValid {
				t.Fatalf("SchemaValid = %v, want %v (err=%v)", out.SchemaValid, tc.wantValid, err)
			}
			if (err != nil) == tc.wantValid {
				t.Fatalf("error = %v, want error=%v", err, !tc.wantValid)
			}
			if tc.wantCat != "" && out.ErrorCategory != tc.wantCat {
				t.Fatalf("ErrorCategory = %q, want %q (detail=%s)", out.ErrorCategory, tc.wantCat, out.ErrorDetail)
			}
			if tc.wantChoice != "" {
				if got := out.Result.Answers["failure-disposition"].Choice; got != tc.wantChoice {
					t.Fatalf("choice = %q, want %q", got, tc.wantChoice)
				}
				if out.Result.Usage.InputTokens != 5 {
					t.Fatalf("input tokens = %d, want 5", out.Result.Usage.InputTokens)
				}
			}
		})
	}
}

func TestDecideOnceTimeoutIsClassified(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(block)

	req := decision.DecisionRequest{
		State: "state",
		Questions: []decision.Question{{
			ID: "q", Type: decision.QuestionChoice, Criteria: "c", Choices: []string{"A", "B"},
		}},
	}
	out, err := DecideOnce(context.Background(), srv.URL, "fixture", 50*time.Millisecond, req)
	if err == nil {
		t.Fatal("want a timeout error")
	}
	if out.ErrorCategory != "timeout" {
		t.Fatalf("ErrorCategory = %q, want timeout (detail=%s)", out.ErrorCategory, out.ErrorDetail)
	}
	if out.SchemaValid {
		t.Fatal("a timeout must never be schema-valid")
	}
}
