package model

import "testing"

func TestStageClassPrecedenceAndValidation(t *testing.T) {
	cfg := Route{Stages: StageClasses{Review: ClassLarge}}
	env := map[string]string{}
	lookup := func(k string) string { return env[k] }

	if c, err := StageClass(cfg, StageReview, lookup); err != nil || c != ClassLarge {
		t.Fatalf("config review = %q, %v; want large", c, err)
	}
	if c, err := StageClass(cfg, StagePlan, lookup); err != nil || c != "" {
		t.Fatalf("unset plan = %q, %v; want unpinned", c, err)
	}
	env["SOP_MODEL_STAGE_REVIEW_CLASS"] = "small"
	if c, err := StageClass(cfg, StageReview, lookup); err != nil || c != ClassSmall {
		t.Fatalf("env review = %q, %v; want small (env wins)", c, err)
	}
	env["SOP_MODEL_STAGE_JEV_CLASS"] = "huge"
	if _, err := StageClass(cfg, StageJEV, lookup); err == nil {
		t.Fatal("unknown env class must be rejected")
	}
	if err := (Route{Stages: StageClasses{Plan: "huge"}}).Validate(); err == nil {
		t.Fatal("unknown stages.plan must fail validation")
	}
	if (Route{Stages: StageClasses{Plan: ClassSmall}}).Configured() {
		t.Fatal("stages alone must not activate the class table")
	}
}

// TestStageDirect pins the direct provider/model stage pin and its
// misconfigurations: half set, unknown provider, and a conflicting class pin.
func TestStageDirect(t *testing.T) {
	env := func(kv map[string]string) func(string) string { return func(k string) string { return kv[k] } }
	p, n, c := StageProviderEnvKey(StageReview), StageNameEnvKey(StageReview), StageEnvKey(StageReview)

	sel, ok, err := StageDirect(StageReview, env(map[string]string{p: "openai_compatible", n: "bonsai"}))
	if err != nil || !ok || sel.Provider != "openai_compatible" || sel.Model != "bonsai" || sel.Class != "" {
		t.Fatalf("direct pin = %+v, %v, %v", sel, ok, err)
	}
	if _, ok, err := StageDirect(StageReview, env(nil)); ok || err != nil {
		t.Errorf("unset: ok=%v err=%v, want not pinned", ok, err)
	}
	for name, kv := range map[string]map[string]string{
		"half set":         {p: "openai_compatible"},
		"unknown provider": {p: "nope", n: "m"},
		"class conflict":   {p: "ollama", n: "m", c: "large"},
	} {
		if _, _, err := StageDirect(StageReview, env(kv)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// TestSensitiveModel pins the sensitive model settings: set, unset, half set,
// and an unknown provider.
func TestSensitiveModel(t *testing.T) {
	env := func(kv map[string]string) func(string) string { return func(k string) string { return kv[k] } }
	sel, ok, err := SensitiveModel(env(map[string]string{EnvSensitiveProvider: "ollama", EnvSensitiveName: "mistral-large-4:cloud"}))
	if err != nil || !ok || sel.Provider != "ollama" || sel.Model != "mistral-large-4:cloud" || sel.Reason != ReasonSensitiveModel {
		t.Fatalf("sensitive model = %+v, %v, %v", sel, ok, err)
	}
	if _, ok, err := SensitiveModel(env(nil)); ok || err != nil {
		t.Errorf("unset: ok=%v err=%v", ok, err)
	}
	for name, kv := range map[string]map[string]string{
		"half set":         {EnvSensitiveName: "m"},
		"unknown provider": {EnvSensitiveProvider: "nope", EnvSensitiveName: "m"},
	} {
		if _, _, err := SensitiveModel(env(kv)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
