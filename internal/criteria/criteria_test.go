package criteria

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixedNow() time.Time { return time.Unix(0, 0).UTC() }

// bindings builds an authorised binding set from the operator source.
func bindings(t *testing.T, pairs ...[2]string) []Binding {
	t.Helper()
	var b strings.Builder
	b.WriteString("version: 1\nbindings:\n")
	for _, p := range pairs {
		fmt.Fprintf(&b, "  - id: %q\n    command: %q\n", p[0], p[1])
	}
	bs, err := ParseBindings([]byte(b.String()))
	if err != nil {
		t.Fatalf("ParseBindings(%q): %v", b.String(), err)
	}
	return bs
}

func baseOpts(t *testing.T) Options {
	t.Helper()
	return Options{
		Dir: t.TempDir(), TaskID: "T1", Attempt: 1, AttemptID: "T1-a1",
		Revision: "rev-1", WorkspaceState: "clean", Now: fixedNow,
	}
}

func verifyOneCriterion(t *testing.T, criterion, command string) Outcome {
	t.Helper()
	o := baseOpts(t)
	o.Criteria = []string{criterion}
	o.Bindings = bindings(t, [2]string{NormalizeCriterion(criterion), command})
	ev, err := Verify(context.Background(), o)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(ev.Outcomes) != 1 {
		t.Fatalf("outcomes = %d, want 1", len(ev.Outcomes))
	}
	return ev.Outcomes[0]
}

func TestSuccessfulVerifierIsMet(t *testing.T) {
	o := verifyOneCriterion(t, "Application starts", "true")
	if o.State != Met {
		t.Fatalf("state = %s (%s), want MET", o.State, o.Reason)
	}
}

func TestExitCodeSuccessIndependentOfOutput(t *testing.T) {
	// `true` prints nothing; exit code alone is the default assertion.
	o := verifyOneCriterion(t, "Quiet", "true")
	if o.State != Met || o.ExitCode != 0 {
		t.Fatalf("state=%s exit=%d, want MET/0", o.State, o.ExitCode)
	}
}

