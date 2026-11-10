package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/git"
	"github.com/imhttran/agentic-sop/internal/outcome"
	"github.com/imhttran/agentic-sop/internal/planacceptance"
)

const historicalOutcomeUsage = `usage:
  sop outcome --plan <PLAN.md> [--archive <dir>] --accept|--reject <criterion> --evidence <text> --evidence-ref <path=...,sha256=...> [--annotation <id>] --by <name>
  sop outcome --plan <PLAN.md> [--archive <dir>] --annotate <criterion> --reviewed-evidence <text> --reason <text> --evidence-ref <path=...,sha256=...> --by <name>
  sop outcome --plan <PLAN.md> [--archive <dir>] --run-check <criterion> [--tested-revision <commit>] [--by <name>]`

const historicalVerifyUsage = "usage: sop verify --plan <PLAN.md> [--archive <dir>] [--json]"

type historicalOutcomeArgs struct {
	plan           string
	archive        string
	accept         string
	reject         string
	annotate       string
	runCheck       string
	evidence       string
	evidenceRefs   []string
	reviewed       string
	reason         string
	annotation     string
	by             string
	testedRevision string
}

func runHistoricalOutcome(args []string, stdout, stderr io.Writer, getwd func() (string, error)) int {
	var in historicalOutcomeArgs
	for i := 0; i < len(args); i++ {
		a := args[i]
		take := func() (string, bool) {
			if i+1 >= len(args) {
				return "", false
			}
			i++
			return args[i], true
		}
		var ok bool
		switch {
		case a == "--plan":
			in.plan, ok = take()
		case strings.HasPrefix(a, "--plan="):
			in.plan = strings.TrimPrefix(a, "--plan=")
			ok = true
		case a == "--archive":
			in.archive, ok = take()
		case strings.HasPrefix(a, "--archive="):
			in.archive = strings.TrimPrefix(a, "--archive=")
			ok = true
		case a == "--accept":
			in.accept, ok = take()
		case strings.HasPrefix(a, "--accept="):
			in.accept = strings.TrimPrefix(a, "--accept=")
			ok = true
		case a == "--reject":
			in.reject, ok = take()
		case strings.HasPrefix(a, "--reject="):
			in.reject = strings.TrimPrefix(a, "--reject=")
			ok = true
		case a == "--annotate":
			in.annotate, ok = take()
		case strings.HasPrefix(a, "--annotate="):
			in.annotate = strings.TrimPrefix(a, "--annotate=")
			ok = true
		case a == "--run-check":
			in.runCheck, ok = take()
		case strings.HasPrefix(a, "--run-check="):
			in.runCheck = strings.TrimPrefix(a, "--run-check=")
			ok = true
		case a == "--evidence":
			in.evidence, ok = take()
		case strings.HasPrefix(a, "--evidence="):
			in.evidence = strings.TrimPrefix(a, "--evidence=")
			ok = true
		case a == "--evidence-ref":
			var v string
			v, ok = take()
			if ok {
				in.evidenceRefs = append(in.evidenceRefs, v)
			}
		case strings.HasPrefix(a, "--evidence-ref="):
			in.evidenceRefs = append(in.evidenceRefs, strings.TrimPrefix(a, "--evidence-ref="))
			ok = true
		case a == "--reviewed-evidence":
			in.reviewed, ok = take()
		case strings.HasPrefix(a, "--reviewed-evidence="):
			in.reviewed = strings.TrimPrefix(a, "--reviewed-evidence=")
			ok = true
		case a == "--reason":
			in.reason, ok = take()
		case strings.HasPrefix(a, "--reason="):
			in.reason = strings.TrimPrefix(a, "--reason=")
			ok = true
		case a == "--annotation":
			in.annotation, ok = take()
		case strings.HasPrefix(a, "--annotation="):
			in.annotation = strings.TrimPrefix(a, "--annotation=")
			ok = true
		case a == "--by":
			in.by, ok = take()
		case strings.HasPrefix(a, "--by="):
			in.by = strings.TrimPrefix(a, "--by=")
			ok = true
		case a == "--tested-revision":
			in.testedRevision, ok = take()
		case strings.HasPrefix(a, "--tested-revision="):
			in.testedRevision = strings.TrimPrefix(a, "--tested-revision=")
			ok = true
		default:
			fmt.Fprintln(stderr, historicalOutcomeUsage)
			return exitUsage
		}
		if !ok {
			fmt.Fprintln(stderr, historicalOutcomeUsage)
			return exitUsage
		}
	}
	if strings.TrimSpace(in.plan) == "" {
		fmt.Fprintln(stderr, historicalOutcomeUsage)
		return exitUsage
	}
	actions := 0
	for _, v := range []string{in.accept, in.reject, in.annotate, in.runCheck} {
		if strings.TrimSpace(v) != "" {
			actions++
		}
	}
	if actions != 1 {
		fmt.Fprintln(stderr, "historical acceptance: choose exactly one of --accept, --reject, --annotate or --run-check")
		fmt.Fprintln(stderr, historicalOutcomeUsage)
		return exitUsage
	}

	dir, ok := projectDir(getwd, stderr)
	if !ok {
		return exitError
	}
	ctx, err := planacceptance.LoadContext(dir, in.plan, in.archive)
	if err != nil {
		fmt.Fprintf(stderr, "historical acceptance: %v\n", err)
		return exitError
	}
	designated, err := loadDesignatedApprovers(dir)
	if err != nil {
		fmt.Fprintf(stderr, "historical acceptance: %v\n", err)
		return exitError
	}

	refs, err := parseEvidenceRefs(in.evidenceRefs)
	if err != nil {
		fmt.Fprintf(stderr, "historical acceptance: %v\n", err)
		return exitUsage
	}

	switch {
	case in.annotate != "":
		ann, err := planacceptance.RecordAnnotation(ctx, planacceptance.Annotation{
			CriterionID:      in.annotate,
			ReviewedEvidence: in.reviewed,
			Reason:           in.reason,
			EvidenceRefs:     refs,
			Approver:         in.by,
		}, designated)
		if err != nil {
			fmt.Fprintf(stderr, "historical acceptance: %v\n", err)
			return exitError
		}
		fmt.Fprintf(stdout, "recorded manual-evidence annotation: criterion=%s annotation=%s approver=%s\n", ann.CriterionID, ann.ID, ann.Approver)
		fmt.Fprintf(stdout, "  archived evidence: %s\n", ann.ArchivedManualEvidence)
		fmt.Fprintf(stdout, "  reviewed evidence: %s\n", ann.ReviewedEvidence)
		return exitOK
	case in.accept != "" || in.reject != "":
		criterion, decision := in.accept, outcome.DecisionAccept
		if in.reject != "" {
			criterion, decision = in.reject, outcome.DecisionReject
		}
		d, err := planacceptance.RecordDecision(ctx, outcome.AcceptanceDecision{
			CriterionID:  criterion,
			Decision:     decision,
			Evidence:     in.evidence,
			Approver:     in.by,
			AnnotationID: in.annotation,
			EvidenceRefs: refs,
		}, designated)
		if err != nil {
			fmt.Fprintf(stderr, "historical acceptance: %v\n", err)
			return exitError
		}
		fmt.Fprintf(stdout, "recorded historical-plan %s decision: criterion=%s approver=%s recorded_at=%s\n", d.Decision, d.CriterionID, d.Approver, d.RecordedAt)
		return exitOK
	default:
		return runHistoricalCheck(dir, ctx, in, designated, stdout, stderr)
	}
}

