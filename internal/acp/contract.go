// Package acp implements the Agent Client Protocol (ACP) transport that lets a
// protocol client (for example an editor) drive SOP over stdio.
//
// Framing (pinned contract)
//
// Frames are newline-delimited JSON-RPC 2.0: one JSON document per
// '\n'-terminated line, for requests and responses alike. A '\r' directly
// before the newline is tolerated. There is no Content-Length header. A frame
// whose payload exceeds MaxFrameSize is rejected once with an actionable error
// and the reader resynchronizes on the next newline, so malformed input cannot
// desynchronize the stream. A trailing fragment that ends at EOF without a
// newline is treated as the final frame; a final empty read is a clean EOF and
// shuts the server down with no error.
//
// stdout is protocol-only: newProtocolWriter is the single serialization point
// allowed to write frames there, so concurrent responses can interleave whole
// frames but never bytes within a frame. Every diagnostic goes to stderr
// through the diagnostics writer, which prefixes "acp: ", redacts
// credential-shaped values, and never touches the protocol stream.
//
// Methods implemented in this build
//
//	initialize    The client declares its protocol version. An unset or null
//	              version defers to the server; the string "v1" and the number
//	              1 are the accepted spellings of the pinned version. Anything
//	              else is rejected with CodeVersionUnsupported and a message
//	              naming the supported version. On success the server replies
//	              with {protocolVersion, agentCapabilities, agentInfo{name,
//	              version}} and marks the session initialized; agentCapabilities
//	              is derived solely from the contract constants below. The
//	              response's protocolVersion is the numeric revision
//	              (ProtocolVersionNumber), the wire form ACP clients validate.
//	session/new   Params {cwd, mcpServers:[{name, command, args, env}]} in,
//	              result {sessionId} out. Params are decoded strictly: an
//	              unknown field or malformed argument is rejected with
//	              CodeInvalidParams. The session record lives in memory only;
//	              nothing is persisted and no workflow execution starts here.
//
// session/prompt submits one governed text prompt for the session's bound
// project through the injected submission bridge (prompt.go). The result
// outcome is started, already_running, reconciled (a persisted attempt for
// the same submission identity is returned instead of duplicate work), or
// busy (a contended project writer lock: nothing started, nothing queued).
// The bridge is supplied by the host process; a server without one rejects
// session/prompt with CodeCapabilityUnsupported.
//
// session/cancel (ACP-006) cancels the session's foreground governed work
// through the host-supplied cancel seam (PromptSubmission.Cancel in
// prompt.go, handler in cancel.go). Params are exactly {sessionId}; the shape is
// decoded strictly, so an unknown field (for example jobId or submissionKey) is
// rejected as invalid params rather than reinterpreted. The response carries
// outcome cancelled, the job id, stopReason cancelled, and the documented stop
// bound
// (CancelStopBound). A server whose bridge lacks the seam rejects
// session/cancel with CodeCapabilityUnsupported instead of claiming
// cancellation support. Cancellation is recorded distinctly from failure and
// from completion: exactly one authoritative result per job identity wins the
// race with normal completion, and the event buffer receives at most one
// terminal session/update frame for one job. On a clean EOF (client
// disconnect) the server requests safe cancellation of the session's
// foreground work within the stop bound, then reconciles the final state by
// re-reading the persisted records; it never restarts work automatically and
// never fabricates a completion (cancel.go).
//
// Outbound progress (ACP-004)
//
// A server built with a progress source (WithProgress, progress.go) streams
// structured SOP progress on the session/update notification: task identity,
// current stage, verification outcome, blockers, and final evidence
// references, plus the SOP workflow state. session/update is outbound-only:
// the server never dispatches it, and an inbound id-less frame still receives
// no response. Only the protocol-defined stop reasons below are ever emitted;
// SOP states (LOCAL_DONE, verified completion, waiting for approval, blocked)
// travel as session/update payload fields, never as protocol status values. A
// finished prompt turn is marked by the session/prompt response's stopReason
// member, which stays distinct from the SOP states riding in session/update.
// The buffered delivery coalesces intermediate progress within
// EventBufferBound and preserves terminal outcomes (delivery.go).
//
// The turn's answer does not stop at the first snapshot (follow.go). A started
// job keeps streaming after the response: each persisted state change is one
// session/update frame, and the terminal state is followed by one output frame
// carrying the job's persisted output excerpt, which the host supplies through
// ProgressSource.Output. A prompt turn that started no streamable work answers
// with one notice frame repeating its own outcome instead of silence, and a job
// that is already terminal when the prompt returns is answered immediately with
// no stream at all. Every stream is bounded by FollowBound, stops at the
// terminal state, and ends with the connection; the output frame is evidence,
// carrying exactly the excerpt the host returned.
//
// session/load is a valid ACP method whose loadSession capability this build
// does not advertise (AgentCapabilities pins it to false). It is rejected with
// CodeCapabilityUnsupported and a message naming the supported surface instead
// of a generic not-found. No inbound ACP notification method is implemented in
// this build, so a frame without an id is consumed with no response and no
// dispatch. Method ordering is enforced: a request other than initialize
// before initialize succeeds is rejected with CodeNotInitialized.
//
// # Pinned version
//
// Planning inspected ACP documentation only: v1 is the released protocol and
// v2 is a draft. No ACP Go library exists in the module graph, so the version
// and the message shapes above are pinned here in code rather than imported
// from an unverified dependency. Two exported constants spell the pinned
// revision: ProtocolVersionNumber is the numeric revision used in the
// initialize response, and ProtocolVersion is the string alias accepted on
// input and used in error messages.
//
// Bounds
//
//	MaxFrameSize     Maximum input frame size in bytes; see Serve.
//	EventBufferBound Maximum outbound event frames buffered before further
//	                 pushes are rejected with ErrEventBufferFull and an
//	                 actionable diagnostic, so a stalled client cannot grow
//	                 server memory without limit. The outbound stage
//	                 (progress.go, delivery.go) translates structured progress
//	                 into session/update frames, coalesces intermediate
//	                 progress within this bound, and preserves terminal
//	                 outcomes through the drain to the client.
package acp

