// Live progress and output after a prompt (ACP-004 follow-up).
//
// A session/prompt turn ends at the JSON-RPC response: the adapter starts
// governed work and returns, because the started job runs as a detached child
// below the host seam. Before this stage the only session/update frame a client
// received was the single snapshot read at submission time, so an editor
// rendered "SOP job … is running" and then nothing: the job finished, and its
// result stayed in .agent-sdlc/runs/mcp-work/<job>/output.log. Two further
// outcomes — a reconciled submission and an unchanged approval blocker — carry
// no job identity at all, so they rendered as a dead agent.
//
// This stage closes that gap without inventing state:
//
//   - A prompt result with no job identity answers with one notice frame
//     carrying the result's own outcome and message, instead of silence.
//   - A result naming a job emits the frame its persisted record supports. A
//     terminal record is answered immediately, followed by the persisted output
//     excerpt the host reads through ProgressSource.Output.
//   - A still-running record starts one follow stream: every state CHANGE is
//     one session/update frame (never a repeat of the state already sent), and
//     the terminal state is followed by that output excerpt. The stream ends at
//     the terminal state, at FollowBound, or when Serve stops, whichever comes
//     first.
//
// One stream owns one (session, job): a repeated prompt for the same running
// job re-uses the live answer instead of starting a second stream, and the
// stream releases its claim when it ends, so a later prompt about the same job
// (a reconciled resend) is answered again from that job's then-current record.
//
// Every frame is built from a fresh ProgressSource or Output read. A missing
// seam, a failed read, or an absent record emits no progress frame and records
// a diagnostic: this stage never fabricates a state, an output, or a completion.
// The output frame is evidence, carrying exactly the excerpt the host returned.
package acp

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// FollowBound bounds one follow stream. A job that is still running when the
	// bound expires stops streaming and leaves a diagnostic; the persisted record
	// stays authoritative and can be read again (sop_work_status, sop status).
	// The value is a contract decision: a stream is a convenience over persisted
	// state, so it must be bounded rather than outlive the connection's interest.
	FollowBound = time.Hour

	// MaxOutputChunkBytes bounds the persisted output excerpt one frame carries.
	// The host already tails the log; this is the second bound, so an excerpt a
	// host returns whole can never grow one frame without limit. Truncation keeps
	// the END of the excerpt, because that is the part carrying the result.
	MaxOutputChunkBytes = 16 << 10
)

// followPollInterval is how often a live stream re-reads the persisted record.
// It is a variable so tests can shorten it, the same way the CLI layer shortens
// its project-lock probe bound; the value itself is a contract decision.
var followPollInterval = 750 * time.Millisecond

// Display notice kinds this stage adds to the pinned session/update envelope
// (buildNoticeFrame in approval.go). The notice member names why the frame
// exists, so a client renders the text without interpreting SOP vocabulary.
const (
	// NoticePromptOutcome answers a prompt turn that started no streamable work:
	// the turn's own outcome and message, repeated rather than interpreted.
	NoticePromptOutcome = "prompt_outcome"
	// NoticeJobOutput carries the persisted output excerpt behind a terminal job
	// state.
	NoticeJobOutput = "job_output"
)

// jobStream is one live follow of one job in one session. It is stream-local
// state: the claim that serializes the goroutine lives on the Session.
type jobStream struct {
	sess  *Session
	jobID string
	// last is the state already sent for this job, so a stream never repeats a
	// snapshot the client already has.
	last string
}

