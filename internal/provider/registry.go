package provider

import (
	"fmt"
	"sort"
)

// Registry holds the providers available to a run. It is constructed and passed
// explicitly (the composition root builds it) rather than living in a
// package-level variable, so tests are isolated and startup is deterministic.
//
// A Registry is read-only after construction: it never selects a model class,
// never executes anything, and never touches SOP state.
type Registry struct {
	providers map[ID]Provider
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{providers: map[ID]Provider{}}
}

// Register adds a provider. Registering the same id twice is an error rather than
// a silent overwrite, and a provider whose id is unknown or empty is rejected, so
// a typo is caught at construction.
func (r *Registry) Register(p Provider) error {
	if p == nil {
		return fmt.Errorf("provider: cannot register a nil provider")
	}
	id := p.ID()
	if !id.Valid() {
		return fmt.Errorf("provider: cannot register %w %q (want %s)", ErrUnknownProvider, id, joinIDs(knownIDs))
	}
	if _, exists := r.providers[id]; exists {
		return fmt.Errorf("provider: %w for %q", ErrDuplicateProvider, id)
	}
	r.providers[id] = p
	return nil
}

// Get returns the provider registered under id. An unknown or unregistered id is
// an error that names what is available.
func (r *Registry) Get(id ID) (Provider, error) {
	if !id.Valid() {
		return nil, fmt.Errorf("provider: %w %q (want %s)", ErrUnknownProvider, id, joinIDs(knownIDs))
	}
	p, ok := r.providers[id]
	if !ok {
		return nil, fmt.Errorf("provider: %w %q (registered: %s)", ErrNotRegistered, id, joinIDs(r.IDs()))
	}
	return p, nil
}

// IDs returns the registered provider ids in canonical order.
func (r *Registry) IDs() []ID {
	ids := make([]ID, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// List returns the registered providers sorted by id, so iteration order is
// deterministic regardless of registration order.
func (r *Registry) List() []Provider {
	ids := r.IDs()
	out := make([]Provider, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.providers[id])
	}
	return out
}

// Len returns the number of registered providers.
func (r *Registry) Len() int { return len(r.providers) }
