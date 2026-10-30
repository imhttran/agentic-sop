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
