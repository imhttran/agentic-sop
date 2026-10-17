package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/git"
)

// Bounded committed-content review context.
//
// REVIEW normally receives only the invocation's scoped working-tree diff. When a
// task's implementation was already committed before this invocation, its paths
// have no diff chunk, so the reviewer cannot see them at all. This file appends a
// small, deterministic, clearly-labelled section of the committed content of the
// task's OWN scoped paths (the run's recorded change set plus the declared report
// deliverables) for exactly the paths that are absent from the invocation diff.
//
// It is review context only. It never decides a gate, never changes when review
// runs, never reads the working tree (so a staged or unstaged change can never be
// presented as committed content), never establishes task ownership, and never
// fails the run: an unavailable path or an unavailable Git result is omitted.

// Limits for the committed-content section. They are deliberately small: the
// section supplements the invocation diff, it does not replace it.
const (
	maxCommittedReviewFiles      = 8
	maxCommittedReviewFileBytes  = 16 << 10 // 16 KiB per file
	maxCommittedReviewTotalBytes = 64 << 10 // 64 KiB in total
)

// committedReviewHeader introduces the appended section. The wording states the
// two facts a reviewer must not misread: the content is committed at HEAD, and it
// is explicitly NOT a change made by the current invocation.
const committedReviewHeader = `===== EXISTING COMMITTED CONTENT (not changed in this invocation) =====
The files below are this task's own scoped paths whose content is committed at HEAD
and is absent from this invocation's diff. They are read-only review context. They
are NOT changes made by this invocation and must not be reported as mutations of it,
and the last-touch commit is provenance only, not proof of task ownership.`

// reviewInput composes the exact review input for one task: the scoped
// working-tree diff, followed by the bounded committed-content section. ownChange
// is false for a completion the invocation does not own (a task that required no
// change, a legacy no-change completion, or a verified ALREADY_SATISFIED), and the
// committed section is then never added, so those reviews keep the existing input
// byte for byte. A nil error with an empty string means the task legitimately has
// nothing to review. The scoped-diff error is propagated unchanged, so the
// existing fail-closed behaviour (ErrReviewScopeUnestablished) is preserved.
func reviewInput(ctx context.Context, dir string, scope reviewScope, diff string, changeRequired, ownChange bool, reports ...string) (string, error) {
	scoped, err := scopedReviewDiff(scope, diff, changeRequired)
	if err != nil {
		return "", err
	}
	if !ownChange {
		return scoped, nil
	}
	return appendCommittedReviewContext(scoped, reviewExistingContext(ctx, dir, scope, diff, reports...)), nil
}

// appendCommittedReviewContext appends the committed-content section to the
// review input. An empty section returns the input unchanged, so a review with no
// committed context is byte-identical to the scoped diff alone and keeps the
// existing review cache identity.
func appendCommittedReviewContext(reviewDiff, section string) string {
	if section == "" {
		return reviewDiff
	}
	if strings.TrimSpace(reviewDiff) == "" {
		return section
	}
	return reviewDiff + "\n\n" + section
}

// reviewExistingContext returns the committed-content section for the scoped paths
// that are absent from the invocation diff, or an empty string when there is
// nothing to add. It is deterministic, bounded, and never returns an error: every
// exclusion and budget outcome is either an explicit note or a silent omission,
// and the run outcome is unchanged either way.
func reviewExistingContext(ctx context.Context, dir string, scope reviewScope, diff string, reports ...string) string {
	if scope.empty() {
		return ""
	}
	// Paths named by the invocation diff are this invocation's own change set; they
	// are never presented as committed content. The check uses the full working-tree
	// diff (before scope filtering), so a scope-filtered chunk is still recognised.
	inDiff := map[string]bool{}
	for _, path := range changedFiles(diff) {
		inDiff[path] = true
	}
	adapter := git.New(dir)
	var blocks, omittedForBudget []string
	policyOmitted := 0
	total := 0
	for _, path := range scope.paths {
		if inDiff[path] {
			continue
		}
		if !includeCommittedReviewPath(path, reports...) {
			policyOmitted++
			continue
		}
		if len(blocks) >= maxCommittedReviewFiles {
			omittedForBudget = append(omittedForBudget, path)
			continue
		}
		entry, data, err := adapter.HeadFile(ctx, path, maxCommittedReviewFileBytes)
		if err != nil {
			if errors.Is(err, git.ErrHeadFileTooLarge) {
				omittedForBudget = append(omittedForBudget, path)
			} else {
				// Absent at HEAD, a directory, a symbolic link, or a submodule, or
				// Git is unavailable. Never fall back to the working tree.
				policyOmitted++
			}
			continue
		}
		if total+int(entry.Size) > maxCommittedReviewTotalBytes {
			omittedForBudget = append(omittedForBudget, path)
			continue
		}
		if looksBinary(data) {
			policyOmitted++
			continue
		}
		provenance, _ := adapter.LastCommit(ctx, path)
		blocks = append(blocks, renderCommittedBlock(path, data, provenance))
		total += len(data)
	}
	if len(blocks) == 0 {
		// Nothing to add: keep the review input byte-identical to the scoped diff so
		// the existing behaviour and review cache identity are untouched.
		return ""
	}
	var b strings.Builder
	b.WriteString(committedReviewHeader)
	for _, block := range blocks {
		b.WriteString("\n\n")
		b.WriteString(block)
	}
	if len(omittedForBudget) > 0 {
		sort.Strings(omittedForBudget)
		fmt.Fprintf(&b, "\n\nOmitted for budget (%d path(s), no content included): %s", len(omittedForBudget), strings.Join(omittedForBudget, ", "))
	}
	if policyOmitted > 0 {
		fmt.Fprintf(&b, "\n\nExcluded by policy (%d path(s)): sensitive, binary, unsafe, or not a file committed at HEAD. Their content is not included.", policyOmitted)
	}
	return b.String()
}

