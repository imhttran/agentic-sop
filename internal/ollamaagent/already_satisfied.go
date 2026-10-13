package ollamaagent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

const alreadySatisfiedInstruction = `If the requested implementation already exists, use the supplied already-satisfied verification contract: inspect its concrete evidence, run every required validation, and explicitly return ALREADY_SATISFIED. Do not manufacture changes. Without verified proof, implement the requested change. The existing discovery, stale, and iteration limits remain in force.`

// satisfactionProof is invocation-local verification, separate from mutation
// and discovery progress. Validation never resets the stale streak.
type satisfactionProof struct {
	baseline string
	reads    map[string]bool
	commands map[string]bool
	// ran records that a run_command call was attempted (so a not-executed required
	// validation is distinguishable from one that executed but failed).
	ran map[string]bool
	// evidenceCorrectionUsed records that the one permitted evidence-only correction has
	// already been requested for this invocation, so a claim is corrected at most once.
	evidenceCorrectionUsed bool
}

func (h *Harness) satisfactionProof(ctx context.Context, req agent.Request) *satisfactionProof {
	p := &satisfactionProof{reads: map[string]bool{}, commands: map[string]bool{}, ran: map[string]bool{}}
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

// missingCriterionEvidence returns the declared criteria that the model's submitted
// evidence does not cover (matched by normalized text), together with whether every
// OTHER verifier condition already holds (completed status, no mutation, a baseline,
// a parsed evidence block whose entries all map to files the agent actually read, all
// required validation commands executed, and an unchanged repository fingerprint). It
// is used ONLY to decide whether the single permitted evidence-only correction is
// appropriate; verifySatisfied remains the sole authority on acceptance.
func (h *Harness) missingCriterionEvidence(ctx context.Context, req agent.Request, st *executionState, p *satisfactionProof, final map[string]any) ([]string, bool) {
	if st.mutationObserved || p.baseline == "" || final["status"] != string(agent.OutcomeCompleted) {
		return nil, false
	}
	data, _ := json.Marshal(final["evidence"])
	var evidence agent.CompletionEvidence
	if json.Unmarshal(data, &evidence) != nil {
		return nil, false
	}
	// A criterion counts as covered only when it has an evidence entry whose EVERY
	// cited path was actually read — exactly the condition verifySatisfiedDetailed
	// enforces. An entry that cites any unread (or fabricated) path leaves its criterion
	// uncovered, so the one-shot correction is offered; the claim is still rejected
	// while it keeps an unread path, because the verifier never weakens.
	covered := map[string]bool{}
	for _, item := range evidence.Acceptance {
		key := normalizeCriterion(item.Criterion)
		if key == "" || covered[key] || len(item.Paths) == 0 {
			continue
		}
		allRead := true
		for _, path := range item.Paths {
			if !filepath.IsAbs(path) {
				path = filepath.Join(h.tools.Root(), path)
			}
			canonical, err := filepath.EvalSymlinks(filepath.Clean(path))
			if err != nil || !p.reads[canonical] {
				allRead = false
				break
			}
		}
		if allRead {
			covered[key] = true
		}
	}
	var missing []string
	for _, criterion := range req.AcceptanceCriteria {
		if key := normalizeCriterion(criterion); key != "" && !covered[key] {
			missing = append(missing, criterion)
		}
	}
	if len(missing) == 0 {
		return nil, false
	}
	for _, command := range req.ValidationCommands {
		identity := commandIdentity(command)
		if identity == "" || !p.commands[identity] {
			return nil, false
		}
	}
	after, err := h.tools.RepositoryFingerprint(ctx)
	if err != nil || after != p.baseline {
		return nil, false
	}
	return missing, true
}

// alreadySatisfiedEvidenceCorrection is the single, bounded, evidence-only request the
// harness sends when an ALREADY_SATISFIED claim is otherwise sound but does not map
// every declared criterion. It forbids any repository change and demands that each
// criterion name a file the agent actually read; it never substitutes for evidence.
func alreadySatisfiedEvidenceCorrection(missing []string) string {
	var b strings.Builder
	b.WriteString("Your ALREADY_SATISFIED claim was rejected: these declared acceptance criteria have no inspected-file evidence:\n")
	for _, criterion := range missing {
		fmt.Fprintf(&b, "- %s\n", criterion)
	}
	b.WriteString("Do NOT change any file. Read the relevant repository files if you have not already, then return the same object with EVERY criterion mapped to files you actually read:\n")
	b.WriteString(`{"status":"completed","completion":"ALREADY_SATISFIED","changes_expected":false,"evidence":{"acceptance":[{"criterion":"<exact criterion>","paths":["<file you read>"]}]}}`)
	b.WriteString("\nA path you did not read, a fabricated claim, or any repository change is rejected. This is the only correction allowed for this claim.")
	return b.String()
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
	}
}

// verify accepts only an explicit claim with complete caller-owned criteria,
// inspected files and every caller-owned validation command actually passing.
// Unknown or changed repository state fails closed. The model selects evidence
// links; it cannot manufacture tool results or substitute easier validations.
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
	RejectDeliverableMissing       RejectionCode = "DELIVERABLE_MISSING"
	RejectValidationNotExecuted    RejectionCode = "VALIDATION_NOT_EXECUTED"
	RejectValidationFailed         RejectionCode = "VALIDATION_FAILED"
	RejectFingerprintMismatch      RejectionCode = "FINGERPRINT_MISMATCH"
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
			if err != nil || !p.reads[canonical] {
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
