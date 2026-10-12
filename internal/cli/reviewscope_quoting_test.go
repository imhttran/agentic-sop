package cli

import (
	"strings"
	"testing"
)

// TestDiffChunkPathDecodesHeaderPaths is the table-driven regression for the
// review-scope safety bug: diffChunkPath must decode git's C-quoted header tokens
// (the quote wraps the a/ or b/ prefix too), tab-separated timestamps, rename
// lines (quoted and unquoted), and /dev/null sides. A line that is not one of
// these headers, or a header side naming no real path, reports ok=false.
func TestDiffChunkPathDecodesHeaderPaths(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
		ok   bool
	}{
		{
			name: "unquoted_add_side",
			line: "+++ b/a.go",
			want: "a.go",
			ok:   true,
		},
		{
			name: "unquoted_remove_side",
			line: "--- a/a.go",
			want: "a.go",
			ok:   true,
		},
		{
			name: "unquoted_space_with_tab_timestamp",
			line: "--- a/sp ace.go\t2024-01-01 00:00:00 +0000",
			want: "sp ace.go",
			ok:   true,
		},
		{
			name: "c_quoted_non_ascii_remove_side",
			line: "--- \"a/caf\\303\\251.go\"",
			want: "caf\u00e9.go",
			ok:   true,
		},
		{
			name: "c_quoted_non_ascii_add_side",
			line: "+++ \"b/caf\\303\\251.go\"",
			want: "caf\u00e9.go",
			ok:   true,
		},
		{
			name: "c_quoted_with_space",
			line: "+++ \"b/sp ace.go\"",
			want: "sp ace.go",
			ok:   true,
		},
		{
			name: "c_quoted_backslash",
			line: "--- \"a/dir\\\\sub.go\"",
			want: "dir\\sub.go",
			ok:   true,
		},
		{
			name: "quoted_rename_from",
			line: "rename from \"old\\303\\251.go\"",
			want: "old\u00e9.go",
			ok:   true,
		},
		{
			name: "unquoted_rename_to",
			line: "rename to new.go",
			want: "new.go",
			ok:   true,
		},
		{
			name: "dev_null_remove_side",
			line: "--- /dev/null",
			want: "",
			ok:   false,
		},
		{
			name: "dev_null_add_side",
			line: "+++ /dev/null",
			want: "",
			ok:   false,
		},
		{
			name: "content_line_not_a_header",
			line: "@@ -1 +1 @@",
			want: "",
			ok:   false,
		},
		{
			name: "malformed_unterminated_quote",
			line: "--- \"a/unterminated.go",
			want: "",
			ok:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := diffChunkPath(tc.line)
			if ok != tc.ok {
				t.Fatalf("diffChunkPath(%q) ok = %v, want %v", tc.line, ok, tc.ok)
			}
			if got != tc.want {
				t.Errorf("diffChunkPath(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}

// TestFilterDiffToScopeQuotedHeaders is the end-to-end regression for silent
// review-coverage loss: a fully C-quoted chunk must survive the scope filter, a
// genuinely out-of-scope chunk must still be dropped, and a header that cannot be
// decoded to a real path must never silently drop a chunk that may be in scope.
func TestFilterDiffToScopeQuotedHeaders(t *testing.T) {
	quotedNonASCII := "diff --git \"a/caf\\303\\251.go\" \"b/caf\\303\\251.go\"\n" +
		"index 814f4a4..4cb29ea 100644\n" +
		"--- \"a/caf\\303\\251.go\"\n" +
		"+++ \"b/caf\\303\\251.go\"\n" +
		"@@ -1,2 +1,3 @@\n" +
		" one\n" +
		" two\n" +
		"+three\n"

	outOfScope := "diff --git a/unrelated.go b/unrelated.go\n" +
		"--- a/unrelated.go\n" +
		"+++ b/unrelated.go\n" +
		"@@ -1 +1 @@\n" +
		"-old\n" +
		"+new\n"

	cases := []struct {
		name    string
		diff    string
		scope   []string
		want    []string
		wantNot []string
	}{
		{
			name:  "c_quoted_non_ascii_in_scope",
			diff:  quotedNonASCII,
			scope: []string{"caf\u00e9.go"},
			want:  []string{"b/caf\\303\\251.go"},
		},
		{
			name:    "c_quoted_non_ascii_out_of_scope_dropped",
			diff:    quotedNonASCII,
			scope:   []string{"other.go"},
			wantNot: []string{"caf"},
		},
		{
			name: "quoted_with_space_in_scope",
			diff: "diff --git \"a/sp ace.go\" \"b/sp ace.go\"\n" +
				"--- \"a/sp ace.go\"\n" +
				"+++ \"b/sp ace.go\"\n" +
				"@@ -1 +1 @@\n-old\n+new\n",
			scope: []string{"sp ace.go"},
			want:  []string{"b/sp ace.go"},
		},
		{
			name: "unquoted_space_tab_timestamp_in_scope",
			diff: "diff --git a/sp ace.go b/sp ace.go\n" +
				"--- a/sp ace.go\t2024-01-01 00:00:00 +0000\n" +
				"+++ b/sp ace.go\t2024-01-01 00:00:00 +0000\n" +
				"@@ -1 +1 @@\n-old\n+new\n",
			scope: []string{"sp ace.go"},
			want:  []string{"sp ace.go"},
		},
		{
			name: "quoted_and_unquoted_rename_in_scope",
			diff: "diff --git \"a/old\\303\\251.go\" b/new.go\n" +
				"similarity index 100%\n" +
				"rename from \"old\\303\\251.go\"\n" +
				"rename to new.go\n",
			scope: []string{"old\u00e9.go", "new.go"},
			want:  []string{"rename to new.go"},
		},
		{
			name: "rename_out_of_scope_source_drops_chunk",
			diff: "diff --git \"a/old\\303\\251.go\" b/new.go\n" +
				"similarity index 100%\n" +
				"rename from \"old\\303\\251.go\"\n" +
				"rename to new.go\n",
			scope:   []string{"new.go"},
			wantNot: []string{"old"},
		},
		{
			name: "dev_null_addition_in_scope",
			diff: "diff --git a/new.go b/new.go\n" +
				"new file mode 100644\n" +
				"--- /dev/null\n" +
				"+++ b/new.go\n" +
				"@@ -0,0 +1,1 @@\n+x\n",
			scope: []string{"new.go"},
			want:  []string{"+++ b/new.go"},
		},
		{
			name: "dev_null_deletion_in_scope",
			diff: "diff --git a/gone.go b/gone.go\n" +
				"deleted file mode 100644\n" +
				"--- a/gone.go\n" +
				"+++ /dev/null\n" +
				"@@ -1 +0,0 @@\n-gone\n",
			scope: []string{"gone.go"},
			want:  []string{"--- a/gone.go"},
		},
		{
			name:    "in_scope_kept_out_of_scope_dropped",
			diff:    quotedNonASCII + outOfScope,
			scope:   []string{"caf\u00e9.go"},
			want:    []string{"b/caf\\303\\251.go"},
			wantNot: []string{"unrelated.go"},
		},
		{
			name: "malformed_header_keeps_in_scope_chunk",
			diff: "diff --git a/keep.go b/keep.go\n" +
				"--- \"a/keep.go\n" +
				"+++ \"b/keep.go\n" +
				"@@ -1 +1 @@\n-old\n+new\n",
			scope: []string{"keep.go"},
			want:  []string{"keep.go"},
		},
		{
			name: "diff_git_only_chunk_kept",
			diff: "diff --git a/mode.go b/mode.go\n" +
				"old mode 100644\n" +
				"new mode 100755\n",
			scope: []string{"mode.go"},
			want:  []string{"old mode 100644"},
		},
		{
			name: "malformed_header_does_not_resurrect_out_of_scope_chunk",
			diff: "diff --git a/unrelated.go b/unrelated.go\n" +
				"--- a/unrelated.go\n" +
				"+++ b/unrelated.go\n" +
				"@@ -1 +1 @@\n" +
				"--- garbage malformed content\n" +
				"+new\n",
			scope:   []string{"ours.go"},
			wantNot: []string{"unrelated"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := filterDiffToScope(tc.diff, newReviewScope(tc.scope, nil))
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("filtered diff omitted %q:\n%s", w, got)
				}
			}
			for _, w := range tc.wantNot {
				if strings.Contains(got, w) {
					t.Errorf("filtered diff retained out-of-scope %q:\n%s", w, got)
				}
			}
		})
	}
}

// TestScopedReviewDiffRetainsQuotedInScopeChange proves the full review-scope
// seam (not just the filter) keeps a fully C-quoted in-scope change instead of
// dropping it, which was the silent-coverage-loss failure.
func TestScopedReviewDiffRetainsQuotedInScopeChange(t *testing.T) {
	diff := "diff --git \"a/caf\\303\\251.go\" \"b/caf\\303\\251.go\"\n" +
		"--- \"a/caf\\303\\251.go\"\n" +
		"+++ \"b/caf\\303\\251.go\"\n" +
		"@@ -1 +1 @@\n-old\n+new\n"
	scope := newReviewScope([]string{"caf\u00e9.go"}, nil)
	got, err := scopedReviewDiff(scope, diff, true)
	if err != nil {
		t.Fatalf("scopedReviewDiff: %v", err)
	}
	if !strings.Contains(got, "b/caf\\303\\251.go") {
		t.Errorf("fully C-quoted in-scope change was dropped (silent coverage loss):\n%s", got)
	}
}