// ServerName identifies SOP in the initialize handshake.
const ServerName = "sop"

// ProtocolVersionNumber is the pinned ACP protocol revision as a number. The
// initialize response sends this numeric wire form, which ACP clients (for
// example Zed 1.26.2) validate against a numeric protocol revision. The string
// alias ProtocolVersion ("v1") remains accepted on input, so a client may
// declare either spelling and the server accepts both.
const ProtocolVersionNumber = 1

// ProtocolVersion is the string alias of the pinned ACP protocol revision this
// build speaks. It is the accepted string spelling on input and the value named
// in version-unsupported error messages. The initialize response carries the
// numeric ProtocolVersionNumber instead; derive both from this pinned revision,
// never from a second copy of the value.
const ProtocolVersion = "v1"

// Method names on the pinned ACP surface.
const (
	// MethodInitialize performs version negotiation and capability
	// advertisement; it must complete before any other method.
	MethodInitialize = "initialize"
	// MethodSessionNew creates one in-memory session record.
	MethodSessionNew = "session/new"
	// MethodSessionPrompt submits one governed text prompt for a bound
	// session through injected submission bridge seams (see prompt.go).
	MethodSessionPrompt = "session/prompt"
	// MethodSessionCancel cancels one session's foreground governed work
	// through the host-supplied cancel seam (see cancel.go and the Cancel
	// seam in prompt.go). The response's stopReason is StopReasonCancelled
	// and the work's recorded SOP outcome is cancelled, kept distinct from
	// failed and completed.
	MethodSessionCancel = "session/cancel"
	// MethodSessionLoad resumes an existing session through session/load. It
	// is named so an unsupported request can be rejected with the exact
	// capability reason instead of a generic method-not-found.
	MethodSessionLoad = "session/load"
	// MethodSessionUpdate is the outbound notification that streams session
	// progress and evidence to the client (see progress.go). This build sends
	// it but never dispatches it: a client-sent session/update frame has no
	// id, receives no response, and carries no SOP meaning inbound.
	MethodSessionUpdate = "session/update"
)

// SupportedMethods lists the request method names implemented in this build.
// Initialize advertises nothing beyond this surface; treat the slice as
// read-only. MethodSessionUpdate is outbound-only and intentionally absent.
var SupportedMethods = []string{MethodInitialize, MethodSessionNew, MethodSessionPrompt, MethodSessionCancel}

// CapabilityLoadSession is the ACP capability for session resumption.
const CapabilityLoadSession = "loadSession"

