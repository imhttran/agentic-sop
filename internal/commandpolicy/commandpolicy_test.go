package commandpolicy

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want Class
	}{
		{"go test", []string{"go", "test", "./..."}, Safe},
		{"go build", []string{"go", "build", "./..."}, Safe},
		{"git diff", []string{"git", "diff"}, Safe},
		{"git status", []string{"git", "status", "--short"}, Safe},
		{"git with global flag and value", []string{"git", "-C", "repo", "diff"}, Safe},
		{"git commit", []string{"git", "commit", "-m", "x"}, RequiresApproval},
		{"git push", []string{"git", "push", "origin", "main"}, RequiresApproval},
		{"git push force", []string{"git", "push", "--force"}, Denied},
		{"git push -f", []string{"git", "push", "-f", "origin", "main"}, Denied},
		{"git push force-with-lease", []string{"git", "push", "--force-with-lease"}, Denied},
		{"git push force-with-lease value", []string{"git", "push", "--force-with-lease=origin/main", "main"}, Denied},
		{"git alone", []string{"git"}, RequiresApproval},
		{"unknown program", []string{"make", "test"}, RequiresApproval},
		{"go tidy (writes)", []string{"go", "mod", "tidy"}, RequiresApproval},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Classify(tc.argv)
			if err != nil {
				t.Fatalf("Classify(%v) error: %v", tc.argv, err)
			}
			if got != tc.want {
				t.Errorf("Classify(%v) = %s, want %s", tc.argv, got, tc.want)
			}
		})
	}
}

// Inline git configuration can name a program git later executes, so a read-only
// subcommand is no longer read-only. It needs approval; it must never classify
// SAFE. The global position is the only place git reads these options, so
// subcommand flags such as `git log -c` and `git grep -c` stay SAFE.
func TestClassifyGitConfigOverride(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want Class
	}{
		// Injected configuration: refused SAFE, routed to approval.
		{"config via -c with diff driver", []string{"git", "-c", "diff.external=./evil.sh", "diff"}, RequiresApproval},
		{"config via -c with fsmonitor", []string{"git", "-c", "core.fsmonitor=id", "status"}, RequiresApproval},
		{"config via -c with pager", []string{"git", "-c", "core.pager=id", "log"}, RequiresApproval},
		{"config via -c with alias", []string{"git", "-c", "alias.ci=!id", "ci"}, RequiresApproval},
		{"config via attached -c", []string{"git", "-ccore.pager=id", "log"}, RequiresApproval},
		{"config via --config-env", []string{"git", "--config-env=diff.external=EVIL", "diff"}, RequiresApproval},
		{"config via --config-env with value", []string{"git", "--config-env", "diff.external=EVIL", "diff"}, RequiresApproval},
		{"config after a global option", []string{"git", "-C", "repo", "-c", "core.pager=id", "log"}, RequiresApproval},
		{"config after --no-pager", []string{"git", "--no-pager", "-c", "core.pager=id", "log"}, RequiresApproval},

		// Legitimate Git workflows keep their classification.
		{"plain status", []string{"git", "status", "--short"}, Safe},
		{"plain diff", []string{"git", "diff"}, Safe},
		{"log with combined flag", []string{"git", "log", "-c"}, Safe},
		{"grep count flag", []string{"git", "grep", "-c", "needle"}, Safe},
		{"read-only with global directory", []string{"git", "-C", "repo", "diff"}, Safe},
		{"log oneline", []string{"git", "log", "--oneline"}, Safe},
		{"push still needs approval", []string{"git", "push", "origin", "main"}, RequiresApproval},
		{"force push still denied", []string{"git", "push", "--force"}, Denied},
		{"force push denied under -c", []string{"git", "-c", "color.ui=false", "push", "--force", "origin", "main"}, Denied},
		{"force-with-lease denied under -c", []string{"git", "-c", "color.ui=false", "push", "--force-with-lease", "origin", "main"}, Denied},
		{"-f denied under attached -c", []string{"git", "-ccolor.ui=false", "push", "-f", "origin", "main"}, Denied},
		{"force push denied after -C", []string{"git", "-C", "repo", "push", "--force"}, Denied},
		{"plain push under -c still needs approval", []string{"git", "-c", "color.ui=false", "push", "origin", "main"}, RequiresApproval},
		{"bundled -fu denied", []string{"git", "push", "-fu", "origin", "main"}, Denied},
		{"unambiguous --force-with prefix denied", []string{"git", "push", "--force-with", "origin", "main"}, Denied},
		{"--force-if-includes denied", []string{"git", "push", "--force-if-includes", "origin", "main"}, Denied},
		{"plus refspec forces", []string{"git", "push", "origin", "+main"}, Denied},
		{"deletion refspec denied", []string{"git", "push", "origin", ":main"}, Denied},
		{"--mirror denied", []string{"git", "push", "--mirror", "origin"}, Denied},
		{"--delete denied", []string{"git", "push", "--delete", "origin", "feat"}, Denied},
		{"unknown push flag fails closed", []string{"git", "push", "--no-verify", "origin", "main"}, Denied},
		{"set-upstream still needs approval", []string{"git", "push", "-u", "origin", "main"}, RequiresApproval},
		{"dry-run still needs approval", []string{"git", "push", "--dry-run", "origin", "main"}, RequiresApproval},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Classify(tc.argv)
			if err != nil {
				t.Fatalf("Classify(%v) error: %v", tc.argv, err)
			}
			if got != tc.want {
				t.Errorf("Classify(%v) = %s, want %s", tc.argv, got, tc.want)
			}
		})
	}
}

func TestClassifyEmpty(t *testing.T) {
	if _, err := Classify(nil); err == nil {
		t.Error("expected an error for an empty command")
	}
	if _, err := Classify([]string{"  "}); err == nil {
		t.Error("expected an error for a blank program")
	}
}

func TestPolicyTightens(t *testing.T) {
	cases := []struct {
		name   string
		policy Policy
		argv   []string
		want   Class
	}{
		{
			name:   "deny a normally safe command",
			policy: Policy{Deny: [][]string{{"go", "test"}}},
			argv:   []string{"go", "test", "./..."},
			want:   Denied,
		},
		{
			name:   "require approval for a normally safe command",
			policy: Policy{Approve: [][]string{{"git", "diff"}}},
			argv:   []string{"git", "diff"},
			want:   RequiresApproval,
		},
		{
			name:   "deny a normally approved command",
			policy: Policy{Deny: [][]string{{"git", "push"}}},
			argv:   []string{"git", "push", "origin", "main"},
			want:   Denied,
		},
		{
			name:   "cannot loosen a denied command",
			policy: Policy{Approve: [][]string{{"git", "push", "--force"}}},
			argv:   []string{"git", "push", "--force"},
			want:   Denied,
		},
		{
			name:   "prefix must match whole tokens",
			policy: Policy{Deny: [][]string{{"git", "push", "--force"}}},
			argv:   []string{"git", "push", "origin", "main"},
			want:   RequiresApproval,
		},
		{
			name:   "no policy rule leaves the default",
			policy: Policy{},
			argv:   []string{"go", "build", "./..."},
			want:   Safe,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.policy.Classify(tc.argv)
			if err != nil {
				t.Fatalf("Classify(%v) error: %v", tc.argv, err)
			}
			if got != tc.want {
				t.Errorf("Classify(%v) = %s, want %s", tc.argv, got, tc.want)
			}
		})
	}
}
