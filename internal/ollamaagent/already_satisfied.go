package ollamaagent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

const alreadySatisfiedInstruction = `If the requested implementation already exists, use the supplied already-satisfied verification contract: inspect its concrete evidence, run every required validation, and explicitly return ALREADY_SATISFIED. Do not manufacture changes. Without verified proof, implement the requested change. The existing discovery, stale, and iteration limits remain in force.`

// alreadySatisfiedContract is the single definition of the already-satisfied
// verification contract, shared by the standing task prompt and the one bounded
// existing-work verification request.
const alreadySatisfiedContract = `If the implementation already exists, read the relevant files and run EVERY required validation command successfully. Then explicitly return {"status":"completed","completion":"ALREADY_SATISFIED","changes_expected":false,"evidence":{"acceptance":[{"criterion":"<exact supplied criterion>","paths":["<successfully read repository file>"]}]}}. Map EVERY criterion to concrete inspected files. SOP verifies tool results and unchanged repository state. Reads, narration, no-op writes, and model assertions alone cannot establish completion. Existing discovery, stale, and iteration bounds still apply.`

// satisfactionProof is invocation-local verification, separate from mutation
// and discovery progress. Validation never resets the stale streak.
type satisfactionProof struct {
	baseline string
	reads    map[string]bool
	commands map[string]bool
	// ran records that a run_command call was attempted (so a not-executed required
	// validation is distinguishable from one that executed but failed).
	ran map[string]bool
	// commandText maps a command identity to the operator-facing command text, so an
	// evidence correction can name the validation commands the harness actually observed
	// execute successfully instead of relying on the model recollection.
	commandText map[string]string
	// correctionOpen is true while the single bounded correction phase is open: it reopens
	// read-only repository tools so the model can act on corrective feedback even after the
	// discovery window has closed. correctionToolCalls and correctionTurns are the phase own
	// budgets; they are never the discovery, stale, attempt, or retry counters.
	correctionOpen      bool
	correctionToolCalls int
	correctionTurns     int
	// evidenceCorrectionUsed records that the one permitted evidence-only correction has
	// already been requested for this invocation, so a claim is corrected at most once.
	evidenceCorrectionUsed bool
	// existingWorkRequested records that the single bounded existing-work verification
	// request has already been made for this invocation, so it is requested at most once.
	existingWorkRequested bool
}

func (h *Harness) satisfactionProof(ctx context.Context, req agent.Request) *satisfactionProof {
	p := &satisfactionProof{reads: map[string]bool{}, commands: map[string]bool{}, ran: map[string]bool{}, commandText: map[string]string{}}
	if len(req.AcceptanceCriteria) != 0 && len(req.ValidationCommands) != 0 {
		p.baseline, _ = h.tools.RepositoryFingerprint(ctx)
	}
	return p
}

func commandIdentity(command string) string {
	argv, err := toolharness.SplitCommand(strings.TrimSpace(command))
	if err != nil || len(argv) == 0 {
		return ""
	}
	data, _ := json.Marshal(argv)
	return string(data)
}

