package recovery

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/model"
)

func TestDiscoveryOnlyAttemptIsNoProgress(t *testing.T) {
	cases := []AttemptEvidence{
		{Kind: failure.IncompleteImplementation, DiscoveryOnly: true},
		{Kind: failure.NoProgress, RepoChanged: true},
		{Kind: failure.TestFailure},
	}
	for _, ev := range cases {
		for range 2 { // deterministic
			if got := Classify(ev); got != ImplementationNoProgress {
				t.Fatalf("Classify(%+v) = %s, want %s", ev, got, ImplementationNoProgress)
			}
		}
	}
	if got := Classify(AttemptEvidence{Kind: failure.TestFailure, RepoChanged: true}); got != Productive {
		t.Fatalf("repo change = %s, want productive", got)
	}
	if got := Classify(AttemptEvidence{MissingPrerequisite: "cargo", RepoChanged: true}); got != MissingPrerequisite {
		t.Fatalf("missing prerequisite = %s", got)
	}
}

func TestPermissionDenialIsAuthorizationFailure(t *testing.T) {
	p := Policy{Enabled: true, MaxEscalations: 2, Replan: true, MaxReplans: 1}
	cases := []AttemptEvidence{
		{Kind: failure.TestFailure, DeniedTools: 1, DiscoveryOnly: true},
		{Kind: failure.ApprovalRequired, RepoChanged: true},
		{Kind: failure.SecurityBoundary, MissingPrerequisite: "go"},
	}
	for _, ev := range cases {
		if got := Classify(ev); got != AuthorizationFailure {
			t.Fatalf("Classify(%+v) = %s, want authorization", ev, got)
		}
		base := Evidence{Class: model.ClassSmall, Kind: ev.Kind, Attempt: 1}
		d, rec := Recover(p, ev, "", base, Budget{Max: 4})
		if d.Action != ActionHuman || d.Reason != ReasonAuthorization || rec.Class != AuthorizationFailure {
			t.Fatalf("Recover(%+v) = %+v, want human authorization", ev, d)
		}
	}
}

func TestUnchangedFailureNotRetried(t *testing.T) {
	ev := AttemptEvidence{Kind: failure.TransientProvider, Signature: "sig-a"}
	base := Evidence{Class: model.ClassSmall, Kind: failure.TransientProvider, Attempt: 2}
	for _, p := range []Policy{{}, {Enabled: true, MaxEscalations: 2}} {
		d, rec := Recover(p, ev, "sig-a", base, Budget{Max: 4})
		if d.Action != ActionHuman || d.Reason != ReasonUnchangedFailure || rec.Change != "" {
			t.Fatalf("policy %+v: unchanged failure = %+v, want human unchanged", p, d)
		}
	}
	// A changed signature is new evidence and may be retried.
	d, rec := Recover(Policy{Enabled: true, MaxEscalations: 2}, ev, "sig-b", base, Budget{Max: 4})
	if d.Action != ActionRetrySame || rec.Change != ChangeNewEvidence {
		t.Fatalf("changed failure = %+v %+v, want retry_same with new evidence", d, rec)
	}
	// Repeated discovery with no repository change gets ONE bounded checkpoint
	// recovery, then a second stop is refused.
	disc := AttemptEvidence{Kind: failure.IncompleteImplementation, DiscoveryOnly: true, Signature: "sig-c"}
	d, rec = Recover(Policy{Enabled: true, MaxEscalations: 2}, disc, "", Evidence{Class: model.ClassSmall, Kind: disc.Kind}, Budget{Max: 4})
	if d.Action != ActionRetrySame || d.Reason != ReasonResumeCheckpoint || rec.Change != ChangeResumeCheckpoint {
		t.Fatalf("first discovery-only = %+v %+v, want one checkpoint retry", d, rec)
	}
	disc.PriorRecoveries = 1
	d, _ = Recover(Policy{Enabled: true, MaxEscalations: 2}, disc, "", Evidence{Class: model.ClassSmall, Kind: disc.Kind}, Budget{Max: 4})
	if d.Action != ActionHuman || d.Reason != ReasonNoMaterialChange {
		t.Fatalf("second discovery-only = %+v, want human no material change", d)
	}
	// An unchanged signature is never retried, even on the first recovery.
	d, _ = Recover(Policy{Enabled: true, MaxEscalations: 2}, disc, "sig-c", Evidence{Class: model.ClassSmall, Kind: disc.Kind}, Budget{Max: 4})
	if d.Action != ActionHuman || d.Reason != ReasonUnchangedFailure {
		t.Fatalf("unchanged discovery-only = %+v, want human unchanged", d)
	}
}

