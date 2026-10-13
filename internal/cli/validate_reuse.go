package cli

// This file wires the extended verifcache/2 identity and the VER-002 portable
// cross-worktree store into `sop validate --cache`, and provides the read-only
// reuse-decision surfacing consumed by `sop validate` / `sop report` /
// `sop verify`.
//
// Reuse stays fail-closed: an entry is reused as PASS only for an exact
// Identity equivalence AND a VERIFIED verdict (verifcache.Decide). A dirty
// working tree is always an explicit miss. The shared store is opt-in via
// SOP_VERIFY_CACHE_SHARED and disabled by default, so the default lookup/store
// behavior is local-only and unchanged apart from the added provenance line.
// Surfacing only reads the append-only decision record; it never rewrites an
// archive and never transitions state.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/commandpolicy"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/validate"
	"github.com/imhttran/agentic-sop/internal/verifcache"
)

// validateReuse is the read-only reuse provenance produced for one validate
// run: the evidence key digest, whether the result was reused or run, the store
// that answered the lookup, and the invalidation reason on a miss. The zero
// value means reuse was not consulted.
type validateReuse struct {
	// Consulted is true only when the cache path was actually evaluated.
	Consulted bool
	// KeyDigest is the identity digest of the current state (Identity.Digest).
	KeyDigest string
	// Reused reports whether the run reused prior VERIFIED evidence.
	Reused bool
	// Store names the store that answered the lookup (local/shared), empty when
	// reuse was not consulted.
	Store string
	// Reason is the fail-closed invalidation reason on a miss: dirty tree, no
	// prior evidence, identity mismatch, or a non-VERIFIED verdict.
	Reason string
	// Scope is the validation scope identity of the current state.
	Scope string
	// MatchPresent reports that an exact-identity entry existed but was not
	// reusable (for example non-VERIFIED evidence), so operators can distinguish
	// "no evidence" from "evidence present but rejected".
	MatchPresent bool
	// RecordErr is set when an append-only decision record could not be
	// written, so a lost audit record is surfaced rather than dropped.
	RecordErr string
}