// normalizeCriterion folds incidental formatting differences (case, surrounding and
// repeated whitespace) so a criterion is matched by its content, not its exact bytes.
// Coverage is still required in full, so matching by content never weakens the gate.
func normalizeCriterion(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// Correction-phase limits. The phase is a strictly bounded reopening of the read-only
// repository tools for the single corrective opportunity, so it can never become an
// unbounded loop: it terminates on either budget, on the corrected claim, or on the
// existing discovery/stale termination, whichever comes first.
const (
	// correctionToolCalls bounds the read-only interactions the correction phase may execute.
	// One model turn yields at most one tool call (turnToolCall), so the turn budget below
	// already caps the phase at three tool calls; this bound states the effective limit
	// rather than an unreachable ceiling.
	correctionToolCalls = 3
	// correctionIterations bounds the model turns the correction phase may consume.
	correctionIterations = 3
)

// evidenceProblem is one invalid criterion citation, reported with the same reason the
// verifier would use.
type evidenceProblem struct {
	Criterion string
	Path      string
	Reason    string // "missing", "directory", or "unread"
}

// correctionNeed is the deterministic, structured set of reasons a submitted claim cannot be
// accepted. It exists only to build corrective feedback; it never decides acceptance.
type correctionNeed struct {
	MissingCriteria []string
	InvalidPaths    []evidenceProblem
	NotExecuted     []string
	Failed          []string
}

// actionable reports whether the claim has anything the model can actually correct.
func (n correctionNeed) actionable() bool {
	return len(n.MissingCriteria) > 0 || len(n.InvalidPaths) > 0 || len(n.NotExecuted) > 0 || len(n.Failed) > 0
}

// correctableRejection reports whether a rejection is one the bounded correction may
// address: invalid or missing criterion evidence, or a required validation command that did
// not execute or failed. Every other rejection falls through unchanged.
func correctableRejection(code RejectionCode) bool {
	switch code {
	case RejectMissingCriterionEvidence, RejectUnreadEvidencePath, RejectDirectoryEvidence,
		RejectValidationNotExecuted, RejectValidationFailed:
		return true
	}
	return false
}

// classifyEvidencePath reports why a cited criterion path is not valid evidence: "missing"
// when it does not resolve, "directory" when it names a directory, and "unread" when the
// harness never observed the model read it. An empty reason means the path is read-backed
// evidence. It mirrors verifySatisfiedDetailed exactly, so feedback and verdict cannot
// disagree.
func (h *Harness) classifyEvidencePath(path string, p *satisfactionProof) (string, string) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(h.tools.Root(), path)
	}
	canonical, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", "missing"
	}
	if info, statErr := os.Stat(canonical); statErr == nil && info.IsDir() {
		return canonical, "directory"
	}
	if !p.reads[canonical] {
		return canonical, "unread"
	}
	return canonical, ""
}

// correctionNeed returns the deterministic reasons a submitted claim cannot be accepted,
// together with whether the single correction is appropriate. It mirrors the verifier
// preconditions (no mutation, a baseline, a completed status, an unchanged fingerprint) but
// unlike the acceptance predicate it REPORTS every unmet condition -- invalid criterion
// evidence, required validation commands that never executed or failed -- so one corrective
// feedback can name everything the model can fix. It never decides acceptance;
// verifySatisfied remains the sole authority.
func (h *Harness) correctionNeed(ctx context.Context, req agent.Request, st *executionState, p *satisfactionProof, final map[string]any) (correctionNeed, bool) {
	var need correctionNeed
	if st.mutationObserved || p.baseline == "" || final["status"] != string(agent.OutcomeCompleted) {
		return need, false
	}
	data, _ := json.Marshal(final["evidence"])
	var evidence agent.CompletionEvidence
	if json.Unmarshal(data, &evidence) != nil {
		return need, false
	}
	covered := map[string]bool{}
	for _, item := range evidence.Acceptance {
		key := normalizeCriterion(item.Criterion)
		if key == "" || covered[key] || len(item.Paths) == 0 {
			continue
		}
		allRead := true
		for _, path := range item.Paths {
			if _, reason := h.classifyEvidencePath(path, p); reason != "" {
				need.InvalidPaths = append(need.InvalidPaths, evidenceProblem{Criterion: item.Criterion, Path: path, Reason: reason})
				allRead = false
			}
		}
		if allRead {
			covered[key] = true
		}
	}
	for _, criterion := range req.AcceptanceCriteria {
		if key := normalizeCriterion(criterion); key != "" && !covered[key] {
			need.MissingCriteria = append(need.MissingCriteria, criterion)
		}
	}
	waived := map[string]bool{}
	for _, w := range req.ValidationWaivers {
		if id := commandIdentity(w); id != "" {
			waived[id] = true
		}
	}
	for _, command := range req.ValidationCommands {
		identity := commandIdentity(command)
		if identity == "" || waived[identity] {
			continue
		}
		switch {
		case p.commands[identity]:
			// succeeded: nothing to correct
		case p.ran[identity]:
			need.Failed = append(need.Failed, command)
		default:
			need.NotExecuted = append(need.NotExecuted, command)
		}
	}
	if !need.actionable() {
		return need, false
	}
	after, err := h.tools.RepositoryFingerprint(ctx)
	if err != nil || after != p.baseline {
		return need, false
	}
	return need, true
}

