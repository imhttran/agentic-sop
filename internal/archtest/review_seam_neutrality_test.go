package archtest

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// RSH-005 provider/model-neutrality guard for the review and change-attribution
// seam in internal/cli.
//
// The existing guard in this package (TestArchitectureGuard and its corePackages
// inventory) protects the Phase 8 core and policy/governance packages by import
// graph. It does NOT cover the review and change-attribution seam in
// internal/cli, which decides WHICH change set is offered to review and HOW
// changed files are attributed. This check proves those changes introduce no
// provider-, model-, or domain-specific behavior into that seam.
//
// The invariant: the seam is decided by declared capabilities, repository paths,
// and evidence — never by a provider name, a model name, or a domain branch. The
// guard is a stdlib-only source scan (offline, CI-safe), in the same style as the
// rest of this package.
//
// Maintainable scope definition
// ----------------------------
// The seam predicate is deliberately NOT a fixed file list. It is a documented,
// provider-neutral predicate over the production .go files of internal/cli:
//
//	A production internal/cli .go source file participates in the review or
//	change-attribution seam when it references the review request/report/
//	provider contract (the review.Request / review.Report / review.Provider
//	types), the review-session cache (cachedReview / cacheReview), OR the
//	change-attribution contract (the reviewScope / newReviewScope /
//	scopedReviewDiff / changedFiles members that decide which paths are
//	offered to review).
//
// The predicate names no provider, model, or domain, and uses only stdlib text
// matching. internal/cli/run.go satisfies it: it contains the run-level review
// caller (reviewProvider, provider.Review(ctx, review.Request{...})) and the
// change-attribution/scope construction (newReviewScope(rn.ChangedFiles(), ...)
// and scopedReviewDiff).
//
// seamScope is the maintainable, self-checking scope definition. It derives the
// covered set from the predicate (discoverReviewSeamFiles) and asserts it against
// the explicit reviewed list (reviewSeamFiles) in BOTH directions:
//
//   - a declared reviewed file absent from the derived set fails (a removed or
//     renamed file can never silently shrink coverage);
//   - a derived seam file absent from the declared list fails (a newly added
//     seam file can never silently escape the guard).
//
// Adding a future review-integration point therefore forces a deliberate
// declaration update; it cannot be silently omitted. The guard remains a
// stdlib-only, offline, provider/model-neutral scan and adds no dependency.

// reviewSeamFiles is the explicit, reviewed list of internal/cli production files
// that implement the review input scoping and the review/change-attribution seam.
// It must agree with the predicate-derived set (see seamScope and
// assertSeamScopeAgrees); every entry must exist on disk
// (TestReviewSeamNeutralityFilesExist). It includes internal/cli/run.go: run.go
// hosts the run-level review caller and the review-scope/change-attribution call
// sites, so it belongs to the seam. The remaining entries are the run-session
// review cache (session.go), the change-attribution helper (mutation.go), and the
// alternate review integration point exposed over MCP (mcp.go); each participates
// in the seam that decides which change set is offered to review and how changed
// files are attributed.
var reviewSeamFiles = []string{
	"internal/cli/run.go",
	"internal/cli/reviewscope.go",
	"internal/cli/jev.go",
	"internal/cli/review.go",
	"internal/cli/session.go",
	"internal/cli/mutation.go",
	"internal/cli/mcp.go",
}

// reviewSeamDir is the package whose production .go files the predicate scans.
const reviewSeamDir = "internal/cli"

// reviewSeamPredicateMarkers are the provider-neutral contract markers that decide
// seam membership. A production internal/cli .go file participates in the review
// or change-attribution seam when it references the review request/report/provider
// contract, the review-session cache, or the change-attribution scope contract. The
// markers are declared types, functions, and method names of those contracts; they
// name no provider, model, or domain. Matching is a case-sensitive substring scan of
// the source, so the markers are deliberately the exact identifiers used by the seam.
var reviewSeamPredicateMarkers = []string{
	// review request/report/provider contract
	"review.Report",
	"review.Request",
	"review.Provider",
	// review-session cache of clean review results
	"cachedReview",
	"cacheReview",
	// change-attribution / review-scope contract
	"reviewScope",
	"newReviewScope",
	"scopedReviewDiff",
	"changedFiles",
}

// providerModelTokens are the provider, model, and vendor names that must never
// appear in the provider-neutral review seam. Matching is case-insensitive so a
// literal provider/model reference is caught while unrelated identifiers are not
// affected by case.
var providerModelTokens = []string{
	"ollama",
	"openai",
	"anthropic",
	"claude",
	"qwen",
	"deepseek",
	"gemini",
	"llamacpp",
	"llama.cpp",
	"mlx",
	"gpt-4",
	"gpt-3",
}

// seamScope is the derived-and-declared scope of the review seam. It holds the
// predicate-derived set (Discovered) and the explicit reviewed list (Declared),
// so the completeness check can compare them in both directions without
// depending on the repository's current contents.
type seamScope struct {
	// Declared is the explicit, reviewed list of seam files. Adding a seam file
	// forces a deliberate update here.
	Declared []string
	// Discovered is the predicate-derived set of seam files.
	Discovered []string
}

