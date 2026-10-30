package model

import (
	"fmt"
	"strings"
)

// LocalitySource is the closed, typed provenance of a resolved locality: which
// kind of input supplied the local/cloud decision. It is non-secret and never
// model-generated prose, so persisted evidence stays auditable.
type LocalitySource string

const (
	// LocalitySourceExplicit: an operator set the locality in configuration (the
	// models: block or a SOP_MODEL_* environment variable).
	LocalitySourceExplicit LocalitySource = "explicit"
	// LocalitySourceInferred: only a reliable model-name inference supplied the
	// locality (see InferLocality).
	LocalitySourceInferred LocalitySource = "inferred"
	// LocalitySourceRuntime: authoritative runtime evidence supplied by the caller
	// (see Inputs.LocalityEvidence) supplied the locality. The model package never
	// probes a runtime itself.
	LocalitySourceRuntime LocalitySource = "runtime"
	// LocalitySourceDefault: no layer, evidence, or inference supplied the locality;
	// the built-in default applied.
	LocalitySourceDefault LocalitySource = "default"
)

// Valid reports whether s is one of the defined locality sources.
func (s LocalitySource) Valid() bool {
	switch s {
	case LocalitySourceExplicit, LocalitySourceInferred, LocalitySourceRuntime, LocalitySourceDefault:
		return true
	default:
		return false
	}
}

// String renders the source; an empty value renders as empty so an older persisted
// record without a locality source re-encodes as omitted.
func (s LocalitySource) String() string { return string(s) }

// cloudSuffix is the exact model-name suffix that marks a cloud model for the
// ollama provider. Only this exact suffix is reliable; a name that merely contains
// "cloud" is not.
const cloudSuffix = ":cloud"

// InferLocality infers a model's locality from its provider and name, returning
// ok=false when no reliable inference applies. It returns cloud only for the
// ollama provider and a name ending in the exact ":cloud" suffix. Every other
// provider, and every name without that suffix (including names that merely
// contain "cloud", Claude names, and arbitrary custom-provider models), returns
// ok=false. It is pure: it does no I/O.
func InferLocality(provider, name string) (Locality, bool) {
	if strings.TrimSpace(provider) != "ollama" {
		return "", false
	}
	if strings.HasSuffix(strings.TrimSpace(name), cloudSuffix) {
		return LocalityCloud, true
	}
	return "", false
}

// LocalityRole names which of a class's two model slots a locality reference
// describes. It drives the exact settings an error message names.
type LocalityRole string

const (
	// LocalityRolePrimary is a class's primary model.
	LocalityRolePrimary LocalityRole = "primary"
	// LocalityRoleFallback is a class's configured fallback model.
	LocalityRoleFallback LocalityRole = "fallback"
)

// LocalityRef identifies a provider/model reference whose locality must be
// resolved. Class and Role let an error name the exact settings to set.
type LocalityRef struct {
	Class    Class
	Role     LocalityRole
	Provider string
	Model    string
}

// ModelSetting returns the exact configuration key that sets this reference's
// locality: models.<class>.locality for a primary, models.<class>.fallback.locality
// for a fallback.
func (r LocalityRef) ModelSetting() string {
	if r.Role == LocalityRoleFallback {
		return "models." + string(r.Class) + ".fallback.locality"
	}
	return "models." + string(r.Class) + ".locality"
}

// EnvSetting returns the exact environment variable that sets this reference's
// locality: SOP_MODEL_<CLASS>_LOCALITY for a primary,
// SOP_MODEL_<CLASS>_FALLBACK_LOCALITY for a fallback.
func (r LocalityRef) EnvSetting() string {
	if r.Role == LocalityRoleFallback {
		return ClassFallbackEnvKey(r.Class, FallbackFieldLocality)
	}
	return ClassEnvKey(r.Class, fieldLocality)
}

// String renders the reference for an error message.
func (r LocalityRef) String() string {
	return fmt.Sprintf("class %q %s provider %q model %q (set %s or %s)", r.Class, r.Role, r.Provider, r.Model, r.ModelSetting(), r.EnvSetting())
}

// LocalityUnknownError lists every reference whose locality could not be
// resolved: the class, role (primary or fallback), provider, model, and the exact
// settings to set. It is returned rather than guessing a default.
type LocalityUnknownError struct {
	Refs []LocalityRef
}

// Error lists each unresolved reference with its class, role, provider, model,
// and the exact models.<class>...locality / SOP_MODEL_<CLASS>..._LOCALITY settings.
func (e *LocalityUnknownError) Error() string {
	if e == nil || len(e.Refs) == 0 {
		return "model locality: unresolved locality"
	}
	lines := make([]string, 0, len(e.Refs))
	for _, r := range e.Refs {
		lines = append(lines, r.String())
	}
	return "model locality: cannot resolve locality for " + strings.Join(lines, "; ")
}

// LocalityConflictError reports a fail-closed disagreement between two locality
// inputs for one reference. From and To name the sources, and Settings names the
// exact settings an operator should set.
type LocalityConflictError struct {
	Ref      LocalityRef
	Explicit Locality
	Runtime  Locality
	Inferred Locality
	Message  string
}