func runHistoricalCheck(dir string, ctx *planacceptance.Context, in historicalOutcomeArgs, designated []string, stdout, stderr io.Writer) int {
	m, ok := planacceptance.FindMapping(ctx, in.runCheck)
	if !ok {
		fmt.Fprintf(stderr, "historical acceptance: criterion %q is not in the archived plan\n", in.runCheck)
		return exitError
	}
	if m.Kind != outcome.EvidenceCheck {
		fmt.Fprintf(stderr, "historical acceptance: criterion %q is not check-mapped; use --accept for manual criteria\n", in.runCheck)
		return exitError
	}
	cfg, err := config.LoadDir(dir)
	if err != nil && !errors.Is(err, config.ErrNotFound) {
		fmt.Fprintf(stderr, "historical acceptance: %v\n", err)
		return exitError
	}
	argv, err := authorizeAcceptanceCommandWith(m.Command, cfg.CargoAcceptanceCheckEnabled(), dir)
	if err != nil {
		fmt.Fprintf(stderr, "historical acceptance: %v\n", err)
		return exitError
	}
	beforeHead, beforeDirty, err := historicalSourceState(dir)
	if err != nil {
		fmt.Fprintf(stderr, "historical acceptance: %v\n", err)
		return exitError
	}
	if len(beforeDirty) > 0 {
		fmt.Fprintf(stderr, "historical acceptance: working tree is dirty; clean it before running an archived check: %s\n", strings.Join(beforeDirty, ", "))
		return exitError
	}
	if in.testedRevision != "" && beforeHead != in.testedRevision {
		fmt.Fprintf(stderr, "historical acceptance: tested-revision assertion failed: HEAD is %s, expected %s\n", beforeHead, in.testedRevision)
		return exitError
	}

	runErr, run := runAcceptanceCheck(dir, argv)
	afterHead, afterDirty, stateErr := historicalSourceState(dir)
	if stateErr != nil {
		fmt.Fprintf(stderr, "historical acceptance: %v\n", stateErr)
		return exitError
	}
	sourceUnchanged := afterHead == beforeHead && len(afterDirty) == 0

	logPath, logSHA, err := planacceptance.WriteCheckLog(ctx, m.CriterionID, run.full)
	if err != nil {
		fmt.Fprintf(stderr, "historical acceptance: write check log: %v\n", err)
		return exitError
	}
	ce, err := planacceptance.RecordCheck(ctx, planacceptance.CheckEvidence{
		CriterionID:     m.CriterionID,
		CheckRef:        m.CheckRef,
		Command:         m.Command,
		Argv:            argv,
		Class:           "SAFE",
		Executed:        true,
		Passed:          runErr == nil,
		ExitCode:        run.exitCode,
		Output:          run.output,
		OutputSHA256:    sha256HexString(run.output),
		LogPath:         logPath,
		LogSHA256:       logSHA,
		TestedRevision:  beforeHead,
		TestedDirty:     false,
		SourceUnchanged: sourceUnchanged,
		RecordedBy:      displayOr(in.by, "operator"),
	})
	if err != nil {
		fmt.Fprintf(stderr, "historical acceptance: %v\n", err)
		return exitError
	}
	if !sourceUnchanged {
		fmt.Fprintf(stderr, "historical acceptance: check recorded but not accepted: the checked source changed during execution (HEAD %s -> %s; dirty: %s)\n", beforeHead, afterHead, strings.Join(afterDirty, ", "))
		return exitError
	}
	if runErr != nil {
		fmt.Fprintf(stderr, "historical acceptance: check recorded but not accepted: %v\n", runErr)
		return exitError
	}
	fmt.Fprintf(stdout, "recorded historical check: criterion=%s command=%q tested_revision=%s class=SAFE\n", ce.CriterionID, ce.Command, ce.TestedRevision)
	return exitOK
}

