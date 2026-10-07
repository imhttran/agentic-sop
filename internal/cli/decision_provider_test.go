package cli

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/decision"
)

// TestDecisionProviderOffByDefault proves the external decision capability is
// OFF by default: the default configuration constructs no provider, and the
// factory is nonetheless wired at the composition root.
func TestDecisionProviderOffByDefault(t *testing.T) {
	cfg := config.Default()
	p, err := decisionProviderFromConfig(cfg)
	if err != nil || p != nil {
		t.Fatalf("a default configuration must construct no decision provider: p=%v err=%v", p, err)
	}
	if defaultDeps().newDecisionProvider == nil {
		t.Fatal("the decision-provider factory must be wired at the composition root")
	}
	// An existing installation with no decision configuration behaves as before.
	p, err = defaultDeps().newDecisionProvider(cfg)
	if err != nil || p != nil {
		t.Fatalf("no configured provider must be a strict no-op: p=%v err=%v", p, err)
	}
}

func TestDecisionProviderDeterministic(t *testing.T) {
	cfg := config.Default()
	cfg.Decision.Enabled = true
	cfg.Decision.Provider = "deterministic"
	p, err := decisionProviderFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if p == nil || p.Name() != "deterministic" {
		t.Fatalf("provider=%v, want the in-process deterministic provider", p)
	}
	d, err := p.Decide(nil, decision.Request{UseCase: "k", Subject: "small change"})
	if err != nil || decision.Validate(d) != nil {
		t.Fatalf("deterministic provider must satisfy the contract: d=%+v err=%v", d, err)
	}
}

func TestDecisionProviderCommand(t *testing.T) {
	cfg := config.Default()
	cfg.Decision.Enabled = true
	cfg.Decision.Provider = "command"
	cfg.Decision.Command = []string{"/bin/echo"}
	p, err := decisionProviderFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if p == nil || p.Name() != "command" {
		t.Fatalf("provider=%v, want the provider-neutral process adapter", p)
	}
}

// TestDecisionProviderFactoryFailsClosed proves unknown or misconfigured
// providers fail closed and that no provider name is granted authority: every
// unknown name — including names of specific engines — is rejected by the same
// path, so there is no provider-name policy branch.
func TestDecisionProviderFactoryFailsClosed(t *testing.T) {
	for _, name := range []string{"skynet", "clef", "ollama", "nimble", "systemone"} {
		cfg := config.Default()
		cfg.Decision.Enabled = true
		cfg.Decision.Provider = name
		if p, err := decisionProviderFromConfig(cfg); err == nil || p != nil {
			t.Errorf("provider %q must fail closed (no provider, an error): p=%v err=%v", name, p, err)
		}
	}
	cfg := config.Default()
	cfg.Decision.Enabled = true
	cfg.Decision.Provider = "command"
	cfg.Decision.Command = nil
	if p, err := decisionProviderFromConfig(cfg); err == nil || p != nil {
		t.Fatalf("provider command without an executable must fail closed: p=%v err=%v", p, err)
	}
}
