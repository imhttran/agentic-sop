package agent

import (
	"context"
	"strings"
	"testing"
)

// LLM08 hidden-context boundary tests.
//
// These exercise the real OpenAI-compatible request construction: a test server
// captures the serialized body and the Authorization header exactly as the
// provider sends them. The assertions distinguish transport authentication (the
// header) from model-visible content (the JSON body), and prove that protected
// environment values are not interpolated into a request while ordinary context
// is still carried.

// hiddenContextCanary is a synthetic, non-secret value that stands in for a
// protected project value. It is credential-shaped so a shape-based scanner would
// also recognise it, but it grants nothing.
const hiddenContextCanary = "sk-llm08-canary-not-a-real-secret"

// TestSerializedRequestSeparatesAuthenticationFromContent is the LLM08
// request-boundary test: the provider credential travels only in the
// Authorization header, and neither the credential nor an unrelated protected
// environment value appears in the serialized model message.
func TestSerializedRequestSeparatesAuthenticationFromContent(t *testing.T) {
	clearOpenAICompatibleExecutionEnv(t)
	srv, got := captureOpenAICompatibleServer(t, "ok")
	t.Setenv(EnvOpenAICompatibleBaseURL, srv.URL)
	t.Setenv(EnvOpenAICompatibleModel, "m")
	t.Setenv(EnvOpenAICompatibleAPIKey, "sk-provider-credential-canary")
	// A protected value the provider never reads. It stands in for a project
	// secret that happens to be present in the process environment.
	t.Setenv("SOP_TEST_PROTECTED_CANARY", hiddenContextCanary)

	a, err := NewOpenAICompatibleExecution(srv.URL, "m")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleExecution: %v", err)
	}
	req := Request{
		Capability:         Review,
		Task:               "Review the bounded context assembly for hidden-context exposure.",
		Input:              "Legitimate evidence: internal/prompt/compile.go keeps the task and instructions separate.",
		OutputRequirements: "Return the requested JSON document.",
	}
	if _, err := a.Generate(context.Background(), req); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if got.auth != "Bearer sk-provider-credential-canary" {
		t.Errorf("Authorization = %q, want the transport credential only", got.auth)
	}
	for _, secret := range []string{"sk-provider-credential-canary", hiddenContextCanary} {
		if strings.Contains(got.rawBody, secret) {
			t.Errorf("serialized model message carries a protected value: %s", got.rawBody)
		}
	}
	// Positive control: the ordinary, non-secret context is still carried, so the
	// exclusion above is not achieved by dropping model input.
	for _, want := range []string{"Legitimate evidence", "internal/prompt/compile.go"} {
		if !strings.Contains(got.rawBody, want) {
			t.Errorf("serialized model message dropped ordinary context %q: %s", want, got.rawBody)
		}
	}
}

// TestSerializedRequestCarriesNoSystemPromptSecret proves the transport does not
// copy any environment-shaped secret into the message: only the request fields
// the caller supplied are serialized.
func TestSerializedRequestCarriesNoSystemPromptSecret(t *testing.T) {
	clearOpenAICompatibleExecutionEnv(t)
	srv, got := captureOpenAICompatibleServer(t, "ok")
	t.Setenv(EnvOpenAICompatibleBaseURL, srv.URL)
	t.Setenv(EnvOpenAICompatibleAPIKey, "sk-provider-credential-canary")
	t.Setenv("SOP_TEST_PROTECTED_CANARY", hiddenContextCanary)

	a, err := NewOpenAICompatibleExecution(srv.URL, "m")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleExecution: %v", err)
	}
	if _, err := a.Generate(context.Background(), validRequest(Plan)); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got.auth != "Bearer sk-provider-credential-canary" {
		t.Fatalf("Authorization = %q, want the transport credential", got.auth)
	}
	if strings.Contains(got.rawBody, hiddenContextCanary) || strings.Contains(got.rawBody, "sk-provider-credential-canary") {
		t.Fatalf("request body carries a credential or protected value: %s", got.rawBody)
	}
}
