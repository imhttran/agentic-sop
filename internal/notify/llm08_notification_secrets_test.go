package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// LLM08 notification-boundary checks for the two gaps the shape-only detector
// left open:
//
//  1. an exact value of a secret-named environment variable that has no
//     credential shape reached the outbox and the channel verbatim;
//  2. credential-shaped text in ResolutionCommand reached the outbox and the
//     channel verbatim.
//
// The tests use synthetic canaries that are not real credentials and a local,
// fake transport. They never send a real Slack or webhook message.

// llm08ResolutionShape is a synthetic credential shape. It is not a real secret.
const llm08ResolutionShape = "sk-llm08-resolution-canary-7c4a91d2"

// llm08OpaqueSecret is a synthetic secret value with no credential shape. It is
// not a real secret.
const llm08OpaqueSecret = "llm08opaque7c4a91d2e6b3"

// waitingParams builds a valid waiting_for_human event input.
func waitingParams() Params {
	return Params{
		Kind:              KindWaitingForHuman,
		Project:           "demo",
		TaskID:            "task-1",
		Attempt:           1,
		Transition:        "WAITING_FOR_HUMAN",
		Approval:          pendingApproval(),
		ResolutionCommand: "sop approve appr-1 --approve",
	}
}

// TestLLM08ResolutionCommandCredentialShapeRedacted proves a credential-shaped
// resolution command is redacted before the event is persisted or delivered, and
// that the redacted text does not read as a directly executable command.
func TestLLM08ResolutionCommandCredentialShapeRedacted(t *testing.T) {
	p := waitingParams()
	p.ResolutionCommand = "sop approve appr-1 --token " + llm08ResolutionShape
	e, err := NewEvent(p)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if strings.Contains(e.ResolutionCommand, llm08ResolutionShape) {
		t.Fatalf("resolution command leaked the credential: %q", e.ResolutionCommand)
	}
	if !strings.Contains(e.ResolutionCommand, redactionMarker) {
		t.Fatalf("resolution command was not marked as redacted: %q", e.ResolutionCommand)
	}
	if !strings.Contains(e.ResolutionCommand, "not directly executable") {
		t.Fatalf("redacted resolution command implies it is directly executable: %q", e.ResolutionCommand)
	}
}

// TestLLM08CredentialFreeResolutionCommandPreserved proves an ordinary resolution
// command is persisted and delivered exactly as supplied.
func TestLLM08CredentialFreeResolutionCommandPreserved(t *testing.T) {
	p := waitingParams()
	e, err := NewEvent(p)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if e.ResolutionCommand != p.ResolutionCommand {
		t.Fatalf("credential-free resolution command changed: %q != %q", e.ResolutionCommand, p.ResolutionCommand)
	}
	if strings.Contains(e.ResolutionCommand, "not directly executable") {
		t.Fatalf("credential-free resolution command was marked non-executable: %q", e.ResolutionCommand)
	}
}

// TestLLM08KnownSecretValuesRedactedInEveryKind proves exact secret values the
// caller supplies are removed from Reason, NextAction, and Outcome for completed,
// failed, and waiting_for_human events.
func TestLLM08KnownSecretValuesRedactedInEveryKind(t *testing.T) {
	t.Run("completed", func(t *testing.T) {
		p := completedParams()
		p.Outcome = "completed while holding " + llm08OpaqueSecret
		p.NextAction = "rotate " + llm08OpaqueSecret + " and merge"
		p.Secrets = []string{llm08OpaqueSecret}
		e, err := NewEvent(p)
		if err != nil {
			t.Fatalf("NewEvent: %v", err)
		}
		for field, v := range map[string]string{"outcome": e.Outcome, "next_action": e.NextAction} {
			if strings.Contains(v, llm08OpaqueSecret) {
				t.Fatalf("%s leaked the secret: %q", field, v)
			}
			if !strings.Contains(v, redactionMarker) {
				t.Fatalf("%s was not marked as redacted: %q", field, v)
			}
		}
	})

	t.Run("failed", func(t *testing.T) {
		p := llm08FailedParams(false)
		p.Reason = "validation failed with " + llm08OpaqueSecret
		p.NextAction = "rotate " + llm08OpaqueSecret + " and retry"
		p.Secrets = []string{llm08OpaqueSecret}
		e, err := NewEvent(p)
		if err != nil {
			t.Fatalf("NewEvent: %v", err)
		}
		for field, v := range map[string]string{"reason": e.Reason, "next_action": e.NextAction} {
			if strings.Contains(v, llm08OpaqueSecret) {
				t.Fatalf("%s leaked the secret: %q", field, v)
			}
			if !strings.Contains(v, redactionMarker) {
				t.Fatalf("%s was not marked as redacted: %q", field, v)
			}
		}
	})

	t.Run("waiting_for_human", func(t *testing.T) {
		p := waitingParams()
		p.Approval.Reason = "the gate needs " + llm08OpaqueSecret
		p.ResolutionCommand = "sop approve appr-1 --token " + llm08OpaqueSecret
		p.Secrets = []string{llm08OpaqueSecret}
		e, err := NewEvent(p)
		if err != nil {
			t.Fatalf("NewEvent: %v", err)
		}
		for field, v := range map[string]string{"reason": e.Reason, "resolution_command": e.ResolutionCommand} {
			if strings.Contains(v, llm08OpaqueSecret) {
				t.Fatalf("%s leaked the secret: %q", field, v)
			}
			if !strings.Contains(v, redactionMarker) {
				t.Fatalf("%s was not marked as redacted: %q", field, v)
			}
		}
		if !strings.Contains(e.ResolutionCommand, "not directly executable") {
			t.Fatalf("redacted resolution command implies it is directly executable: %q", e.ResolutionCommand)
		}
	})
}

