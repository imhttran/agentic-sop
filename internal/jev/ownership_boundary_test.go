package jev

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// Ownership/security boundary tests (P3-014).
//
// These tests pin the security property the JEV boundary exists to guarantee:
// JEV is a non-owner analysis capability. It cannot transition task state,
// mutate SOP persistence, mark validation/review successful, or bypass a
// human/quality gate. The property is asserted structurally (the boundary
// surface exposes no mutation/transition/gate affordance) and behaviorally
// (the deterministic fake cannot mutate its input and never fabricates a pass).
//
// All tests here are deterministic and side-effect free: the fakes read no
// time, randomness, environment, files, or network, so they are race-safe.

// mutationAffordanceNames are method names that would let JEV own SOP lifecycle
// state. Their presence on the boundary surface would violate the ownership
// boundary. The set is intentionally broad: it names the transition,
// persistence, gate, and Git/PR/merge authorities JEV must never hold.
var mutationAffordanceNames = map[string]bool{
	"Commit":     true,
	"Push":       true,
	"Merge":      true,
	"PR":         true,
	"Pull":       true,
	"Save":       true,
	"Persist":    true,
	"Store":      true,
	"Write":      true,
	"Update":     true,
	"Transition": true,
	"Approve":    true,
	"Reject":     true,
	"Decline":    true,
	"Pass":       true,
	"Fail":       true,
	"Success":    true,
	"Complete":   true,
	"SetState":   true,
	"SetStatus":  true,
}

// TestAnalyzerBoundaryExposesOnlyAnalyze is the core structural security
// assertion: the Analyzer interface — the ONLY way SOP talks to JEV — declares
// exactly one method, Analyze, with exactly the signature
// Analyze(context.Context, Request) (Result, error). Any other method (or any
// other signature) could hand JEV an authority SOP owns.
func TestAnalyzerBoundaryExposesOnlyAnalyze(t *testing.T) {
	iface := reflect.TypeOf((*Analyzer)(nil)).Elem()
	if iface.Kind() != reflect.Interface {
		t.Fatalf("Analyzer kind = %s, want interface", iface.Kind())
	}
	if iface.NumMethod() != 1 {
		names := make([]string, 0, iface.NumMethod())
		for i := 0; i < iface.NumMethod(); i++ {
			names = append(names, iface.Method(i).Name)
		}
		t.Fatalf("Analyzer declares %d methods %v, want exactly Analyze", iface.NumMethod(), names)
	}

	m := iface.Method(0)
	if m.Name != "Analyze" {
		t.Fatalf("Analyzer method = %q, want Analyze", m.Name)
	}
	// reflect.Type.Method reports the method type WITHOUT a receiver, so the
	// interface method takes exactly (context.Context, Request).
	if m.Type.NumIn() != 2 {
		t.Fatalf("Analyze takes %d inputs, want 2 (context.Context, Request)", m.Type.NumIn())
	}
	if got := m.Type.In(0); got.String() != "context.Context" {
		t.Errorf("Analyze first parameter = %s, want context.Context", got)
	}
	if got := m.Type.In(1); got != reflect.TypeOf(Request{}) {
		t.Errorf("Analyze request parameter = %s, want jev.Request", got)
	}
	if m.Type.NumOut() != 2 {
		t.Fatalf("Analyze returns %d values, want 2 (Result, error)", m.Type.NumOut())
	}
	if got := m.Type.Out(0); got != reflect.TypeOf(Result{}) {
		t.Errorf("Analyze first result = %s, want jev.Result", got)
	}
	if got := m.Type.Out(1); got.String() != "error" {
		t.Errorf("Analyze second result = %s, want error", got)
	}
}

// TestAnalyzerSurfaceHasNoMutationAffordance asserts the boundary's exported
// method set carries no name that would grant JEV lifecycle authority.
func TestAnalyzerSurfaceHasNoMutationAffordance(t *testing.T) {
	iface := reflect.TypeOf((*Analyzer)(nil)).Elem()
	for i := 0; i < iface.NumMethod(); i++ {
		name := iface.Method(i).Name
		if mutationAffordanceNames[name] {
			t.Errorf("Analyzer exposes mutating method %q; JEV must not own lifecycle state", name)
		}
	}
}

// forbiddenFieldTypes are types that would smuggle an ownership/handle into the
// boundary's data: SOP runtime, persistence, state store, gate, git/PR client.
var forbiddenFieldTypes = map[string]bool{
	"*store.Store":   true,
	"store.Store":    true,
	"*run.Run":       true,
	"run.Run":        true,
	"*config.Config": true,
	"config.Config":  true,
	"*git.Git":       true,
	"git.Git":        true,
	"*quality.Gate":  true,
	"quality.Gate":   true,
}

// TestBoundaryDataCarriesNoOwnershipHandle asserts the boundary's data types —
// Request, Result, Finding, and the structured Evidence — carry no field of a
// runtime/persistence/state/gate type. JEV receives read-only snapshots, never a
// handle it could use to mutate task state or bypass a gate.
func TestBoundaryDataCarriesNoOwnershipHandle(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(Request{}),
		reflect.TypeOf(Result{}),
		reflect.TypeOf(Finding{}),
		reflect.TypeOf(Evidence{}),
		reflect.TypeOf(EvidenceItem{}),
	} {
		assertNoOwnershipHandle(t, typ, typ.String())
	}
}

