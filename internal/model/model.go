// Package model resolves SOP's optional model-routing layer: which model (and
// provider) a run uses, selected by a small/medium/large class from a layered
// configuration.
//
// Precedence, highest first:
//
//  1. a CLI override (the `--model-class` flag),
//  2. environment variables (SOP_MODEL_*), which a .env file may supply,
//  3. the `models:` block in .agent-sdlc/config.yaml,
//  4. built-in defaults (see DefaultRoute).
//
// The layer is opt-in. Without any `models:` configuration, any SOP_MODEL_*
// environment, or a CLI override, Resolve reports the layer inactive and callers
// keep the existing agent selection — so an existing installation is unchanged.
// The built-in defaults do not change that: they fill a class's model once
// routing is active, they never activate it.
//
// A class's provider, model, and locality are merged field by field from the
// configuration and environment layers. The built-in default for a class applies
// only when neither layer mentions that class at all, so a bare `--model-class`
// resolves to a usable model with no configuration, while a class a layer does
// mention is used as written (a partially configured class stays partial, so the
// fallback and reporting rules below still apply).
//
// Locality (local or cloud) is descriptive metadata and a fallback guard, not an
// execution mode. Two distinct fallbacks exist:
//
//   - the config-completeness fallback (fallback_class): when the chosen class has
//     no model at all, another class supplies one. A local class with no model and
//     a cloud fallback fails rather than quietly using the cloud model unless
//     allow_cloud_fallback_for_local is set.
//   - the local-first runtime fallback (a class's configured `fallback` model):
//     when a LOCAL class's primary model cannot be served by its local runtime,
//     the class's configured fallback (by default a cloud model) runs instead. It
//     is enabled by the fallback being configured, and only a caller's positive
//     observation of unavailability selects it (the CLI's applyLocalFallback). It
//     is never triggered by a generation failure, a validation failure, a review
//     finding, or a quality gate — those keep the existing retry/recovery
//     behavior. Resolve reports the candidate as Result.LocalFallback; it never
//     probes a runtime and never applies it.
//
// This package holds no secrets: a Selection records only the class, provider,
// model name, locality, and the source layer, never a credential.
package model

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Class is a model tier. It names a slot in the routing table, not a model.
type Class string

const (
	ClassSmall  Class = "small"
	ClassMedium Class = "medium"
	ClassLarge  Class = "large"
)

// Classes is the routing table's class order, used for stable iteration and
// error messages.
var Classes = []Class{ClassSmall, ClassMedium, ClassLarge}

// Valid reports whether c is a known class.
func (c Class) Valid() bool {
	switch c {
	case ClassSmall, ClassMedium, ClassLarge:
		return true
	default:
		return false
	}
}

// ParseClass parses a class value, returning an actionable error naming the
// accepted values for an unknown one.
func ParseClass(s string) (Class, error) {
	c := Class(strings.ToLower(strings.TrimSpace(s)))
	if c.Valid() {
		return c, nil
	}
	return "", fmt.Errorf("unknown model class %q (want %s)", strings.TrimSpace(s), joinClasses())
}

// Locality is where a class's model runs: locally or in the cloud. It is
// descriptive (it never changes the execution mode) and guards against a silent
// local-to-cloud fallback.
type Locality string

const (
	LocalityLocal Locality = "local"
	LocalityCloud Locality = "cloud"
)

// Valid reports whether l is a known locality.
func (l Locality) Valid() bool {
	switch l {
	case LocalityLocal, LocalityCloud:
		return true
	default:
		return false
	}
}

// ParseLocality parses a locality value, returning an actionable error naming the
// accepted values for an unknown one.
func ParseLocality(s string) (Locality, error) {
	l := Locality(strings.ToLower(strings.TrimSpace(s)))
	if l.Valid() {
		return l, nil
	}
	return "", fmt.Errorf("unknown locality %q (want local, cloud)", strings.TrimSpace(s))
}

// Source records which layer supplied the resolved model selection. It is
// non-secret evidence, safe to log or display.
type Source string

