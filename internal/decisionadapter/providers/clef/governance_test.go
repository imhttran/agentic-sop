package clef

// governance_test.go proves, offline and without contacting any backend, that
// the Clef adapter's provider output carries no governance authority. Clef only
// translates and normalizes; it never decides CONTINUE/BLOCK/APPROVE/REJECT/
// COMMIT/MERGE, never performs a lifecycle transition, and never performs an
// approval transition. Approval gates, execution thresholds, and safety
// enforcement belong to agentic-sop (SOP), not to this adapter.
//
// These tests also assert provider neutrality: the public decision package must
// contain no Clef-specific branch or identifier and must not gain a
// provider-specific type from this adapter.

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/decision"
)

// authoritativeGovernanceVocabulary enumerates the governance-authority words
// that must never appear as authoritative decisions in provider output:
// CONTINUE/BLOCK/APPROVE/REJECT/COMMIT/MERGE.
var authoritativeGovernanceVocabulary = []string{
	"CONTINUE",
	"BLOCK",
	"APPROVE",
	"REJECT",
	"COMMIT",
	"MERGE",
}

// lifecycleTransitionIdentifiers are the lifecycle/approval transition verbs a
// governance authority would expose on provider output. Provider output must
// carry none of them: transitions are SOP-owned.
var lifecycleTransitionIdentifiers = []string{
	"transition",
	"lifecycle",
	"promote",
	"rollback",
	"advance",
	"approval",
	"approve",
	"reject",
	"block",
	"continue",
	"commit",
	"merge",
}

// clefIdentifiers are the Clef-specific identifiers that must not leak into
// the public decision package.
var clefIdentifiers = []string{
	"clef",
	"Clef",
	"CLEF",
	"SystemOne",
	"systemOne",
	"systemone",
	"noul",
	"Noul",
	"NOUL",
	"clef-flash",
}

// exportedFieldNames returns the exported field names of a struct value.
func exportedFieldNames(v any) []string {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Struct {
		return nil
	}
	var names []string
	for i := 0; i < rv.NumField(); i++ {
		f := rv.Type().Field(i)
		if f.PkgPath != "" {
			continue // unexported
		}
		names = append(names, f.Name)
	}
	sort.Strings(names)
	return names
}

// questionByID returns the requested question with the given ID, or nil.
func questionByID(req decision.DecisionRequest, id string) *decision.Question {
	for i := range req.Questions {
		if req.Questions[i].ID == id {
			return &req.Questions[i]
		}
	}
	return nil
}

// assertNoGovernanceAuthorityOnResult asserts a successful DecisionResult is
// ordinary normalized data: it carries no authoritative governance verdict,
// no lifecycle transition, and no approval transition field or value.
func assertNoGovernanceAuthorityOnResult(t *testing.T, r decision.DecisionResult) {
	t.Helper()

	for id, answer := range r.Answers {
		switch answer.Type {
		case decision.AnswerChoice, decision.AnswerBoolean, decision.AnswerScore:
		default:
			t.Errorf("answer %q has non-neutral type %q", id, answer.Type)
		}
		if answer.Probability < 0 || answer.Probability > 1 {
			t.Errorf("answer %q: probability %v out of [0,1]", id, answer.Probability)
		}
		if answer.Confidence != nil && (*answer.Confidence < 0 || *answer.Confidence > 1) {
			t.Errorf("answer %q: confidence %v out of [0,1]", id, *answer.Confidence)
		}
	}

	// The neutral DecisionResult surface must not have grown an authority
	// field.
	for _, field := range exportedFieldNames(r) {
		for _, word := range authoritativeGovernanceVocabulary {
			if strings.EqualFold(field, word) {
				t.Errorf("DecisionResult exposes authoritative field %q", field)
			}
		}
		for _, word := range lifecycleTransitionIdentifiers {
			if strings.EqualFold(field, word) {
				t.Errorf("DecisionResult exposes lifecycle/approval transition field %q", field)
			}
		}
	}
}

