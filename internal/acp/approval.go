// Planner questions and approval blockers over ACP (ACP-005).
//
// # Contract note and pinned presentation rules
//
// ACP displays questions and approval blockers and refuses non-authorizing
// responses. Resolution stays in the existing interfaces; ACP adds no new
// resolution method and never records an approval or an answer.
//
// Question contract symbols (owner: internal/question)
//
//   - question.Question carries ID (stable record id), Identity and Kind
//     (the duplicate key question.Question.Key), Body, State, Revision, and
//     Options (internal/question/question.go).
//   - State is the closed vocabulary question.State: PENDING, ANSWERED,
//     APPLIED, REJECTED, SUPERSEDED. Only PENDING is displayed here as
//     awaiting an answer.
//   - The existing supported answer procedure: record an answer through
//     internal/question's answer submission (question.go, service.go,
//     store.go, reconcile.go), an answer carrying Answer.ExpectedRevision
//     set to the displayed revision plus a fresh Answer.IdempotencyKey.
//     Recorded submission outcomes are the closed set question.OutcomeKind:
//     ACCEPTED, DUPLICATE, STALE, REJECTED, APPLIED. ACP names this
//     procedure in its display and adds no new answer method: the pinned
//     method surface (contract.go SupportedMethods) stays initialize,
//     session/new, session/prompt.
//
// Approval contract (owner: internal/approval)
//
// Policy approval stays in the existing SOP approval interface (package
// internal/approval, approval.go), and ACP consumes only the identity the
// host read. The blocker identity this work uses is seam-supplied
// (ApprovalBlocker.ID); the pinned host-side construction is the approval
// record id when the host read one, otherwise BlockerIdentity(jobID,
// blockerText). An unchanged blocker is detectable because the identity is
// derived only from authoritative state obtained through fresh seam reads
// and is compared against the identity this session last reported.
//
// # Stale and unrelated responses
//
// The ACP side is a read-only refusal layer. A response that does not match
// the displayed question or approval identity, or an answer whose claimed
// revision does not match the displayed question revision, is reported as
// non-authorizing and leaves the question and the approval pending. The
// conditions mirror internal/question's staleness contract (ErrStale,
// OutcomeStale, Answer.ExpectedRevision, reconcile.go); the authoritative
// checks stay in internal/question and internal/approval, which alone accept
// or refuse a submission.
//
// Non-authorizing channels (pinned)
//
// Free-form chat text, an ACP permission grant, and a planner recommendation
// never change approval or question state. Recorded answer on permission
// grants: this build's pinned request surface (contract.go SupportedMethods)
// has no permission-grant method, and inbound notifications are consumed
// without dispatch (transport.go), so no code path can turn a permission
// grant into approval. Prompt text reaches approval state only here, and
// only through the token grammar parseResponseClaims pins below; every
// other word is inert. A question response is only ever an identity match,
// and even a matching response is directed to the existing procedure, not
// recorded over ACP.
//
// # Unchanged approval blocker
//
// When a prompt meets a blocker whose identity equals the identity the
// session already reported, the adapter stops and reports the unchanged
// blocker: it does not call the governed start machinery again, so no
// attempt or retry budget is consumed, and it does not touch the submission
// replay registry. A different blocker identity is reported normally.
//
// # Continuation after external resolution
//
// After external resolution through the SOP approval interface, continuation
// runs only when a fresh authoritative read through this seam shows the
// blocker resolved; an in-memory value that looks resolved without a fresh
// read never starts continuation. The continuation entry point is the
// existing contract in internal/question/continuation.go, backed by package
// internal/continuation; application is not execution and is not approval.
//
// Seam rules mirror ProgressSource (progress.go): a missing seam emits no
// question or blocker display and runs no gating, never fabricating state;
// a read error is recorded as a diagnostic and emits no display.
package acp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Session/prompt outcomes this presentation layer reports. These are result
// values, never protocol status values; the turn still ends with a pinned
// stop reason.
const (
	// WorkOutcomeNotAuthorizing: a prompt carried a response claim that does
	// not match the displayed question or approval identity (unrelated), or
	// whose revision is stale. It cannot authorize work.
	WorkOutcomeNotAuthorizing = "not_authorizing"
	// WorkOutcomeBlockedUnchanged: the prompt met the same approval blocker
	// identity the session already reported; the adapter stopped without
	// retrying and without consuming an attempt or retry budget.
	WorkOutcomeBlockedUnchanged = "blocked_unchanged"
	// WorkOutcomeResolveExternally: the prompt carried a response that
	// matches a displayed question or approval identity, but resolution is
	// not accepted over ACP: record it through the existing supported
	// procedure (internal/question answers, internal/approval approvals).
	WorkOutcomeResolveExternally = "resolve_externally"
)