const (
	// SourceCLI: a CLI override selected the class.
	SourceCLI Source = "cli"
	// SourceEnv: SOP_MODEL_* environment (including a .env file) supplied a value.
	SourceEnv Source = "env"
	// SourceConfig: the models: block in config.yaml supplied a value.
	SourceConfig Source = "config"
	// SourceDefault: no layer supplied a value; built-in defaults applied.
	SourceDefault Source = "default"
	// SourceRouter: SOP's deterministic router (internal/router) chose the class
	// from task/evidence signals, with no CLI override. It is the automatic-routing
	// source and is used only when the router's feature flag is enabled.
	SourceRouter Source = "router"
	// SourceCloudFallback: a class's configured runtime fallback supplied the
	// selection because the class's primary model runs locally and the local
	// runtime could not serve it. Unlike the other values it is not a configuration
	// layer; it records a runtime-availability outcome.
	SourceCloudFallback Source = "cloud-fallback"
)

// Reason is the deterministic, non-secret explanation of why a selection was
// made. It is one of a small set of fixed phrases — never model-generated prose,
// and never parsed to drive a decision — so the routing choice stays auditable.
const (
	// ReasonCLIClass: the --model-class flag chose the class.
	ReasonCLIClass = "explicit CLI model class"
	// ReasonEnvClass: SOP_MODEL_DEFAULT_CLASS chose the class.
	ReasonEnvClass = "environment default class"
	// ReasonConfigClass: models.default_class chose the class.
	ReasonConfigClass = "project-configured default class"
	// ReasonBuiltinClass: no layer chose a class; the built-in default applied.
	ReasonBuiltinClass = "built-in default class"
	// ReasonFallbackClass: the chosen class had no model, so the fallback class
	// supplied the selection.
	ReasonFallbackClass = "fallback class used"
	// ReasonLocalFallback: the chosen class's primary model runs locally and its
	// local runtime could not serve it, so the class's configured cloud fallback
	// supplied the selection instead.
	ReasonLocalFallback = "local runtime unavailable; cloud fallback"
)

// RoutingReasonManual is the deterministic reason recorded when a manual
// --model-class override (rather than the automatic router) chose the class. It
// is fixed, non-prose evidence.
const RoutingReasonManual = "manual model-class override"

// knownProviders is the provider allow-list a routed class may name. It mirrors
// the canonical provider identifiers (internal/provider.KnownIDs); an unknown name
// fails at resolution rather than reaching the agent constructor.
//
// internal/model cannot import internal/provider (provider imports model, so the
// reverse would be an import cycle), so the set is duplicated here and kept in
// sync by a test (provider_identity_test.go) that compares it with
// provider.KnownIDs(). That test, not this comment, is what prevents drift.
var knownProviders = map[string]bool{
	"ollama":   true,
	"llamacpp": true,
	"command":  true,
	"mlx":      true,
}

