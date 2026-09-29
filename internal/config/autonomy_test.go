package config

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/autonomy"
)

// Autonomy configuration tests: the block is optional and additive, an omitted
// block resolves to the conservative default, explicit flags override the level,
// and a bad level/category is a load-time error.

// TestAutonomyOmittedDefaultsToBalanced proves requirement 22: an existing
// configuration with no autonomy block keeps the conservative, backward-compatible
// default and is never silently switched to maximum autonomy.
func TestAutonomyOmittedDefaultsToBalanced(t *testing.T) {
	c, err := Parse([]byte("project:\n  name: a\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := c.AutonomyLevel(); got != autonomy.Balanced {
		t.Errorf("level = %s, want BALANCED", got)
	}
	p := c.AutonomyPolicy()
	if !p.AutoRetry || !p.AutoContinue || !p.AutoFix {
		t.Errorf("balanced policy must automate retry/continue/fix: %+v", p)
	}
	if p.AutoReconcileSafeChanges {
		t.Error("balanced must not auto-reconcile safe changes (a high-level capability)")
	}
	if p.Level != autonomy.Balanced {
		t.Errorf("policy level = %s, want BALANCED", p.Level)
	}
}

// TestAutonomyLevelHigh proves the recommended dogfooding level.
func TestAutonomyLevelHigh(t *testing.T) {
	c, err := Parse([]byte("project:\n  name: a\nautonomy:\n  level: high\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	p := c.AutonomyPolicy()
	if p.Level != autonomy.High || !p.AutoReconcileSafeChanges {
		t.Errorf("policy = %+v, want HIGH with safe reconciliation", p)
	}
}

// TestAutonomyLevelLow proves the conservative level withholds automatic fixes.
func TestAutonomyLevelLow(t *testing.T) {
	c, err := Parse([]byte("project:\n  name: a\nautonomy:\n  level: low\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	p := c.AutonomyPolicy()
	if p.Level != autonomy.Low || p.AutoFix {
		t.Errorf("policy = %+v, want LOW without automatic fixes", p)
	}
	if !p.AutoRetry || !p.AutoContinue {
		t.Error("low must still permit the safe, non-mutating recoveries")
	}
}

// TestAutonomyExplicitFlagsOverride proves an explicit flag overrides the level.
func TestAutonomyExplicitFlagsOverride(t *testing.T) {
	c, err := Parse([]byte("project:\n  name: a\nautonomy:\n  level: high\n  auto_fix: false\n  auto_retry: false\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	p := c.AutonomyPolicy()
	if p.AutoFix || p.AutoRetry {
		t.Errorf("policy = %+v, want the explicit false overrides applied", p)
	}
	if !p.AutoContinue {
		t.Error("an untouched flag must keep the level's setting")
	}
}

// TestAutonomyRequireHumanOverride proves an explicit require_human list replaces
// the defaults.
func TestAutonomyRequireHumanOverride(t *testing.T) {
	c, err := Parse([]byte("project:\n  name: a\nautonomy:\n  level: high\n  require_human:\n    - destructive\n    - external_publish\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	p := c.AutonomyPolicy()
	if len(p.RequireHuman) != 2 || p.RequireHuman[0] != autonomy.CategoryDestructive {
		t.Errorf("RequireHuman = %v, want [destructive external_publish]", p.RequireHuman)
	}
}

// TestAutonomyDefaultsRequireAuthorityBoundaries proves the default list reserves
// the authority boundaries for humans.
func TestAutonomyDefaultsRequireAuthorityBoundaries(t *testing.T) {
	c, err := Parse([]byte("project:\n  name: a\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := map[autonomy.RiskCategory]bool{
		autonomy.CategoryDestructive: true, autonomy.CategoryIrreversible: true,
		autonomy.CategorySecuritySensitive: true, autonomy.CategoryAmbiguousRequirements: true,
		autonomy.CategoryExternalPublish: true,
	}
	got := c.AutonomyPolicy().RequireHuman
	if len(got) != len(want) {
		t.Fatalf("RequireHuman = %v, want %d entries", got, len(want))
	}
	for _, cat := range got {
		if !want[cat] {
			t.Errorf("unexpected category %q", cat)
		}
	}
}

func TestAutonomyRejectsUnknownLevel(t *testing.T) {
	if _, err := Parse([]byte("project:\n  name: a\nautonomy:\n  level: turbo\n")); err == nil {
		t.Fatal("expected an error for an unknown autonomy level")
	}
}

func TestAutonomyRejectsUnknownCategory(t *testing.T) {
	if _, err := Parse([]byte("project:\n  name: a\nautonomy:\n  require_human:\n    - vibes\n")); err == nil {
		t.Fatal("expected an error for an unknown require_human category")
	}
}

// TestTemplateDocumentsAutonomy proves the generated template carries a resolvable
// autonomy block and remains on the conservative default.
func TestTemplateDocumentsAutonomy(t *testing.T) {
	c, err := Parse([]byte(Template("book-rag")))
	if err != nil {
		t.Fatalf("template does not parse: %v", err)
	}
	if got := c.AutonomyLevel(); got != autonomy.Balanced {
		t.Errorf("template autonomy level = %s, want BALANCED", got)
	}
}