// openCorrectionPhase starts the bounded correction phase. It resets ONLY the phase own
// budgets, never the discovery, stale, attempt, or retry counters.
func (p *satisfactionProof) openCorrectionPhase() {
	p.correctionOpen = true
	p.correctionToolCalls = 0
	p.correctionTurns = 0
}

// closeCorrectionPhase ends the phase. A closed phase is never reopened.
func (p *satisfactionProof) closeCorrectionPhase() { p.correctionOpen = false }

// correctionTurn grants the phase one model turn, closing it when the turn budget is spent
// so the phase terminates even if the model never returns a final claim.
func (p *satisfactionProof) correctionTurn() bool {
	if p == nil || !p.correctionOpen {
		return false
	}
	if p.correctionTurns >= correctionIterations {
		p.correctionOpen = false
		return false
	}
	p.correctionTurns++
	return true
}

// correctionPermits reports whether the bounded correction phase authorizes this read-only
// interaction, consuming the phase tool-call budget and closing the phase when it is spent.
// Only the read-only repository tools and non-mutating commands are ever permitted, so the
// phase can neither write nor bypass command policy (the tool harness still classifies and
// may refuse every command). It never credits discovery and never resets a counter.
func (p *satisfactionProof) correctionPermits(name string, args map[string]any) bool {
	if p == nil || !p.correctionOpen {
		return false
	}
	switch name {
	case toolharness.ToolReadFile, toolharness.ToolListFiles, toolharness.ToolSearchFiles:
	case toolharness.ToolRunCommand:
		command, _ := args["command"].(string)
		if commandMutates(command) {
			return false
		}
	default:
		return false
	}
	if p.correctionToolCalls >= correctionToolCalls {
		p.correctionOpen = false
		return false
	}
	p.correctionToolCalls++
	return true
}

// alreadySatisfiedEvidenceCorrection is the single, bounded, corrective request the harness
// sends when an ALREADY_SATISFIED claim is otherwise sound but cannot be accepted. It names
// the harness-owned eligible evidence (files actually read, commands actually executed) and
// everything still wrong (invalid citations, required commands not executed or failed). It
// never substitutes for evidence and never weakens the verifier.
func alreadySatisfiedEvidenceCorrection(need correctionNeed, observedReads, succeeded []string) string {
	var b strings.Builder
	b.WriteString("Your ALREADY_SATISFIED claim was rejected. Fix exactly what is listed, then return the same object once more.\n")
	if len(need.MissingCriteria) > 0 {
		b.WriteString("Declared acceptance criteria with no valid evidence:\n")
		for _, criterion := range need.MissingCriteria {
			fmt.Fprintf(&b, "- %s\n", criterion)
		}
	}
	if len(need.InvalidPaths) > 0 {
		b.WriteString("Invalid criterion evidence (a criterion is covered only by files you actually read):\n")
		for _, problem := range need.InvalidPaths {
			fmt.Fprintf(&b, "- criterion=%q path=%s (%s)\n", problem.Criterion, problem.Path, problem.Reason)
		}
	}
	writeEvidenceList(&b, "Files the harness observed you read in this invocation (eligible criterion evidence):", observedReads)
	writeEvidenceList(&b, "Validation commands the harness observed execute successfully:", succeeded)
	writeEvidenceList(&b, "Required validation commands NOT executed (run them):", need.NotExecuted)
	writeEvidenceList(&b, "Required validation commands that FAILED (fix and re-run them):", need.Failed)
	b.WriteString("A criterion paths list must name FILES the harness observed you read with read_file. A directory, a package path, or a validation command you ran is NOT file evidence for a criterion and is rejected.\n")
	b.WriteString("Do NOT change any file. Read the relevant repository files if you have not already, then return the same object with EVERY criterion mapped to files you actually read:\n")
	b.WriteString(`{"status":"completed","completion":"ALREADY_SATISFIED","changes_expected":false,"evidence":{"acceptance":[{"criterion":"<exact criterion>","paths":["<file you read>"]}]}}`)
	b.WriteString("\nA path you did not read, a fabricated claim, or any repository change is rejected. This is the only correction allowed for this claim.")
	return b.String()
}