// TestLLM08AdapterPayloadsOmitKnownSecretValues proves the Slack encoder and the
// native webhook body built from the event carry no exact secret value. The
// receiver is a local httptest server; no real message is sent.
func TestLLM08AdapterPayloadsOmitKnownSecretValues(t *testing.T) {
	p := waitingParams()
	p.Approval.Reason = "the gate needs " + llm08OpaqueSecret
	p.ResolutionCommand = "sop approve appr-1 --token " + llm08OpaqueSecret
	p.Secrets = []string{llm08OpaqueSecret}
	e, err := NewEvent(p)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}

	slack, err := EncodeSlackPayload(e)
	if err != nil {
		t.Fatalf("EncodeSlackPayload: %v", err)
	}
	if strings.Contains(string(slack), llm08OpaqueSecret) {
		t.Fatalf("slack payload leaked the secret: %s", slack)
	}
	if !strings.Contains(string(slack), "not directly executable") {
		t.Fatalf("slack payload presents the redacted command as runnable: %s", slack)
	}

	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(data))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	ch := NewWebhookChannel(server.URL, server.Client())
	ack, err := ch.Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1})
	if err != nil || ack != AckConfirmed {
		t.Fatalf("webhook Send: ack=%q err=%v", ack, err)
	}
	if len(bodies) != 1 {
		t.Fatalf("receiver saw %d bodies, want 1", len(bodies))
	}
	if strings.Contains(bodies[0], llm08OpaqueSecret) {
		t.Fatalf("native webhook payload leaked the secret: %s", bodies[0])
	}
}

// TestLLM08KnownSecretValueCannotDisarmShapeDetection proves the composition runs
// the shape detector against the original text. Replacing an exact value first
// would turn an assignment key or token prefix into a marker, disarm the detector,
// and leave the credential that the shape matched exposed.
func TestLLM08KnownSecretValueCannotDisarmShapeDetection(t *testing.T) {
	t.Run("assignment key overlap", func(t *testing.T) {
		p := completedParams()
		p.Outcome = "login with password=actual-production-credential"
		p.Secrets = []string{"password"}
		e, err := NewEvent(p)
		if err != nil {
			t.Fatalf("NewEvent: %v", err)
		}
		if strings.Contains(e.Outcome, "actual-production-credential") {
			t.Fatalf("exact-value replacement disarmed the assignment shape: %q", e.Outcome)
		}
		if !strings.Contains(e.Outcome, redactionMarker) {
			t.Fatalf("assignment was not marked as redacted: %q", e.Outcome)
		}
	})

	t.Run("token prefix overlap", func(t *testing.T) {
		p := completedParams()
		p.Outcome = "credential ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ012345"
		p.Secrets = []string{"ghp_"}
		e, err := NewEvent(p)
		if err != nil {
			t.Fatalf("NewEvent: %v", err)
		}
		if strings.Contains(e.Outcome, "ABCDEFGHIJKLMNOPQRSTUVWXYZ012345") {
			t.Fatalf("exact-value replacement disarmed the token shape: %q", e.Outcome)
		}
	})
}

// TestLLM08KnownSecretValuePreservesIdentity proves exact-value redaction does not
// change the deterministic event id or the approval binding.
func TestLLM08KnownSecretValuePreservesIdentity(t *testing.T) {
	base := completedParams()
	base.OccurredAt = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	control, err := NewEvent(base)
	if err != nil {
		t.Fatalf("NewEvent control: %v", err)
	}

	secret := base
	secret.Outcome = "completed while holding " + llm08OpaqueSecret
	secret.Secrets = []string{llm08OpaqueSecret}
	redacted, err := NewEvent(secret)
	if err != nil {
		t.Fatalf("NewEvent secret: %v", err)
	}
	if control.ID != redacted.ID {
		t.Fatalf("redaction changed the event id: %q != %q", control.ID, redacted.ID)
	}

	approval := waitingParams()
	approval.OccurredAt = base.OccurredAt
	approvalEvent, err := NewEvent(approval)
	if err != nil {
		t.Fatalf("NewEvent approval: %v", err)
	}
	if approvalEvent.ApprovalRequestID != "appr-1" {
		t.Fatalf("approval binding changed: %q", approvalEvent.ApprovalRequestID)
	}
}

// TestLLM08EventSerializationOmitsKnownSecretValues proves the raw secret
// collection is not stored on the event and cannot be serialized with it.
func TestLLM08EventSerializationOmitsKnownSecretValues(t *testing.T) {
	p := completedParams()
	p.Outcome = "completed while holding " + llm08OpaqueSecret
	p.Secrets = []string{llm08OpaqueSecret}
	e, err := NewEvent(p)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	body, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	if strings.Contains(string(body), llm08OpaqueSecret) {
		t.Fatalf("serialized event carried the secret collection: %s", body)
	}
	if !strings.Contains(string(body), redactionMarker) {
		t.Fatalf("serialized event is not marked redacted: %s", body)
	}
}
