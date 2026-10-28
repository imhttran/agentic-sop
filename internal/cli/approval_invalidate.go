package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/imhttran/agentic-sop/internal/approval"
	"github.com/imhttran/agentic-sop/internal/planflow"
	"github.com/imhttran/agentic-sop/internal/store"
)

// approvalInvalidateUsage is the one-line usage for the supported invalidation
// command.
const approvalInvalidateUsage = "usage: sop approval invalidate <task-id> --reason TEXT --by NAME"

// invalidateOptions are the parsed arguments of `sop approval invalidate`.
type invalidateOptions struct {
	id     string
	reason string
	by     string
}

// parseInvalidateArgs parses
// `sop approval invalidate <task-id> --reason TEXT --by NAME`. Both --reason and
// --by are mandatory: an invalidation is an explicit, audited operator action,
// and the CLI refuses to relay one that omits the operator or the justification
// rather than defaulting either. The refusal happens before any store access, so
// a missing input records nothing.
func parseInvalidateArgs(args []string, stderr io.Writer) (invalidateOptions, bool) {
	var opts invalidateOptions
	fail := func() (invalidateOptions, bool) {
		fmt.Fprintln(stderr, approvalInvalidateUsage)
		return invalidateOptions{}, false
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
	// The required operator authorization is validated here and revalidated by
	// the application boundary, which is the single authority for the error.
	if strings.TrimSpace(opts.reason) == "" || strings.TrimSpace(opts.by) == "" {
		fmt.Fprintln(stderr, "approval invalidate: --reason and --by are required")
		fmt.Fprintln(stderr, approvalInvalidateUsage)
		return invalidateOptions{}, false
	}
	return opts, true
}

// runApprovalInvalidate invalidates a PENDING approval request because the plan it
// authorized no longer matches the task definition after an accepted revision. It
// routes entirely through the approval application boundary — it never reads or
// writes approval.json or approval-history.json directly — so the boundary's
// gating, fail-closed ordering, and idempotency are the only semantics. It surfaces
// the boundary's distinct refusal errors as distinct nonzero exits.
func runApprovalInvalidate(args []string, stdout, stderr io.Writer, d deps) int {
	opts, ok := parseInvalidateArgs(args, stderr)
	if !ok {
		return exitUsage
	}

	dir, st, ok := openApprovalState(d, stderr, "approval invalidate")
	if !ok {
		return exitError
	}
	defer st.Close()

	svc := approvalService(st, dir)
	in, err := invalidationEvidence(dir, st, opts.id)
	if err != nil {
		fmt.Fprintf(stderr, "approval invalidate: %v\n", err)
		return exitError
	}
	in.Reason = opts.reason
	in.By = opts.by

	res, err := svc.Invalidate(opts.id, in)
	if err != nil {
		// The boundary is the single authority for exactly why an invalidation was
		// refused; the CLI reports its error verbatim and never resolves it itself.
		fmt.Fprintf(stderr, "approval invalidate: %v\n", err)
		return exitError
	}

	if res.Idempotent {
		fmt.Fprintf(stdout, "already invalidated: %s\n", opts.id)
		writeApprovalView(stdout, res.View)
		return exitOK
	}
	fmt.Fprintf(stdout, "invalidated %s\n", opts.id)
	writeApprovalView(stdout, res.View)
	return exitOK
}

// invalidationEvidence resolves the revision evidence the application boundary
// needs: the current task definition digest the request's definition is compared
// with, and the reconciliation provenance the boundary needs for a legacy request
// that recorded no digest. The boundary decides applicability; this helper only
// supplies what the persisted state can prove. An unreadable definition or
// provenance read never supplies a guess: the boundary then refuses the
// invalidation because the revision is unproven.
func invalidationEvidence(projectDir string, st *store.Store, taskID string) (approval.InvalidateInput, error) {
	var in approval.InvalidateInput

	identity, err := planflow.TaskDefinitionIdentity(projectDir, taskID)
	if err != nil {
		return in, err
	}
	if identity.Present {
		in.CurrentDefinitionDigest = identity.Digest
	}
	in.CurrentPlanFingerprint = activePlanFingerprint(projectDir)

	provenance, err := planflow.TaskReconciliationProvenance(projectDir, taskID)
	if err != nil {
		return in, err
	}
	if provenance.Present {
		in.DefinitionAcceptedAsReconciled = provenance.Reconciled
		in.ReconciledAt = provenance.RecordedAt
	}

	// A task record is not required for the revision proof: an unknown task id is
	// refused by the boundary with its own message. The store read here only keeps
	// an unreadable store from being mistaken for proven revision.
	if _, err := st.Get(taskID); err != nil && !store.IsNotFound(err) {
		return in, err
	}
	return in, nil
}
