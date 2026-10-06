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

	result, cached := validateWithCache(context.Background(), dir, cfg, useCache)
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
	if result.Passed() {
		return exitOK
	}
	return exitError
}

// validateWithCache runs the configured validation, reusing a cached result when the
// verification cache is enabled and the identity matches exactly.
//
// The cache is consulted only for a clean working tree. HEAD alone does not identify a
// dirty tree, so an uncommitted change is never served from cache: that is a false miss,
// never a false hit. The identity also includes the command set, the relevant
// environment, and the verifier version, so a changed command, toolchain, or SOP version
// invalidates the entry.
func validateWithCache(ctx context.Context, dir string, cfg *config.Config, useCache bool) (testrunner.SuiteResult, bool) {
	if !useCache {
		return validate.Run(ctx, dir, cfg.Validation), false
	}
	head, dirty := repoState(dir)
	if dirty {
		return validate.Run(ctx, dir, cfg.Validation), false
	}

	id := verifcache.Identity{
		Schema:      verifcache.SchemaVersion,
		Repository:  verifcache.RepositoryIdentity(head, false, ""),
		Command:     verifcache.CommandIdentity(checkCommands(cfg.Validation)),
		Environment: verifcache.EnvironmentIdentity(validationEnv()),
		Verifier:    "sop/validate@" + Version,
	}
	cache := loadVerifyCache(dir)
	if hit := cache.Lookup(id); hit.Hit {
		var suite testrunner.SuiteResult
		if json.Unmarshal([]byte(hit.Entry.Output), &suite) == nil {
			return suite, true
		}
	}

	suite := validate.Run(ctx, dir, cfg.Validation)
	if data, err := json.Marshal(suite); err == nil {
		cache.Store(id, verifcache.Result{Passed: suite.Passed(), Output: string(data), Source: "sop validate"})
		_ = saveVerifyCache(dir, cache)
	}
	return suite, false
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

// validationEnv is the relevant environment a cached verification depends on. The
// toolchain resolves through PATH, so a different toolchain invalidates the entry.
func validationEnv() map[string]string {
	return map[string]string{"PATH": os.Getenv("PATH")}
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