func TestOutputAssertionMet(t *testing.T) {
	o := baseOpts(t)
	o.Criteria = []string{"Greeting"}
	o.Bindings = bindings(t, [2]string{"Greeting", "echo ok-marker"})
	// add an explicit assertion via a second parse
	bs, err := ParseBindings([]byte("version: 1\nbindings:\n  - id: \"Greeting\"\n    command: \"echo ok-marker\"\n    output_must_contain: \"ok-marker\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	o.Bindings = bs
	ev, _ := Verify(context.Background(), o)
	if ev.Outcomes[0].State != Met {
		t.Fatalf("state=%s (%s), want MET", ev.Outcomes[0].State, ev.Outcomes[0].Reason)
	}
}

func TestFailedVerifierIsNotMet(t *testing.T) {
	o := verifyOneCriterion(t, "Fails", "false")
	if o.State != NotMet || o.ExitCode == 0 {
		t.Fatalf("state=%s exit=%d, want NOT_MET/non-zero", o.State, o.ExitCode)
	}
}

func TestFailedAssertionIsNotMet(t *testing.T) {
	bs, err := ParseBindings([]byte("version: 1\nbindings:\n  - id: c\n    command: \"echo actual\"\n    output_must_contain: expected\n"))
	if err != nil {
		t.Fatal(err)
	}
	o := baseOpts(t)
	o.Criteria = []string{"c"}
	o.Bindings = bs
	ev, _ := Verify(context.Background(), o)
	if ev.Outcomes[0].State != NotMet {
		t.Fatalf("state=%s, want NOT_MET", ev.Outcomes[0].State)
	}
}

func TestMissingVerifierIsUnavailable(t *testing.T) {
	o := baseOpts(t)
	o.Criteria = []string{"Unbound"}
	o.Bindings = bindings(t, [2]string{"other", "true"})
	ev, _ := Verify(context.Background(), o)
	if ev.Outcomes[0].State != Unavailable {
		t.Fatalf("state=%s, want UNAVAILABLE", ev.Outcomes[0].State)
	}
	if ev.AllMetVerified() {
		t.Error("AllMetVerified must be false for a missing verifier")
	}
}

func TestMalformedBindingRejected(t *testing.T) {
	for _, doc := range []string{
		"version: 1\nbindings:\n  - id: x\n    command: \"\"\n",
		"version: 1\nbindings:\n  - criterion: \"\"\n    command: \"true\"\n",
		"version: 1\nbindings:\n  - id: x\n    command: \"true\"\n  - id: x\n    command: \"false\"\n",
	} {
		if _, err := ParseBindings([]byte(doc)); err == nil {
			t.Errorf("expected rejection:\n%s", doc)
		}
	}
}

func TestUnauthorizedCommandRejected(t *testing.T) {
	for _, cmd := range []string{"sh -c 'echo hi; rm -rf /'", "rm -rf $HOME", "cat *", "echo `id`"} {
		doc := "version: 1\nbindings:\n  - id: x\n    command: " + fmt.Sprintf("%q", cmd) + "\n"
		if _, err := ParseBindings([]byte(doc)); err == nil {
			t.Errorf("expected rejection of unauthorized command %q", cmd)
		}
	}
}

func TestWorkspaceEscapeRejected(t *testing.T) {
	for _, cmd := range []string{"cat ../etc/passwd", "cat /etc/passwd", "cat a/../b", "cat sub/.."} {
		doc := "version: 1\nbindings:\n  - id: x\n    command: " + fmt.Sprintf("%q", cmd) + "\n"
		if _, err := ParseBindings([]byte(doc)); err == nil {
			t.Errorf("expected rejection of workspace-escaping command %q", cmd)
		}
	}
}

func TestTimeoutIsUnavailable(t *testing.T) {
	o := baseOpts(t)
	o.Criteria = []string{"slow"}
	o.Bindings = bindings(t, [2]string{"slow", "sleep 5"})
	o.Timeout = 100 * time.Millisecond
	ev, _ := Verify(context.Background(), o)
	if ev.Outcomes[0].State != Unavailable {
		t.Fatalf("state=%s, want UNAVAILABLE", ev.Outcomes[0].State)
	}
}

func TestCancellationIsUnavailable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	o := baseOpts(t)
	o.Criteria = []string{"c"}
	o.Bindings = bindings(t, [2]string{"c", "true"})
	ev, _ := Verify(ctx, o)
	if ev.Outcomes[0].State != Unavailable {
		t.Fatalf("state=%s, want UNAVAILABLE", ev.Outcomes[0].State)
	}
}

func TestDuplicateBindingRuntimeIsUnavailable(t *testing.T) {
	o := baseOpts(t)
	o.Criteria = []string{"c"}
	o.Bindings = []Binding{
		{CriterionID: "c", Command: []string{"true"}, authorised: true},
		{CriterionID: "c", Command: []string{"false"}, authorised: true},
	}
	ev, _ := Verify(context.Background(), o)
	if ev.Outcomes[0].State != Unavailable {
		t.Fatalf("state=%s, want UNAVAILABLE for duplicate bindings", ev.Outcomes[0].State)
	}
}

func TestForgedOperatorSourceRefused(t *testing.T) {
	// A caller-constructed binding is inert: authorisation is provenance-based,
	// never a caller-set flag.
	o := baseOpts(t)
	o.Criteria = []string{"c"}
	o.Bindings = []Binding{{CriterionID: "c", Command: []string{"true"}}}
	ev, _ := Verify(context.Background(), o)
	if ev.Outcomes[0].State != Unavailable {
		t.Fatalf("state=%s, want UNAVAILABLE for a forged (unauthorised) binding", ev.Outcomes[0].State)
	}
}

func TestUnauthorizedExecutableIsUnavailable(t *testing.T) {
	o := verifyOneCriterion(t, "c", "definitely-not-a-real-binary-xyz")
	if o.State != Unavailable {
		t.Fatalf("state=%s, want UNAVAILABLE", o.State)
	}
}

func TestParseBindingsAreOperatorAuthorized(t *testing.T) {
	bs := bindings(t, [2]string{"app starts", "go test ./..."})
	if len(bs) != 1 || !bs[0].Authorized() || bs[0].CriterionID != "app starts" || bs[0].Digest() == "" {
		t.Fatalf("binding = %+v", bs)
	}
}

func TestLegacyCriterionCompatibility(t *testing.T) {
	// Legacy free-text criterion, normalized to a stable id.
	if NormalizeCriterion("  Application   Starts ") != "application starts" {
		t.Fatalf("normalization = %q", NormalizeCriterion("  Application   Starts "))
	}
	bs, err := ParseBindings([]byte("version: 1\nbindings:\n  - criterion: \"Application Starts\"\n    command: \"true\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if bs[0].CriterionID != "application starts" {
		t.Fatalf("legacy id = %q", bs[0].CriterionID)
	}
	o := baseOpts(t)
	o.Criteria = []string{"Application   starts"}
	o.Bindings = bs
	ev, _ := Verify(context.Background(), o)
	if ev.Outcomes[0].State != Met {
		t.Fatalf("legacy criterion did not match: %s", ev.Outcomes[0].State)
	}
}

func TestExplicitCriterionIDWins(t *testing.T) {
	bs, err := ParseBindings([]byte("version: 1\nbindings:\n  - id: \"criterion.app-starts\"\n    criterion: \"Application starts\"\n    command: \"true\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if bs[0].CriterionID != "criterion.app-starts" {
		t.Fatalf("explicit id = %q, want criterion.app-starts", bs[0].CriterionID)
	}
}

func TestEvidenceBindsContextAndRoundTrips(t *testing.T) {
	o := baseOpts(t)
	o.Workspace = "/ws/one"
	o.Criteria = []string{"app starts"}
	o.Bindings = bindings(t, [2]string{"app starts", "true"})
	ev, err := Verify(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if ev.TaskID != "T1" || ev.Attempt != 1 || ev.AttemptID != "T1-a1" || ev.Revision != "rev-1" || ev.Workspace != "/ws/one" {
		t.Fatalf("context not bound: %+v", ev)
	}
	if err := ev.Valid(); err != nil {
		t.Fatalf("Valid: %v", err)
	}
	if !ev.AllMetVerified() {
		t.Fatal("expected AllMetVerified")
	}
	runDir := filepath.Join(t.TempDir(), ".agent-sdlc", "runs", "T1")
	path, err := WriteEvidence(runDir, ev)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var back Evidence
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("evidence not valid JSON: %v", err)
	}
	if back.BindingsDigest != ev.BindingsDigest || len(back.Outcomes) != 1 || back.Outcomes[0].VerifierDigest == "" {
		t.Fatalf("round-trip = %+v", back)
	}
	if !json.Valid(data) {
		t.Fatal("evidence should be valid JSON")
	}
}

func TestBareMetEvidenceIsNotAccepted(t *testing.T) {
	// An artifact that merely contains "MET" but lacks identity/digests is invalid.
	bare := Evidence{
		Version:  EvidenceVersion,
		Outcomes: []Outcome{{CriterionID: "c", State: Met, Command: []string{"true"}, VerifierDigest: "d"}},
	}
	if err := bare.Valid(); err == nil {
		t.Fatal("expected Valid to reject evidence missing task/attempt/revision/workspace/bindings digest")
	}
	if bare.AllMetVerified() {
		t.Fatal("bare MET evidence must not be AllMetVerified")
	}
	noDigest := Evidence{
		Version: EvidenceVersion, TaskID: "T", AttemptID: "A", Revision: "r", Workspace: "/w", BindingsDigest: "b",
		Outcomes: []Outcome{{CriterionID: "c", State: Met, Command: []string{"true"}}},
	}
	if err := noDigest.Valid(); err == nil {
		t.Fatal("expected Valid to reject a MET outcome without a verifier digest")
	}
}

func TestFreshnessRejectsMismatches(t *testing.T) {
	o := baseOpts(t)
	o.Workspace = "/ws/one"
	o.Criteria = []string{"app starts"}
	o.Bindings = bindings(t, [2]string{"app starts", "true"})
	ev, _ := Verify(context.Background(), o)
	good := Freshness{TaskID: "T1", Attempt: 1, AttemptID: "T1-a1", Revision: "rev-1", Workspace: "/ws/one", WorkspaceState: "clean", BindingsDigest: ev.BindingsDigest}
	if !ev.Fresh(good) {
		t.Fatal("matching context must be fresh")
	}

	wrongTask := good
	wrongTask.TaskID = "T2"
	wrongAttempt := good
	wrongAttempt.Attempt = 2
	wrongAttemptID := good
	wrongAttemptID.AttemptID = "T1-a2"
	staleRev := good
	staleRev.Revision = "rev-2"
	staleWS := good
	staleWS.Workspace = "/ws/two"
	dirtyState := good
	dirtyState.WorkspaceState = "dirty"
	changedDef := good
	changedDef.BindingsDigest = "deadbeef"
	emptyExp := Freshness{}

	for _, tc := range []struct {
		name string
		exp  Freshness
	}{
		{"wrong task", wrongTask}, {"wrong attempt", wrongAttempt}, {"wrong attempt id", wrongAttemptID},
		{"stale revision", staleRev}, {"stale workspace", staleWS}, {"changed workspace state", dirtyState},
		{"changed verifier definition", changedDef}, {"empty expectation", emptyExp},
	} {
		if ev.Fresh(tc.exp) {
			t.Errorf("%s: evidence must not be fresh", tc.name)
		}
	}
}

func TestChangedVerifierDefinitionChangesDigest(t *testing.T) {
	a := bindings(t, [2]string{"c", "true"})
	b := bindings(t, [2]string{"c", "false"})
	if a[0].Digest() == b[0].Digest() {
		t.Fatal("different verifier definitions must have different digests")
	}
	if BindingsDigest(a) == BindingsDigest(b) {
		t.Fatal("different binding sets must have different aggregate digests")
	}
}

func TestAllMetSemantics(t *testing.T) {
	if (Evidence{}).AllMet() {
		t.Error("empty evidence must not be AllMet")
	}
	if !(Evidence{Outcomes: []Outcome{{State: Met}, {State: Met}}}).AllMet() {
		t.Error("all-MET must be AllMet")
	}
	if (Evidence{Outcomes: []Outcome{{State: Met}, {State: Unavailable}}}).AllMet() {
		t.Error("non-MET must not be AllMet")
	}
}

func TestSplitCommandReuseRejectsUnterminatedQuote(t *testing.T) {
	if _, err := ParseBindings([]byte("version: 1\nbindings:\n  - id: x\n    command: \"echo 'unterminated\"\n")); err == nil {
		t.Error("expected rejection of an unterminated quote (via the shared command policy)")
	}
	if !errors.Is(ErrNoBindings, ErrNoBindings) { // keep errors import used
		t.Fatal("unreachable")
	}
}