// Notice kinds carried in the sop.notice member of a display frame.
const (
	NoticePlannerQuestion  = "planner_question"
	NoticeApprovalBlocker  = "approval_blocker"
	NoticeApprovalResolved = "approval_resolved"
)

// QuestionStatePending pins the pending spelling of question.StatePending
// (internal/question/question.go).
const QuestionStatePending = "PENDING"

// approvalStatePending is the pending spelling ACP displays for an approval
// blocker; authority over the real state stays with the host's read.
const approvalStatePending = "pending"

// PlannerQuestion mirrors one authoritative question record as the host's
// read seam supplies it. The fields map to internal/question.Question.
type PlannerQuestion struct {
	ID       string
	Identity string
	Kind     string
	Body     string
	// State is the closed question.State vocabulary ("PENDING", ...).
	State    string
	Revision int
	Options  []string
}

// ApprovalBlocker mirrors one authoritative approval gate the host read.
// ID is the blocker identity: the approval record id when the host read one,
// otherwise BlockerIdentity(jobID, blockerText).
type ApprovalBlocker struct {
	ID             string
	Kind           string
	Detail         string
	OperatorAction string
	State          string
}

// QuestionApprovalState is one fresh reading of the project's pending
// planner questions and approval blockers.
type QuestionApprovalState struct {
	Questions []PlannerQuestion
	Blockers  []ApprovalBlocker
}

// QuestionApprovalSource is the host-supplied read seam for planner
// questions and approval blockers, mirroring ProgressSource (progress.go).
// A missing function or a missing source means the server emits no question
// or blocker display, runs no approval gating, and never fabricates state.
type QuestionApprovalSource struct {
	// Read performs a fresh authoritative read for projectDir. It must be a
	// read: the bridge consumes internal/question and internal/approval and
	// never writes them.
	Read func(projectDir string) (QuestionApprovalState, error)
}

// WithQuestionApproval wires the host's question and approval read seam.
// Without it the server emits no question or blocker display and no approval
// gating.
func WithQuestionApproval(src *QuestionApprovalSource) ServerOption {
	return func(s *Server) { s.qa = src }
}

// missingQuestionApproval reports whether the seam is unconfigured.
func (s *Server) missingQuestionApproval() bool {
	return s.qa == nil || s.qa.Read == nil
}

// BlockerIdentity builds the fallback blocker identity: the job identity and
// the blocker text. The host uses the approval record id when it actually
// read one; this helper is the deterministic fallback construction pinned in
// the file-level contract note.
func BlockerIdentity(jobID, blockerText string) string {
	return "job:" + strings.TrimSpace(jobID) + "|" + strings.TrimSpace(blockerText)
}

// Per-session display bookkeeping under the server mutex.

// reportedBlockerIdentity returns the joined blocker identity the session
// already reported, or "" when none has been reported.
func (s *Server) reportedBlockerIdentity(sessionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reportedBlockers[sessionID]
}

// setReportedBlockerIdentity records the joined blocker identity this
// session reported ("" clears the record: the fresh read resolved it).
func (s *Server) setReportedBlockerIdentity(sessionID, identity string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reportedBlockers == nil {
		s.reportedBlockers = map[string]string{}
	}
	s.reportedBlockers[sessionID] = identity
}

// reportedQuestionIdentity returns the pending-question set identity the
// session already displayed.
func (s *Server) reportedQuestionIdentity(sessionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reportedQuestions[sessionID]
}

