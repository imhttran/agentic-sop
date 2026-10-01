package model

import (
	"strings"
	"testing"
)

// lookup returns an environment lookup backed by a map.
func lookup(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// mediumEnv is a fully specified medium class in the environment layer.
func mediumEnv() map[string]string {
	return map[string]string{
		ClassEnvKey(ClassMedium, fieldProvider): "ollama",
		ClassEnvKey(ClassMedium, fieldName):     "env-medium",
		ClassEnvKey(ClassMedium, fieldLocality): "cloud",
	}
}

// mediumConfig is a fully specified medium class in the config layer.
func mediumConfig() ClassConfig {
	return ClassConfig{Provider: "ollama", Name: "config-medium", Locality: LocalityCloud}
}

func TestResolveInactiveWithoutConfiguration(t *testing.T) {
	res, err := Resolve(Inputs{Lookup: lookup(nil)})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Active {
		t.Fatalf("routing must be inactive with no configuration, got %+v", res)
	}
	if res.Selection != (Selection{}) {
		t.Fatalf("inactive resolution must yield a zero selection, got %+v", res.Selection)
	}
}

func TestResolveConfigLayerOverridesDefaults(t *testing.T) {
	res, err := Resolve(Inputs{
		Config: Route{Medium: mediumConfig()},
		Lookup: lookup(nil),
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !res.Active {
		t.Fatal("routing must be active when the config block sets a class")
	}
	want := Selection{Class: ClassMedium, Provider: "ollama", Model: "config-medium", Locality: LocalityCloud, Source: SourceConfig, Reason: ReasonBuiltinClass}
	if res.Selection != want {
		t.Fatalf("selection = %+v, want %+v", res.Selection, want)
	}
}

func TestResolveEnvOverridesConfig(t *testing.T) {
	res, err := Resolve(Inputs{
		Config: Route{Medium: mediumConfig()},
		Lookup: lookup(mediumEnv()),
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Selection.Model != "env-medium" {
		t.Fatalf("model = %q, want the environment value", res.Selection.Model)
	}
	if res.Selection.Source != SourceEnv {
		t.Fatalf("source = %q, want %q", res.Selection.Source, SourceEnv)
	}
	if res.Selection.Reason != ReasonBuiltinClass {
		t.Fatalf("reason = %q, want %q", res.Selection.Reason, ReasonBuiltinClass)
	}
}

func TestResolveEnvOnlyWithoutConfig(t *testing.T) {
	res, err := Resolve(Inputs{Lookup: lookup(mediumEnv())})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !res.Active || res.Selection.Class != ClassMedium || res.Selection.Source != SourceEnv {
		t.Fatalf("selection = %+v, want an active env-sourced medium selection", res)
	}
	if res.Selection.Reason != ReasonBuiltinClass {
		t.Fatalf("reason = %q, want %q (no layer chose a class)", res.Selection.Reason, ReasonBuiltinClass)
	}
}

func TestResolveCLIOverridesEnvironment(t *testing.T) {
	env := map[string]string{
		EnvDefaultClass:                        "small",
		ClassEnvKey(ClassSmall, fieldProvider): "ollama",
		ClassEnvKey(ClassSmall, fieldName):     "small-env",
		ClassEnvKey(ClassSmall, fieldLocality): "local",
		ClassEnvKey(ClassLarge, fieldProvider): "ollama",
		ClassEnvKey(ClassLarge, fieldName):     "large-env",
		ClassEnvKey(ClassLarge, fieldLocality): "cloud",
	}
	res, err := Resolve(Inputs{Lookup: lookup(env), CLIClass: "large"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Selection.Class != ClassLarge || res.Selection.Model != "large-env" {
		t.Fatalf("selection = %+v, want the CLI-selected large class", res.Selection)
	}
	if res.Selection.Source != SourceCLI {
		t.Fatalf("source = %q, want %q", res.Selection.Source, SourceCLI)
	}
	if res.Selection.Reason != ReasonCLIClass {
		t.Fatalf("reason = %q, want %q", res.Selection.Reason, ReasonCLIClass)
	}
}

func TestResolveInvalidClassFromEnv(t *testing.T) {
	_, err := Resolve(Inputs{Lookup: lookup(map[string]string{EnvDefaultClass: "gigantic"})})
	if err == nil {
		t.Fatal("expected an error for an unknown model class")
	}
	if !strings.Contains(err.Error(), EnvDefaultClass) || !strings.Contains(err.Error(), "gigantic") {
		t.Fatalf("error must name the variable and value: %v", err)
	}
}

func TestResolveInvalidLocalityFromEnv(t *testing.T) {
	env := mediumEnv()
	env[ClassEnvKey(ClassMedium, fieldLocality)] = "mars"
	_, err := Resolve(Inputs{Lookup: lookup(env)})
	if err == nil {
		t.Fatal("expected an error for an unknown locality")
	}
	if !strings.Contains(err.Error(), ClassEnvKey(ClassMedium, fieldLocality)) {
		t.Fatalf("error must name the offending variable: %v", err)
	}
}

func TestResolveInvalidClassFromCLI(t *testing.T) {
	_, err := Resolve(Inputs{Lookup: lookup(mediumEnv()), CLIClass: "huge"})
	if err == nil || !strings.Contains(err.Error(), "--model-class") {
		t.Fatalf("error must name the flag: %v", err)
	}
}

func TestResolveRefusesSilentLocalToCloudFallback(t *testing.T) {
	cfg := Route{
		DefaultClass:  ClassSmall,
		FallbackClass: ClassMedium,
		Small:         ClassConfig{Locality: LocalityLocal},
		Medium:        ClassConfig{Provider: "ollama", Name: "cloud-medium", Locality: LocalityCloud},
	}
	_, err := Resolve(Inputs{Config: cfg, Lookup: lookup(nil)})
	if err == nil {
		t.Fatal("expected an error refusing a silent local-to-cloud fallback")
	}
	if !strings.Contains(err.Error(), EnvAllowCloudFallbackForLocal) {
		t.Fatalf("error must name the flag that allows the fallback: %v", err)
	}

	allowed := true
	cfg.AllowCloudFallbackForLocal = &allowed
	res, err := Resolve(Inputs{Config: cfg, Lookup: lookup(nil)})
	if err != nil {
		t.Fatalf("Resolve with cloud fallback allowed: %v", err)
	}
	if res.Selection.Class != ClassMedium || res.Selection.Locality != LocalityCloud {
		t.Fatalf("selection = %+v, want the cloud fallback class", res.Selection)
	}
	if !res.Selection.Fallback || res.Selection.Reason != ReasonFallbackClass {
		t.Fatalf("selection = %+v, want fallback=true and reason %q", res.Selection, ReasonFallbackClass)
	}
}

func TestResolvePartialClassWithoutFallbackFails(t *testing.T) {
	// A class a layer mentions is used as written; a partial one has no complete
	// model, and with no distinct fallback class resolution fails and names the
	// variables to set. The built-in default does not fill a class a layer
	// mentions, so the fallback rule stays meaningful.
	_, err := Resolve(Inputs{
		Config: Route{DefaultClass: ClassLarge, Large: ClassConfig{Locality: LocalityCloud}},
		Lookup: lookup(nil),
	})
	if err == nil {
		t.Fatal("expected an error for a partially configured class with no fallback")
	}
	if !strings.Contains(err.Error(), ClassEnvKey(ClassLarge, fieldName)) {
		t.Fatalf("error must name the variables to set: %v", err)
	}
}

func TestResolveUnconfiguredClassUsesBuiltinDefault(t *testing.T) {
	// DefaultClass names large but no layer defines large: the built-in route
	// supplies a complete model, so the run resolves without configuration.
	res, err := Resolve(Inputs{Config: Route{DefaultClass: ClassLarge}, Lookup: lookup(nil)})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := Selection{Class: ClassLarge, Provider: "ollama", Model: "deepseek-v4.1-flash:cloud", Locality: LocalityCloud, Source: SourceDefault, Reason: ReasonConfigClass}
	if res.Selection != want {
		t.Fatalf("selection = %+v, want %+v", res.Selection, want)
	}
}

func TestResolveCLIClassUsesBuiltinDefault(t *testing.T) {
	// A bare --model-class resolves from the built-in route with no configuration.
	res, err := Resolve(Inputs{Lookup: lookup(nil), CLIClass: "small"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := Selection{Class: ClassSmall, Provider: "ollama", Model: "qwen3:4b", Locality: LocalityLocal, Source: SourceCLI, Reason: ReasonCLIClass}
	if res.Selection != want {
		t.Fatalf("selection = %+v, want %+v", res.Selection, want)
	}
}

func TestResolveEnvDefaultClassReason(t *testing.T) {
	env := mediumEnv()
	env[EnvDefaultClass] = "medium"
	res, err := Resolve(Inputs{Lookup: lookup(env)})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Selection.Reason != ReasonEnvClass {
		t.Fatalf("reason = %q, want %q", res.Selection.Reason, ReasonEnvClass)
	}
}

func TestResolveBuiltinDefaultsTable(t *testing.T) {
	// Each class resolves to its documented built-in model when a layer selects it
	// but no layer defines it.
	cases := []struct {
		class    string
		provider string
		model    string
		locality Locality
	}{
		{"small", "ollama", "qwen3:4b", LocalityLocal},
		{"medium", "ollama", "nemotron-3-super:cloud", LocalityCloud},
		{"large", "ollama", "deepseek-v4.1-flash:cloud", LocalityCloud},
	}
	for _, tc := range cases {
		t.Run(tc.class, func(t *testing.T) {
			res, err := Resolve(Inputs{Lookup: lookup(nil), CLIClass: tc.class})
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			sel := res.Selection
			if sel.Provider != tc.provider || sel.Model != tc.model || sel.Locality != tc.locality {
				t.Fatalf("selection = %+v, want %s/%s/%s", sel, tc.provider, tc.model, tc.locality)
			}
		})
	}
}

func TestRouteDefaultIsNotActivating(t *testing.T) {
	// Adding built-in routes must not turn routing on by itself: with no explicit
	// layer, resolution stays inactive even though the built-in route is defined.
	if res, err := Resolve(Inputs{Lookup: lookup(nil)}); err != nil || res.Active {
		t.Fatalf("routing must stay inactive with no explicit layer: %+v, %v", res, err)
	}
	// The built-in route is complete, so every class it defines resolves.
	for _, c := range Classes {
		if DefaultRoute().classConfig(c).empty() {
			t.Fatalf("built-in route has no entry for class %q", c)
		}
	}
}

func TestResolveDoesNotCarrySecrets(t *testing.T) {
	const secret = "supersecret-token"
	env := mediumEnv()
	env["SOP_LLAMACPP_API_KEY"] = secret
	env["SOP_API_TOKEN"] = secret

	res, err := Resolve(Inputs{Lookup: lookup(env)})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	for _, field := range []string{
		string(res.Selection.Class),
		res.Selection.Provider,
		res.Selection.Model,
		string(res.Selection.Locality),
		string(res.Selection.Source),
	} {
		if strings.Contains(field, secret) {
			t.Fatalf("resolved selection leaked a secret: %q", field)
		}
	}
}

func TestRouteValidate(t *testing.T) {
	cases := []struct {
		name string
		rt   Route
		want string
	}{
		{"bad default class", Route{DefaultClass: "big"}, "default_class"},
		{"bad fallback class", Route{FallbackClass: "big"}, "fallback_class"},
		{"bad locality", Route{Small: ClassConfig{Locality: "space"}}, "locality"},
		{"bad provider", Route{Small: ClassConfig{Provider: "gpt"}}, "provider"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rt.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate = %v, want an error mentioning %q", err, tc.want)
			}
		})
	}
	if err := (Route{DefaultClass: ClassSmall, Medium: mediumConfig()}).Validate(); err != nil {
		t.Fatalf("valid route rejected: %v", err)
	}
}

func TestParseClassAndLocality(t *testing.T) {
	if c, err := ParseClass(" Medium "); err != nil || c != ClassMedium {
		t.Fatalf("ParseClass = (%q, %v)", c, err)
	}
	if _, err := ParseClass("xxl"); err == nil {
		t.Fatal("expected an error for an unknown class")
	}
	if l, err := ParseLocality("Cloud"); err != nil || l != LocalityCloud {
		t.Fatalf("ParseLocality = (%q, %v)", l, err)
	}
	if _, err := ParseLocality("orbit"); err == nil {
		t.Fatal("expected an error for an unknown locality")
	}
}

func TestResolveInvalidProviderFromEnv(t *testing.T) {
	env := mediumEnv()
	env[ClassEnvKey(ClassMedium, fieldProvider)] = "gpt"
	_, err := Resolve(Inputs{Lookup: lookup(env)})
	if err == nil || !strings.Contains(err.Error(), ClassEnvKey(ClassMedium, fieldProvider)) {
		t.Fatalf("error must name the offending variable: %v", err)
	}
}

func boolPtr(b bool) *bool { return &b }

// TestResolveRoutedClass verifies that a class chosen by SOP's router (Phase 3.5)
// activates the layer, wins over the configured/environment default class, and
// resolves through the built-in defaults with no other configuration.
func TestResolveRoutedClass(t *testing.T) {
	res, err := Resolve(Inputs{
		Lookup:       lookup(nil),
		RoutedClass:  ClassSmall,
		RoutedReason: "isolated low-risk task",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !res.Active {
		t.Fatal("a routed class must activate the layer")
	}
	want := Selection{
		Class:    ClassSmall,
		Provider: "ollama",
		Model:    "qwen3:4b",
		Locality: LocalityLocal,
		Source:   SourceRouter,
		Reason:   "isolated low-risk task",
	}
	if res.Selection != want {
		t.Fatalf("selection = %+v, want %+v", res.Selection, want)
	}
}

// TestResolveCLIOverrideBeatsRoutedClass verifies the precedence rule: a manual
// --model-class override always wins over the automatic router, so an operator's
// explicit choice is never silently replaced.
func TestResolveCLIOverrideBeatsRoutedClass(t *testing.T) {
	res, err := Resolve(Inputs{
		Lookup:      lookup(nil),
		CLIClass:    "large",
		RoutedClass: ClassSmall,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Selection.Class != ClassLarge || res.Selection.Source != SourceCLI {
		t.Fatalf("selection = %+v, want the CLI-selected large class", res.Selection)
	}
}

// TestResolveRoutedClassOverridesDefaultClass verifies that a routed class beats
// an environment default class but still resolves its model from the layers.
func TestResolveRoutedClassOverridesDefaultClass(t *testing.T) {
	env := mediumEnv()
	env[EnvDefaultClass] = "medium"
	res, err := Resolve(Inputs{
		Lookup:      lookup(env),
		RoutedClass: ClassMedium,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Selection.Class != ClassMedium || res.Selection.Source != SourceRouter {
		t.Fatalf("selection = %+v, want the routed medium class", res.Selection)
	}
}

func TestRoutingEnabledPrecedence(t *testing.T) {
	// Default: disabled with no layer set.
	if on, err := RoutingEnabled(Route{}, lookup(nil)); err != nil || on {
		t.Fatalf("default = (%v, %v), want false", on, err)
	}
	// Config enables it.
	if on, err := RoutingEnabled(Route{RoutingEnabled: boolPtr(true)}, lookup(nil)); err != nil || !on {
		t.Fatalf("config = (%v, %v), want true", on, err)
	}
	// Environment overrides config (both directions).
	if on, err := RoutingEnabled(Route{RoutingEnabled: boolPtr(false)}, lookup(map[string]string{EnvRoutingEnabled: "true"})); err != nil || !on {
		t.Fatalf("env true over config false = (%v, %v), want true", on, err)
	}
	if on, err := RoutingEnabled(Route{RoutingEnabled: boolPtr(true)}, lookup(map[string]string{EnvRoutingEnabled: "false"})); err != nil || on {
		t.Fatalf("env false over config true = (%v, %v), want false", on, err)
	}
	// An invalid boolean fails clearly.
	if _, err := RoutingEnabled(Route{}, lookup(map[string]string{EnvRoutingEnabled: "maybe"})); err == nil {
		t.Fatal("expected an actionable error for an invalid boolean")
	}
}

// TestDefaultRouteTierModels pins the built-in tier table: SMALL is local-first
// with a cloud runtime fallback, and MEDIUM/LARGE are cloud models.
func TestDefaultRouteTierModels(t *testing.T) {
	small := DefaultRoute().classConfig(ClassSmall)
	if small.Name != "qwen3:4b" || small.Locality != LocalityLocal {
		t.Fatalf("small primary = %+v, want local qwen3:4b", small)
	}
	if small.Fallback == nil || small.Fallback.Name != "nemotron-3-nano:30b-cloud" || small.Fallback.Locality != LocalityCloud {
		t.Fatalf("small fallback = %+v, want nemotron-3-nano:30b-cloud/cloud", small.Fallback)
	}
	if med := DefaultRoute().classConfig(ClassMedium); med.Name != "nemotron-3-super:cloud" {
		t.Fatalf("medium default = %q, want nemotron-3-super:cloud", med.Name)
	}
	if lg := DefaultRoute().classConfig(ClassLarge); lg.Name != "deepseek-v4.1-flash:cloud" {
		t.Fatalf("large default = %q, want deepseek-v4.1-flash:cloud", lg.Name)
	}
}

// TestResolveSmallLocalFallbackCandidate verifies that a LOCAL class with a
// configured fallback reports the fallback as a candidate while keeping the local
// selection primary: Resolve never probes a runtime and never switches on its own.
func TestResolveSmallLocalFallbackCandidate(t *testing.T) {
	res, err := Resolve(Inputs{Lookup: lookup(nil), RoutedClass: ClassSmall})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := Selection{Class: ClassSmall, Provider: "ollama", Model: "qwen3:4b", Locality: LocalityLocal, Source: SourceRouter}
	if res.Selection != want {
		t.Fatalf("selection = %+v, want the local primary %+v", res.Selection, want)
	}
	if res.LocalFallback == nil {
		t.Fatal("a local class with a configured fallback must report the candidate")
	}
	fb := *res.LocalFallback
	if fb.Model != "nemotron-3-nano:30b-cloud" || fb.Locality != LocalityCloud || fb.Source != SourceCloudFallback {
		t.Fatalf("fallback = %+v, want the small cloud fallback", fb)
	}
	if fb.Class != ClassSmall || fb.Provider != "ollama" || !fb.Fallback || fb.Reason != ReasonLocalFallback {
		t.Fatalf("fallback = %+v, want class small / provider ollama / fallback=true", fb)
	}
}

// TestResolveCloudClassHasNoLocalFallback proves a cloud class is never a
// fallback candidate, so the caller can never swap a cloud model for a fallback.
func TestResolveCloudClassHasNoLocalFallback(t *testing.T) {
	for _, c := range []Class{ClassMedium, ClassLarge} {
		res, err := Resolve(Inputs{Lookup: lookup(nil), RoutedClass: c})
		if err != nil {
			t.Fatalf("%s: Resolve: %v", c, err)
		}
		if res.LocalFallback != nil {
			t.Fatalf("%s: a cloud class must have no local fallback, got %+v", c, res.LocalFallback)
		}
	}
}

// TestResolveSmallFallbackOverride verifies the env override for the fallback
// model, and that an unset fallback provider/locality inherit the primary's
// provider and default to cloud.
func TestResolveSmallFallbackOverride(t *testing.T) {
	env := map[string]string{ClassFallbackEnvKey(ClassSmall, FallbackFieldName): "my-cloud:latest"}
	res, err := Resolve(Inputs{Lookup: lookup(env), RoutedClass: ClassSmall})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.LocalFallback == nil {
		t.Fatal("the overridden fallback must be reported")
	}
	if res.LocalFallback.Model != "my-cloud:latest" {
		t.Fatalf("fallback model = %q, want my-cloud:latest", res.LocalFallback.Model)
	}
	if res.LocalFallback.Provider != "ollama" {
		t.Fatalf("fallback provider = %q, want the primary's provider (ollama)", res.LocalFallback.Provider)
	}
	if res.LocalFallback.Locality != LocalityCloud {
		t.Fatalf("fallback locality = %q, want cloud (the default)", res.LocalFallback.Locality)
	}
}

// TestResolveExplicitSmallModelIsPrimary verifies that an explicitly configured
// local SMALL model is kept as the primary (never replaced by the built-in
// qwen3:4b), and that its fallback is reported beside it.
func TestResolveExplicitSmallModelIsPrimary(t *testing.T) {
	env := map[string]string{
		ClassEnvKey(ClassSmall, fieldProvider): "ollama",
		ClassEnvKey(ClassSmall, fieldName):     "my-local-model",
		ClassEnvKey(ClassSmall, fieldLocality): "local",
	}
	res, err := Resolve(Inputs{Lookup: lookup(env), RoutedClass: ClassSmall})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Selection.Model != "my-local-model" || res.Selection.Locality != LocalityLocal {
		t.Fatalf("selection = %+v, want the configured local model kept as primary", res.Selection)
	}
	if res.LocalFallback == nil || res.LocalFallback.Model != "nemotron-3-nano:30b-cloud" {
		t.Fatalf("fallback = %+v, want the built-in small fallback", res.LocalFallback)
	}
}

// TestResolveFallbackOnlyDoesNotFillPrimary proves a fallback alone does not
// replace the class's primary: the class still resolves through the built-in
// default, and the fallback is reported beside it.
func TestResolveFallbackOnlyDoesNotFillPrimary(t *testing.T) {
	env := map[string]string{ClassFallbackEnvKey(ClassSmall, FallbackFieldName): "my-cloud:latest"}
	res, err := Resolve(Inputs{Lookup: lookup(env), RoutedClass: ClassSmall})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Selection.Model != "qwen3:4b" {
		t.Fatalf("primary = %q, want the built-in qwen3:4b (a fallback never fills the primary)", res.Selection.Model)
	}
}

// TestResolveFallbackConfigValidation proves an unknown fallback provider or
// locality fails with an actionable error rather than being ignored.
func TestResolveFallbackConfigValidation(t *testing.T) {
	bad := Route{Small: ClassConfig{Fallback: &FallbackConfig{Name: "x", Provider: "gpt"}}}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "fallback.provider") {
		t.Fatalf("Validate = %v, want an error naming the fallback provider", err)
	}
	badLoc := Route{Small: ClassConfig{Fallback: &FallbackConfig{Name: "x", Locality: "orbit"}}}
	if err := badLoc.Validate(); err == nil || !strings.Contains(err.Error(), "fallback.locality") {
		t.Fatalf("Validate = %v, want an error naming the fallback locality", err)
	}
}

// TestResolveInvalidFallbackEnvIsActionable proves an unknown fallback value from
// the environment names the offending variable.
func TestResolveInvalidFallbackEnvIsActionable(t *testing.T) {
	env := map[string]string{ClassFallbackEnvKey(ClassSmall, FallbackFieldLocality): "mars"}
	_, err := Resolve(Inputs{Lookup: lookup(env), RoutedClass: ClassSmall})
	if err == nil || !strings.Contains(err.Error(), ClassFallbackEnvKey(ClassSmall, FallbackFieldLocality)) {
		t.Fatalf("error = %v, want the offending variable named", err)
	}
}