// agrees returns the two-way differences between the declared and discovered
// sets: declared files absent from the discovered set, and discovered files
// absent from the declared list. Both directions are non-empty when the scope is
// inconsistent.
func (s seamScope) agrees() (declaredMissing, discoveredMissing []string) {
	declared := toSet(s.Declared)
	discovered := toSet(s.Discovered)
	for _, f := range s.Declared {
		if !discovered[f] {
			declaredMissing = append(declaredMissing, f)
		}
	}
	for _, f := range s.Discovered {
		if !declared[f] {
			discoveredMissing = append(discoveredMissing, f)
		}
	}
	sort.Strings(declaredMissing)
	sort.Strings(discoveredMissing)
	return declaredMissing, discoveredMissing
}

// covers reports whether path is in the covered (declared) set.
func (s seamScope) covers(path string) bool {
	return toSet(s.Declared)[path]
}

// toSet builds a membership set from a path list.
func toSet(paths []string) map[string]bool {
	set := make(map[string]bool, len(paths))
	for _, p := range paths {
		set[p] = true
	}
	return set
}

// discoverReviewSeamFiles derives the seam covered set from the documented,
// provider-neutral predicate over the production .go files of dirRel. Filesystem
// errors are returned so the caller can fail loudly rather than silently protect
// nothing. Test files (_test.go) are excluded: the predicate classifies
// production code only.
func discoverReviewSeamFiles(root, dirRel string) ([]string, error) {
	dir := filepath.Join(root, filepath.FromSlash(dirRel))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		p := filepath.Join(dir, name)
		src, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		if seamSourceMatches(string(src)) {
			files = append(files, dirRel+"/"+name)
		}
	}
	sort.Strings(files)
	return files, nil
}

// seamSourceMatches applies the provider-neutral seam predicate to a production
// source file's text. It is factored out so the predicate can be exercised with
// synthetic inputs, independent of the repository's current contents.
func seamSourceMatches(src string) bool {
	for _, marker := range reviewSeamPredicateMarkers {
		if strings.Contains(src, marker) {
			return true
		}
	}
	return false
}

// assertSeamScopeAgrees fails when the declared and discovered seam sets disagree
// in either direction: a declared seam file that is not discovered (removed or
// renamed) and a discovered seam file that is not declared (a new seam file).
func assertSeamScopeAgrees(t *testing.T, scope seamScope) {
	t.Helper()
	declaredMissing, discoveredMissing := scope.agrees()
	for _, f := range declaredMissing {
		t.Errorf("declared review-seam file %s is not classified by the provider-neutral seam predicate; a removed or renamed seam file must not silently shrink coverage", f)
	}
	for _, f := range discoveredMissing {
		t.Errorf("internal/cli/%s participates in the review or change-attribution seam but is absent from the declared reviewed list; declare it in reviewSeamFiles so it cannot silently escape the neutrality guard", strings.TrimPrefix(f, reviewSeamDir+"/"))
	}
}

// TestReviewSeamStaysProviderAndModelNeutral fails if a provider, model, or vendor
// name appears in the internal/cli review and change-attribution seam. The seam
// must be decided by paths, capabilities, and evidence, so a provider/model name
// there is a neutrality regression. It scans the derived-and-declared covered set,
// so a newly declared seam file is covered automatically.
func TestReviewSeamStaysProviderAndModelNeutral(t *testing.T) {
	root := moduleRoot(t)
	scope := seamScope{Declared: reviewSeamFiles}
	discovered, err := discoverReviewSeamFiles(root, reviewSeamDir)
	if err != nil {
		t.Fatalf("discover review seam files: %v", err)
	}
	scope.Discovered = discovered
	if !scope.covers("internal/cli/run.go") {
		t.Fatal("the covered review-seam set must include internal/cli/run.go")
	}
	for _, rel := range scope.Declared {
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if sourceNamesProviderModelToken(string(src), providerModelTokens) {
			t.Errorf("%s names a provider/model token; the review and change-attribution seam must stay provider- and model-neutral", rel)
		}
	}
}

// TestReviewSeamNeutralityFilesExist asserts every declared seam file exists, so the
// guard cannot silently protect nothing. A declared file that does not exist fails.
func TestReviewSeamNeutralityFilesExist(t *testing.T) {
	root := moduleRoot(t)
	for _, rel := range reviewSeamFiles {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || info.IsDir() {
			t.Errorf("declared review-seam file %s does not exist (err=%v)", rel, err)
		}
	}
}

