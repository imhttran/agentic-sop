package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/testrunner"
	"github.com/imhttran/agentic-sop/internal/validate"
	"github.com/imhttran/agentic-sop/internal/verifcache"
)

// runValidate runs the configured build/test/lint commands. With --cache it consults
// the CTX-007 Verification Cache for an exact match before running, and records the
// result for later reuse.
func runValidate(args []string, stdout, stderr io.Writer, getwd func() (string, error)) int {
	useCache := false
	for _, arg := range args {
		switch arg {
		case "--cache":
			useCache = true
		default:
			fmt.Fprintln(stderr, "usage: sop validate [--cache]")
			return exitUsage
		}
	}

	dir, ok := projectDir(getwd, stderr)
	if !ok {
		return exitError
	}

	cfg, err := config.LoadDir(dir)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			fmt.Fprintln(stderr, "no configuration found; run `sop init` first")
			return exitError
		}
		fmt.Fprintf(stderr, "validate: %v\n", err)
		return exitError
	}

	if !validate.Enabled(cfg.Validation) {
		fmt.Fprintln(stdout, "no validation commands configured")
		return exitOK
	}

	result, cached, reuse := validateWithCache(context.Background(), dir, cfg, useCache)
	for _, r := range result.Results {
		fmt.Fprintf(stdout, "%-6s %-12s %s (%.2fs)\n", r.Status, r.Category, r.Command, r.Duration.Seconds())
		if r.Status != testrunner.Pass {
			if detail := strings.TrimSpace(r.Stderr); detail != "" {
				fmt.Fprintf(stdout, "\n%s\n", detail)
			}
			break
		}
	}

	verdict := "validation: FAIL"
	if result.Passed() {
		verdict = "validation: PASS"
	}
	if cached {
		verdict += " (cached)"
	}
	fmt.Fprintln(stdout, verdict)
	// Read-only reuse provenance. It names the evidence key, the reuse-vs-run
	// decision, the answering store, and the miss reason. It changes no PASS/FAIL
	// semantics and performs no state transition.
	writeReuseProvenance(stdout, reuse)
	if result.Passed() {
		return exitOK
	}
	return exitError
}

// validateWithCache runs the configured validation, reusing a cached result when the
// verification cache is enabled and the extended identity matches exactly.
//
// The cache is consulted only for a clean working tree. HEAD alone does not identify a
// dirty tree, so an uncommitted change is never served from cache: that is a false miss,
// never a false hit. The identity is the extended verifcache/2 identity: effective
// validation inputs (path + content hash, including uncommitted changes), the command
// set, the relevant environment and toolchain, the command-policy classification, the
// validation scope, and the verifier version. Reuse occurs only for an exact identity
// match AND a VERIFIED verdict; every mismatch, and failed/partial/hold/incomplete/
// untrusted evidence, runs. A dirty tree is always an explicit miss.
//
// The lookup/store route through the opt-in shared cross-worktree store, disabled by
// default via SOP_VERIFY_CACHE_SHARED, so the default path is local-only. Each decision
// is recorded append-only when a run-relative record path is available.
func validateWithCache(ctx context.Context, dir string, cfg *config.Config, useCache bool) (testrunner.SuiteResult, bool, validateReuse) {
	if !useCache {
		return validate.Run(ctx, dir, cfg.Validation), false, validateReuse{}
	}
	head, dirty := repoState(dir)
	id := buildValidateIdentity(dir, cfg, head, dirty)
	reuse := validateReuse{Consulted: true, KeyDigest: id.Digest(), Scope: id.Scope}
	if dirty {
		// A dirty working tree is never served from cache: an explicit miss with
		// the fail-closed reason.
		reuse.Reason = "dirty working tree is never served from cache"
		if err := recordDecision(decisionRecordPath(dir), id, false, verifcache.SourceLocal, reuse.Reason, verifcache.Entry{}); err != nil {
			reuse.RecordErr = err.Error()
		}
		return validate.Run(ctx, dir, cfg.Validation), false, reuse
	}

	cache := loadVerifyCache(dir)
	shared := sharedStoreFromEnv()
	// SharedStore.Lookup consults the local cache first and, when the shared store
	// is enabled, the shared store; it returns the first exact identity match.
	hit, source := shared.Lookup(cache, id)
	if hit.Hit {
		if decision := verifcache.Decide(hit.Entry, id); decision.Reusable {
			var suite testrunner.SuiteResult
			if json.Unmarshal([]byte(hit.Entry.Output), &suite) == nil {
				reuse.Reused = true
				reuse.Store = source
				reuse.Reason = decision.Reason
				if err := recordDecision(decisionRecordPath(dir), id, true, source, decision.Reason, hit.Entry); err != nil {
					reuse.RecordErr = err.Error()
				}
				return suite, true, reuse
			}
		} else {
			// An exact-match entry that is not reusable (non-VERIFIED evidence) is a
			// miss: it must never be reused as PASS.
			reuse.Reason = decision.Reason
			reuse.MatchPresent = true
		}
	}
	if !hit.Hit {
		reuse.Reason = verifcache.Decide(verifcache.Entry{}, id).Reason
	}

	suite := validate.Run(ctx, dir, cfg.Validation)
	if data, err := json.Marshal(suite); err == nil {
		entry, err := shared.Store(cache, id, verifcache.Result{Passed: suite.Passed(), Output: string(data), Source: "sop validate", Verdict: verdictOf(suite)})
		if err == nil {
			_ = saveVerifyCache(dir, cache)
		}
		if err := recordDecision(decisionRecordPath(dir), id, false, source, reuse.Reason, entry); err != nil {
			reuse.RecordErr = err.Error()
		}
	}
	return suite, false, reuse
}

// verdictOf classifies a fresh validation result for the cache's fail-closed reuse
// rule. A passing suite is VERIFIED; a failing suite is FAILED. Only VERIFIED
// evidence may ever be reused as PASS.
func verdictOf(suite testrunner.SuiteResult) verifcache.Verdict {
	if suite.Passed() {
		return verifcache.VerdictVerified
	}
	return verifcache.VerdictFailed
}

// checkCommands lists the configured verification commands in run order.
func checkCommands(v config.Validation) []string {
	checks := validate.Checks(v)
	out := make([]string, 0, len(checks))
	for _, c := range checks {
		out = append(out, c.Command)
	}
	return out
}

func verifyCachePath(dir string) string {
	return filepath.Join(dir, stateDirName, "context", "verify-cache.json")
}

func loadVerifyCache(dir string) *verifcache.Cache {
	data, err := os.ReadFile(verifyCachePath(dir))
	if err != nil {
		return verifcache.New()
	}
	cache, err := verifcache.Unmarshal(data)
	if err != nil {
		return verifcache.New()
	}
	return cache
}

func saveVerifyCache(dir string, cache *verifcache.Cache) error {
	data, err := cache.Marshal()
	if err != nil {
		return err
	}
	path := verifyCachePath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