// setReportedQuestionIdentity records the displayed pending-question set.
func (s *Server) setReportedQuestionIdentity(sessionID, identity string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reportedQuestions == nil {
		s.reportedQuestions = map[string]string{}
	}
	s.reportedQuestions[sessionID] = identity
}

// approvalGate consults the seam BEFORE any submission machinery runs. It
// returns a result map for a handled turn (refused response, externally
// directed response, or unchanged blocker) and nil when the governed flow
// should proceed. A missing seam and a read error both return nil: no
// display, gating, or failure is invented from missing evidence.
func (s *Server) approvalGate(sess *Session, text string) (map[string]any, *Error) {
	if s.missingQuestionApproval() {
		return nil, nil
	}
	state, err := s.qa.Read(sess.ProjectDir)
	if err != nil {
		s.diag.Printf("read planner question and approval state for project %s: %v; no question or blocker display emitted, no response classified", shorten(sess.ProjectID, 64), err)
		return nil, nil
	}
	if res := s.classifyResponseClaims(state, text); res != nil {
		return res, nil
	}
	return s.gateApprovalBlockers(sess, state), nil
}

// responseClaim is one recognized response identity inside prompt text. The
// grammar below is the ONLY path from free-form text to a claim; nothing
// else in a prompt can approve, answer, retry, or authorize anything.
type responseClaim struct {
	Kind     string // "approval" or "answer"
	ID       string
	Revision string // digits, or "" when none was claimed
}

// parseResponseClaims extracts the deterministic response-claim tokens from
// prompt text: approval:<id> and answer:<id> or answer:<id>@<revision>.
// Claims are matched case-insensitively on the prefix, verbatim on the id.
func parseResponseClaims(text string) []responseClaim {
	var claims []responseClaim
	for _, field := range strings.Fields(text) {
		trimmed := strings.Trim(field, "()[]{}<>,.;:!?'\"")
		lower := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(lower, "approval:"):
			id := strings.TrimSpace(trimmed[len("approval:"):])
			if id != "" {
				claims = append(claims, responseClaim{Kind: "approval", ID: id})
			}
		case strings.HasPrefix(lower, "answer:"):
			rest := strings.TrimSpace(trimmed[len("answer:"):])
			id, rev := rest, ""
			if at := strings.LastIndex(rest, "@"); at >= 0 {
				id, rev = strings.TrimSpace(rest[:at]), strings.TrimSpace(rest[at+1:])
			}
			if id != "" {
				claims = append(claims, responseClaim{Kind: "answer", ID: id, Revision: rev})
			}
		}
	}
	return claims
}

// classifyResponseClaims matches claims against the fresh authoritative
// read. The first claim in text order determines the outcome, so the same
// text always produces the same result. A stale or unrelated claim is
// refused as non-authorizing and leaves the question and the approval
// pending; a matching claim is directed to the existing procedure and is
// never recorded over ACP.
func (s *Server) classifyResponseClaims(state QuestionApprovalState, text string) map[string]any {
	for _, claim := range parseResponseClaims(text) {
		switch claim.Kind {
		case "answer":
			q := findQuestion(state.Questions, claim.ID)
			switch {
			case q == nil:
				return nonAuthorizingResult(claim, fmt.Sprintf("no pending planner question with identifier %q is displayed in this session: a response that does not match the displayed question identity is unrelated and cannot authorize work", claim.ID))
			case q.State != QuestionStatePending:
				return nonAuthorizingResult(claim, fmt.Sprintf("planner question %q is %s, not pending: an answer that does not match the displayed pending question cannot authorize work", claim.ID, q.State))
			case claim.Revision != "" && claim.Revision != fmt.Sprintf("%d", q.Revision):
				return nonAuthorizingResult(claim, fmt.Sprintf("stale answer: the claimed revision %s does not match the displayed revision %d of question %q, so the response is stale and cannot authorize work", claim.Revision, q.Revision, claim.ID))
			default:
				return resolveExternallyResult(claim.Kind, claim.ID, fmt.Sprintf("ACP does not record answers: record the answer for question %s through the existing supported question answer procedure (internal/question), an answer with expected revision %d and an idempotency key; recorded outcomes are the closed set ACCEPTED, DUPLICATE, STALE, REJECTED, APPLIED", claim.ID, q.Revision))
			}
		case "approval":
			if findBlocker(state.Blockers, claim.ID) == nil {
				return nonAuthorizingResult(claim, fmt.Sprintf("no approval blocker with identifier %q is displayed in this session: an approval response that does not match the displayed blocker identity is unrelated and cannot authorize work", claim.ID))
			}
			return resolveExternallyResult(claim.Kind, claim.ID, "ACP never records approval: resolve this approval through the existing SOP approval interface (internal/approval), then resend this prompt")
		}
	}
	return nil
}