func runHistoricalVerify(args []string, stdout, stderr io.Writer, getwd func() (string, error)) int {
	plan, archive, jsonOut := "", "", false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--plan":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, historicalVerifyUsage)
				return exitUsage
			}
			i++
			plan = args[i]
		case strings.HasPrefix(a, "--plan="):
			plan = strings.TrimPrefix(a, "--plan=")
		case a == "--archive":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, historicalVerifyUsage)
				return exitUsage
			}
			i++
			archive = args[i]
		case strings.HasPrefix(a, "--archive="):
			archive = strings.TrimPrefix(a, "--archive=")
		case a == "--json":
			jsonOut = true
		default:
			fmt.Fprintln(stderr, historicalVerifyUsage)
			return exitUsage
		}
	}
	if strings.TrimSpace(plan) == "" {
		fmt.Fprintln(stderr, historicalVerifyUsage)
		return exitUsage
	}
	dir, ok := projectDir(getwd, stderr)
	if !ok {
		return exitError
	}
	ctx, err := planacceptance.LoadContext(dir, plan, archive)
	if err != nil {
		fmt.Fprintf(stderr, "historical verify: %v\n", err)
		return exitError
	}
	designated, err := loadDesignatedApprovers(dir)
	if err != nil {
		fmt.Fprintf(stderr, "historical verify: %v\n", err)
		return exitError
	}
	rep, err := planacceptance.Evaluate(ctx, designated)
	if err != nil {
		fmt.Fprintf(stderr, "historical verify: %v\n", err)
		return exitError
	}
	if rep.Corrupt != nil {
		fmt.Fprintf(stderr, "historical verify: ledger corrupt: %v\n", rep.Corrupt)
		return exitError
	}
	if jsonOut {
		payload := map[string]any{
			"plan_id":         rep.PlanID,
			"source":          rep.Source,
			"disposition":     rep.Disposition,
			"recorded_at":     rep.RecordedAt,
			"plan_identity":   rep.PlanIdentity,
			"archive_dir":     rep.ArchiveDir,
			"current_head":    rep.CurrentHead,
			"counts":          rep.Counts,
			"accepted":        rep.Accepted,
			"check_revisions": rep.CheckRevisions,
			"criteria":        rep.Criteria,
		}
		out, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "historical verify: %v\n", err)
			return exitError
		}
		fmt.Fprintln(stdout, string(out))
		return exitOK
	}

	fmt.Fprintf(stdout, "historical plan: %s\n", rep.PlanID)
	fmt.Fprintf(stdout, "source: %s\n", rep.Source)
	fmt.Fprintf(stdout, "lifecycle: %s at %s (read-only; not acceptance)\n", displayOr(rep.Disposition, "UNKNOWN"), displayOr(rep.RecordedAt, "unknown"))
	fmt.Fprintf(stdout, "archive: %s\n", rep.ArchiveDir)
	fmt.Fprintf(stdout, "plan identity: sha256:%s\n", rep.PlanIdentity)
	fmt.Fprintf(stdout, "current HEAD: %s\n", displayOr(rep.CurrentHead, "unknown"))
	fmt.Fprintf(stdout, "mapped: %d (%d manual, %d check)\n", rep.Counts.Mapped, rep.Counts.Manual, rep.Counts.Check)
	fmt.Fprintf(stdout, "decision-eligible: %d; manually accepted: %d; check passed: %d; check unverified: %d; stale: %d; rejected: %d\n",
		rep.Counts.DecisionEligible, rep.Counts.ManualAccepted, rep.Counts.CheckPassed, rep.Counts.Unverified, rep.Counts.Stale, rep.Counts.Rejected)
	fmt.Fprintf(stdout, "accepted: %d/%d\n", rep.Accepted, rep.Counts.Mapped)
	fmt.Fprintf(stdout, "archived observation: not recorded in the archive; %d manual mapping(s) decision-eligible (no historical-observation flag is inferred)\n", rep.Counts.DecisionEligible)
	revs := "none"
	if len(rep.CheckRevisions) > 0 {
		revs = strings.Join(rep.CheckRevisions, ", ")
	}
	fmt.Fprintf(stdout, "evidence scope: manual decisions verified at report time; accepted check revisions: %s; current-tree verified: no\n", revs)
	fmt.Fprintln(stdout, "criteria:")
	for _, cr := range rep.Criteria {
		kind := string(cr.Mapping.Kind)
		if kind == "" {
			kind = "unmapped"
		}
		rev := ""
		if cr.Revision != "" {
			rev = fmt.Sprintf(" (tested revision %s)", cr.Revision)
		}
		fmt.Fprintf(stdout, "  %-28s [%-6s] %s%s: %s\n", cr.Mapping.CriterionID, kind, cr.Status, rev, cr.Evidence)
		if cr.ArchivedEvidence != "" {
			fmt.Fprintf(stdout, "      archived evidence: %s\n", cr.ArchivedEvidence)
		}
		if cr.ReviewedEvidence != "" {
			fmt.Fprintf(stdout, "      reviewed evidence: %s\n", cr.ReviewedEvidence)
		}
	}
	return exitOK
}