// TestReviewSeamScopeIsComplete asserts the declared reviewed list and the
// predicate-derived seam set agree in both directions on the current repository:
// no declared file is missing from the derived set (no silent shrink) and no
// discovered seam file is undeclared (no silent gap). It also asserts the covered
// set includes internal/cli/run.go.
func TestReviewSeamScopeIsComplete(t *testing.T) {
	root := moduleRoot(t)
	discovered, err := discoverReviewSeamFiles(root, reviewSeamDir)
	if err != nil {
		t.Fatalf("discover review seam files: %v", err)
	}
	scope := seamScope{Declared: reviewSeamFiles, Discovered: discovered}
	if !scope.covers("internal/cli/run.go") {
		t.Fatal("the covered review-seam set must include internal/cli/run.go")
	}
	assertSeamScopeAgrees(t, scope)
}

// TestReviewSeamNeutralityDetectsRepresentativeToken is the negative control: the
// detection predicate must actually fire on a synthetic provider-naming source, so
// the guard cannot pass vacuously.
func TestReviewSeamNeutralityDetectsRepresentativeToken(t *testing.T) {
	src := "// served by ollama\npackage cli\n"
	if !sourceNamesProviderModelToken(src, providerModelTokens) {
		t.Fatal("the neutrality predicate must flag a provider name; detection is vacuous")
	}
}

// sourceNamesProviderModelToken reports whether src names any of the deny-listed
// provider/model tokens. It is factored out so the negative control exercises the
// same predicate the guard uses.
func sourceNamesProviderModelToken(src string, tokens []string) bool {
	lower := strings.ToLower(src)
	for _, token := range tokens {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

// TestReviewSeamPredicateMatchesContractMarkers proves the predicate is non-vacuous
// and provider-neutral: a synthetic source that references the review
// request/report contract (or the change-attribution scope contract) is classified,
// while unrelated source is not.
func TestReviewSeamPredicateMatchesContractMarkers(t *testing.T) {
	seam := "package cli\n\nfunc f(r review.Report) {}\n"
	if !seamSourceMatches(seam) {
		t.Error("a production source referencing the review report contract must satisfy the seam predicate")
	}
	attribution := "package cli\n\nvar s = reviewScope{}\n"
	if !seamSourceMatches(attribution) {
		t.Error("a production source referencing the change-attribution scope contract must satisfy the seam predicate")
	}
	unrelated := "package cli\n\nfunc helper() int { return 1 }\n"
	if seamSourceMatches(unrelated) {
		t.Error("unrelated production source must not satisfy the seam predicate")
	}
}

// TestReviewSeamCompletenessFailsOnDeclaredMissing proves one direction of the
// completeness check: a declared seam file absent from the discovered set fails.
// It uses a synthetic scope so the failure logic is exercised independently of the
// repository's current contents.
func TestReviewSeamCompletenessFailsOnDeclaredMissing(t *testing.T) {
	scope := seamScope{
		Declared:   []string{"internal/cli/run.go", "internal/cli/removed_seam.go"},
		Discovered: []string{"internal/cli/run.go"},
	}
	declaredMissing, discoveredMissing := scope.agrees()
	if len(declaredMissing) != 1 || declaredMissing[0] != "internal/cli/removed_seam.go" {
		t.Fatalf("expected the absent declared file to be reported, got declaredMissing=%v", declaredMissing)
	}
	if len(discoveredMissing) != 0 {
		t.Fatalf("expected no undiscovered gap, got discoveredMissing=%v", discoveredMissing)
	}
}

// TestReviewSeamCompletenessFailsOnDiscoveredMissing proves the other direction:
// a discovered seam file undeclared in the explicit reviewed list fails, so a new
// seam file cannot silently escape the guard. It uses a synthetic scope so the
// failure logic is exercised independently of the repository's current contents.
func TestReviewSeamCompletenessFailsOnDiscoveredMissing(t *testing.T) {
	scope := seamScope{
		Declared:   []string{"internal/cli/run.go"},
		Discovered: []string{"internal/cli/run.go", "internal/cli/new_seam.go"},
	}
	declaredMissing, discoveredMissing := scope.agrees()
	if len(discoveredMissing) != 1 || discoveredMissing[0] != "internal/cli/new_seam.go" {
		t.Fatalf("expected the undeclared discovered file to be reported, got discoveredMissing=%v", discoveredMissing)
	}
	if len(declaredMissing) != 0 {
		t.Fatalf("expected no declared-without-discovery gap, got declaredMissing=%v", declaredMissing)
	}
}

// TestReviewSeamScopeCoversRunGo asserts, on the current repository, that the
// covered set includes internal/cli/run.go and that the predicate classifies it.
func TestReviewSeamScopeCoversRunGo(t *testing.T) {
	const runGo = "internal/cli/run.go"
	scope := seamScope{Declared: reviewSeamFiles}
	if !scope.covers(runGo) {
		t.Fatalf("the covered review-seam set must include %s", runGo)
	}
	root := moduleRoot(t)
	src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(runGo)))
	if err != nil {
		t.Fatalf("read %s: %v", runGo, err)
	}
	if !seamSourceMatches(string(src)) {
		t.Fatalf("%s must satisfy the provider-neutral seam predicate", runGo)
	}
}
