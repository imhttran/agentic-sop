package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/imhttran/agentic-sop/internal/activity"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/perf"
	"github.com/imhttran/agentic-sop/internal/planflow"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/review"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/taskfile"
	"github.com/imhttran/agentic-sop/internal/testrunner"
)

// runOptionalJEV invokes the optional JEV analysis at the quality seam, between
// review and the quality decision, but only when JEV is enabled. It performs no
// state transition, persistence mutation, or Git/PR/merge action; it returns
// evidence the gate may weigh, or nil when JEV did not run.
//
// Enablement is the single gate: with JEV disabled the analyzer factory is never
// consulted, so JEV cannot run. An absent analyzer (no implementation wired) is
// not an error — the lifecycle proceeds exactly as if JEV did not exist.
//
// The returned evidence is stamped with the task and run that produced it, so a
// finding stays associated with its provenance wherever it is consumed. When JEV
// runs, its invocation metrics (count, duration, provider/model, tool calls,
// finding count, blocking finding count) are recorded on rec and carried on the
// evidence. Those metrics are diagnostic only — they never affect the verdict,
// and a missing value never implies PASS or FAIL.
//
// rn is the task's run, whose accumulated change evidence (bootstrapped from the
// activity stream for a legacy task) is the task's changed files; dir is the
// repository root used to supply bounded file context when the current diff is
// empty. Both widen JEV's view from the current invocation to the whole task.
func runOptionalJEV(ctx context.Context, cfg config.Config, d deps, spec *taskfile.Spec, rn *runpkg.Run, diff string, suite testrunner.SuiteResult, report review.Report, dir string, rec *perf.Recorder) *jevRunEvidence {
	if !cfg.JEVActive() {
		return nil
	}
	if d.newJEVAnalyzer == nil {
		// No implementation wired: JEV stays optional and non-fatal.
		return nil
	}

	analyzer, err := d.newJEVAnalyzer(cfg)
	if err != nil || analyzer == nil {
		// A disabled/misconfigured JEV is never fatal to the lifecycle; record
		// nothing rather than failing the run.
		return nil
	}

	// JEV will run: assemble its task-scoped change evidence. A task whose
	// implementation predates changed-file persistence bootstraps it here from the
	// run's activity stream, so the whole task is visible to JEV instead of only the
	// (possibly empty) current invocation.
	taskFiles := taskChangeEvidence(rn)

	activity.FromContext(ctx).Emit(activity.StageJEV, "analyzing changes", "")
	outcome := runpkg.RunJEV(ctx, analyzer, buildJEVInvocation(spec, diff, suite, report, taskFiles, dir))
	ev := jevEvidence(outcome)
	if ev == nil {
		return nil
	}

	// Capture the invocation metrics. JEV cost/time is recorded apart from
	// PLAN/IMPLEMENT/FIX and validation/review, so the two are distinguishable.
	// A fail-closed JEV carries a zero Result and so reports no findings, but
	// still records the invocation and its cost, which is never a pass signal.
	metrics := perf.JEV{
		TotalMS:          outcome.DurationMS,
		Provider:         outcome.Provider,
		Model:            outcome.Model,
		ToolCalls:        outcome.ToolCalls,
		Findings:         len(outcome.Result.Findings),
		BlockingFindings: quality.JEVBlockingFindings(cfg.Quality.JEVFailOn(), ev),
	}
	if rec != nil {
		rec.JEVInvocation(metrics)
	}

	return &jevRunEvidence{TaskID: spec.ID, RunID: rn.State().ID, Evidence: ev, Result: outcome.Result, Metrics: &metrics}
}

