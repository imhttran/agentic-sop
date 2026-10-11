package planflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// TestCanonicalProseFoldsPureEmphasisMarkers proves a pure Markdown emphasis-marker
// difference (*x* versus _x_) in literal-free prose is folded to one canonical form,
// so an editor reformat of emphasis is not mistaken for a definition change.
func TestCanonicalProseFoldsPureEmphasisMarkers(t *testing.T) {
	cases := []struct{ a, b string }{
		{"*x*", "_x_"},
		{"wrap *word* here", "wrap _word_ here"},
		{"the *quick* brown fox", "the _quick_ brown fox"},
		{"a *b* and *c* done", "a _b_ and _c_ done"},
		{"(*parenthesised*)", "(_parenthesised_)"},
	}
	for _, c := range cases {
		if canonicalProse(c.a) != canonicalProse(c.b) {
			t.Errorf("canonicalProse(%q) = %q, canonicalProse(%q) = %q; want equal",
				c.a, canonicalProse(c.a), c.b, canonicalProse(c.b))
		}
	}
}

// TestCanonicalProseDoesNotFoldIntraWordUnderscore proves an intra-word underscore
// (snake_case versus snakecase) stays a real change: emphasis folding must only
// touch a paired delimiter around word-boundary content.
func TestCanonicalProseDoesNotFoldIntraWordUnderscore(t *testing.T) {
	if canonicalProse("snake_case") == canonicalProse("snakecase") {
		t.Error("snake_case must not be folded to snakecase")
	}
	if canonicalProse("the token snake_case here") == canonicalProse("the token snakecase here") {
		t.Error("a prose token containing snake_case must not be rewritten")
	}
	if canonicalProse("a*b*c") == canonicalProse("abc") {
		t.Error("an intra-word asterisk must not be folded")
	}
}

// TestCanonicalProseFoldsEmbeddedEmphasisInLongerProse proves emphasis embedded in
// longer literal-free prose is folded consistently.
func TestCanonicalProseFoldsEmbeddedEmphasisInLongerProse(t *testing.T) {
	a := "The *objective* is to build a component."
	b := "The _objective_ is to build a component."
	if canonicalProse(a) != canonicalProse(b) {
		t.Errorf("canonicalProse(%q) != canonicalProse(%q)", a, b)
	}
}

// TestCanonicalProsePreservesLiterals proves a field containing backticks, single
// or double quotes, ~~~, or an indented line is returned unchanged, so emphasis
// markers inside code spans or quoted values are NOT folded.
func TestCanonicalProsePreservesLiterals(t *testing.T) {
	literals := []string{
		"`*x*`",
		"`_x_`",
		"\"*x*\"",
		"'*x*'",
		"~~~\n*x*\n~~~",
		"    *x*",
	}
	for _, s := range literals {
		if got := canonicalProse(s); got != s {
			t.Errorf("canonicalProse(%q) = %q; want byte-exact", s, got)
		}
	}
	// An intra-word underscore or asterisk value inside a literal is unaltered.
	if got := canonicalProse("`snake_case`"); got != "`snake_case`" {
		t.Errorf("canonicalProse(`snake_case`) = %q; want byte-exact", got)
	}
}

// TestCanonicalProseSubstantiveChangesRemainReal proves a synonym, negation,
// number, path, or identifier difference still produces unequal canonical output.
func TestCanonicalProseSubstantiveChangesRemainReal(t *testing.T) {
	cases := []struct{ a, b string }{
		{"add a widget", "remove a widget"},           // negation
		{"create the file", "create the directory"},   // synonym/word
		{"retry 3 times", "retry 5 times"},            // number
		{"write to docs/PLAN.md", "write to PLAN.md"}, // path
		{"set snake_case", "set snakecase"},           // identifier
		{"call fooBar", "call foo_bar"},               // identifier
	}
	for _, c := range cases {
		if canonicalProse(c.a) == canonicalProse(c.b) {
			t.Errorf("canonicalProse(%q) == canonicalProse(%q); want a real change", c.a, c.b)
		}
	}
}

// TestEmphasisOnlyDefinitionIsEquivalent proves an executed task whose definition
// differs only by emphasis markers is reported unchanged/equivalent, not as a
// changed-executed conflict, while a substantive executable-semantics difference
// stays material.
func TestEmphasisOnlyDefinitionIsEquivalent(t *testing.T) {
	base := &domain.Task{
		ID:                 "S001",
		Title:              "*Application* skeleton",
		Objective:          "Create the *skeleton*.",
		AcceptanceCriteria: "- *starts*",
		ExecutionMode:      domain.ExecutionImplement,
	}
	edited := &domain.Task{
		ID:                 "S001",
		Title:              "_Application_ skeleton",
		Objective:          "Create the _skeleton_.",
		AcceptanceCriteria: "- _starts_",
		ExecutionMode:      domain.ExecutionImplement,
	}
	if !sameTaskDefinition(base, edited) {
		t.Error("a task differing only by emphasis markers must be the same definition")
	}
	if !equivalentExecutedChange(base, edited) {
		t.Error("a pure-emphasis change must be an equivalent executed change")
	}

	// A substantive title change is still a real definition change.
	substantive := *edited
	substantive.Title = "Renamed skeleton"
	if sameTaskDefinition(base, &substantive) {
		t.Error("a substantive title change must remain a real change")
	}
	// A change to acceptance criteria is a material executable-semantics change.
	material := *edited
	material.AcceptanceCriteria = "- _stops_"
	if equivalentExecutedChange(base, &material) {
		t.Error("an acceptance-criteria change is not an equivalent executed change")
	}
}