// writeEvidenceList renders one labeled list, saying so explicitly when it is empty rather
// than omitting the section.
func writeEvidenceList(b *strings.Builder, heading string, items []string) {
	b.WriteString(heading + "\n")
	if len(items) == 0 {
		b.WriteString("- (none)\n")
		return
	}
	for _, item := range items {
		fmt.Fprintf(b, "- %s\n", item)
	}
}

func (p *satisfactionProof) observe(root, name string, args map[string]any, result string, err error, commandSucceeded bool) {
	if name == toolharness.ToolReadFile {
		identity, ok := discoveryIdentity(root, name, args, result, err)
		if ok {
			p.reads[identity.path] = true
		}
	}
	if name == toolharness.ToolRunCommand {
		command, _ := args["command"].(string)
		identity := commandIdentity(command)
		p.ran[identity] = true
		p.commands[identity] = err == nil && commandSucceeded
		if text := strings.TrimSpace(command); text != "" && identity != "" && p.commandText != nil {
			p.commandText[identity] = text
		}
	}
}

// verify accepts only an explicit claim with complete caller-owned criteria,
// inspected files and every caller-owned validation command actually passing.
// Unknown or changed repository state fails closed. The model selects evidence
// links; it cannot manufacture tool results or substitute easier validations.
// observedReads returns the repository-relative paths the harness observed the model read
// in this invocation, sorted. It is the harness-owned list of eligible criterion evidence,
// so a correction can name files the model actually read instead of trusting recollection.
// A path outside the repository root is reported verbatim.
func (p *satisfactionProof) observedReads(root string) []string {
	if p == nil || len(p.reads) == 0 {
		return nil
	}
	// Reads are recorded with symlinks resolved, so resolve the root the same way
	// (on macOS a temp root under /var is really /private/var).
	if canonical, err := filepath.EvalSymlinks(root); err == nil {
		root = canonical
	}
	prefix := strings.TrimSuffix(filepath.Clean(root), string(filepath.Separator)) + string(filepath.Separator)
	out := make([]string, 0, len(p.reads))
	for path := range p.reads {
		if strings.HasPrefix(path, prefix) {
			out = append(out, filepath.ToSlash(strings.TrimPrefix(path, prefix)))
			continue
		}
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

// succeededCommands returns the validation commands the harness observed execute
// successfully in this invocation, sorted. They are verified separately from criterion
// evidence and can never be cited in a criterion paths list.
func (p *satisfactionProof) succeededCommands() []string {
	if p == nil || len(p.commands) == 0 {
		return nil
	}
	out := make([]string, 0, len(p.commands))
	for identity, ok := range p.commands {
		if !ok {
			continue
		}
		text := strings.TrimSpace(p.commandText[identity])
		if text == "" {
			text = identity
		}
		out = append(out, text)
	}
	sort.Strings(out)
	return out
}

// canRequestExistingWork reports whether the one bounded existing-work verification
// request may be made now. It applies only when the reply claims completion (and is not
// itself an already-satisfied object), this invocation observed NO repository mutation,
// the verification contract is available (a fingerprint baseline was captured), neither
// bounded verification request has been used, and a model turn remains inside the existing
// iteration ceiling. It changes no counter and resets no budget: an invocation with no turn
// left, with a mutation already observed, or with a bounded request already used falls
// through to the existing behaviour unchanged.
func (p *satisfactionProof) canRequestExistingWork(final map[string]any, st *executionState, iteration, maxIterations int) bool {
	if p == nil || p.existingWorkRequested || p.evidenceCorrectionUsed || p.baseline == "" {
		return false
	}
	if st == nil || st.mutationObserved {
		return false
	}
	if final["status"] != string(agent.OutcomeCompleted) || final["completion"] == agent.AlreadySatisfied {
		return false
	}
	return iteration < maxIterations
}

// existingWorkVerificationRequest restates the already-satisfied verification contract
// once, when an invocation reports completion without producing an attempt-local change.
// It asks only for evidence-backed verification of work that already exists; it never asks
// for invented work and never weakens an acceptance condition.
func existingWorkVerificationRequest(req agent.Request) string {
	var b strings.Builder
	b.WriteString("This invocation reported completion but produced no repository change. Do not invent work.\n")
	b.WriteString("If the implementation already exists in the repository, read the relevant files, run every required validation command, and return the existing already-satisfied object once. If it does not exist, report that truthfully instead.\n\n")
	fmt.Fprintf(&b, "Already-satisfied verification contract:\nAcceptance criteria: %q\nRequired validation commands: %q\n", req.AcceptanceCriteria, req.ValidationCommands)
	b.WriteString(alreadySatisfiedContract)
	return b.String()
}

// RejectionCode is a precise, machine-readable reason an ALREADY_SATISFIED claim was
// rejected. It is diagnostic only: it never changes the acceptance conditions.
type RejectionCode string

const (
	RejectRepositoryMutation       RejectionCode = "REPOSITORY_MUTATION"
	RejectMissingBaseline          RejectionCode = "MISSING_BASELINE"
	RejectCompletionStatus         RejectionCode = "COMPLETION_STATUS"
	RejectMalformedEvidence        RejectionCode = "MALFORMED_EVIDENCE"
	RejectMissingCriterionEvidence RejectionCode = "MISSING_CRITERION_EVIDENCE"
	RejectUnreadEvidencePath       RejectionCode = "UNREAD_EVIDENCE_PATH"
	// RejectDirectoryEvidence: a cited path exists but names a directory (or other
	// non-regular file). It is never criterion evidence, because the contract requires
	// files the harness observed being read. The verification outcome is unchanged.
	RejectDirectoryEvidence     RejectionCode = "DIRECTORY_EVIDENCE"
	RejectDeliverableMissing    RejectionCode = "DELIVERABLE_MISSING"
	RejectValidationNotExecuted RejectionCode = "VALIDATION_NOT_EXECUTED"
	RejectValidationFailed      RejectionCode = "VALIDATION_FAILED"
	RejectFingerprintMismatch   RejectionCode = "FINGERPRINT_MISMATCH"
)

// SatisfactionRejection is the structured reason a claim was rejected. It carries only
// identifiers (no raw model output, no secrets), so it is safe to persist.
type SatisfactionRejection struct {
	Code        RejectionCode `json:"code"`
	Detail      string        `json:"detail,omitempty"`
	Criterion   string        `json:"criterion,omitempty"`
	Path        string        `json:"path,omitempty"`
	Command     string        `json:"command,omitempty"`
	Deliverable string        `json:"deliverable,omitempty"`
}

// String renders the rejection as a compact, secret-free diagnostic line.
func (r SatisfactionRejection) String() string {
	parts := []string{string(r.Code)}
	if r.Criterion != "" {
		parts = append(parts, "criterion="+r.Criterion)
	}
	if r.Path != "" {
		parts = append(parts, "path="+r.Path)
	}
	if r.Command != "" {
		parts = append(parts, "command="+r.Command)
	}
	if r.Deliverable != "" {
		parts = append(parts, "deliverable="+r.Deliverable)
	}
	if r.Detail != "" {
		parts = append(parts, "detail="+r.Detail)
	}
	return strings.Join(parts, " ")
}

// verifySatisfied is the boolean acceptance predicate: it accepts only an explicit
// claim with complete, read-backed criterion coverage, all required validation
// executed (or explicitly waived) and an unchanged repository fingerprint. It never
// consults a model claim. verifySatisfiedDetailed returns the structured reason.
func (h *Harness) verifySatisfied(ctx context.Context, req agent.Request, st *executionState, p *satisfactionProof, final map[string]any) bool {
	ok, _ := h.verifySatisfiedDetailed(ctx, req, st, p, final)
	return ok
}

// verifySatisfiedDetailed is verifySatisfied with a machine-readable rejection reason.
// The acceptance conditions are identical; only the diagnostic differs.
func (h *Harness) verifySatisfiedDetailed(ctx context.Context, req agent.Request, st *executionState, p *satisfactionProof, final map[string]any) (bool, SatisfactionRejection) {
	if st.mutationObserved {
		return false, SatisfactionRejection{Code: RejectRepositoryMutation, Detail: "a repository mutation was observed"}
	}
	if p.baseline == "" {
		return false, SatisfactionRejection{Code: RejectMissingBaseline, Detail: "no repository baseline was captured"}
	}
	if final["status"] != string(agent.OutcomeCompleted) {
		return false, SatisfactionRejection{Code: RejectCompletionStatus, Detail: "completion status is not completed"}
	}
	data, _ := json.Marshal(final["evidence"])
	var evidence agent.CompletionEvidence
	if json.Unmarshal(data, &evidence) != nil {
		return false, SatisfactionRejection{Code: RejectMalformedEvidence, Detail: "evidence block did not parse"}
	}
	covered := map[string]bool{}
	for _, item := range evidence.Acceptance {
		key := normalizeCriterion(item.Criterion)
		if key == "" {
			return false, SatisfactionRejection{Code: RejectMissingCriterionEvidence, Detail: "an evidence entry has an empty criterion"}
		}
		if covered[key] {
			return false, SatisfactionRejection{Code: RejectMissingCriterionEvidence, Criterion: item.Criterion, Detail: "duplicate criterion evidence"}
		}
		if len(item.Paths) == 0 {
			return false, SatisfactionRejection{Code: RejectMissingCriterionEvidence, Criterion: item.Criterion, Detail: "criterion has no evidence paths"}
		}
		for _, path := range item.Paths {
			if !filepath.IsAbs(path) {
				path = filepath.Join(h.tools.Root(), path)
			}
			canonical, err := filepath.EvalSymlinks(filepath.Clean(path))
			if err != nil {
				return false, SatisfactionRejection{Code: RejectUnreadEvidencePath, Criterion: item.Criterion, Path: path, Detail: "evidence path was not read (or does not exist)"}
			}
			if info, statErr := os.Stat(canonical); statErr == nil && info.IsDir() {
				return false, SatisfactionRejection{Code: RejectDirectoryEvidence, Criterion: item.Criterion, Path: path, Detail: "evidence path is a directory; cite a file you read, never a directory, a package path, or a command"}
			}
			if !p.reads[canonical] {
				return false, SatisfactionRejection{Code: RejectUnreadEvidencePath, Criterion: item.Criterion, Path: path, Detail: "evidence path was not read (or does not exist)"}
			}
		}
		covered[key] = true
	}
	for _, criterion := range req.AcceptanceCriteria {
		key := normalizeCriterion(criterion)
		if key == "" {
			return false, SatisfactionRejection{Code: RejectMissingCriterionEvidence, Detail: "a declared criterion is blank"}
		}
		if !covered[key] {
			return false, SatisfactionRejection{Code: RejectMissingCriterionEvidence, Criterion: criterion, Detail: "no evidence covers this criterion"}
		}
	}
	for _, deliverable := range req.Deliverables {
		path := strings.TrimSpace(deliverable)
		if path == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(h.tools.Root(), filepath.FromSlash(path))); err != nil {
			return false, SatisfactionRejection{Code: RejectDeliverableMissing, Deliverable: path, Detail: "required deliverable is absent"}
		}
	}
	waived := map[string]bool{}
	for _, w := range req.ValidationWaivers {
		if id := commandIdentity(w); id != "" {
			waived[id] = true
		}
	}
	for _, command := range req.ValidationCommands {
		identity := commandIdentity(command)
		if identity == "" {
			return false, SatisfactionRejection{Code: RejectValidationNotExecuted, Command: command, Detail: "validation command could not be identified"}
		}
		if waived[identity] {
			continue
		}
		if !p.ran[identity] {
			return false, SatisfactionRejection{Code: RejectValidationNotExecuted, Command: command, Detail: "required validation was not executed"}
		}
		if !p.commands[identity] {
			return false, SatisfactionRejection{Code: RejectValidationFailed, Command: command, Detail: "required validation executed but did not succeed"}
		}
	}
	after, err := h.tools.RepositoryFingerprint(ctx)
	if err != nil || after != p.baseline {
		return false, SatisfactionRejection{Code: RejectFingerprintMismatch, Detail: "repository fingerprint changed during verification"}
	}
	// Discard model-provided validation claims and mutation counts. Preserve only
	// executed commands and verified inspection links in the completion artifact.
	evidence.ValidationCommands = append([]string(nil), req.ValidationCommands...)
	final["evidence"] = evidence
	final["repository_mutations"] = 0
	final["changes_expected"] = false
	return true, SatisfactionRejection{}
}
