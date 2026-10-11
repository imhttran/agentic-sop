package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/imhttran/agentic-sop/internal/approval"
)

// approvalSupersedeUsage is the one-line usage for the supported supersession
// command.
const approvalSupersedeUsage = "usage: sop approval supersede <task-id> --reason TEXT --by NAME"

// supersedeOptions are the parsed arguments of `sop approval supersede`.
type supersedeOptions struct {
	id     string
	reason string
	by     string
}

// parseSupersedeArgs parses
// `sop approval supersede <task-id> --reason TEXT --by NAME`. Both --reason and
// --by are mandatory: a supersession is an explicit, audited operator action, and
// the CLI refuses to relay one that omits the operator or the justification rather
// than defaulting either.
func parseSupersedeArgs(args []string, stderr io.Writer) (supersedeOptions, bool) {
	var opts supersedeOptions
	fail := func() (supersedeOptions, bool) {
		fmt.Fprintln(stderr, approvalSupersedeUsage)
		return supersedeOptions{}, false
	}
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--reason":
			if i+1 >= len(args) {
				return fail()
			}
			i++
			opts.reason = args[i]
		case strings.HasPrefix(a, "--reason="):
			opts.reason = strings.TrimPrefix(a, "--reason=")
		case a == "--by":
			if i+1 >= len(args) {
				return fail()
			}
			i++
			opts.by = args[i]
		case strings.HasPrefix(a, "--by="):
			opts.by = strings.TrimPrefix(a, "--by=")
		case strings.HasPrefix(a, "-"):
			return fail()
		default:
			if opts.id != "" {
				return fail()
			}
			opts.id = a
		}
	}
	if strings.TrimSpace(opts.id) == "" {
		return fail()
	}
	// The required operator authorization is validated here and revalidated by the
	// application boundary, which is the single authority for the error.
	if strings.TrimSpace(opts.reason) == "" || strings.TrimSpace(opts.by) == "" {
		fmt.Fprintln(stderr, "approval supersede: --reason and --by are required")
		fmt.Fprintln(stderr, approvalSupersedeUsage)
		return supersedeOptions{}, false
	}
	return opts, true
}

// runApprovalSupersede supersedes a stale PENDING approval request whose task is
// already satisfied, recording an audited reason and operator. It routes entirely
// through the approval application boundary — it never reads or writes approval.json
// or approval-history.json directly — so the boundary's gating, fail-closed
// ordering, and idempotency are the only semantics. It surfaces the boundary's
// distinct refusal errors as distinct nonzero exits.
func runApprovalSupersede(args []string, stdout, stderr io.Writer, d deps) int {
	opts, ok := parseSupersedeArgs(args, stderr)
	if !ok {
		return exitUsage
	}

	dir, st, ok := openApprovalState(d, stderr, "approval supersede")
	if !ok {
		return exitError
	}
	defer st.Close()

	res, err := approvalService(st, dir).Supersede(opts.id, approval.SupersedeInput{Reason: opts.reason, By: opts.by})
	if err != nil {
		// The boundary is the single authority for exactly why a supersession was
		// refused; the CLI reports its error verbatim and never resolves it itself.
		fmt.Fprintf(stderr, "approval supersede: %v\n", err)
		return exitError
	}
	if res.Idempotent {
		fmt.Fprintf(stdout, "already superseded: %s\n", opts.id)
		return exitOK
	}
	v := res.View
	fmt.Fprintf(stdout, "superseded %s\n", opts.id)
	fmt.Fprintf(stdout, "Status: %s\n", v.Status)
	if v.DecidedBy != "" {
		fmt.Fprintf(stdout, "Superseded by: %s\n", v.DecidedBy)
	}
	if v.Note != "" {
		fmt.Fprintf(stdout, "Reason: %s\n", v.Note)
	}
	if !v.DecidedAt.IsZero() {
		fmt.Fprintf(stdout, "Superseded at: %s\n", v.DecidedAt.Format("2006-01-02T15:04:05Z07:00"))
	}
	return exitOK
}
