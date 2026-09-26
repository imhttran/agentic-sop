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