// gateApprovalBlockers implements the unchanged-blocker stop and the display
// bookkeeping. It returns a result map only for the unchanged-blocker stop;
// every other path returns nil so the governed flow proceeds normally.
func (s *Server) gateApprovalBlockers(sess *Session, state QuestionApprovalState) map[string]any {
	identity := blockerIdentitySet(state.Blockers)
	last := s.reportedBlockerIdentity(sess.ID)
	switch {
	case identity != "" && identity == last:
		return map[string]any{
			"outcome":       WorkOutcomeBlockedUnchanged,
			"condition":     "unchanged_approval_blocker",
			"blockerIds":    nonEmptyList(blockerIDList(state.Blockers)),
			"approvalState": approvalStatePending,
			"message":       "an approval blocker with this identity was already reported and has not changed: this prompt stopped instead of retrying it, no attempt or retry budget was consumed, and approval remains pending until resolved through the existing SOP approval interface",
		}
	case identity != "":
		// First report, or a changed blocker identity: report normally.
		s.setReportedBlockerIdentity(sess.ID, identity)
		s.setReportedQuestionIdentity(sess.ID, questionIdentitySet(state.Questions))
		lines := append(blockerLines(state.Blockers), questionLines(state.Questions)...)
		s.emitNotice(sess, NoticeApprovalBlocker, strings.Join(lines, "\n"), blockerIDList(state.Blockers), pendingQuestionIDs(state.Questions))
		return nil
	default:
		if last != "" {
			// The fresh read shows the previously reported blocker resolved;
			// continuation follows the existing contract only after this
			// fresh read.
			text := fmt.Sprintf("The previously reported approval blocker identity %q no longer matches the fresh authoritative read: the blocker is resolved, continuation follows the existing continuation contract (internal/question/continuation.go, backed by internal/continuation) only after this fresh read, and policy approval was resolved through the SOP approval interface, not through ACP.", last)
			s.emitNotice(sess, NoticeApprovalResolved, text, nil, pendingQuestionIDs(state.Questions))
			s.setReportedBlockerIdentity(sess.ID, "")
		}
		if next := questionIdentitySet(state.Questions); next != s.reportedQuestionIdentity(sess.ID) {
			s.setReportedQuestionIdentity(sess.ID, next)
			if ql := questionLines(state.Questions); len(ql) > 0 {
				s.emitNotice(sess, NoticePlannerQuestion, strings.Join(ql, "\n"), nil, pendingQuestionIDs(state.Questions))
			}
		}
		return nil
	}
}

// Display frame construction. The frames reuse the pinned session/update
// envelope and replace the structured progress member with a notice member
// that carries only what the seam actually read, so a display frame never
// invents a job, task, or evidence identity.

// noticeSOPFields is the structured notice payload.
type noticeSOPFields struct {
	Notice    string   `json:"notice"`
	Blockers  []string `json:"blockers,omitempty"`
	Questions []string `json:"questions,omitempty"`
}

// noticeNotification is the pinned display params shape.
type noticeNotification struct {
	SessionID string             `json:"sessionId"`
	Update    progressUpdateBody `json:"update"`
	SOP       noticeSOPFields    `json:"sop"`
}

// noticeFrame is the pinned display envelope. A struct fixes the member
// order, so the same notice always marshals to identical bytes.
type noticeFrame struct {
	JSONRPC string             `json:"jsonrpc"`
	Method  string             `json:"method"`
	Params  noticeNotification `json:"params"`
}

