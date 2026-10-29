package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/imhttran/agentic-sop/internal/approval"
	"github.com/imhttran/agentic-sop/internal/planflow"
	"github.com/imhttran/agentic-sop/internal/projectlock"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

// approvalInvalidateAttestedUsage is the one-line usage for the attested path.
const approvalInvalidateAttestedUsage = "usage: sop approval invalidate-attested <task-id> --reason TEXT --by NAME --expected-request ID --obsolete-requirement TEXT --replacement-definition TEXT --expect-definition-digest SHA"

// attestedOptions are the parsed arguments of `sop approval invalidate-attested`.
type attestedOptions struct {
	id, reason, by, expected, obsolete, replacement, expectDigest string
}

// parseAttestedArgs parses the attested-invalidation arguments. Every value is
// mandatory: an attested invalidation is an explicit, audited operator action, and the
// CLI refuses to relay one that omits any part of the statement.
func parseAttestedArgs(args []string, stderr io.Writer) (attestedOptions, bool) {
	var opts attestedOptions
	fail := func() (attestedOptions, bool) {
		fmt.Fprintln(stderr, approvalInvalidateAttestedUsage)
		return attestedOptions{}, false
	}
	value := func(i *int) (string, bool) {
		if *i+1 >= len(args) {
			return "", false
		}
		*i++
		return args[*i], true
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--reason" || strings.HasPrefix(a, "--reason="):
			v, ok := flagValue(a, &i, args)
			if !ok {
				return fail()
			}
			opts.reason = v
		case a == "--by" || strings.HasPrefix(a, "--by="):
			v, ok := flagValue(a, &i, args)
			if !ok {
				return fail()
			}
			opts.by = v
		case a == "--expected-request" || strings.HasPrefix(a, "--expected-request="):
			v, ok := flagValue(a, &i, args)
			if !ok {
				return fail()
			}
			opts.expected = v
		case a == "--obsolete-requirement" || strings.HasPrefix(a, "--obsolete-requirement="):
			v, ok := flagValue(a, &i, args)
			if !ok {
				return fail()
			}
			opts.obsolete = v
		case a == "--replacement-definition" || strings.HasPrefix(a, "--replacement-definition="):
			v, ok := flagValue(a, &i, args)
			if !ok {
				return fail()
			}
			opts.replacement = v
		case a == "--expect-definition-digest" || strings.HasPrefix(a, "--expect-definition-digest="):
			v, ok := flagValue(a, &i, args)
			if !ok {
				return fail()
			}
			opts.expectDigest = v
		case strings.HasPrefix(a, "-"):
			return fail()
		default:
			if opts.id != "" {
				return fail()
			}
			opts.id = a
		}
	}
	_ = value
	if strings.TrimSpace(opts.id) == "" || strings.TrimSpace(opts.reason) == "" || strings.TrimSpace(opts.by) == "" ||
		strings.TrimSpace(opts.expected) == "" || strings.TrimSpace(opts.obsolete) == "" ||
		strings.TrimSpace(opts.replacement) == "" || strings.TrimSpace(opts.expectDigest) == "" {
		fmt.Fprintln(stderr, "approval invalidate-attested: --reason, --by, --expected-request, --obsolete-requirement, --replacement-definition and --expect-definition-digest are required")
		fmt.Fprintln(stderr, approvalInvalidateAttestedUsage)
		return attestedOptions{}, false
	}
	return opts, true
}

// flagValue returns "--name value" or "--name=value".
func flagValue(a string, i *int, args []string) (string, bool) {
	if eq := strings.IndexByte(a, '='); eq >= 0 {
		return a[eq+1:], true
	}
	if *i+1 >= len(args) {
		return "", false
	}
	*i++
	return args[*i], true
}

// runApprovalInvalidateAttested invalidates a legacy PENDING request on the operator's
// present-day attestation. It routes through the approval boundary, which owns every
// guard; the CLI only supplies the persisted evidence the guards check.
func runApprovalInvalidateAttested(args []string, stdout, stderr io.Writer, d deps) int {
	opts, ok := parseAttestedArgs(args, stderr)
	if !ok {
		return exitUsage
	}
	dir, st, ok := openApprovalState(d, stderr, "approval invalidate-attested")
	if !ok {
		return exitError
	}
	defer st.Close()

	// The evidence (definition digest and text, reconciliation provenance) and the
	// invalidation decision must see the same state, so both run under one project
	// lock. The service is built without its own lock: the lock is not re-entrant.
	lock, err := projectlock.AcquireWait(context.Background(), dir, 0)
	if err != nil {
		fmt.Fprintf(stderr, "approval invalidate-attested: %v\n", err)
		return exitError
	}
	defer lock.Release()

	in, err := attestedEvidence(dir, st, opts)
	if err != nil {
		fmt.Fprintf(stderr, "approval invalidate-attested: %v\n", err)
		return exitError
	}

	svc := approval.NewWithLock(st, func(taskID string) approval.Record {
		return runpkg.At(runpkg.Dir(dir, taskID))
	}, nil)
	res, err := svc.InvalidateAttested(opts.id, in)
	if err != nil {
		fmt.Fprintf(stderr, "approval invalidate-attested: %v\n", err)
		return exitError
	}
	if res.Idempotent {
		fmt.Fprintf(stdout, "already invalidated (attested): %s\n", opts.id)
		writeApprovalView(stdout, res.View)
		return exitOK
	}
	fmt.Fprintf(stdout, "invalidated (attested) %s\n", opts.id)
	writeApprovalView(stdout, res.View)
	return exitOK
}

// attestedEvidence gathers the persisted state the boundary's guards check: the
// current definition digest and text, and whether the plan provenance names this task.
func attestedEvidence(projectDir string, st *store.Store, opts attestedOptions) (approval.InvalidateAttestedInput, error) {
	var in approval.InvalidateAttestedInput
	in.Reason = opts.reason
	in.By = opts.by
	in.ExpectedRequestID = opts.expected
	in.ObsoleteRequirement = opts.obsolete
	in.ReplacementDefinition = opts.replacement
	in.ExpectDefinitionDigest = opts.expectDigest
	in.CurrentPlanFingerprint = activePlanFingerprint(projectDir)

	identity, err := planflow.TaskDefinitionIdentity(projectDir, opts.id)
	if err != nil {
		return in, err
	}
	if identity.Present {
		in.CurrentDefinitionDigest = identity.Digest
	}
	text, _ := planflow.TaskDefinitionText(projectDir, opts.id)
	in.CurrentDefinitionText = text

	provenance, err := planflow.TaskReconciliationProvenance(projectDir, opts.id)
	if err != nil {
		return in, err
	}
	in.TaskReconciled = provenance.Reconciled
	in.DefinitionAcceptedAsReconciled = provenance.Accepted

	if _, err := st.Get(opts.id); err != nil && !store.IsNotFound(err) {
		return in, err
	}
	return in, nil
}