// KnownProviders returns the provider names a routed class may name, in a stable
// order. It mirrors provider.KnownIDs and exists so an external test can assert
// the two lists agree without introducing an import cycle.
func KnownProviders() []string {
	names := make([]string, 0, len(knownProviders))
	for name := range knownProviders {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ClassConfig is one class's routing in the config file (and, reused, the shape
// of the environment layer). Every field is optional: an unset field inherits
// from a lower-precedence layer.
type ClassConfig struct {
	Provider string   `yaml:"provider"`
	Name     string   `yaml:"name"`
	Locality Locality `yaml:"locality"`
	// Fallback is an alternate model for this class, consulted only when the
	// class's primary model runs locally and its local runtime cannot serve it
	// (see Resolve). It is a pointer so "no fallback configured" is
	// distinguishable from an empty one.
	Fallback *FallbackConfig `yaml:"fallback"`
}

func (cc ClassConfig) empty() bool {
	return strings.TrimSpace(cc.Provider) == "" &&
		strings.TrimSpace(cc.Name) == "" &&
		strings.TrimSpace(string(cc.Locality)) == "" &&
		cc.Fallback == nil
}

// FallbackConfig is the alternate model a class may run on when its primary model
// runs locally and the local runtime cannot serve it. An unset field inherits:
// the provider from the class's primary model, and the locality defaults to cloud
// (a fallback for a local class is a cloud model unless it says otherwise).
type FallbackConfig struct {
	Provider string   `yaml:"provider"`
	Name     string   `yaml:"name"`
	Locality Locality `yaml:"locality"`
}

func (f FallbackConfig) empty() bool {
	return strings.TrimSpace(f.Provider) == "" &&
		strings.TrimSpace(f.Name) == "" &&
		strings.TrimSpace(string(f.Locality)) == ""
}

// Route is the routing table: the default and fallback class, the local-to-cloud
// fallback policy, and the per-class models. It is the `models:` block of
// config.yaml and, reused, the parsed shape of the SOP_MODEL_* environment.
type Route struct {
	DefaultClass               Class `yaml:"default_class"`
	FallbackClass              Class `yaml:"fallback_class"`
	AllowCloudFallbackForLocal *bool `yaml:"allow_cloud_fallback_for_local"`
	// RoutingEnabled turns on the automatic model-class router (internal/router),
	// which selects a class per task from typed task/JEV evidence. It is a pointer
	// so an omitted value is distinguishable from an explicit false; both resolve
	// to disabled. It is separate from Configured(): it does not, on its own, change
	// the class table, and the router is OFF by default so an existing project is
	// unchanged. SOP_MODEL_ROUTING_ENABLED overrides it.
	RoutingEnabled *bool `yaml:"routing_enabled"`
	// EscalationEnabled turns on the bounded execution-recovery escalation policy
	// (Phase 5, internal/recovery): after an attempt fails a quality gate, SOP may
	// retry the task on the next larger class. It is a pointer so an omitted value
	// is distinguishable from an explicit false; both resolve to disabled. Like
	// RoutingEnabled it does NOT make the route table Configured() — enabling
	// escalation alone neither activates a class table nor changes the agent
	// selection, and it is OFF by default so an existing project is unchanged.
	// SOP_MODEL_ESCALATION_ENABLED overrides it.
	EscalationEnabled *bool `yaml:"escalation_enabled"`
	// MaxEscalations bounds automatic escalation (Phase 5). It is only consulted
	// when EscalationEnabled resolves true. It is a pointer so an omitted value is
	// distinguishable from an explicit 0: an omitted value resolves to
	// DefaultMaxEscalations, while an explicit 0 disables escalation.
	// SOP_MODEL_MAX_ESCALATIONS overrides it.
	MaxEscalations *int        `yaml:"max_escalations"`
	Small          ClassConfig `yaml:"small"`
	Medium         ClassConfig `yaml:"medium"`
	Large          ClassConfig `yaml:"large"`
}

// DefaultRoute is the built-in routing table: the default class, and the
// provider/model/locality each class uses when neither the config nor the
// environment names it. It is the base layer of resolution and the single owner
// of these defaults — CLI, config, and provider code never restate them.
//
// `small` is local-first with a cloud runtime fallback: its primary model is a
// local Ollama model, and its configured fallback (`nemotron-3-nano:30b-cloud`)
// runs only when the local runtime cannot serve the primary (see Resolve).
//
// Defining complete built-in routes does NOT activate routing: Resolve is active
// only when an explicit layer (the `models:` block, a SOP_MODEL_* variable, or
// --model-class) is present, so an existing installation's agent selection is
// unchanged. FallbackClass is intentionally left empty: the fallback class
// defaults to the selected class unless a layer names one.
func DefaultRoute() Route {
	return Route{
		DefaultClass: ClassMedium,
		Small: ClassConfig{
			Provider: "ollama", Name: "qwen3:4b", Locality: LocalityLocal,
			Fallback: &FallbackConfig{Provider: "ollama", Name: "nemotron-3-nano:30b-cloud", Locality: LocalityCloud},
		},
		Medium: ClassConfig{Provider: "ollama", Name: "nemotron-3-super:cloud", Locality: LocalityCloud},
		Large:  ClassConfig{Provider: "ollama", Name: "deepseek-v4.1-flash:cloud", Locality: LocalityCloud},
	}
}

// classConfig returns the entry for a class.
func (r Route) classConfig(c Class) ClassConfig {
	switch c {
	case ClassSmall:
		return r.Small
	case ClassMedium:
		return r.Medium
	case ClassLarge:
		return r.Large
	default:
		return ClassConfig{}
	}
}

// setClassConfig stores an entry for a class.
func (r *Route) setClassConfig(c Class, cc ClassConfig) {
	switch c {
	case ClassSmall:
		r.Small = cc
	case ClassMedium:
		r.Medium = cc
	case ClassLarge:
		r.Large = cc
	}
}

// Configured reports whether any routing value was set: a default or fallback
// class, the fallback policy, or any class model.
func (r Route) Configured() bool {
	if r.DefaultClass != "" || r.FallbackClass != "" || r.AllowCloudFallbackForLocal != nil {
		return true
	}
	for _, c := range Classes {
		if !r.classConfig(c).empty() {
			return true
		}
	}
	return false
}

// Validate rejects an unknown class, locality, or provider with a focused error.
// An omitted (empty) value is valid: it means "inherit".
func (r Route) Validate() error {
	if r.DefaultClass != "" && !r.DefaultClass.Valid() {
		return fmt.Errorf("model routing: unknown default_class %q (want %s)", r.DefaultClass, joinClasses())
	}
	if r.FallbackClass != "" && !r.FallbackClass.Valid() {
		return fmt.Errorf("model routing: unknown fallback_class %q (want %s)", r.FallbackClass, joinClasses())
	}
	for _, c := range Classes {
		cc := r.classConfig(c)
		if loc := strings.TrimSpace(string(cc.Locality)); loc != "" && !cc.Locality.Valid() {
			return fmt.Errorf("model routing: unknown %s.locality %q (want local, cloud)", c, loc)
		}
		if provider := strings.TrimSpace(cc.Provider); provider != "" && !knownProviders[provider] {
			return fmt.Errorf("model routing: unknown %s.provider %q (want %s)", c, provider, joinProviders())
		}
		if fb := cc.Fallback; fb != nil {
			if loc := strings.TrimSpace(string(fb.Locality)); loc != "" && !fb.Locality.Valid() {
				return fmt.Errorf("model routing: unknown %s.fallback.locality %q (want local, cloud)", c, loc)
			}
			if provider := strings.TrimSpace(fb.Provider); provider != "" && !knownProviders[provider] {
				return fmt.Errorf("model routing: unknown %s.fallback.provider %q (want %s)", c, provider, joinProviders())
			}
		}
	}
	if r.MaxEscalations != nil && *r.MaxEscalations < 0 {
		return fmt.Errorf("model escalation: max_escalations must not be negative (got %d)", *r.MaxEscalations)
	}
	return nil
}

// Environment variable names for the routing layer.
const (
	// EnvDefaultClass overrides models.default_class.
	EnvDefaultClass = "SOP_MODEL_DEFAULT_CLASS"
	// EnvFallbackClass overrides models.fallback_class.
	EnvFallbackClass = "SOP_MODEL_FALLBACK_CLASS"
	// EnvAllowCloudFallbackForLocal overrides models.allow_cloud_fallback_for_local.
	EnvAllowCloudFallbackForLocal = "SOP_MODEL_ALLOW_CLOUD_FALLBACK_FOR_LOCAL"
	// EnvRoutingEnabled overrides models.routing_enabled (the automatic
	// model-class router). It defaults to false.
	EnvRoutingEnabled = "SOP_MODEL_ROUTING_ENABLED"
	// EnvEscalationEnabled overrides models.escalation_enabled (the bounded
	// execution-recovery escalation policy). It defaults to false.
	EnvEscalationEnabled = "SOP_MODEL_ESCALATION_ENABLED"
	// EnvMaxEscalations overrides models.max_escalations. It defaults to
	// DefaultMaxEscalations.
	EnvMaxEscalations = "SOP_MODEL_MAX_ESCALATIONS"
)

// DefaultMaxEscalations is the built-in bound on automatic escalation (Phase 5):
// small -> medium -> large is exactly two escalations, after which the existing
// human/terminal boundary applies.
const DefaultMaxEscalations = 2

// Class env field names.
const (
	fieldProvider = "PROVIDER"
	fieldName     = "NAME"
	fieldLocality = "LOCALITY"
)

// ClassEnvKey returns the environment variable naming a class field, for example
// SOP_MODEL_MEDIUM_NAME. It is exported so error messages and documentation can
// name the exact variable to set.
func ClassEnvKey(c Class, field string) string {
	return "SOP_MODEL_" + strings.ToUpper(string(c)) + "_" + strings.ToUpper(field)
}

// ClassFallbackEnvKey returns the environment variable naming a class's fallback
// field, for example SOP_MODEL_SMALL_FALLBACK_NAME. It is exported for the same
// reason as ClassEnvKey.
func ClassFallbackEnvKey(c Class, field string) string {
	return ClassEnvKey(c, "FALLBACK_"+field)
}

// FallbackFieldProvider / FallbackFieldName / FallbackFieldLocality name the
// fallback fields for ClassFallbackEnvKey, so error messages and tests agree with
// the parser.
const (
	FallbackFieldProvider = "PROVIDER"
	FallbackFieldName     = "NAME"
	FallbackFieldLocality = "LOCALITY"
)

// routeFromEnv parses the SOP_MODEL_* environment into a Route, returning an
// actionable error for an unknown class, locality, or boolean.
func routeFromEnv(lookup func(string) string) (Route, error) {
	get := func(key string) string { return strings.TrimSpace(lookup(key)) }

	var r Route
	if v := get(EnvDefaultClass); v != "" {
		c, err := ParseClass(v)
		if err != nil {
			return Route{}, fmt.Errorf("model routing: %s: %w", EnvDefaultClass, err)
		}
		r.DefaultClass = c
	}
	if v := get(EnvFallbackClass); v != "" {
		c, err := ParseClass(v)
		if err != nil {
			return Route{}, fmt.Errorf("model routing: %s: %w", EnvFallbackClass, err)
		}
		r.FallbackClass = c
	}
	if v := get(EnvAllowCloudFallbackForLocal); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Route{}, fmt.Errorf("model routing: %s: invalid boolean %q (want true or false)", EnvAllowCloudFallbackForLocal, v)
		}
		r.AllowCloudFallbackForLocal = &b
	}
	if v := get(EnvRoutingEnabled); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Route{}, fmt.Errorf("model routing: %s: invalid boolean %q (want true or false)", EnvRoutingEnabled, v)
		}
		r.RoutingEnabled = &b
	}

	for _, c := range Classes {
		var cc ClassConfig
		if v := get(ClassEnvKey(c, fieldProvider)); v != "" {
			if !knownProviders[v] {
				return Route{}, fmt.Errorf("model routing: %s: unknown provider %q (want %s)", ClassEnvKey(c, fieldProvider), v, joinProviders())
			}
			cc.Provider = v
		}
		cc.Name = get(ClassEnvKey(c, fieldName))
		if v := get(ClassEnvKey(c, fieldLocality)); v != "" {
			loc, err := ParseLocality(v)
			if err != nil {
				return Route{}, fmt.Errorf("model routing: %s: %w", ClassEnvKey(c, fieldLocality), err)
			}
			cc.Locality = loc
		}
		var fb FallbackConfig
		if v := get(ClassFallbackEnvKey(c, FallbackFieldProvider)); v != "" {
			if !knownProviders[v] {
				return Route{}, fmt.Errorf("model routing: %s: unknown provider %q (want %s)", ClassFallbackEnvKey(c, FallbackFieldProvider), v, joinProviders())
			}
			fb.Provider = v
		}
		fb.Name = get(ClassFallbackEnvKey(c, FallbackFieldName))
		if v := get(ClassFallbackEnvKey(c, FallbackFieldLocality)); v != "" {
			loc, err := ParseLocality(v)
			if err != nil {
				return Route{}, fmt.Errorf("model routing: %s: %w", ClassFallbackEnvKey(c, FallbackFieldLocality), err)
			}
			fb.Locality = loc
		}
		if !fb.empty() {
			cc.Fallback = &fb
		}
		if !cc.empty() {
			r.setClassConfig(c, cc)
		}
	}
	return r, nil
}