// identityInputs enumerates the effective validation inputs for the configured
// validation and hashes their current contents, including uncommitted changes.
// The set is the project configuration file plus any check command that names a
// single repository-relative file; each entry is a path + content hash pair so a
// changed relevant input changes the effective-input digest. It is deterministic
// and duplicate-free.
func identityInputs(dir string, cfg *config.Config) []verifcache.Input {
	seen := map[string]bool{}
	var out []verifcache.Input
	add := func(rel string) {
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel == "" || rel == "." || seen[rel] {
			return
		}
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			// An input that does not exist contributes no pair; the digest is
			// then computed over the inputs that do exist, which is still a
			// deterministic, content-sensitive key.
			return
		}
		seen[rel] = true
		out = append(out, verifcache.Input{Path: rel, Hash: hashBytes(data)})
	}
	// The configuration declares the validation; a change to it must invalidate.
	add(config.FileName)
	for _, c := range validate.Checks(cfg.Validation) {
		if isRepoFile(c.Command) {
			add(c.Command)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// isRepoFile reports whether a check command names a repository-relative file
// that can be hashed (for example a script path), rather than an arbitrary argv
// whose content is not a single input. Only unambiguous single-token relative
// paths qualify; everything else contributes no input.
func isRepoFile(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" || strings.ContainsAny(cmd, " \t") {
		return false
	}
	if filepath.IsAbs(cmd) || strings.HasPrefix(cmd, "..") {
		return false
	}
	return filepath.Ext(cmd) != ""
}

// hashBytes returns the hex sha256 of data, the caller-supplied content hash
// consumed by verifcache.EffectiveInputDigest.
func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// validateScope derives the validation scope identity for a validate run: the
// set of criterion/check identifiers the evidence covers. It is built from the
// configured validation categories and their commands, so evidence for one
// configured scope is never reused for another.
func validateScope(cfg *config.Config) string {
	ids := make([]string, 0, 8)
	for _, c := range validate.Checks(cfg.Validation) {
		if category := strings.TrimSpace(string(c.Category)); category != "" {
			ids = append(ids, "category:"+category)
		}
		if strings.TrimSpace(c.Command) != "" {
			ids = append(ids, "check:"+c.Command)
		}
	}
	return verifcache.ScopeIdentity(ids)
}

// validateCommandPolicy classifies each configured check command with the
// deterministic command-policy classifier, so a policy change that reclassifies
// a command changes the identity's CommandPolicy component and invalidates the
// entry. Classification is classification-only: no command is executed. A
// command that cannot be classified contributes an explicit UNCLASSIFIED class,
// which is conservative (a weaker key, never a stronger one).
func validateCommandPolicy(cfg *config.Config) []string {
	checks := validate.Checks(cfg.Validation)
	out := make([]string, 0, len(checks))
	for _, c := range checks {
		argv := strings.Fields(strings.TrimSpace(c.Command))
		class, err := commandpolicy.Classify(argv)
		if err != nil || class == "" {
			out = append(out, "UNCLASSIFIED")
			continue
		}
		out = append(out, string(class))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// toolchainGoVersion resolves the Go toolchain version fact via `go version`,
// falling back to an empty version when the toolchain cannot be resolved. It is
// a process/runtime fact only; no verification command runs.
func toolchainGoVersion() string {
	out, err := exec.Command("go", "version").Output()
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(out))
	if len(fields) >= 3 {
		return fields[2]
	}
	return ""
}

// buildValidateIdentity composes the extended verifcache/2 identity for a
// validate run: the repository revision (HEAD + dirty + effective-input content
// digest), the effective validation inputs (path + content hash, including
// uncommitted changes), the command set, the environment (PATH + toolchain
// identity), the command-policy classification, the validation scope, and the
// verifier identity. Every component participates in Identity.Digest, so a
// changed input, PATH/toolchain, scope, or command yields a different key.
func buildValidateIdentity(dir string, cfg *config.Config, head string, dirty bool) verifcache.Identity {
	inputs := identityInputs(dir, cfg)
	inputsDigest := verifcache.EffectiveInputDigest(inputs)
	policy := validateCommandPolicy(cfg)
	env := verifcache.EnvironmentDigest(verifcache.EnvironmentInputs{
		PATH:          os.Getenv("PATH"),
		GoVersion:     toolchainGoVersion(),
		GOOS:          runtime.GOOS,
		GOARCH:        runtime.GOARCH,
		CommandPolicy: policy,
	})
	return verifcache.Identity{
		Schema:        verifcache.SchemaVersion,
		Repository:    verifcache.RepositoryIdentity(head, dirty, inputsDigest),
		Inputs:        inputsDigest,
		Command:       verifcache.CommandIdentity(checkCommands(cfg.Validation)),
		Environment:   env,
		CommandPolicy: verifcache.CommandPolicyIdentity(policy),
		Scope:         validateScope(cfg),
		Verifier:      "sop/validate@" + Version,
	}
}

// sharedStoreFromEnv resolves the opt-in shared cross-worktree store. It is
// disabled by default: an unset or blank SOP_VERIFY_CACHE_SHARED yields a
// disabled store, so lookup/store is local-only and behavior is unchanged.
func sharedStoreFromEnv() verifcache.SharedStore {
	return verifcache.SharedFromEnv(os.Getenv)
}

// currentValidateRunID resolves the run id a validate invocation should attribute
// its decision record to. It uses the plan-level run id the report/verify surfaces
// already read; when none is recorded there is no run directory to place the
// record in, so recording is skipped.
func currentValidateRunID(dir string) string {
	return activePlanID(dir)
}

// decisionRecordPath returns the run-relative reuse-decision record path when a
// run id is available, or "" when there is none (so callers can skip recording).
func decisionRecordPath(dir string) string {
	runID := currentValidateRunID(dir)
	if runID == "" {
		return ""
	}
	return verifcache.DecisionPath(dir, runID)
}

// recordDecision appends exactly one reuse-decision record for the decision, when
// a run-relative record path is available. Recording is append-only and never
// rewrites prior records. Its error is returned (never silently swallowed), so a
// caller that requires an audit trail can surface a lost decision; it never
// changes the reuse decision itself.
func recordDecision(path string, id verifcache.Identity, hit bool, source, reason string, pri verifcache.Entry) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	rec := verifcache.NewDecisionRecord(id, hit, source, sourceRun(path), pri, reason)
	return verifcache.AppendDecision(path, rec)
}

// sourceRun names the run whose decision this is, derived from the decision-record
// path's run directory. It is provenance for the record itself.
func sourceRun(decisionPath string) string {
	return filepath.Base(filepath.Dir(decisionPath))
}

// writeReuseProvenance renders the read-only reuse provenance line for `sop
// validate`. It names the evidence key, the reuse-vs-run decision, the store that
// answered, and the invalidation reason on a miss. It changes no PASS/FAIL
// semantics and performs no state transition.
func writeReuseProvenance(w io.Writer, r validateReuse) {
	if !r.Consulted {
		return
	}
	decision := "run"
	if r.Reused {
		decision = "reuse"
	}
	fmt.Fprintf(w, "reuse: %s (key %s)\n", decision, shortDigest(r.KeyDigest))
	if r.RecordErr != "" {
		fmt.Fprintf(w, "  warning: decision record not written: %s\n", r.RecordErr)
	}
	if r.Reused {
		fmt.Fprintf(w, "  store:  %s\n", r.Store)
		return
	}
	if r.MatchPresent {
		fmt.Fprintln(w, "  match:  exact identity found but not reusable")
	}
	if r.Reason != "" {
		fmt.Fprintf(w, "  reason: %s\n", r.Reason)
	}
}

// shortDigest abbreviates a digest for display while staying recognizable and
// deterministic. An empty digest renders as "unknown".
func shortDigest(digest string) string {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return "unknown"
	}
	if len(digest) > 12 {
		return digest[:12]
	}
	return digest
}