// TestProviderOutputCarriesNoGovernanceAuthority drives a fully stubbed Clef
// provider (no backend contact) and asserts the produced DecisionResult is
// ordinary provider output with no authoritative CONTINUE/BLOCK/APPROVE/REJECT/
// COMMIT/MERGE verdict, no lifecycle transition, and no approval transition.
func TestProviderOutputCarriesNoGovernanceAuthority(t *testing.T) {
	p := New(enabledConfig(), stubTransport{resp: systemOneResponse{
		Model:   "clef-flash",
		Answers: baseAnswers(),
		Usage:   systemOneUsage{InputTokens: 1, OutputTokens: 1},
	}})

	got, err := p.Decide(context.Background(), multiRequest())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	assertNoGovernanceAuthorityOnResult(t, got)

	if got.Provider != ProviderName {
		t.Errorf("Provider = %q, want %q", got.Provider, ProviderName)
	}
	// No answer choice may escape the caller-supplied request options.
	for id, answer := range got.Answers {
		if answer.Type != decision.AnswerChoice {
			continue
		}
		q := questionByID(multiRequest(), id)
		if q == nil || !q.Allows(answer.Choice) {
			t.Errorf("answer %q returned choice %q outside the requested options", id, answer.Choice)
		}
	}
}

// TestLiteralDomainDataRemainsOrdinaryData asserts a caller may use a
// governance-sounding literal (for example a choice named "BLOCK") as ordinary
// domain data: the adapter returns it as a plain normalized choice and does not
// reinterpret it as an authority transition.
func TestLiteralDomainDataRemainsOrdinaryData(t *testing.T) {
	req := decision.DecisionRequest{
		State: "reviewing a deployment",
		Questions: []decision.Question{{
			ID:       "verdict",
			Type:     decision.QuestionChoice,
			Criteria: "pick the literal label",
			Choices:  []string{"APPROVE", "BLOCK"},
		}},
	}
	answers := map[string]systemOneAnswer{
		"verdict": {Type: systemOneTypeChoice, Choice: "BLOCK"},
	}
	p := New(enabledConfig(), stubTransport{resp: systemOneResponse{Model: "clef-flash", Answers: answers}})

	got, err := p.Decide(context.Background(), req)
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	answer := got.Answers["verdict"]
	if answer.Type != decision.AnswerChoice {
		t.Fatalf("answer type = %q, want %q", answer.Type, decision.AnswerChoice)
	}
	if answer.Choice != "BLOCK" {
		t.Errorf("choice = %q, want %q as ordinary data", answer.Choice, "BLOCK")
	}
	if err := got.Validate(); err != nil {
		t.Errorf("result did not validate as ordinary data: %v", err)
	}
}

// TestProviderFailureCarriesNoGovernanceAuthority asserts failure output is a
// normalized decision.ProviderError with a neutral error kind only.
func TestProviderFailureCarriesNoGovernanceAuthority(t *testing.T) {
	cases := map[string]error{
		"malformed response": &malformedResponseError{Err: errors.New("decode")},
		"transport failure":  errors.New("backend exploded"),
		"unsupported":        ErrUnsupported,
	}
	for name, cause := range cases {
		t.Run(name, func(t *testing.T) {
			p := New(enabledConfig(), stubTransport{err: cause})
			got, err := p.Decide(context.Background(), choiceRequest())
			if err == nil {
				t.Fatalf("Decide() error = nil, want failure")
			}
			if len(got.Answers) != 0 {
				t.Errorf("Answers = %+v, want empty on failure", got.Answers)
			}

			var perr *decision.ProviderError
			if !errors.As(err, &perr) {
				t.Fatalf("error = %v, want *decision.ProviderError", err)
			}
			switch perr.Kind {
			case decision.KindInvalidRequest, decision.KindUnavailable,
				decision.KindMalformedResponse, decision.KindProviderFailure:
			default:
				t.Errorf("Kind = %q, want a neutral error kind", perr.Kind)
			}
		})
	}
}

// TestDisabledProviderFailsClosedWithNoAuthority asserts a disabled Clef
// provider never yields a governance verdict.
func TestDisabledProviderFailsClosedWithNoAuthority(t *testing.T) {
	p := New(Config{}, stubTransport{resp: systemOneResponse{Answers: baseAnswers()}})
	got, err := p.Decide(context.Background(), choiceRequest())
	if err == nil {
		t.Fatal("Decide() error = nil, want disabled failure")
	}
	assertProviderErrorKind(t, err, decision.KindUnavailable)
	assertNoSuccess(t, got)
}

