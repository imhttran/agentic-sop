package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/internal/provider"
	"github.com/imhttran/agentic-sop/internal/provider/command"
)

func TestCommandProviderLimits(t *testing.T) {
	p := command.New()
	if p.ID() != provider.Command {
		t.Fatalf("ID = %q", p.ID())
	}
	if h := p.Health(context.Background()); h.Status != provider.HealthUnknown {
		t.Fatalf("health = %+v, want unknown", h)
	}
	if _, err := p.Models(context.Background()); !errors.Is(err, provider.ErrDiscoveryUnsupported) {
		t.Fatalf("Models err = %v, want ErrDiscoveryUnsupported", err)
	}
	caps, err := p.Capabilities(context.Background(), "anything")
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}
	if caps.Chat.Known() {
		t.Fatalf("command capabilities must be undetermined, got %v", caps.Chat)
	}
}