// Inputs are the layers Resolve reads. Config is the models: block from
// config.yaml; Lookup reads the environment (nil means os.Getenv) and is where a
// loaded .env file's values appear; CLIClass is the --model-class override.
type Inputs struct {
	Config   Route
	Lookup   func(string) string
	CLIClass string
	// RoutedClass is the class selected by SOP's deterministic router (Phase 3.5)
	// when automatic routing is enabled. It is used only when no CLI override is
	// present, so a manual --model-class always wins. Its zero value means "no
	// routed class" and leaves the existing class selection unchanged.
	RoutedClass Class
	// RoutedReason is the deterministic explanation for RoutedClass. It is one of
	// the router's fixed reason phrases, never model-generated prose.
	RoutedReason string
}

// Selection is the resolved model selection: the class, the provider and model
// it names, the locality, the layer that supplied it, whether a fallback
// supplied it, and why. It carries no credential, so it is safe to persist as
// non-secret routing evidence.
type Selection struct {
	Class    Class    `json:"class"`
	Provider string   `json:"provider"`
	Model    string   `json:"model"`
	Locality Locality `json:"locality"`
	Source   Source   `json:"source"`
	// Fallback reports whether a fallback supplied this selection rather than the
	// primary model: either the config-completeness fallback class
	// (ReasonFallbackClass), or a class's configured runtime cloud fallback
	// (ReasonLocalFallback).
	Fallback bool `json:"fallback"`
	// Reason is the deterministic explanation of the selection (see the Reason*
	// constants). It is a fixed phrase, never model prose.
	Reason string `json:"reason"`
}