// includeCommittedReviewPath reports whether a scoped path may appear in the
// committed-content section. It rejects an unsafe or escaping path, Git metadata,
// SOP's own state and generated reports, and a sensitive path, so neither an
// unrelated nor a secret file can reach a reviewer.
func includeCommittedReviewPath(path string, reports ...string) bool {
	if !safeRepoPath(path) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "" || clean == "." {
		return false
	}
	if clean == ".git" || strings.HasPrefix(clean, ".git/") {
		return false
	}
	if isSOPPath(clean, reports...) {
		return false
	}
	return !isSensitiveReviewPath(clean)
}

// sensitiveReviewDirs are directories whose contents are never review context:
// cloud, SSH, and similar credential stores.
var sensitiveReviewDirs = map[string]bool{
	".aws":    true,
	".ssh":    true,
	".kube":   true,
	".gnupg":  true,
	"secrets": true,
}

// sensitiveReviewExtensions are file extensions that denote key material.
var sensitiveReviewExtensions = map[string]bool{
	".pem": true, ".key": true, ".p12": true, ".pfx": true, ".keystore": true, ".jks": true,
}

// isSensitiveReviewPath reports whether a repository-relative path names a
// sensitive file: a file in a credential directory, a well-known credential file
// (.env, .npmrc, .netrc, an SSH private key, a credentials file), key material, or
// a name that begins with "secret". It is deliberately conservative, because an
// excluded path is merely absent from review context whereas an included secret
// would be disclosed to a reviewer and to the review provider.
func isSensitiveReviewPath(path string) bool {
	slash := filepath.ToSlash(path)
	segments := strings.Split(slash, "/")
	for _, segment := range segments[:len(segments)-1] {
		if sensitiveReviewDirs[strings.ToLower(segment)] {
			return true
		}
	}
	base := segments[len(segments)-1]
	lower := strings.ToLower(base)
	if sensitiveReviewDirs[lower] || sensitiveReviewExtensions[strings.ToLower(filepath.Ext(base))] {
		return true
	}
	switch {
	case lower == ".env", strings.HasPrefix(lower, ".env."):
		return true
	case lower == ".npmrc", lower == ".netrc", lower == ".git-credentials":
		return true
	case strings.HasPrefix(lower, "credentials"):
		return true
	case strings.HasPrefix(lower, "id_rsa"), strings.HasPrefix(lower, "id_dsa"),
		strings.HasPrefix(lower, "id_ecdsa"), strings.HasPrefix(lower, "id_ed25519"):
		return true
	case strings.HasPrefix(lower, "secret"), strings.HasSuffix(lower, ".secret"),
		strings.Contains(lower, ".secret."):
		return true
	}
	return false
}

// looksBinary mirrors the Git adapter's binary heuristic (a NUL byte in the first
// 8000 bytes). A binary file is never included: its bytes are not reviewable
// source and could carry arbitrary data into the review context.
func looksBinary(data []byte) bool {
	n := len(data)
	if n > 8000 {
		n = 8000
	}
	return bytes.IndexByte(data[:n], 0) >= 0
}

// renderCommittedBlock renders one committed file with explicit provenance.
// Framing, ordering, and content are deterministic. A missing provenance line is
// simply omitted.
func renderCommittedBlock(path string, data []byte, provenance string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "--- path: %s (committed at HEAD; not changed in this invocation)\n", path)
	if provenance != "" {
		fmt.Fprintf(&b, "--- last commit touching this path: %s (provenance only; not proof of task ownership)\n", provenance)
	}
	fmt.Fprintf(&b, "----- begin %s -----\n", path)
	b.Write(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "----- end %s -----\n", path)
	return b.String()
}