// TestEmphasisChangeInSemanticsIsMaterial proves an emphasis change that also
// alters an executable-semantics field (acceptance criteria, execution mode, or
// dependencies) still classifies as material.
func TestEmphasisChangeInSemanticsIsMaterial(t *testing.T) {
	base := &domain.Task{
		ID:                 "S001",
		Title:              "Task",
		Objective:          "Do it.",
		AcceptanceCriteria: "- *starts*",
		ExecutionMode:      domain.ExecutionImplement,
		DependencyIDs:      []string{"S000"},
	}
	changedAcceptance := *base
	changedAcceptance.AcceptanceCriteria = "- _stops_"
	if equivalentExecutedChange(base, &changedAcceptance) {
		t.Error("an acceptance-criteria change is material even with emphasis markers")
	}
	changedMode := *base
	changedMode.ExecutionMode = domain.ExecutionVerifyFirst
	if equivalentExecutedChange(base, &changedMode) {
		t.Error("an execution-mode change is material")
	}
	changedDeps := *base
	changedDeps.DependencyIDs = []string{"S002"}
	if equivalentExecutedChange(base, &changedDeps) {
		t.Error("a dependency change is material")
	}
}

// TestEmphasisFoldIsDeterministicWithoutAgent proves the deterministic comparison
// folds emphasis with no agent configured, demonstrating provider/model neutrality.
func TestEmphasisFoldIsDeterministicWithoutAgent(t *testing.T) {
	a := &domain.Task{ID: "S001", Title: "*A* task", Objective: "Do *it*.", AcceptanceCriteria: "- *x*"}
	b := &domain.Task{ID: "S001", Title: "_A_ task", Objective: "Do _it_.", AcceptanceCriteria: "- _x_"}
	if !sameTaskDefinition(a, b) {
		t.Error("emphasis folding must be deterministic without an agent")
	}
	for i := 0; i < 5; i++ {
		if canonicalProse(a.Title) != canonicalProse(b.Title) {
			t.Fatal("emphasis folding is not deterministic")
		}
	}
}

// TestInspectPureEmphasisExecutedChangeIsEquivalent proves the Inspect path
// classifies an executed task changed ONLY by emphasis markers as
// ChangedExecutedEquivalent (not material). No agent is configured.
func TestInspectPureEmphasisExecutedChangeIsEquivalent(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	markExecuted(taskByID(t, st.tasks, "S001"))

	// Change the objective prose only, by emphasis markers. The objective is
	// descriptive text, so the change is semantics-preserving (equivalent).
	emphasised := strings.Replace(planDoc, "Create it.", "Create *it*.", 1)
	write(t, dir, "PLAN.md", emphasised)

	res, err := Inspect(context.Background(), ReconcileOptions{
		Dir:        dir,
		PlanSource: filepath.Join(dir, "PLAN.md"),
		Store:      st,
	})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(res.ChangedExecuted) != 1 || res.ChangedExecuted[0] != "S001" {
		t.Fatalf("ChangedExecuted = %v, want [S001]", res.ChangedExecuted)
	}
	if len(res.ChangedExecutedEquivalent) != 1 || res.ChangedExecutedEquivalent[0] != "S001" {
		t.Errorf("ChangedExecutedEquivalent = %v, want [S001]", res.ChangedExecutedEquivalent)
	}
	if len(res.ChangedExecutedMaterial) != 0 {
		t.Errorf("ChangedExecutedMaterial = %v, want none", res.ChangedExecutedMaterial)
	}
}

// TestInspectSubstantiveExecutedChangeIsMaterial proves the Inspect path classifies
// a substantive executed change as ChangedExecutedMaterial, so a real change is
// never folded away as cosmetic.
func TestInspectSubstantiveExecutedChangeIsMaterial(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	markExecuted(taskByID(t, st.tasks, "S001"))

	// A changed acceptance criterion: an executable-semantics change.
	changed := strings.Replace(planDoc, "- starts", "- starts cleanly", 1)
	write(t, dir, "PLAN.md", changed)

	res, err := Inspect(context.Background(), ReconcileOptions{
		Dir:        dir,
		PlanSource: filepath.Join(dir, "PLAN.md"),
		Store:      st,
	})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(res.ChangedExecutedMaterial) != 1 || res.ChangedExecutedMaterial[0] != "S001" {
		t.Errorf("ChangedExecutedMaterial = %v, want [S001]", res.ChangedExecutedMaterial)
	}
	if len(res.ChangedExecutedEquivalent) != 0 {
		t.Errorf("ChangedExecutedEquivalent = %v, want none", res.ChangedExecutedEquivalent)
	}
}