// streamPromptProgress is the post-response stage for one successful
// session/prompt. It emits the frame the result's persisted evidence supports
// and, for a still-running job, keeps streaming until the record is terminal.
// It never fails the prompt response: every seam failure is a diagnostic and,
// where the result itself carries an answer, that answer is still emitted.
func (s *Server) streamPromptProgress(req *Request, result any) {
	if s.missingProgress() {
		return
	}
	sess := s.getSession(peekSessionID(req.Params))
	if sess == nil {
		return
	}
	res, ok := result.(map[string]any)
	if !ok {
		return
	}
	jobID := strings.TrimSpace(stringMember(res, "jobId"))
	if jobID == "" {
		// No started or reconciled work behind this turn: a busy project, a task
		// mid-run, an unchanged approval blocker, or nothing left to cancel.
		s.emitNotice(sess, NoticePromptOutcome, promptOutcomeText(res), nil, nil)
		return
	}
	snap, found, err := s.progress.Progress(sess.ProjectDir, jobID)
	if err != nil {
		s.diag.Printf("read SOP progress for job %s in project %s: %v; the answer reports the prompt outcome instead of a progress frame",
			shorten(jobID, 64), shorten(sess.ProjectID, 64), err)
		s.emitNotice(sess, NoticePromptOutcome, promptOutcomeText(res), nil, nil)
		return
	}
	if !found {
		s.emitNotice(sess, NoticePromptOutcome,
			promptOutcomeText(res)+" (no persisted job record for "+jobID+" yet)", nil, nil)
		return
	}
	if snap.State != ProgressStateRunning {
		// Already terminal (a reconciled submission): answer with its own record
		// and the output behind it, with nothing to poll.
		s.emitJobFrame(sess, snap)
		s.emitOutput(sess, jobID)
		return
	}
	if !s.beginStream(sess, jobID) {
		// A live stream already answers for this job in this session; a second
		// frame would repeat what the client already has.
		return
	}
	s.emitJobFrame(sess, snap)
	go s.follow(&jobStream{sess: sess, jobID: jobID, last: snap.State})
}

// emitJobFrame queues one progress frame for snap: intermediate while the job
// runs, terminal otherwise, so the delivery policy preserves the terminal one.
func (s *Server) emitJobFrame(sess *Session, snap ProgressSnapshot) {
	frame, outcome, err := buildProgressFrame(sess.ID, snap)
	if err != nil {
		s.diag.Printf("build session/update for job %s: %v; no frame emitted", shorten(snap.JobID, 64), err)
		return
	}
	if outcome {
		_ = s.EnqueueOutcome(frame)
		return
	}
	_ = s.EnqueueProgress(frame)
}

// emitOutput queues one output frame carrying the persisted output excerpt for
// jobID, read through the host seam. A missing seam, an empty excerpt, or a read
// failure emits nothing: the frame is evidence, never a placeholder. The frame
// rides the outcome path because it belongs to the turn's final answer, so
// coalescing never drops it while progress frames still stream.
func (s *Server) emitOutput(sess *Session, jobID string) {
	if s.progress == nil || s.progress.Output == nil {
		return
	}
	excerpt, err := s.progress.Output(sess.ProjectDir, jobID)
	if err != nil {
		s.diag.Printf("read persisted output for job %s in project %s: %v; no output frame emitted",
			shorten(jobID, 64), shorten(sess.ProjectID, 64), err)
		return
	}
	excerpt = strings.TrimSpace(excerpt)
	if excerpt == "" {
		return
	}
	frame, err := buildNoticeFrame(sess.ID, NoticeJobOutput, outputText(jobID, excerpt), nil, nil)
	if err != nil {
		s.diag.Printf("build %s session/update for job %s: %v; no frame emitted", NoticeJobOutput, shorten(jobID, 64), err)
		return
	}
	_ = s.EnqueueOutcome(frame)
}

// follow is the stream loop for one live job: it re-reads the persisted record
// on the poll interval and ends on the terminal state, on FollowBound, or when
// Serve stops, whichever comes first. It releases the stream claim however it
// ends, so a later prompt about the same job is answered again.
func (s *Server) follow(st *jobStream) {
	defer s.endStream(st.sess, st.jobID)
	ticker := time.NewTicker(followPollInterval)
	defer ticker.Stop()
	bound := time.NewTimer(FollowBound)
	defer bound.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-bound.C:
			s.diag.Printf("stream for job %s stopped at the %s bound; the persisted record stays authoritative and can be read again",
				shorten(st.jobID, 64), FollowBound)
			return
		case <-ticker.C:
		}
		if s.stepStream(st) {
			return
		}
	}
}