func TestRecoveryRecordsReasonAndChange(t *testing.T) {
	cases := []struct {
		p    Policy
		ev   AttemptEvidence
		prev string
		want Action
	}{
		{Policy{Enabled: true, MaxEscalations: 2}, AttemptEvidence{Kind: failure.TestFailure, Signature: "s"}, "s", ActionEscalate},
		{Policy{Replan: true, MaxReplans: 1}, AttemptEvidence{Kind: failure.TestFailure, Signature: "s"}, "s", ActionReplan},
		{Policy{Enabled: true, MaxEscalations: 2}, AttemptEvidence{Kind: failure.TransientProvider, Signature: "s"}, "t", ActionRetrySame},
		{Policy{}, AttemptEvidence{Kind: failure.IncompleteImplementation, RepoChanged: true}, "", ActionRetrySame},
	}
	for _, c := range cases {
		base := Evidence{Class: model.ClassSmall, Kind: c.ev.Kind, Attempt: 1}
		d, rec := Recover(c.p, c.ev, c.prev, base, Budget{Max: 4})
		if d.Action != c.want {
			t.Fatalf("%+v: action = %s, want %s", c.ev, d.Action, c.want)
		}
		if rec.Reason == "" || rec.Change == "" || rec.Action != d.Action {
			t.Fatalf("%+v: recovery %+v lacks a reason or change", c.ev, rec)
		}
	}
}

func TestProductiveContinuationAllowed(t *testing.T) {
	ev := AttemptEvidence{Kind: failure.IncompleteImplementation, RepoChanged: true, Signature: "s"}
	base := Evidence{Class: model.ClassMedium, Kind: ev.Kind, Attempt: 1}
	d, rec := Recover(Policy{}, ev, "s", base, Budget{Used: 1, Max: 4})
	if d.Action != ActionRetrySame || d.Reason != ReasonContinuation || rec.Change != ChangeProgress {
		t.Fatalf("productive = %+v %+v, want continuation", d, rec)
	}
	d, _ = Recover(Policy{}, ev, "s", base, Budget{Used: 4, Max: 4})
	if d.Action != ActionHuman || d.Reason != ReasonContinuationLimit {
		t.Fatalf("spent budget = %+v, want continuation limit", d)
	}

	// A productive attempt that also crosses an authority boundary must not
	// auto-continue: Decide's safety/fail-closed checks run first.
	boundary := AttemptEvidence{Kind: failure.AmbiguousContract, RepoChanged: true, Signature: "s"}
	d, _ = Recover(Policy{}, boundary, "s", Evidence{Class: model.ClassMedium, Kind: boundary.Kind}, Budget{Used: 0, Max: 4})
	if d.Action != ActionHuman {
		t.Fatalf("productive boundary = %+v, want human", d)
	}
}

