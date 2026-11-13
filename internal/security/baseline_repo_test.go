package security

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// The committed baseline is an approved artifact. These tests pin exactly which
// findings it may suppress, which sets were approved (D4-2/D4-3/D4-4), and which
// exposures must stay visible: the run_command residual (Group D) and the
// non-blocking MEDIUM findings (Group E).

const (
	repoBaselineRoot = "../.."
	repoBaselineRel  = ".agent-sdlc/security-baseline.json"

	// D4-5 R2: the residual is deliberately NOT baselined.
	residualFingerprint = "semgrep:3076a05322acb6e35b02c87144da3b8a6dfbe8eac96a7679614c4d5402973771"

	digestGroupA   = "4596c8ccc60f4d1b58d8ce3e9195d1455089dd320fdb4b5fe00ebdbf9fab1ef9"
	digestGroupB   = "2e5f765470497c31239a3203c5a4d40023e76a7f602ed847d21479727bec9b03"
	digestGroupC   = "b61b105f2838651008a731a2e89ef3902e76b6ea1c579fc245ec069776d094bc"
	digestApproved = "d24bd19fef574619cf6f2fc392ef6acd0b2dde2df92ba53c7bfa76fe60224c31"
)

// groupBFingerprints are the five approved test-only Semgrep HIGH findings. They
// cannot be selected by expiry or rule alone because pre-existing entries share
// both, so the exact set is pinned here.
var groupBFingerprints = []string{
	"semgrep:ccb4bb42ca610fd2963268b516385bba795ee7c294c4ad0f7968bcd03d25725c",
	"semgrep:fab835e9941bb1ad0ab32bf7e51f0a1f30944f2d500ba43cf844b247c4c35718",
	"semgrep:11ef53417d1b9eedc8e24e1593bdfa38b6d8df035e3265fce7c61bff2aa3e5c2",
	"semgrep:fbedd5f6a2ed946cd639a20ce584f6ab65c3788767ea09cbb60e2c08294313b2",
	"semgrep:765bec591c74b4bc0a7bfbee12daaeb711a7123391832670c4d7d8d2a99d9f28",
}

// retiredFingerprints are the 16 stale entries removed by the D4 maintenance
// change. Their rows remain in git history (commit 1064b80f) and their rationale
// in matrix §2; they must never come back silently.
var retiredFingerprints = []string{
	"semgrep:e8a0b7e0647ce66aaef103f0705ab4a036ab69bc4f02ec9aeae0f0e1f35316dd",
	"semgrep:78d6fce54bcddcec7fecce1d57b85a0bca44d0d35bb59a1827d44268d0de3725",
	"semgrep:bce81695879c635bb1894f6dfa12b0e3ec4a306aa5cd935750d1cb8018c8048d",
	"semgrep:5e4ca22cffafe0022e8a379bf3923e1c9a0356aaf8981b5b7f985ac9c7f9b17f",
	"semgrep:dfd17358d313f17a0b7d29f6165dbae892ad87254f44ff94578ce3d9d8aeb7a4",
	"semgrep:cd2dacdbe1fe084913193ef4582be496d3552ea84c2250f543e70663821f916e",
	"semgrep:6e0e49455f2627426687c1e0cc00e8d12401df91f700f1b849826940d1bb14d1",
	"semgrep:5d918d1490e4e6cec109a5b2cc278df1fec6a39db70d536df0921d76fdeddf8a",
	"semgrep:819e8d960cbaa81049fb4ba0a730ba8a716de4e39b3e1e437c7b3da85e027e37",
	"semgrep:ecc01dc3634cb18e2c81bba972ca62940d1ab3345e44ea4e39b0dfb16fc459e4",
	"semgrep:f9f6b86b583ac0c7c48c655215c73cab10f601eaa10af2bb56fe49b4a402e00a",
	"semgrep:331eef7a50809fae26080144035531412b17956e61bfbe9813288f25ead3a55e",
	"semgrep:df4e9848d6aa031ce1d4a27e7262d449ba8bec5573f4ef0b86efc7a9102326d7",
	"semgrep:82f8cbd035e31b4adffe1f201507e29781b9626f8acb040142ad16ee46216576",
	"semgrep:11fb9bdc2aa5607323d07d686df44459f31e57e5e91c0745e3d21a8fb779672d",
	"semgrep:90576e4e8d83cebe41710b35562e65f7d78cccbd10cccf1bc41c38c47f66c8ef",
}