// stepStream performs one stream step: re-read the persisted record and emit at
// most one frame for a state the client has not already seen. It reports true
// when the stream is finished — the record reached a terminal state, the record
// is gone, or the progress seam disappeared.
func (s *Server) stepStream(st *jobStream) bool {
	if s.missingProgress() {
		return true
	}
	snap, found, err := s.progress.Progress(st.sess.ProjectDir, st.jobID)
	if err != nil {
		s.diag.Printf("read SOP progress for job %s in project %s: %v; no frame emitted and the stream continues",
			shorten(st.jobID, 64), shorten(st.sess.ProjectID, 64), err)
		return false
	}
	if !found {
		s.diag.Printf("job %s no longer has a persisted record in project %s: the stream ends without a frame, and no outcome is claimed",
			shorten(st.jobID, 64), shorten(st.sess.ProjectID, 64))
		return true
	}
	if snap.State == st.last {
		return false
	}
	st.last = snap.State
	s.emitJobFrame(st.sess, snap)
	s.PumpEvents()
	if snap.State == ProgressStateRunning {
		return false
	}
	s.emitOutput(st.sess, st.jobID)
	s.PumpEvents()
	return true
}

// beginStream claims the (session, job) stream and reports whether this caller
// owns it. The claim keeps a repeated prompt for the same running job from
// starting a second stream. The map lives with the in-memory session, so it is
// bounded by the jobs that session submitted and dies with the session.
func (s *Server) beginStream(sess *Session, jobID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess.streaming == nil {
		sess.streaming = map[string]bool{}
	}
	if sess.streaming[jobID] {
		return false
	}
	sess.streaming[jobID] = true
	return true
}

// endStream releases the claim so a later prompt about the same job is answered
// from that job's then-current record instead of re-using a finished stream.
func (s *Server) endStream(sess *Session, jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(sess.streaming, jobID)
}

// stopStreams closes the stream termination channel once, so every live stream
// returns instead of writing to a connection that ended.
func (s *Server) stopStreams() {
	s.doneOnce.Do(func() { close(s.done) })
}

// promptOutcomeText renders the answer a prompt turn gets when it started no
// streamable work: the result's own outcome and message, bounded and never
// reinterpreted. A result with neither still gets a sentence, so a turn is never
// silent.
func promptOutcomeText(res map[string]any) string {
	outcome := strings.TrimSpace(stringMember(res, "outcome"))
	message := strings.TrimSpace(stringMember(res, "message"))
	switch {
	case outcome != "" && message != "":
		return "SOP outcome " + outcome + ": " + message
	case message != "":
		return "SOP: " + message
	case outcome != "":
		return "SOP outcome " + outcome + "."
	default:
		return "SOP answered without an outcome or message; read the project's persisted state with sop status."
	}
}

// outputText renders the output frame body: the job identity, then the excerpt
// the host bounded. An excerpt longer than MaxOutputChunkBytes is cut from the
// front, because the end carries the result, and the cut is advanced to a rune
// boundary so a frame never carries half a rune.
func outputText(jobID, excerpt string) string {
	if len(excerpt) > MaxOutputChunkBytes {
		excerpt = excerpt[len(excerpt)-MaxOutputChunkBytes:]
		for len(excerpt) > 0 && !utf8.RuneStart(excerpt[0]) {
			excerpt = excerpt[1:]
		}
		excerpt = "…" + excerpt
	}
	return fmt.Sprintf("SOP job %s output:\n\n%s", jobID, excerpt)
}

// stringMember returns the string member k of res, or "" when it is absent or
// not a string, so a malformed result reads as missing evidence rather than
// panic.
func stringMember(res map[string]any, k string) string {
	v, _ := res[k].(string)
	return v
}