func TestFallbackRespectsRouting(t *testing.T) {
	p := Policy{Enabled: true, MaxEscalations: 2}
	small := model.Result{Active: true, Selection: model.Selection{Class: model.ClassSmall}}

	sel, rec, ok := Fallback(p, ImplementationNoProgress, small, "", "", false, 0)
	if !ok || sel.Class != model.ClassMedium || rec.Change != ChangeClass {
		t.Fatalf("next class = %+v %+v %v, want medium", sel, rec, ok)
	}
	// Never below the router/adaptiveroute baseline.
	if sel, _, ok := Fallback(p, ImplementationNoProgress, small, model.ClassLarge, "", false, 0); !ok || sel.Class != model.ClassLarge {
		t.Fatalf("baseline = %+v %v, want large", sel, ok)
	}
	// An operator override is kept.
	if _, rec, ok := Fallback(p, ImplementationNoProgress, small, "", model.ClassSmall, false, 0); ok || rec.Reason != ReasonOverrideKept {
		t.Fatalf("override = %+v %v, want refusal", rec, ok)
	}
	// The escalation budget is respected.
	if _, rec, ok := Fallback(p, ImplementationNoProgress, small, "", "", false, 2); ok || rec.Reason != ReasonEscalationLimit {
		t.Fatalf("budget = %+v %v, want refusal", rec, ok)
	}
	// Local-to-cloud requires permission.
	cloud := model.Selection{Class: model.ClassSmall, Locality: model.LocalityCloud, Fallback: true}
	local := model.Result{Active: true, Selection: model.Selection{Class: model.ClassSmall, Locality: model.LocalityLocal}, LocalFallback: &cloud}
	if _, rec, ok := Fallback(p, MissingPrerequisite, local, "", "", false, 0); ok || rec.Reason != ReasonCloudRefused {
		t.Fatalf("cloud refused = %+v %v", rec, ok)
	}
	if sel, rec, ok := Fallback(p, MissingPrerequisite, local, "", "", true, 0); !ok || sel != cloud || rec.Change != ChangeLocalFallback {
		t.Fatalf("cloud allowed = %+v %+v %v", sel, rec, ok)
	}
	// A baseline outside the class ladder can never become a fallback class.
	if _, rec, ok := Fallback(p, ImplementationNoProgress, small, model.Class("xl"), "", false, 0); ok || rec.Reason != ReasonNoLargerClass {
		t.Fatalf("invalid baseline = %+v %v, want refusal", rec, ok)
	}
	// Never after an authorization failure.
	if _, rec, ok := Fallback(p, AuthorizationFailure, local, "", "", true, 0); ok || rec.Reason != ReasonAuthorization {
		t.Fatalf("authorization = %+v %v, want refusal", rec, ok)
	}
}

func TestExhaustionSingleBlockerReport(t *testing.T) {
	step := Recovery{Attempt: 1, Class: ImplementationNoProgress, Action: ActionEscalate, Reason: ReasonFallbackClass, Change: ChangeClass}
	last := Recovery{Attempt: 2, Class: ImplementationNoProgress, Action: ActionHuman, Reason: ReasonUnchangedFailure}
	b := Budgets{Escalations: Budget{Used: 1, Max: 1}, Continuations: Budget{Used: 0, Max: 4}}

	bs := Blockers{}
	for range 3 {
		r := Exhausted("SOP-004", []Recovery{step, last, last}, ImplementationNoProgress, b)
		bs.Record(r)
	}
	if len(bs) != 1 {
		t.Fatalf("reports = %d, want 1", len(bs))
	}
	r := bs["SOP-004"]
	if r.NextAction != NextChangeApproach || r.LastReason != ReasonUnchangedFailure || len(r.History) != 2 || r.Budgets != b {
		t.Fatalf("report = %+v", r)
	}
	for class, want := range map[AttemptClass]string{
		AuthorizationFailure: NextGrantPermission,
		MissingPrerequisite:  NextSupplyPrerequisite,
		Productive:           NextReviewContinuations,
	} {
		if r := Exhausted("t", nil, class, Budgets{}); r.NextAction != want || r.LastReason == "" {
			t.Fatalf("%s: report = %+v", class, r)
		}
	}
}