func historicalSourceState(dir string) (string, []string, error) {
	g := git.New(dir)
	head, err := g.Head(context.Background())
	if err != nil {
		return "", nil, err
	}
	entries, err := g.StatusPaths(context.Background())
	if err != nil {
		return "", nil, err
	}
	var dirty []string
	for _, e := range entries {
		p := filepath.ToSlash(e.Path)
		// SOP-owned state and the plan-scoped audit are not source changes.
		if p == config.DirName || strings.HasPrefix(p, config.DirName+"/") {
			continue
		}
		dirty = append(dirty, p)
	}
	sort.Strings(dirty)
	return head, dirty, nil
}

func parseEvidenceRefs(specs []string) ([]outcome.EvidenceRef, error) {
	refs := make([]outcome.EvidenceRef, 0, len(specs))
	for _, spec := range specs {
		var ref outcome.EvidenceRef
		for _, part := range strings.Split(spec, ",") {
			key, val, ok := strings.Cut(part, "=")
			if !ok {
				return nil, fmt.Errorf("evidence-ref %q must be comma-separated key=value pairs", spec)
			}
			switch strings.TrimSpace(key) {
			case "path":
				ref.Path = strings.TrimSpace(val)
			case "sha256":
				ref.SHA256 = strings.TrimSpace(val)
			case "revision":
				ref.Revision = strings.TrimSpace(val)
			case "section":
				ref.Section = strings.TrimSpace(val)
			default:
				return nil, fmt.Errorf("evidence-ref %q has unknown key %q", spec, key)
			}
		}
		if ref.Path == "" || ref.SHA256 == "" {
			return nil, fmt.Errorf("evidence-ref %q requires path and sha256", spec)
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func loadDesignatedApprovers(dir string) ([]string, error) {
	cfg, err := config.LoadDir(dir)
	if err != nil && !errors.Is(err, config.ErrNotFound) {
		return nil, err
	}
	return cfg.Human.DesignatedApprovers, nil
}

func sha256HexString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