// buildNoticeFrame builds one deterministic display frame.
func buildNoticeFrame(sessionID, notice, text string, blockerIDs, questionIDs []string) ([]byte, error) {
	return json.Marshal(noticeFrame{
		JSONRPC: "2.0",
		Method:  MethodSessionUpdate,
		Params: noticeNotification{
			SessionID: sessionID,
			Update: progressUpdateBody{
				SessionUpdate: UpdateAgentMessageChunk,
				Content:       progressTextBody{Type: "text", Text: text},
			},
			SOP: noticeSOPFields{
				Notice:    notice,
				Blockers:  nonEmptyList(blockerIDs),
				Questions: nonEmptyList(questionIDs),
			},
		},
	})
}

// emitNotice queues one display frame. Resolution notices are terminal for
// the reported blocker and ride the outcome path, so they survive coalescing.
func (s *Server) emitNotice(sess *Session, notice, text string, blockerIDs, questionIDs []string) {
	frame, err := buildNoticeFrame(sess.ID, notice, text, blockerIDs, questionIDs)
	if err != nil {
		s.diag.Printf("build %s session/update for session %s: %v; no frame emitted", notice, shorten(sess.ID, 64), err)
		return
	}
	if notice == NoticeApprovalResolved {
		_ = s.EnqueueOutcome(frame)
		return
	}
	_ = s.EnqueueProgress(frame)
}

// Deterministic rendering helpers.