// Result is the outcome of resolution. Active is false when no routing layer was
// configured, in which case Selection is the zero value and the caller must keep
// its existing agent selection.
type Result struct {
	Selection Selection
	Active    bool
	// LocalFallback is the selected class's configured runtime fallback, present
	// only when Selection is a LOCAL class with a complete fallback configured. It
	// is a CANDIDATE, not a decision: Resolve never probes a runtime and never
	// applies it. The caller applies it only on a positive observation that the
	// local runtime cannot serve Selection (see the CLI's applyLocalFallback), so
	// the fallback is never triggered by a generation, validation, or gate
	// failure.
	LocalFallback *Selection
}

// Resolve applies the routing layers and returns the selected model. It is pure
// with respect to SOP state — it reads configuration and environment only.
func Resolve(in Inputs) (Result, error) {
	lookup := in.Lookup
	if lookup == nil {
		lookup = os.Getenv
	}

	if err := in.Config.Validate(); err != nil {
		return Result{}, err
	}
	env, err := routeFromEnv(lookup)
	if err != nil {
		return Result{}, err
	}

	cli := strings.TrimSpace(in.CLIClass)
	if !in.Config.Configured() && !env.Configured() && cli == "" && in.RoutedClass == "" {
		return Result{Active: false}, nil
	}

	class, reason, err := selectClass(DefaultRoute(), in.Config, env, cli, in.RoutedClass, in.RoutedReason)
	if err != nil {
		return Result{}, err
	}

	primary := entryFor(class, in.Config, env)
	if primary.complete() {
		sel := selectionFor(class, primary, sourceFor(class, in.Config, env, cli, in.RoutedClass), reason, false)
		// The local-first runtime fallback is a CANDIDATE only: it is reported here
		// and applied by the caller, never by Resolve.
		if fb, ok := localFallback(class, in.Config, env, primary); ok {
			return Result{Active: true, Selection: sel, LocalFallback: &fb}, nil
		}
		return Result{Active: true, Selection: sel}, nil
	}

	// Fallback: SOP_MODEL_FALLBACK_CLASS > models.fallback_class > the selected class.
	fallback := class
	switch {
	case env.FallbackClass != "":
		fallback = env.FallbackClass
	case in.Config.FallbackClass != "":
		fallback = in.Config.FallbackClass
	}
	if fallback != class {
		fb := entryFor(fallback, in.Config, env)
		if fb.complete() {
			if primary.locality == LocalityLocal && fb.locality == LocalityCloud && !allowCloudFallback(in.Config, env) {
				return Result{}, fmt.Errorf("model routing: class %q is local but has no model; refusing to fall back to the cloud model %q for class %q (set %s=true to allow)",
					class, fb.model, fallback, EnvAllowCloudFallbackForLocal)
			}
			return Result{Active: true, Selection: selectionFor(fallback, fb, sourceFor(fallback, in.Config, env, "", ""), ReasonFallbackClass, true)}, nil
		}
	}

	return Result{}, fmt.Errorf("model routing: class %q has no complete model (set %s, %s, and %s, or the models.%s block in config.yaml)",
		class, ClassEnvKey(class, fieldProvider), ClassEnvKey(class, fieldName), ClassEnvKey(class, fieldLocality), class)
}

