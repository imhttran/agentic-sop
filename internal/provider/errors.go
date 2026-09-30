package provider

import "errors"

// Sentinel errors for the provider boundary. Callers use errors.Is to distinguish
// "the model is definitely absent" (ErrModelNotFound) from "the provider cannot
// tell" (ErrDiscoveryUnsupported), which MUST NOT be treated the same way.
var (
	// ErrUnknownProvider: the requested provider name is not one SOP supports.
	ErrUnknownProvider = errors.New("unknown provider")
	// ErrNotRegistered: the provider is known but none is registered for it in
	// this registry.
	ErrNotRegistered = errors.New("provider not registered")
	// ErrDuplicateProvider: two providers were registered under one id.
	ErrDuplicateProvider = errors.New("duplicate provider registration")
	// ErrModelNotFound: the provider enumerated its models and the requested one
	// was absent. This is authoritative absence.
	ErrModelNotFound = errors.New("model not found")
	// ErrDiscoveryUnsupported: the provider cannot enumerate models, so absence
	// is unknown rather than established.
	ErrDiscoveryUnsupported = errors.New("provider cannot enumerate models")
	// ErrProviderUnavailable: the provider is known but unreachable.
	ErrProviderUnavailable = errors.New("provider unavailable")
	// ErrModelRequired: a selection with no model cannot be validated.
	ErrModelRequired = errors.New("model is required")
)