// sortedQuestions returns the questions ordered by trimmed id, nil ids
// excluded.
func sortedQuestions(qs []PlannerQuestion) []PlannerQuestion {
	out := make([]PlannerQuestion, 0, len(qs))
	for _, q := range qs {
		if strings.TrimSpace(q.ID) != "" {
			out = append(out, q)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// sortedBlockers returns the blockers ordered by trimmed id, nil ids
// excluded.
func sortedBlockers(bs []ApprovalBlocker) []ApprovalBlocker {
	out := make([]ApprovalBlocker, 0, len(bs))
	for _, b := range bs {
		if strings.TrimSpace(b.ID) != "" {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// blockerIDList returns the trimmed blocker ids in display order.
func blockerIDList(bs []ApprovalBlocker) []string {
	ids := make([]string, 0, len(bs))
	for _, b := range sortedBlockers(bs) {
		ids = append(ids, strings.TrimSpace(b.ID))
	}
	return ids
}

// blockerIdentitySet is the change key for the blocker report: the sorted,
// joined blocker identities derived only from the fresh authoritative read.
func blockerIdentitySet(bs []ApprovalBlocker) string {
	ids := blockerIDList(bs)
	if len(ids) == 0 {
		return ""
	}
	sort.Strings(ids)
	return strings.Join(ids, "\x00")
}

// pendingQuestionIDs returns the ids of the pending questions in display
// order.
func pendingQuestionIDs(qs []PlannerQuestion) []string {
	ids := make([]string, 0, len(qs))
	for _, q := range sortedQuestions(qs) {
		if q.State == QuestionStatePending {
			ids = append(ids, strings.TrimSpace(q.ID))
		}
	}
	return ids
}

// questionIdentitySet is the change key for the question display: the
// sorted pending ids with their revisions, so a changed revision re-displays.
func questionIdentitySet(qs []PlannerQuestion) string {
	ids := make([]string, 0, len(qs))
	for _, q := range sortedQuestions(qs) {
		if q.State != QuestionStatePending {
			continue
		}
		ids = append(ids, fmt.Sprintf("%s@%d", strings.TrimSpace(q.ID), q.Revision))
	}
	if len(ids) == 0 {
		return ""
	}
	sort.Strings(ids)
	return strings.Join(ids, "\x00")
}

// blockerLines renders the deterministic approval-blocker display. Each
// blocker names its identity, its state, and the precise operator action,
// and directs approval to the existing SOP approval interface. A repeated
// rendering of the same input produces identical bytes.
func blockerLines(bs []ApprovalBlocker) []string {
	var lines []string
	for _, b := range sortedBlockers(bs) {
		head := fmt.Sprintf("Approval blocker %s (%s, state %s)", strings.TrimSpace(b.ID), orDash(b.Kind), orDash(b.State))
		if d := strings.TrimSpace(b.Detail); d != "" {
			head += ": " + d
		}
		lines = append(lines, head)
		action := strings.TrimSpace(b.OperatorAction)
		if action == "" {
			action = "resolve this approval in the existing SOP approval interface"
		}
		lines = append(lines, "  To resolve: "+action+". Policy approval stays in the existing SOP approval interface (internal/approval); ACP chat text, an ACP permission grant, and a planner recommendation never approve anything. Resend this prompt only after resolving there; an unchanged blocker stops this session without a retry.")
	}
	return lines
}

// questionLines renders the deterministic planner-question display for the
// pending questions. Each question carries its identifier, its state, and
// the existing supported response procedure; ACP offers no answer path of
// its own. A question whose state is not pending is not shown as awaiting
// an answer.
func questionLines(qs []PlannerQuestion) []string {
	var lines []string
	for _, q := range sortedQuestions(qs) {
		if q.State != QuestionStatePending {
			continue
		}
		id := strings.TrimSpace(q.ID)
		lines = append(lines, fmt.Sprintf("Planner question %s (identity %s, kind %s, revision %d) is PENDING: %s", id, orDash(q.Identity), orDash(q.Kind), q.Revision, strings.TrimSpace(q.Body)))
		if opts := nonEmptyList(q.Options); opts != nil {
			lines = append(lines, "  Options: "+strings.Join(opts, " | "))
		}
		lines = append(lines, fmt.Sprintf("  Answer through the existing supported question answer procedure (internal/question): record an answer for question %s with expected revision %d and an idempotency key; recorded outcomes are the closed set ACCEPTED, DUPLICATE, STALE, REJECTED, APPLIED (question.OutcomeKind). ACP chat text, an ACP permission grant, and a planner recommendation do not answer or approve anything, and ACP adds no answer method.", id, q.Revision))
	}
	return lines
}

// orDash renders an empty optional value as "-" instead of inventing one.
func orDash(s string) string {
	if t := strings.TrimSpace(s); t != "" {
		return t
	}
	return "-"
}

// Refusal and external-resolution result builders.

// nonAuthorizingResult reports a stale or unrelated response. Approval stays
// pending and nothing authorizes work.
func nonAuthorizingResult(claim responseClaim, reason string) map[string]any {
	return map[string]any{
		"outcome":       WorkOutcomeNotAuthorizing,
		"condition":     "unrelated_or_stale_response",
		"claimKind":     claim.Kind,
		"identity":      claim.ID,
		"approvalState": approvalStatePending,
		"message":       reason + ". Approval and the displayed question remain pending: record the response through the existing supported procedure, never through ACP chat text, an ACP permission grant, or a planner recommendation.",
	}
}

// resolveExternallyResult directs a matching response to the existing
// supported procedure. Approval stays pending and nothing is recorded here.
func resolveExternallyResult(claimKind, id, procedureMessage string) map[string]any {
	return map[string]any{
		"outcome":       WorkOutcomeResolveExternally,
		"condition":     "external_resolution_required",
		"claimKind":     claimKind,
		"identity":      id,
		"approvalState": approvalStatePending,
		"message":       procedureMessage + ". Free-form chat text, an ACP permission grant, and a planner recommendation are not approval, and this response is not recorded over ACP.",
	}
}

// Identity lookups against one fresh reading.

// findQuestion returns the displayed question with the given id, or nil.
func findQuestion(qs []PlannerQuestion, id string) *PlannerQuestion {
	for i := range qs {
		if strings.TrimSpace(qs[i].ID) == strings.TrimSpace(id) {
			return &qs[i]
		}
	}
	return nil
}

// findBlocker returns the displayed blocker with the given id, or nil.
func findBlocker(bs []ApprovalBlocker, id string) *ApprovalBlocker {
	for i := range bs {
		if strings.TrimSpace(bs[i].ID) == strings.TrimSpace(id) {
			return &bs[i]
		}
	}
	return nil
}