// TestCapabilityCarriesNoGovernanceAuthority asserts Capability output is
// provider-neutral: it reports adapter name/enabled/available and unsupported
// operations only, never an authoritative verdict or a transition.
func TestCapabilityCarriesNoGovernanceAuthority(t *testing.T) {
	enabled := New(enabledConfig(), stubTransport{}).Capability(context.Background())
	disabled := New(Config{}, stubTransport{}).Capability(context.Background())

	for name, cap := range map[string]Capability{"enabled": enabled, "disabled": disabled} {
		if cap.Name != ProviderName {
			t.Errorf("%s: Name = %q, want %q", name, cap.Name, ProviderName)
		}
		if name == "disabled" && (cap.Enabled || cap.Available) {
			t.Errorf("disabled capability claimed Enabled=%v Available=%v, want false", cap.Enabled, cap.Available)
		}
		for _, op := range cap.UnsupportedOperations {
			for _, word := range authoritativeGovernanceVocabulary {
				if strings.EqualFold(op, word) {
					t.Errorf("unsupported operation %q is an authority verdict", op)
				}
			}
			for _, word := range lifecycleTransitionIdentifiers {
				if strings.EqualFold(op, word) {
					t.Errorf("unsupported operation %q is a lifecycle/approval transition", op)
				}
			}
		}
		for _, field := range exportedFieldNames(cap) {
			for _, word := range authoritativeGovernanceVocabulary {
				if strings.EqualFold(field, word) {
					t.Errorf("Capability exposes authoritative field %q", field)
				}
			}
			for _, word := range lifecycleTransitionIdentifiers {
				if strings.EqualFold(field, word) {
					t.Errorf("Capability exposes lifecycle/approval transition field %q", field)
				}
			}
		}
	}
}

// decisionPackageDir locates the public decision package relative to this test
// file.
func decisionPackageDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	// internal/providers/clef -> repository root -> decision
	root := filepath.Clean(filepath.Join(dir, "..", "..", ".."))
	return filepath.Join(root, "decision")
}

// TestDecisionPackageIsProviderNeutral asserts the public decision package
// contains no Clef-specific identifier or branch. It parses the decision
// package sources and rejects any Clef-specific token in identifiers or
// literals: the neutral layer must not know that Clef exists.
func TestDecisionPackageIsProviderNeutral(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, decisionPackageDir(t), nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse decision package: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatal("no decision package parsed")
	}

	checked := 0
	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			if strings.HasSuffix(name, "_test.go") {
				continue // neutrality contract is over the package itself
			}
			checked++
			for _, id := range clefIdentifiers {
				found := ""
				ast.Inspect(file, func(n ast.Node) bool {
					if found != "" {
						return false
					}
					switch v := n.(type) {
					case *ast.Ident:
						if strings.Contains(v.Name, id) {
							found = v.Name
						}
					case *ast.BasicLit:
						if strings.Contains(v.Value, id) {
							found = v.Value
						}
					}
					return true
				})
				if found != "" {
					t.Errorf("decision file %s contains Clef-specific token %q in %q", filepath.Base(name), id, found)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no non-test decision package files inspected")
	}
}

// TestDecisionPackageHasNoProviderSpecificType asserts the public decision
// package declares no provider-specific type. Every declared type in the
// package must be provider-neutral.
func TestDecisionPackageHasNoProviderSpecificType(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, decisionPackageDir(t), nil, 0)
	if err != nil {
		t.Fatalf("parse decision package: %v", err)
	}

	checked := 0
	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			for _, decl := range file.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.TYPE {
					continue
				}
				for _, spec := range gd.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					checked++
					for _, id := range clefIdentifiers {
						if strings.Contains(ts.Name.Name, id) {
							t.Errorf("decision package declares provider-specific type %q", ts.Name.Name)
						}
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no decision package types inspected")
	}
}

// TestDecisionPackageHasNoClefBranch asserts the public decision package has no
// Clef-specific branch: no selector, comparison, or string literal keyed on a
// Clef identifier.
func TestDecisionPackageHasNoClefBranch(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, decisionPackageDir(t), nil, 0)
	if err != nil {
		t.Fatalf("parse decision package: %v", err)
	}

	checked := 0
	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			checked++
			for _, id := range clefIdentifiers {
				ast.Inspect(file, func(n ast.Node) bool {
					switch v := n.(type) {
					case *ast.SelectorExpr:
						if strings.Contains(v.Sel.Name, id) {
							t.Errorf("decision file %s has Clef branch selector %q", filepath.Base(name), v.Sel.Name)
						}
					case *ast.Ident:
						if strings.Contains(v.Name, id) {
							t.Errorf("decision file %s has Clef branch identifier %q", filepath.Base(name), v.Name)
						}
					}
					return true
				})
			}
		}
	}
	if checked == 0 {
		t.Fatal("no non-test decision package files inspected")
	}
}