// selectClass picks the class to route and the deterministic reason it was
// chosen: an explicit CLI class, SOP's routed class (automatic routing), an
// environment default class, a project-configured default class, or the built-in
// default. It is the single owner of the class-selection rule.
//
// Precedence: a manual --model-class override always wins over the router, so an
// operator's explicit choice is never silently replaced by automatic routing.
func selectClass(defaults Route, config, env Route, cli string, routed Class, routedReason string) (Class, string, error) {
	switch {
	case cli != "":
		c, err := ParseClass(cli)
		if err != nil {
			return "", "", fmt.Errorf("model routing: --model-class: %w", err)
		}
		return c, ReasonCLIClass, nil
	case routed != "":
		if !routed.Valid() {
			return "", "", fmt.Errorf("model routing: routed class %q is not a known class (want %s)", routed, joinClasses())
		}
		return routed, routedReason, nil
	case env.DefaultClass != "":
		return env.DefaultClass, ReasonEnvClass, nil
	case config.DefaultClass != "":
		return config.DefaultClass, ReasonConfigClass, nil
	default:
		return defaults.DefaultClass, ReasonBuiltinClass, nil
	}
}

// entry is a class's merged routing: per-field, the environment layer over the
// config layer, falling back to the built-in default when neither names the class.
type entry struct {
	provider   string
	model      string
	locality   Locality
	nameSource Source
}