// reuseDecision is one reuse decision read read-only from a run's append-only
// decision record. Unknown is true when no readable decision was found; the
// rendering then degrades to "unknown" without failing the command.
type reuseDecision struct {
	Unknown   bool
	Hit       bool
	KeyDigest string
	Scope     string
	Store     string
	PriorRun  string
	Reason    string
	Time      string
}

// readReuseDecision reads the most recent reuse decision for a run. It is
// inspection only: the decision record is opened read-only and never modified,
// and a missing, empty, or malformed record degrades to Unknown=true without an
// error.
func readReuseDecision(dir, runID string) reuseDecision {
	if strings.TrimSpace(runID) == "" {
		return reuseDecision{Unknown: true}
	}
	data, err := os.ReadFile(verifcache.DecisionPath(dir, runID))
	if err != nil {
		return reuseDecision{Unknown: true}
	}
	var last verifcache.DecisionRecord
	found := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec verifcache.DecisionRecord
		if json.Unmarshal([]byte(line), &rec) != nil {
			// A malformed line is skipped; a later well-formed decision still
			// renders, and a record with no well-formed line degrades to unknown.
			continue
		}
		last = rec
		found = true
	}
	if !found {
		return reuseDecision{Unknown: true}
	}
	return reuseDecision{
		Hit:       last.Hit,
		KeyDigest: last.KeyDigest,
		Scope:     last.Scope,
		Store:     last.Source,
		PriorRun:  last.PriorRun,
		Reason:    last.Reason,
		Time:      last.Time,
	}
}

// writeReuseDecisionSummary renders a run's recorded reuse decision read-only for
// `sop report` / `sop verify`. It renders an "unknown" line when no readable
// decision exists, so surfacing never fails a command and never transitions
// state.
func writeReuseDecisionSummary(w io.Writer, dir, runID string) {
	dec := readReuseDecision(dir, runID)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Reuse decision:")
	if dec.Unknown {
		fmt.Fprintf(w, "  %-9s %s\n", "Evidence:", "unknown (no readable decision record)")
		return
	}
	fmt.Fprintf(w, "  %-9s %s\n", "Evidence:", shortDigest(dec.KeyDigest))
	decision := "run"
	if dec.Hit {
		decision = "reuse"
	}
	fmt.Fprintf(w, "  %-9s %s\n", "Decision:", decision)
	if dec.Store != "" {
		fmt.Fprintf(w, "  %-9s %s\n", "Store:", dec.Store)
	}
	if dec.PriorRun != "" {
		fmt.Fprintf(w, "  %-9s %s\n", "Prior run:", dec.PriorRun)
	}
	if !dec.Hit && dec.Reason != "" {
		fmt.Fprintf(w, "  %-9s %s\n", "Reason:", dec.Reason)
	}
}
