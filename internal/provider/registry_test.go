package provider_test

import (
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/internal/provider"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := provider.NewRegistry()
	p := &stubProvider{id: provider.Ollama}
	if err := reg.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := reg.Get(provider.Ollama)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != provider.Provider(p) {
		t.Fatal("Get returned a different provider")
	}
}

func TestRegistryDuplicateFails(t *testing.T) {
	reg := provider.NewRegistry()
	if err := reg.Register(&stubProvider{id: provider.Ollama}); err != nil {
		t.Fatal(err)
	}
	err := reg.Register(&stubProvider{id: provider.Ollama})
	if !errors.Is(err, provider.ErrDuplicateProvider) {
		t.Fatalf("duplicate Register err = %v, want ErrDuplicateProvider", err)
	}
}

func TestRegistryRejectsUnknownAndNil(t *testing.T) {
	reg := provider.NewRegistry()
	if err := reg.Register(&stubProvider{id: "vllm"}); !errors.Is(err, provider.ErrUnknownProvider) {
		t.Fatalf("unknown-id Register err = %v, want ErrUnknownProvider", err)
	}
	if err := reg.Register(nil); err == nil {
		t.Fatal("Register(nil) must fail")
	}
}

func TestRegistryGetErrors(t *testing.T) {
	reg := provider.NewRegistry()
	if _, err := reg.Get(provider.Ollama); !errors.Is(err, provider.ErrNotRegistered) {
		t.Fatalf("unregistered Get err = %v, want ErrNotRegistered", err)
	}
	if _, err := reg.Get("vllm"); !errors.Is(err, provider.ErrUnknownProvider) {
		t.Fatalf("unknown-id Get err = %v, want ErrUnknownProvider", err)
	}
}

func TestRegistryListDeterministic(t *testing.T) {
	// Register out of canonical order; List must still be sorted by id.
	reg := provider.NewRegistry()
	for _, id := range []provider.ID{provider.MLX, provider.Ollama, provider.Command} {
		if err := reg.Register(&stubProvider{id: id}); err != nil {
			t.Fatal(err)
		}
	}
	want := []provider.ID{provider.Command, provider.MLX, provider.Ollama}
	got := reg.IDs()
	if len(got) != len(want) {
		t.Fatalf("IDs = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("IDs = %v, want %v", got, want)
		}
	}
	if list := reg.List(); len(list) != len(want) || list[0].ID() != provider.Command {
		t.Fatalf("List = %v", list)
	}
	if reg.Len() != 3 {
		t.Fatalf("Len = %d", reg.Len())
	}
}