// complete reports whether an entry fully names a usable model.
func (e entry) complete() bool {
	return e.provider != "" && e.model != "" && e.locality != ""
}

// entryFor merges the routing layers for a class. The config and environment
// entries merge field by field (the environment winning). A class that neither
// layer mentions falls back to its built-in default (DefaultRoute), so a bare
// --model-class resolves to a usable model with no configuration; a class a
// layer does mention is used as written.
func entryFor(c Class, config, env Route) entry {
	var e entry
	if cc := config.classConfig(c); !cc.empty() {
		applyClassConfig(&e, cc, SourceConfig)
	}
	if cc := env.classConfig(c); !cc.empty() {
		applyClassConfig(&e, cc, SourceEnv)
	}
	if e.provider == "" && e.model == "" && e.locality == "" {
		applyClassConfig(&e, DefaultRoute().classConfig(c), SourceDefault)
	}
	return e
}

// applyClassConfig overlays a class config onto an entry, field by field, and
// records the layer that supplied the model name.
func applyClassConfig(e *entry, cc ClassConfig, source Source) {
	if v := strings.TrimSpace(cc.Provider); v != "" {
		e.provider = v
	}
	if v := strings.TrimSpace(cc.Name); v != "" {
		e.model = v
		e.nameSource = source
	}
	if cc.Locality != "" {
		e.locality = cc.Locality
	}
}

// localFallback returns the class's configured runtime fallback selection, or
// ok=false when none applies. It applies only to a LOCAL class with a complete
// fallback model; a cloud class never has one, so a cloud selection is never a
// candidate for the runtime fallback.
func localFallback(c Class, config, env Route, primary entry) (Selection, bool) {
	if primary.locality != LocalityLocal {
		return Selection{}, false
	}
	fb, ok := fallbackEntry(c, config, env, primary)
	if !ok {
		return Selection{}, false
	}
	return selectionFor(c, fb, SourceCloudFallback, ReasonLocalFallback, true), true
}

// fallbackEntry merges a class's fallback config, field by field, from the
// built-in default (base), then the configuration, then the environment (which
// wins). The built-in default is the base layer for the same reason it is the
// base layer of the primary: an operator who names a local model still gets the
// built-in cloud fallback unless they override it.
//
// A fallback with no model is not configured, so ok=false. An unset provider
// inherits the class's primary provider, and an unset locality defaults to cloud.
func fallbackEntry(c Class, config, env Route, primary entry) (entry, bool) {
	var e entry
	if fb := DefaultRoute().classConfig(c).Fallback; fb != nil {
		applyFallback(&e, *fb)
	}
	if fb := config.classConfig(c).Fallback; fb != nil {
		applyFallback(&e, *fb)
	}
	if fb := env.classConfig(c).Fallback; fb != nil {
		applyFallback(&e, *fb)
	}
	if e.model == "" {
		return entry{}, false
	}
	if e.provider == "" {
		e.provider = primary.provider
	}
	if e.locality == "" {
		e.locality = LocalityCloud
	}
	if !e.complete() {
		return entry{}, false
	}
	return e, true
}

// applyFallback overlays a fallback config onto an entry, field by field.
func applyFallback(e *entry, f FallbackConfig) {
	if v := strings.TrimSpace(f.Provider); v != "" {
		e.provider = v
	}
	if v := strings.TrimSpace(f.Name); v != "" {
		e.model = v
	}
	if f.Locality != "" {
		e.locality = f.Locality
	}
}