// jevRunEvidence is a JEV result stamped with the task and run that produced it.
// The identifiers are provenance metadata: they associate the evidence with its
// origin and are not repeated on every finding. It carries the gate-shaped
// evidence (see quality.JEVEvidence) and nothing else; it is never a lifecycle
// transition, and it holds no authority over task state.
type jevRunEvidence struct {
	// TaskID is the authoritative task identifier (spec.ID), when known.
	TaskID string
	// RunID is the current run identifier.
	RunID string
	// Evidence is the gate-shaped JEV evidence.
	Evidence *quality.JEVEvidence
	// Result is the raw JEV result the evidence was shaped from. It is a
	// read-only snapshot: the FIX-bound payload contract (see fixPayloads) is
	// projected from it. A fail-closed evidence (malformed, INCOMPLETE, or
	// ERROR) carries a zero Result, so no payloads are ever fabricated for it.
	Result jev.Result
	// Metrics are the invocation's diagnostic metrics (count, duration,
	// provider/model, tool calls, finding counts). They are metadata only and
	// never feed a decision; a nil value means none were captured, which never
	// implies PASS or FAIL.
	Metrics *perf.JEV
}

// gateEvidence returns the evidence the quality gate weighs, or nil when there
// is none. A nil receiver yields nil, so an absent JEV never reaches the gate.
func (e *jevRunEvidence) gateEvidence() *quality.JEVEvidence {
	if e == nil {
		return nil
	}
	return e.Evidence
}

// fixPayloads projects the JEV result into the FIX-bound payload contract (see
// jev.Result.PayloadsForTask), ordered deterministically by severity then file
// then line so FIX receives a stable, actionable list. Findings retain their
// severity, file, line, category, finding, and evidence, and every payload is
// stamped with the task and run that produced it, so findings stay associated
// with their provenance.
//
// It is a read-only projection: it performs no repository mutation and grants no
// authority — FIX alone decides whether and how to act. A fail-closed evidence
// (malformed, INCOMPLETE, or ERROR) yields no payloads: a fail-closed JEV is
// never turned into a clean FIX context that implies PASS. A nil receiver yields
// no payloads.
func (e *jevRunEvidence) fixPayloads() []jev.FindingPayload {
	if e == nil || e.Evidence == nil || e.Evidence.FailClosed {
		return nil
	}
	payloads := e.Result.PayloadsForTask(e.TaskID, e.RunID)
	sortPayloads(payloads)
	return payloads
}

// actionablePayloads returns the payloads whose severity is blocking under the
// configured JEV policy — exactly the findings the gate fails on and the fix
// loop acts on. It reuses the single JEV severity policy
// (quality.JEVSeverityPriority) so the FIX context and the gate can never
// disagree about what blocks. Non-blocking findings are not promoted.
func (e *jevRunEvidence) actionablePayloads(failOn []string) []jev.FindingPayload {
	payloads := e.fixPayloads()
	if len(payloads) == 0 {
		return nil
	}
	out := payloads[:0:0]
	for _, p := range payloads {
		if quality.JEVSeverityPriority(p.Severity, failOn) == quality.JEVBlocking {
			out = append(out, p)
		}
	}
	return out
}