// assertNoOwnershipHandle walks a type recursively, reporting any field whose
// type names an ownership handle. A pointer, slice, or map of such a type is a
// handle too, so containers are unwrapped.
func assertNoOwnershipHandle(t *testing.T, typ reflect.Type, path string) {
	t.Helper()
	name := typ.String()
	if forbiddenFieldTypes[name] {
		t.Errorf("boundary type %s carries an ownership handle %s", path, name)
		return
	}
	switch typ.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Array:
		assertNoOwnershipHandle(t, typ.Elem(), path+".*")
	case reflect.Map:
		assertNoOwnershipHandle(t, typ.Key(), path+"[key]")
		assertNoOwnershipHandle(t, typ.Elem(), path+"[value]")
	case reflect.Struct:
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if !f.IsExported() {
				continue
			}
			assertNoOwnershipHandle(t, f.Type, path+"."+f.Name)
		}
	}
}

// TestFakeDoesNotMutateRequest proves the fake's Analyze is a pure read: it
// returns without mutating the caller's bounded context, so JEV cannot mutate
// its input (and thus cannot reach through it to SOP state). It also records the
// request as a value snapshot, so tests can assert on the read-only context JEV
// received without the boundary handing over a live handle.
func TestFakeDoesNotMutateRequest(t *testing.T) {
	fake := NewClearFake()
	req := Request{
		Purpose:           PurposeQuality,
		Task:              "task",
		Criteria:          "criteria",
		ChangedFiles:      []string{"a.go", "b.go"},
		RepositoryContext: "diff",
		ValidationResult:  "ok",
		ReviewResult:      "clean",
	}
	before := deepCopyRequest(req)

	if _, err := fake.Analyze(context.Background(), req); err != nil {
		t.Fatalf("Analyze error: %v", err)
	}

	// The caller's Request is unchanged: Analyze is a read, not a mutation of
	// SOP-owned context.
	if !reflect.DeepEqual(req, before) {
		t.Errorf("Analyze mutated its input Request:\n got %+v\nwant %+v", req, before)
	}
	if len(fake.Requests) != 1 {
		t.Fatalf("recorded %d requests, want 1", len(fake.Requests))
	}
	if !reflect.DeepEqual(fake.Requests[0], req) {
		t.Errorf("recorded snapshot = %+v, want %+v", fake.Requests[0], req)
	}
}

// deepCopyRequest returns a value-identical copy of req, with a copied
// ChangedFiles slice, so a mutation can be detected without aliasing.
func deepCopyRequest(req Request) Request {
	cp := req
	cp.ChangedFiles = append([]string(nil), req.ChangedFiles...)
	return cp
}

// TestFakeScenariosNeverFabricatePass drives every deterministic scenario and
// asserts the security-relevant invariants: only the clear scenario yields a
// valid PASS; a provider failure returns an error (never a finding, never a
// pass); the scope/destructive scenarios report findings; and no scenario ever
// produces a *valid* PASS except clear. The malformed scenario may carry a PASS
// status literal, but the fail-closed validator must reject it, so a status
// literal is never a gate bypass.
func TestFakeScenariosNeverFabricatePass(t *testing.T) {
	cases := []struct {
		name    string
		fake    *FakeAnalyzer
		wantErr bool
	}{
		{name: "clear", fake: NewClearFake()},
		{name: "ambiguous", fake: NewAmbiguousFake()},
		{name: "scope-concern", fake: NewScopeConcernFake()},
		{name: "destructive-concern", fake: NewDestructiveConcernFake()},
		{name: "provider-failure", fake: NewProviderFailureFake(), wantErr: true},
		{name: "malformed", fake: NewMalformedFake()},
	}
	if len(cases) != len(Scenarios()) {
		t.Fatalf("scenario table has %d cases, want %d", len(cases), len(Scenarios()))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.fake.Analyze(context.Background(), Request{Task: "t"})
			if tc.wantErr {
				if err == nil {
					t.Fatal("provider failure must return an error, not a result")
				}
				if !errors.Is(err, ErrProviderFailure) {
					t.Errorf("error = %v, want ErrProviderFailure", err)
				}
				if res.Status == StatusPass || len(res.Findings) > 0 {
					t.Errorf("provider failure fabricated a result: %+v", res)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// The security invariant: only the clear scenario yields a VALID PASS.
			validPass := res.Validate() == nil && res.Status == StatusPass
			if want := tc.name == "clear"; validPass != want {
				t.Errorf("scenario %q valid-pass=%v, want %v (result=%+v)", tc.name, validPass, want, res)
			}
		})
	}
}

// TestScenarioPassStatusIsNotAGateBypass documents that a Scenario is a
// machine-readable identifier, not an authority: no scenario value alone is a
// lifecycle transition, and the malformed scenario is exactly the one
// fail-closed validation must reject. A PASS status literal that fails
// validation must never be treated as a pass — only a validated PASS is a pass.
func TestScenarioPassStatusIsNotAGateBypass(t *testing.T) {
	if len(Scenarios()) == 0 {
		t.Fatal("expected a non-empty scenario set")
	}
	// The malformed fake is built to fail closed: its result must be rejected,
	// never silently accepted as a pass.
	res, err := NewMalformedFake().Analyze(context.Background(), Request{})
	if err != nil {
		t.Fatalf("mock analyze error: %v", err)
	}
	if err := res.Validate(); err == nil {
		t.Fatal("malformed result validated; fail-closed boundary is broken")
	}
}