// selectionFor builds a Selection from a merged entry.
func selectionFor(c Class, e entry, source Source, reason string, fallback bool) Selection {
	return Selection{Class: c, Provider: e.provider, Model: e.model, Locality: e.locality, Source: source, Fallback: fallback, Reason: reason}
}

// sourceFor names the layer that supplied the selection: the CLI override when it
// chose the class, the router when it chose the class, otherwise the layer that
// supplied the model name, otherwise the built-in default.
func sourceFor(c Class, config, env Route, cli string, routed Class) Source {
	if strings.TrimSpace(cli) != "" {
		return SourceCLI
	}
	if routed != "" {
		return SourceRouter
	}
	if s := entryFor(c, config, env).nameSource; s != "" {
		return s
	}
	return SourceDefault
}

// RoutingEnabled resolves the automatic model-class router feature flag
// (Phase 3.5 §14): the SOP_MODEL_ROUTING_ENABLED environment overrides the
// models.routing_enabled configuration value, and an omitted value on both layers
// resolves to false. An unparseable environment value is an actionable error.
//
// It is a separate flag from Configured(): enabling the router does not, by
// itself, activate a class table, and the router is OFF by default so an existing
// installation keeps its current agent selection.
func RoutingEnabled(config Route, lookup func(string) string) (bool, error) {
	if lookup == nil {
		lookup = os.Getenv
	}
	if v := strings.TrimSpace(lookup(EnvRoutingEnabled)); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return false, fmt.Errorf("model routing: %s: invalid boolean %q (want true or false)", EnvRoutingEnabled, v)
		}
		return b, nil
	}
	if config.RoutingEnabled != nil {
		return *config.RoutingEnabled, nil
	}
	return false, nil
}

// EscalationEnabled resolves the bounded execution-recovery escalation feature
// flag (Phase 5): the SOP_MODEL_ESCALATION_ENABLED environment overrides the
// models.escalation_enabled configuration value, and an omitted value on both
// layers resolves to false. An unparseable environment value is an actionable
// error.
//
// Like RoutingEnabled it is separate from Configured(): enabling escalation does
// not activate a class table, and it is OFF by default so an existing
// installation's execution behavior is unchanged.
func EscalationEnabled(config Route, lookup func(string) string) (bool, error) {
	if lookup == nil {
		lookup = os.Getenv
	}
	if v := strings.TrimSpace(lookup(EnvEscalationEnabled)); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return false, fmt.Errorf("model escalation: %s: invalid boolean %q (want true or false)", EnvEscalationEnabled, v)
		}
		return b, nil
	}
	if config.EscalationEnabled != nil {
		return *config.EscalationEnabled, nil
	}
	return false, nil
}

// MaxEscalations resolves the escalation bound (Phase 5): the
// SOP_MODEL_MAX_ESCALATIONS environment overrides models.max_escalations, and an
// omitted value on both layers resolves to DefaultMaxEscalations. An explicit 0
// on either layer disables escalation. A non-numeric or negative value is an
// actionable error.
func MaxEscalations(config Route, lookup func(string) string) (int, error) {
	if lookup == nil {
		lookup = os.Getenv
	}
	if v := strings.TrimSpace(lookup(EnvMaxEscalations)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("model escalation: %s: invalid count %q (want a non-negative integer)", EnvMaxEscalations, v)
		}
		return n, nil
	}
	if config.MaxEscalations != nil {
		return *config.MaxEscalations, nil
	}
	return DefaultMaxEscalations, nil
}

// allowCloudFallback reports whether a local class may fall back to a cloud
// model, with the environment overriding the config value. It defaults to false.
func allowCloudFallback(config, env Route) bool {
	if env.AllowCloudFallbackForLocal != nil {
		return *env.AllowCloudFallbackForLocal
	}
	if config.AllowCloudFallbackForLocal != nil {
		return *config.AllowCloudFallbackForLocal
	}
	return false
}

// joinClasses renders the accepted classes for an error message.
func joinClasses() string {
	parts := make([]string, len(Classes))
	for i, c := range Classes {
		parts[i] = string(c)
	}
	return strings.Join(parts, ", ")
}

// joinProviders renders the accepted providers for an error message.
func joinProviders() string {
	return "ollama, llamacpp, command"
}
