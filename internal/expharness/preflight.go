package expharness

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/model"
)

// Policy is what the operator approved for the experiment.
type Policy struct {
	// AllowedProviders is the approved provider allow-list for every class.
	AllowedProviders []string
	// AllowCloud records the operator's decision that a class may be cloud-hosted.
	// False (the default) rejects any class whose locality is not local.
	AllowCloud bool
	// AllowedEnv lists the SOP_* variables that may be set (typically the endpoint
	// variable and SOP_TOOL_AUDIT_LOG). Every other SOP_* variable is an override
	// hazard and fails the preflight, including ones added to SOP later.
	AllowedEnv []string
	// UsedClasses lists the --model-class values the experiment will pass. A built-in
	// cloud fallback candidate (small has one that an overlay cannot remove) is a
	// problem only for these; for other classes it is recorded as a note. Empty means
	// every class is used.
	UsedClasses []string
	// CutoffCommit, when set, requires the project's HEAD to be this commit and no
	// commit outside HEAD's ancestry to be reachable from any ref.
	CutoffCommit string
}

// Resolved is one class's routing as the overlay and SOP's own resolver agree on it.
type Resolved struct {
	Class, Provider, Model, Locality string
}

// Report is the preflight result. Problems empty means the project is safe to run.
type Report struct {
	Problems     []string
	Notes        []string
	Harness      string
	Resolved     []Resolved
	ConfigSHA256 string
	EnvSet       []string // allowed SOP_* variables present, names only
}

// OK reports whether the preflight found no problem.
func (r Report) OK() bool { return len(r.Problems) == 0 }

func (r *Report) fail(format string, a ...any) {
	r.Problems = append(r.Problems, fmt.Sprintf(format, a...))
}

// Preflight validates a project directory and an environment (KEY=VALUE entries, as
// for exec.Cmd.Env) against the policy. It reads and executes nothing against a
// provider.
func Preflight(dir string, environ []string, p Policy) Report {
	var r Report
	env := map[string]string{}
	allowed := map[string]bool{}
	for _, k := range p.AllowedEnv {
		allowed[k] = true
	}
	for _, kv := range environ {
		k, v, _ := strings.Cut(kv, "=")
		env[k] = v
		if !strings.HasPrefix(k, "SOP_") {
			continue
		}
		if allowed[k] {
			r.EnvSet = append(r.EnvSet, k)
		} else {
			r.fail("environment override %s is set; only %v may be", k, p.AllowedEnv)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".env")); err == nil {
		r.fail("project .env present: SOP applies it at startup and it can inject overrides")
	}
	raw, err := os.ReadFile(config.Path(dir))
	if err != nil {
		r.fail("cannot read the config overlay: %v", err)
		return r
	}
	sum := sha256.Sum256(raw)
	r.ConfigSHA256 = hex.EncodeToString(sum[:])
	cfg, err := config.Parse(raw)
	if err != nil {
		r.fail("config overlay invalid: %v", err)
		return r
	}
	r.Harness = cfg.Agent.Harness
	if r.Harness == "" {
		r.fail("agent.harness is not set explicitly (SOP would default it)")
	}
	checkRoute(&r, cfg.Models, env, p)
	if p.CutoffCommit != "" {
		checkCutoff(&r, dir, p.CutoffCommit)
	}
	return r
}

func checkRoute(r *Report, route model.Route, env map[string]string, p Policy) {
	if route.AllowCloudFallbackForLocal != nil && *route.AllowCloudFallbackForLocal {
		r.fail("models.allow_cloud_fallback_for_local is true")
	}
	for name, on := range map[string]*bool{"routing_enabled": route.RoutingEnabled,
		"escalation_enabled": route.EscalationEnabled, "replan_enabled": route.ReplanEnabled} {
		if on != nil && *on {
			r.fail("models.%s is true: the class could change after the request", name)
		}
	}
	classes := map[model.Class]model.ClassConfig{
		model.ClassSmall: route.Small, model.ClassMedium: route.Medium, model.ClassLarge: route.Large}
	for _, c := range model.Classes {
		cc := classes[c]
		if cc.Provider == "" || cc.Name == "" || cc.Locality == "" {
			r.fail("models.%s must set provider, name and locality explicitly", c)
			continue
		}
		if cc.Fallback != nil {
			r.fail("models.%s.fallback is configured: SOP may swap in a different model at run time", c)
		}
		if !slices.Contains(p.AllowedProviders, cc.Provider) {
			r.fail("models.%s.provider %q is not in the approved providers %v", c, cc.Provider, p.AllowedProviders)
		}
		if cc.Locality != model.LocalityLocal && !p.AllowCloud {
			r.fail("models.%s.locality is %q but no cloud decision is recorded", c, cc.Locality)
		}
		// Ask SOP's own resolver what it would pick with THIS environment: it must
		// equal the overlay, with no fallback candidate.
		res, err := model.Resolve(model.Inputs{Config: route, CLIClass: string(c), Lookup: func(k string) string { return env[k] }})
		switch {
		case err != nil:
			r.fail("models.%s does not resolve: %v", c, err)
		case res.Selection.Provider != cc.Provider || res.Selection.Model != cc.Name:
			r.fail("models.%s resolves to %s/%s, not the overlay %s/%s", c,
				res.Selection.Provider, res.Selection.Model, cc.Provider, cc.Name)
		case res.LocalFallback != nil && (len(p.UsedClasses) == 0 || slices.Contains(p.UsedClasses, string(c))):
			r.fail("models.%s carries a cloud fallback candidate (%s/%s) that configuration cannot remove: do not use this class",
				c, res.LocalFallback.Provider, res.LocalFallback.Model)
		case res.LocalFallback != nil:
			r.Notes = append(r.Notes, fmt.Sprintf("models.%s carries cloud fallback candidate %s/%s; class unused", c,
				res.LocalFallback.Provider, res.LocalFallback.Model))
			r.Resolved = append(r.Resolved, Resolved{string(c), cc.Provider, cc.Name, string(cc.Locality)})
		default:
			r.Resolved = append(r.Resolved, Resolved{string(c), cc.Provider, cc.Name, string(cc.Locality)})
		}
	}
}

// checkCutoff requires HEAD == cutoff and no commit reachable from any ref outside
// HEAD's ancestry, so a later fix or erratum cannot be found through history.
func checkCutoff(r *Report, dir, cutoff string) {
	git := func(args ...string) (string, error) {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
		return strings.TrimSpace(string(out)), err
	}
	head, err := git("rev-parse", "HEAD")
	if err != nil || head != cutoff {
		r.fail("HEAD is %q, want cutoff commit %s", head, cutoff)
		return
	}
	if later, err := git("rev-list", "--all", "--not", "HEAD"); err != nil || later != "" {
		r.fail("commits outside HEAD's ancestry are reachable (history after the cutoff): %v", err)
	}
}