// Error renders the conflict with the class, provider, model, and exact setting.
func (e *LocalityConflictError) Error() string {
	return fmt.Sprintf("model locality: %s for %s", e.Message, e.Ref.String())
}

// LocalityResolution is the outcome of ResolveLocality: the resolved locality and
// the source that supplied it.
type LocalityResolution struct {
	Locality Locality
	Source   LocalitySource
}

// evidenceKey keys the locality evidence map by provider and model.
type evidenceKey struct {
	provider string
	model    string
}

// localityEvidenceMap indexes caller-supplied runtime evidence by provider and
// model, trims keys, and ignores an empty provider or model.
func localityEvidenceMap(evidence map[LocalityEvidence]Locality) map[evidenceKey]Locality {
	if len(evidence) == 0 {
		return nil
	}
	out := make(map[evidenceKey]Locality, len(evidence))
	for k, v := range evidence {
		if !v.Valid() {
			continue
		}
		provider := strings.TrimSpace(k.Provider)
		model := strings.TrimSpace(k.Model)
		if provider == "" || model == "" {
			continue
		}
		out[evidenceKey{provider: provider, model: model}] = v
	}
	return out
}

// LocalityEvidence keys a runtime locality observation by provider and model. It
// is supplied by the caller; the model package never probes a runtime.
type LocalityEvidence struct {
	Provider string
	Model    string
}

// ResolveLocality resolves one reference's locality in this order: explicit
// configuration, authoritative runtime evidence, reliable naming inference, then
// the built-in default (when default applies). It fails closed: an explicit value
// that contradicts non-empty runtime evidence or a :cloud inference, and runtime
// evidence that contradicts inference, return a *LocalityConflictError. When no
// source resolves and no default applies, it returns a *LocalityUnknownError.
//
// Parameters:
//   - explicit is the operator-configured locality (empty when unset).
//   - evidence is the caller-supplied runtime evidence map (may be nil).
//   - useDefault selects whether the built-in default may apply. A partially
//     configured class passes false so a default never masquerades as operator
//     configuration; the built-in route's own class values pass true.
//   - def is the built-in default locality for this reference (used only by
//     inference-free classes; usually empty).
func ResolveLocality(ref LocalityRef, explicit Locality, evidence map[LocalityEvidence]Locality, useDefault bool, def Locality) (LocalityResolution, error) {
	idx := localityEvidenceMap(evidence)
	var runtimeLoc Locality
	if idx != nil {
		runtimeLoc = idx[evidenceKey{provider: strings.TrimSpace(ref.Provider), model: strings.TrimSpace(ref.Model)}]
	}
	inferredLoc, inferredOK := InferLocality(ref.Provider, ref.Model)

	// Fail-closed cross-check first: runtime evidence and inference must agree.
	if runtimeLoc != "" && inferredOK && runtimeLoc != inferredLoc {
		return LocalityResolution{}, &LocalityConflictError{
			Ref:      ref,
			Runtime:  runtimeLoc,
			Inferred: inferredLoc,
			Message:  fmt.Sprintf("runtime evidence says %q but the model name infers %q", runtimeLoc, inferredLoc),
		}
	}

	// 1. Explicit configuration.
	if explicit != "" {
		if runtimeLoc != "" && runtimeLoc != explicit {
			return LocalityResolution{}, &LocalityConflictError{
				Ref:      ref,
				Explicit: explicit,
				Runtime:  runtimeLoc,
				Message:  fmt.Sprintf("explicit locality %q contradicts runtime evidence %q", explicit, runtimeLoc),
			}
		}
		if inferredOK && inferredLoc != explicit {
			return LocalityResolution{}, &LocalityConflictError{
				Ref:      ref,
				Explicit: explicit,
				Inferred: inferredLoc,
				Message:  fmt.Sprintf("explicit locality %q contradicts the model-name inference %q", explicit, inferredLoc),
			}
		}
		return LocalityResolution{Locality: explicit, Source: LocalitySourceExplicit}, nil
	}

	// 2. Authoritative runtime evidence.
	if runtimeLoc != "" {
		return LocalityResolution{Locality: runtimeLoc, Source: LocalitySourceRuntime}, nil
	}

	// 3. Reliable naming inference.
	if inferredOK {
		return LocalityResolution{Locality: inferredLoc, Source: LocalitySourceInferred}, nil
	}

	// 4. Built-in default.
	if useDefault && def != "" {
		return LocalityResolution{Locality: def, Source: LocalitySourceDefault}, nil
	}
	if useDefault && ref.Class != "" {
		if d := DefaultRoute().classConfig(ref.Class); ref.Role == LocalityRolePrimary && d.Locality != "" {
			return LocalityResolution{Locality: d.Locality, Source: LocalitySourceDefault}, nil
		}
	}

	return LocalityResolution{}, &LocalityUnknownError{Refs: []LocalityRef{ref}}
}