// sortPayloads orders payloads deterministically by severity rank, then file,
// then line. The order is stable and depends only on the finding values, never on
// model output or map iteration, so FIX context is reproducible.
func sortPayloads(payloads []jev.FindingPayload) {
	sort.SliceStable(payloads, func(i, j int) bool {
		a, b := payloads[i], payloads[j]
		if ra, rb := severityRank(a.Severity), severityRank(b.Severity); ra != rb {
			return ra > rb
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
}

// severityRank orders severities high-to-low for deterministic presentation. It
// knows the review severity vocabulary the JEV policy uses; an unknown severity
// ranks lowest so it never outranks a known one.
func severityRank(sev string) int {
	switch review.Severity(strings.ToUpper(strings.TrimSpace(sev))) {
	case review.Critical:
		return 4
	case review.High:
		return 3
	case review.Medium:
		return 2
	case review.Low:
		return 1
	default:
		return 0
	}
}

// findingsSection renders the FIX-context section for the JEV findings that are
// actionable under the configured policy — exactly the blocking ones, the same
// rule the gate and the fix loop already use (quality.JEVSeverityPriority over
// the fail_on severities). It renders nothing when JEV did not run, produced no
// actionable finding, or failed closed (which carries no finding). The findings
// are projected through the FIX-bound payload contract and rendered in its
// deterministic order.
func (e *jevRunEvidence) findingsSection(failOn []string) string {
	payloads := e.actionablePayloads(failOn)
	if len(payloads) == 0 {
		return ""
	}
	blocks := make([]string, 0, len(payloads))
	for _, p := range payloads {
		blocks = append(blocks, jevPayloadBlock(p))
	}
	return e.sectionHeading() + "\n\n" + strings.Join(blocks, "\n\n")
}

// sectionHeading names the section and its provenance (the task and run the
// evidence came from). Unknown identifiers are omitted rather than rendered
// empty, so the heading degrades cleanly.
func (e *jevRunEvidence) sectionHeading() string {
	var parts []string
	if id := strings.TrimSpace(e.TaskID); id != "" {
		parts = append(parts, "task "+id)
	}
	if id := strings.TrimSpace(e.RunID); id != "" {
		parts = append(parts, "run "+id)
	}
	if len(parts) == 0 {
		return "# JEV Findings"
	}
	return "# JEV Findings (" + strings.Join(parts, ", ") + ")"
}

// jevPayloadBlock renders one actionable JEV payload in the FIX-context shape:
//
//	[HIGH] path/to/file.go:123
//	Category: quality
//	Finding: file handle not closed
//	Evidence: <evidence>
//
// Category classifies the finding; the Finding line is the actionable problem FIX
// must address. They are rendered as distinct lines and never merged, so neither
// can stand in for the other. A payload with no category simply omits the
// Category line rather than folding a category into Finding.
func jevPayloadBlock(p jev.FindingPayload) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[%s]", p.Severity)
	if loc := jevPayloadLocation(p); loc != "" {
		b.WriteString(" " + loc)
	}
	b.WriteByte('\n')

	if cat := strings.TrimSpace(p.Category); cat != "" {
		fmt.Fprintf(&b, "Category: %s\n", cat)
	}

	msg := strings.TrimSpace(p.Finding)
	if msg == "" {
		msg = "(no description)"
	}
	fmt.Fprintf(&b, "Finding: %s\n", msg)

	if ev := strings.TrimSpace(p.Evidence); ev != "" {
		fmt.Fprintf(&b, "Evidence: %s\n", ev)
	}
	return strings.TrimSpace(b.String())
}

// jevPayloadLocation renders a payload's file:line location, degrading to
// whatever is known (a bare line, a bare path, or nothing).
func jevPayloadLocation(p jev.FindingPayload) string {
	path := strings.TrimSpace(p.File)
	switch {
	case path == "":
		return ""
	case p.Line > 0:
		return fmt.Sprintf("%s:%d", path, p.Line)
	default:
		return path
	}
}

// buildJEVInvocation assembles the task-scoped, read-only context JEV receives at
// the quality seam: the task and its criteria, the task's accumulated changed
// files, the repository context, and the validation and review outcomes.
//
// ChangedFiles is the task's accumulated change set, not merely the files the
// current invocation touched: a task implemented across several invocations keeps
// its earlier evidence, so a no-change final invocation still has something for
// JEV to review. RepositoryContext widens the current diff with bounded excerpts
// of the accumulated implementation files the diff does not itself cover.
func buildJEVInvocation(spec *taskfile.Spec, diff string, suite testrunner.SuiteResult, report review.Report, taskFiles []string, dir string) runpkg.JEVInvocation {
	return runpkg.JEVInvocation{
		Task:              spec.Render(),
		Criteria:          strings.Join(spec.AcceptanceCriteria, "\n"),
		ChangedFiles:      taskFiles,
		RepositoryContext: jevRepositoryContext(dir, diff, taskFiles),
		ValidationResult:  suiteSummary(suite),
		ReviewResult:      reviewSummary(report),
	}
}

// jevRepositoryContext assembles the read-only repository context JEV reviews: the
// current invocation's diff, plus bounded excerpts of the task's accumulated
// implementation files that the diff does not itself cover. In the ordinary case
// (the current invocation produced the task's change) the excerpt set is empty and
// the context is exactly the diff, so behavior is unchanged. In the no-change
// final invocation of a task whose earlier work was committed, the diff is empty
// and the excerpts carry the accumulated implementation, so JEV still reviews the
// task rather than an empty change set. A nil/empty result degrades to the diff.
func jevRepositoryContext(dir, diff string, taskFiles []string) string {
	covered := make(map[string]bool)
	for _, f := range changedFiles(diff) {
		covered[f] = true
	}
	var uncovered []string
	for _, f := range taskFiles {
		if !covered[f] {
			uncovered = append(uncovered, f)
		}
	}
	excerpts := taskFileContext(dir, uncovered)
	switch {
	case excerpts == "":
		return diff
	case strings.TrimSpace(diff) == "":
		return excerpts
	default:
		return diff + "\n\n" + excerpts
	}
}

// Task-file context bounds: how many accumulated files are excerpted, how many
// bytes of each, and the total. They keep the context a bounded snapshot rather
// than an unrestricted repository dump.
const (
	maxJEVContextFiles     = 12
	maxJEVContextFileBytes = 4 << 10
	maxJEVContextBytes     = 32 << 10
)

// Optional environment overrides for the task-file context bounds. Raising them
// lets JEV review a larger accumulated change set; an unset, non-numeric, or
// non-positive value keeps the built-in default, so behavior is unchanged when
// they are not set. The assembled prompt is separately bounded by
// SOP_JEV_MAX_PROMPT_RUNES (see internal/jev).
const (
	envJEVContextFiles     = "SOP_JEV_CONTEXT_FILES"
	envJEVContextFileBytes = "SOP_JEV_CONTEXT_FILE_BYTES"
	envJEVContextBytes     = "SOP_JEV_CONTEXT_TOTAL_BYTES"
)

// jevContextFiles is how many accumulated files are excerpted.
func jevContextFiles() int { return positiveEnvInt(envJEVContextFiles, maxJEVContextFiles) }

// jevContextFileBytes is how many bytes of each excerpted file are kept.
func jevContextFileBytes() int {
	return positiveEnvInt(envJEVContextFileBytes, maxJEVContextFileBytes)
}

// jevContextBytes is the total excerpt budget.
func jevContextBytes() int { return positiveEnvInt(envJEVContextBytes, maxJEVContextBytes) }

// positiveEnvInt returns the named environment variable when it parses as a
// positive integer, and fallback otherwise (unset, blank, non-numeric, or <= 0).
func positiveEnvInt(name string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

// taskFileContext renders bounded excerpts of the given repository files for JEV.
// It reads each file under dir, truncating large files and stopping at the total
// bound, skips anything unreadable or outside the repository (a deleted file, a
// directory, an escaping path), and returns "" when nothing could be read. It is
// read-only and holds repository content only — never prompts or secrets beyond
// what the files themselves contain.
func taskFileContext(dir string, files []string) string {
	maxFiles := jevContextFiles()
	maxBytes := jevContextBytes()
	maxFileBytes := jevContextFileBytes()
	var b strings.Builder
	count := 0
	for _, path := range files {
		if count >= maxFiles || b.Len() >= maxBytes {
			break
		}
		if !safeRepoPath(path) || isSOPPath(path) {
			continue
		}
		limit := maxFileBytes
		if remaining := maxBytes - b.Len(); remaining < limit {
			limit = remaining
		}
		if limit <= 0 {
			break
		}
		content, ok := readFileHead(filepath.Join(dir, filepath.FromSlash(path)), limit)
		if !ok {
			continue
		}
		snippet, truncated := truncateContent(content, limit)
		fmt.Fprintf(&b, "### %s\n%s\n", path, snippet)
		if truncated {
			b.WriteString("... (truncated)\n")
		}
		b.WriteByte('\n')
		count++
	}
	if count == 0 {
		return ""
	}
	return strings.TrimSpace("task implementation files (accumulated across the task's invocations):\n\n" + b.String())
}

// readFileHead reads at most limit+1 bytes of a file, so a very large file is not
// read in full merely to be truncated. It reports whether the file could be read.
func readFileHead(path string, limit int) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return "", false
	}
	return string(data), true
}

// safeRepoPath reports whether path is a repository-relative path the context
// reader may open: not absolute, and not escaping the repository root. It is
// defense in depth — paths come from git diffs and tool arguments — so a crafted
// path can never read outside the working tree.
func safeRepoPath(path string) bool {
	if path == "" || filepath.IsAbs(path) || strings.HasPrefix(path, "/") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

// truncateContent cuts s to at most max bytes, preferring a line boundary so the
// excerpt ends somewhere readable. It reports whether it truncated.
func truncateContent(s string, max int) (string, bool) {
	if len(s) <= max {
		return s, false
	}
	cut := strings.LastIndexByte(s[:max], '\n')
	if cut <= 0 {
		cut = max
	}
	return s[:cut], true
}

// taskInvocationChanges returns the repository paths to attribute to the task for
// one invocation: the paths the agent reported changing, or — when the provider
// cannot report them — the paths derived from the working-tree diff. Preferring
// the agent's own evidence keeps unrelated pre-existing dirty files out of the
// task's change set.
func taskInvocationChanges(reported []string, diff string) []string {
	if len(reported) > 0 {
		return reported
	}
	return changedFiles(diff)
}

// recordTaskChanges persists the given paths as task-scoped change evidence in the
// run directory, so a later invocation still knows what the task changed. SOP's
// own state and output paths are dropped first, so a dirty .agent-sdlc/config.yaml
// (an intentional user-owned edit) or a generated report is never attributed to
// the task. It is best-effort: a write failure never changes the run outcome.
func recordTaskChanges(rn *runpkg.Run, paths []string) {
	if rn == nil {
		return
	}
	changed := taskChangedFiles(paths)
	if len(changed) == 0 {
		return
	}
	_ = rn.RecordChangedFiles(changed)
}

// taskChangeEvidence returns the task's accumulated changed files, bootstrapping
// legacy evidence first when none has been recorded. A nil run yields none.
func taskChangeEvidence(rn *runpkg.Run) []string {
	if rn == nil {
		return nil
	}
	bootstrapTaskChangeEvidence(rn)
	return rn.ChangedFiles()
}

// bootstrapTaskChangeEvidence recovers a legacy task's changed files from the run's
// persisted activity stream when no change evidence has been recorded yet. It is a
// no-op when evidence already exists (the normal path is untouched) and attributes
// nothing when the stream carries no reliable mutation evidence, so ambiguity is
// never turned into claimed ownership. The recovered paths are filtered and
// persisted exactly like live evidence, so they become normal provenance for
// future invocations.
func bootstrapTaskChangeEvidence(rn *runpkg.Run) {
	if rn == nil || len(rn.ChangedFiles()) > 0 {
		return
	}
	recordTaskChanges(rn, rn.ActivityChangePaths())
}

// taskChangedFiles drops the paths SOP owns from a task's change set, so SOP's own
// state and output directories are never attributed to a task however the paths
// were derived (agent-reported or diff-derived).
func taskChangedFiles(paths []string) []string {
	var out []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" || isSOPPath(p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// isSOPPath reports whether a repository path is SOP's own state or output,
// which is never a task's change. It is the single definition of the SOP-owned
// prefixes, so filtering recorded paths and filtering context excerpts agree.
func isSOPPath(path string) bool {
	path = filepath.ToSlash(path)
	for _, prefix := range []string{config.DirName, planflow.ReportsDir} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

// jevEvidence maps a JEV outcome to gate evidence. It fails closed: an analyzer
// that errored, or a result that does not validate, becomes a non-pass evidence
// whose reason explains why. An outcome that never ran yields no evidence, so
// the verdict is unchanged.
func jevEvidence(outcome runpkg.JEVOutcome) *quality.JEVEvidence {
	if !outcome.Ran {
		return nil
	}
	if outcome.FailClosed() {
		return &quality.JEVEvidence{
			FailClosed: true,
			Reason:     "JEV analysis failed (fail closed): " + outcome.Error.Error(),
		}
	}
	ev := quality.NewJEVEvidence(outcome.Result)
	return &ev
}

// suiteSummary renders the validation outcome as bounded text.
func suiteSummary(suite testrunner.SuiteResult) string {
	if len(suite.Results) == 0 {
		return ""
	}
	lines := make([]string, 0, len(suite.Results))
	for _, r := range suite.Results {
		lines = append(lines, fmt.Sprintf("%s %s `%s`", r.Status, r.Category, r.Command))
	}
	return strings.Join(lines, "\n")
}

// reviewSummary renders the review outcome as bounded text.
func reviewSummary(report review.Report) string {
	var b strings.Builder
	if s := strings.TrimSpace(report.Summary); s != "" {
		b.WriteString(s)
	}
	for _, f := range report.Findings {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s: %s — %s", f.Severity, f.Title, f.Detail)
	}
	return b.String()
}

// changedFiles extracts the repository-relative paths a diff touched, so JEV
// receives the same change set the gate reasons about. It honors git's rename
// and /dev/null semantics: a deletion's "+++ b/..." side is /dev/null (and its
// "--- a/..." side is the real path), while an addition's "--- a/..." side is
// /dev/null; only the side that names a real path is recorded. Diff header
// lines may carry a trailing tab and timestamp, so the path is trimmed at the
// tab as well as at surrounding whitespace.
func changedFiles(diff string) []string {
	seen := map[string]bool{}
	var files []string
	add := func(path string) {
		path = diffHeaderPath(path)
		if path == "" || path == "/dev/null" || seen[path] {
			return
		}
		seen[path] = true
		files = append(files, path)
	}

	lines := strings.Split(diff, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "--- a/"):
			// The "---" side names a real file unless it is a new file (the
			// "+++" side is /dev/null), in which case the next line carries the
			// real path.
			from := strings.TrimPrefix(line, "--- a/")
			to := ""
			if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "+++ b/") {
				to = strings.TrimPrefix(lines[i+1], "+++ b/")
			}
			if diffHeaderPath(from) == "/dev/null" {
				// New file: the real path is on the "+++" side.
				add(to)
				continue
			}
			add(from)
		case strings.HasPrefix(line, "+++ b/"):
			to := strings.TrimPrefix(line, "+++ b/")
			// A "+++" side that names /dev/null is a deletion; its real path was
			// already recorded from the "---" side above. Record it here only when
			// it names a real path (additions and modifications).
			if diffHeaderPath(to) == "/dev/null" {
				continue
			}
			add(to)
		}
	}
	return files
}

// diffHeaderPath normalizes a path taken from a git diff header line: git appends
// a tab-separated timestamp to some header lines, and the path may carry
// incidental surrounding whitespace. A header naming no path (an empty or
// /dev/null side) is returned unchanged so callers can detect it.
func diffHeaderPath(path string) string {
	if i := strings.IndexByte(path, '\t'); i >= 0 {
		path = path[:i]
	}
	return strings.TrimSpace(path)
}
