// Package model resolves SOP's optional model-routing layer: which model (and
// provider) a run uses, selected by a small/medium/large class from a layered
// configuration.
//
// Precedence, highest first:
//
//  1. a CLI override (the `--model-class` flag),
//  2. environment variables (SOP_MODEL_*), which a .env file may supply,
//  3. the `models:` block in .agent-sdlc/config.yaml,
//  4. built-in defaults.
//
// The layer is opt-in. Without any `models:` configuration, any SOP_MODEL_*
// environment, or a CLI override, Resolve reports the layer inactive and callers
// keep the existing agent selection — so an existing installation is unchanged.
//
// Locality (local or cloud) is descriptive metadata and a fallback guard, not an
// execution mode: resolution never silently switches a local class to a cloud
// model. If a local class has no model and the fallback is a cloud model,
// resolution fails rather than quietly using the cloud model unless
// allow_cloud_fallback_for_local is set.
//
// This package holds no secrets: a Selection records only the class, provider,
// model name, locality, and the source layer, never a credential.
package model

import (
	"fmt"
	"os"
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
)

// knownProviders is the provider allow-list a routed class may name. It mirrors
// the providers the agent layer supports; an unknown name fails at resolution
// rather than reaching the agent constructor.
var knownProviders = map[string]bool{
	"ollama":   true,
	"llamacpp": true,
	"command":  true,
}

// ClassConfig is one class's routing in the config file (and, reused, the shape
// of the environment layer). Every field is optional: an unset field inherits
// from a lower-precedence layer.
type ClassConfig struct {
	Provider string   `yaml:"provider"`
	Name     string   `yaml:"name"`
	Locality Locality `yaml:"locality"`
}

func (cc ClassConfig) empty() bool {
	return strings.TrimSpace(cc.Provider) == "" &&
		strings.TrimSpace(cc.Name) == "" &&
		strings.TrimSpace(string(cc.Locality)) == ""
}

// Route is the routing table: the default and fallback class, the local-to-cloud
// fallback policy, and the per-class models. It is the `models:` block of
// config.yaml and, reused, the parsed shape of the SOP_MODEL_* environment.
type Route struct {
	DefaultClass               Class       `yaml:"default_class"`
	FallbackClass              Class       `yaml:"fallback_class"`
	AllowCloudFallbackForLocal *bool       `yaml:"allow_cloud_fallback_for_local"`
	Small                      ClassConfig `yaml:"small"`
	Medium                     ClassConfig `yaml:"medium"`
	Large                      ClassConfig `yaml:"large"`
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
)

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
}

// Selection is the resolved model selection: the class, the provider and model
// it names, the locality, and the layer that supplied it. It carries no
// credential.
type Selection struct {
	Class    Class
	Provider string
	Model    string
	Locality Locality
	Source   Source
}

// Result is the outcome of resolution. Active is false when no routing layer was
// configured, in which case Selection is the zero value and the caller must keep
// its existing agent selection.
type Result struct {
	Selection Selection
	Active    bool
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
	if !in.Config.Configured() && !env.Configured() && cli == "" {
		return Result{Active: false}, nil
	}

	// Selected class: CLI override > SOP_MODEL_DEFAULT_CLASS > models.default_class
	// > built-in default (medium).
	class := ClassMedium
	switch {
	case cli != "":
		c, err := ParseClass(cli)
		if err != nil {
			return Result{}, fmt.Errorf("model routing: --model-class: %w", err)
		}
		class = c
	case env.DefaultClass != "":
		class = env.DefaultClass
	case in.Config.DefaultClass != "":
		class = in.Config.DefaultClass
	}

	primary := entryFor(class, in.Config, env)
	if primary.complete() {
		return Result{Active: true, Selection: selectionFor(class, primary, sourceFor(class, in.Config, env, cli))}, nil
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
			return Result{Active: true, Selection: selectionFor(fallback, fb, sourceFor(fallback, in.Config, env, ""))}, nil
		}
	}

	return Result{}, fmt.Errorf("model routing: class %q has no model configured (set %s, %s, and %s, or the models.%s block in config.yaml)",
		class, ClassEnvKey(class, fieldProvider), ClassEnvKey(class, fieldName), ClassEnvKey(class, fieldLocality), class)
}

// entry is a class's merged routing: per-field, the environment layer over the
// config layer.
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

// entryFor merges the config and environment entries for a class, the
// environment winning field by field.
func entryFor(c Class, config, env Route) entry {
	var e entry
	if cc := config.classConfig(c); !cc.empty() {
		if v := strings.TrimSpace(cc.Provider); v != "" {
			e.provider = v
		}
		if v := strings.TrimSpace(cc.Name); v != "" {
			e.model = v
			e.nameSource = SourceConfig
		}
		if cc.Locality != "" {
			e.locality = cc.Locality
		}
	}
	if cc := env.classConfig(c); !cc.empty() {
		if v := strings.TrimSpace(cc.Provider); v != "" {
			e.provider = v
		}
		if v := strings.TrimSpace(cc.Name); v != "" {
			e.model = v
			e.nameSource = SourceEnv
		}
		if cc.Locality != "" {
			e.locality = cc.Locality
		}
	}
	return e
}

// selectionFor builds a Selection from a merged entry.
func selectionFor(c Class, e entry, source Source) Selection {
	return Selection{Class: c, Provider: e.provider, Model: e.model, Locality: e.locality, Source: source}
}

// sourceFor names the layer that supplied the selection: the CLI override when it
// chose the class, otherwise the layer that supplied the model name, otherwise
// the built-in default.
func sourceFor(c Class, config, env Route, cli string) Source {
	if strings.TrimSpace(cli) != "" {
		return SourceCLI
	}
	if s := entryFor(c, config, env).nameSource; s != "" {
		return s
	}
	return SourceDefault
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