// Update kinds for session/update notifications, pinned from the ACP v1
// released vocabulary.
const (
	// UpdateAgentMessageChunk streams agent text to the client as one content
	// block. SOP progress rides this pinned kind: the text block renders in
	// any client, and the structured fields travel in the notification's sop
	// member (see buildProgressFrame in progress.go). SOP states never appear
	// as update kinds or other protocol status values; they are payload
	// fields only.
	UpdateAgentMessageChunk = "agent_message_chunk"
)

// Pinned ACP v1 stop reasons — the protocol-defined vocabulary for the
// session/prompt response's stopReason member. These five spellings are the
// only stop reasons this build can emit, and they describe the prompt turn's
// end at the transport boundary. They never describe SOP workflow state:
// LOCAL_DONE, verified completion, waiting for approval, and blocked work are
// carried as session/update payload fields (progress.go), never as stop
// reasons or other protocol status values.
const (
	// StopReasonEndTurn ends a finished prompt turn: the response is final
	// for this turn, whatever the started work's state is.
	StopReasonEndTurn = "end_turn"
	// StopReasonMaxTokens pins the vocabulary value for a turn cut off by its
	// token bound (not emitted by this build today).
	StopReasonMaxTokens = "max_tokens"
	// StopReasonMaxTurnRequests pins the vocabulary value for a turn cut off
	// by its turn-request bound (not emitted by this build today).
	StopReasonMaxTurnRequests = "max_turn_requests"
	// StopReasonRefusal pins the vocabulary value for a refused turn (not
	// emitted by this build today).
	StopReasonRefusal = "refusal"
	// StopReasonCancelled marks a turn whose foreground governed work was
	// stopped by session/cancel: the job's recorded SOP outcome is cancelled —
	// never failed, never completed — and the cancel result doc carries the
	// same value (see cancel.go). The other stop reasons below remain unused
	// vocabulary in this build.
	StopReasonCancelled = "cancelled"
)

// StopReasons lists the pinned stop-reason vocabulary in pin order; treat it
// as read-only. Tests assert that SOP states stay out of this list.
var StopReasons = []string{
	StopReasonEndTurn,
	StopReasonMaxTokens,
	StopReasonMaxTurnRequests,
	StopReasonRefusal,
	StopReasonCancelled,
}

// AgentCapabilities returns the capability object advertised in the initialize
// response. It derives solely from the contract constants: a key set to false
// is a capability this build explicitly does not implement, so a client request
// that needs it is refused with CodeCapabilityUnsupported instead of a silent
// no-op. Keys are added only when this build implements the behavior.
func AgentCapabilities() map[string]any {
	return map[string]any{
		// Session resumption (session/load) is owned by a later ACP stage;
		// advertising false keeps the rejection actionable.
		CapabilityLoadSession: false,
	}
}

// JSON-RPC 2.0 reserved error codes.
const (
	// CodeParseError rejects a frame that is not valid JSON.
	CodeParseError = -32700
	// CodeInvalidRequest rejects valid JSON that is not a usable request: a
	// non-object frame, a missing method, a foreign jsonrpc version, or an
	// oversized frame.
	CodeInvalidRequest = -32600
	// CodeMethodNotFound rejects a method outside the pinned ACP surface.
	CodeMethodNotFound = -32601
	// CodeInvalidParams rejects malformed or unknown-field method arguments.
	CodeInvalidParams = -32602
	// CodeInternalError reports a server-side failure.
	CodeInternalError = -32603
)

// SOP ACP server-range codes (the unregistered server range -32000..-32099).
// No external library fixes these values, so they are pinned here.
const (
	// CodeVersionUnsupported rejects an initialize whose declared protocol
	// version is not the pinned one; the message always names the version.
	CodeVersionUnsupported = -32000
	// CodeCapabilityUnsupported rejects a request for a capability this build
	// does not advertise; the message names the capability.
	CodeCapabilityUnsupported = -32001
	// CodeNotInitialized rejects a method that arrived before initialize
	// succeeded; the message names the required ordering.
	CodeNotInitialized = -32002
)

// Bounds. The values are contract decisions, recorded once here and asserted
// in contract_test.go so a change cannot happen silently.
const (
	// MaxFrameSize is the maximum input frame size in bytes (1 MiB). A larger
	// frame is rejected with CodeInvalidRequest and the reader resynchronizes
	// on the next newline; memory stays bounded to the bound, not the frame.
	MaxFrameSize = 1 << 20
	// EventBufferBound is the maximum number of outbound event frames the
	// server buffers before rejecting further events with ErrEventBufferFull.
	EventBufferBound = 64
)