var (
	approvedExpiry = time.Date(2027, 6, 30, 0, 0, 0, 0, time.UTC)
	prodExpiry     = time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC)
	reviewNow      = time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
)

func loadRepoBaseline(t *testing.T) Baseline {
	t.Helper()
	b, diagnostics, err := LoadBaseline(repoBaselineRoot, repoBaselineRel)
	if err != nil {
		t.Fatalf("load committed baseline: %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("committed baseline reported diagnostics: %v", diagnostics)
	}
	return b
}

// fingerprintSetDigest is the identity used for approval: SHA-256 over the
// sorted, newline-joined fingerprints.
func fingerprintSetDigest(fingerprints []string) string {
	sorted := append([]string(nil), fingerprints...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum[:])
}

func approvedEntries(b Baseline) []BaselineEntry {
	var out []BaselineEntry
	for _, e := range b.Entries {
		if strings.Contains(e.Reason, "D4-") {
			out = append(out, e)
		}
	}
	return out
}

func findingsForCategory(findings []Finding, c Category) []Finding {
	var out []Finding
	for _, f := range findings {
		if f.Category == c {
			out = append(out, f)
		}
	}
	return out
}

func TestRepoBaselineApprovedGroups(t *testing.T) {
	b := loadRepoBaseline(t)
	if len(b.Entries) != 84 {
		t.Fatalf("baseline entries = %d, want 84 (40 retained + 44 approved)", len(b.Entries))
	}

	approved := approvedEntries(b)
	if len(approved) != 44 {
		t.Fatalf("D4-approved entries = %d, want 44", len(approved))
	}
	var fps []string
	for _, e := range approved {
		fps = append(fps, e.Fingerprint)
		if e.Owner == "" || e.Reason == "" {
			t.Errorf("approved entry %s lacks owner or reason", e.Fingerprint)
		}
		if !e.ExpiresAt.Equal(approvedExpiry) && !e.ExpiresAt.Equal(prodExpiry) {
			t.Errorf("approved entry %s expires %s, want one of the approved dates", e.Fingerprint, e.ExpiresAt)
		}
	}
	if got := fingerprintSetDigest(fps); got != digestApproved {
		t.Fatalf("approved set digest = %s, want %s", got, digestApproved)
	}

	// Group A: every gitleaks entry, and nothing else, is an approved fixture.
	var groupA []string
	for _, e := range b.Entries {
		if strings.HasPrefix(e.Fingerprint, "gitleaks:") {
			groupA = append(groupA, e.Fingerprint)
			if !e.ExpiresAt.Equal(approvedExpiry) {
				t.Errorf("group A entry %s expires %s, want %s", e.Fingerprint, e.ExpiresAt, approvedExpiry)
			}
			if !strings.Contains(e.Reason, "D4-2") {
				t.Errorf("gitleaks entry %s is not tagged D4-2", e.Fingerprint)
			}
		}
	}
	if len(groupA) != 27 {
		t.Fatalf("group A entries = %d, want 27", len(groupA))
	}
	if got := fingerprintSetDigest(groupA); got != digestGroupA {
		t.Fatalf("group A digest = %s, want %s", got, digestGroupA)
	}

	// Group B: the five named test-only helpers.
	byFingerprint := make(map[string]BaselineEntry, len(b.Entries))
	for _, e := range b.Entries {
		byFingerprint[e.Fingerprint] = e
	}
	for _, fp := range groupBFingerprints {
		e, ok := byFingerprint[fp]
		if !ok {
			t.Errorf("group B entry %s is missing", fp)
			continue
		}
		if !e.ExpiresAt.Equal(approvedExpiry) {
			t.Errorf("group B entry %s expires %s, want %s", fp, e.ExpiresAt, approvedExpiry)
		}
		if !strings.Contains(e.Reason, "D4-3") {
			t.Errorf("group B entry %s is not tagged D4-3", fp)
		}
	}
	if got := fingerprintSetDigest(groupBFingerprints); got != digestGroupB {
		t.Fatalf("group B digest = %s, want %s", got, digestGroupB)
	}

	// Group C: every production entry expires earlier and is tagged D4-4.
	var groupC []string
	for _, e := range b.Entries {
		if e.ExpiresAt.Equal(prodExpiry) {
			groupC = append(groupC, e.Fingerprint)
			if !strings.Contains(e.Reason, "D4-4") {
				t.Errorf("production entry %s is not tagged D4-4", e.Fingerprint)
			}
			if strings.HasSuffix(e.Path, "_test.go") {
				t.Errorf("production entry %s points at a test file", e.Fingerprint)
			}
		}
	}
	if len(groupC) != 12 {
		t.Fatalf("group C entries = %d, want 12", len(groupC))
	}
	if got := fingerprintSetDigest(groupC); got != digestGroupC {
		t.Fatalf("group C digest = %s, want %s", got, digestGroupC)
	}
}

func TestRepoBaselineRetiredEntriesAreGone(t *testing.T) {
	b := loadRepoBaseline(t)
	have := make(map[string]bool, len(b.Entries))
	for _, e := range b.Entries {
		have[e.Fingerprint] = true
	}
	for _, fp := range retiredFingerprints {
		if have[fp] {
			t.Errorf("retired entry %s is still present", fp)
		}
	}
}

func TestRepoBaselineLeavesGroupDAndGroupEUnsuppressed(t *testing.T) {
	b := loadRepoBaseline(t)
	have := make(map[string]bool, len(b.Entries))
	rules := make(map[string]int, len(b.Entries))
	for _, e := range b.Entries {
		have[e.Fingerprint] = true
		rules[e.RuleID]++
	}
	if have[residualFingerprint] {
		t.Fatalf("the run_command residual is baselined; D4-5 is R2 and must stay open")
	}

	// Group E is omitted. Any approved MEDIUM finding would change this census,
	// and the Group-E-only rules must not appear at all.
	want := map[string]int{
		"dangerous-exec-command":            43, // 26 pre-existing + 5 B + 12 C
		"no-direct-write-to-responsewriter": 3,  // pre-existing only
		"cookie-missing-secure":             3,  // pre-existing only
		"open-redirect":                     3,
		"use-of-unsafe-block":               2,
		"string-formatted-query":            2, // pre-existing only
		"math-random-used":                  1,
		"stripe-access-token":               9,
		"aws-access-token":                  7,
		"generic-api-key":                   7,
		"gitlab-pat":                        3,
		"jwt":                               1,
	}
	if !reflect.DeepEqual(rules, want) {
		t.Fatalf("baseline rule census = %v, want %v", rules, want)
	}
	for _, e := range b.Entries {
		if strings.Contains(e.Reason, "D4-6") || strings.Contains(strings.ToLower(e.Reason), "group e") {
			t.Errorf("entry %s records a Group E approval, which was omitted", e.Fingerprint)
		}
	}
}

// Only the approved fingerprints gain a suppression. The residual and any new
// finding stay visible and keep blocking.
func TestRepoBaselineSuppressesOnlyApprovedFingerprints(t *testing.T) {
	b := loadRepoBaseline(t)
	approved := approvedEntries(b)

	newFinding := Finding{
		Fingerprint: "semgrep:unreviewed-new-finding", RuleID: "dangerous-exec-command",
		Path: "internal/example/new.go", Severity: SeverityHigh, Category: CategorySAST,
	}
	residual := Finding{
		Fingerprint: residualFingerprint, RuleID: "dangerous-exec-command",
		Path: "internal/toolharness/harness.go", Severity: SeverityHigh, Category: CategorySAST,
	}

	findings := make([]Finding, 0, len(approved)+2)
	for _, e := range approved {
		category := CategorySAST
		if strings.HasPrefix(e.Fingerprint, "gitleaks:") {
			category = CategorySecrets
		}
		findings = append(findings, Finding{
			Fingerprint: e.Fingerprint, RuleID: e.RuleID, Path: e.Path,
			Severity: SeverityHigh, Category: category,
		})
	}
	findings = append(findings, newFinding, residual)

	out, _ := b.Apply(findings, reviewNow)
	suppressed := 0
	for _, f := range out {
		if f.Suppressed {
			suppressed++
			if f.Fingerprint == residualFingerprint || f.Fingerprint == newFinding.Fingerprint {
				t.Fatalf("unapproved finding %s was suppressed", f.Fingerprint)
			}
		}
	}
	if suppressed != len(approved) {
		t.Fatalf("suppressed = %d, want exactly the %d approved findings", suppressed, len(approved))
	}

	execs := []Execution{
		{AdapterID: "gitleaks", Category: CategorySecrets, Status: StatusCompleted,
			Coverage: CoverageComplete, RepositoryWide: true,
			Findings: findingsForCategory(out, CategorySecrets)},
		{AdapterID: "semgrep", Category: CategorySAST, Status: StatusCompleted,
			Coverage: CoverageComplete, Languages: []Language{LanguageGo},
			Findings: findingsForCategory(out, CategorySAST)},
	}
	policy := Policy{Enabled: true, Mode: "report-only", Scope: "workspace",
		FailOn: []string{"critical", "high"}, UnknownSeverity: "indeterminate"}
	eval := EvaluatePolicyWithDetection(execs, policy, &Detection{Languages: []Language{LanguageGo}})
	if eval.Verdict != VerdictFail {
		t.Fatalf("verdict = %s, want FAIL while the residual and a new finding block", eval.Verdict)
	}
	blocking := 0
	for _, f := range eval.Findings {
		if f.Suppressed || !SeverityBlocked(policy.FailOn, f.Severity) {
			continue
		}
		blocking++
		if f.Fingerprint != residualFingerprint && f.Fingerprint != newFinding.Fingerprint {
			t.Errorf("unexpected blocking finding %s", f.Fingerprint)
		}
	}
	if blocking != 2 {
		t.Fatalf("blocking = %d, want the residual plus the new finding", blocking)
	}
}

// The approved entries stop suppressing on their expiry dates. Group C expires
// first, so it must be re-reviewed by 2027-03-31.
func TestRepoBaselineExpiriesReturnFindings(t *testing.T) {
	b := loadRepoBaseline(t)
	approved := approvedEntries(b)
	findings := make([]Finding, 0, len(approved))
	for _, e := range approved {
		findings = append(findings, Finding{Fingerprint: e.Fingerprint, RuleID: e.RuleID, Path: e.Path, Severity: SeverityHigh})
	}

	afterC := time.Date(2027, 4, 15, 0, 0, 0, 0, time.UTC)
	out, diagnostics := b.Apply(findings, afterC)
	suppressed := 0
	for _, f := range out {
		if f.Suppressed {
			suppressed++
		}
	}
	if suppressed != 32 {
		t.Fatalf("suppressed after Group C expiry = %d, want 32 (Groups A and B)", suppressed)
	}
	if !strings.Contains(strings.Join(diagnostics, " "), "expired") {
		t.Errorf("expected expired-entry diagnostics, got %v", diagnostics)
	}

	afterAll := time.Date(2027, 7, 1, 0, 0, 0, 0, time.UTC)
	out, _ = b.Apply(findings, afterAll)
	for _, f := range out {
		if f.Suppressed {
			t.Errorf("finding %s is still suppressed after every approved entry expired", f.Fingerprint)
		}
	}
}

// A failed scanner must never yield a passing verdict, baseline or not.
func TestRepoBaselineScannerFailureCannotPass(t *testing.T) {
	execs := []Execution{
		{AdapterID: "gitleaks", Category: CategorySecrets, Status: StatusFailed, Coverage: CoverageUnknown, RepositoryWide: true},
		{AdapterID: "semgrep", Category: CategorySAST, Status: StatusCompleted, Coverage: CoverageComplete, Languages: []Language{LanguageGo}},
	}
	policy := Policy{Enabled: true, Mode: "required", Scope: "workspace",
		RequiredCategories: []string{"secrets", "sast"},
		FailOn:             []string{"critical", "high"}, UnknownSeverity: "indeterminate"}
	eval := EvaluatePolicyWithDetection(execs, policy, &Detection{Languages: []Language{LanguageGo}})
	if eval.Verdict == VerdictPass {
		t.Fatalf("a failed scanner produced PASS; reasons: %v", eval.Reasons)
	}
}
