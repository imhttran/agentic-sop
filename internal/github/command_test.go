package github

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type call struct {
	name string
	args []string
}

func newFakeClient(respond func(name string, args []string) ([]byte, error)) (*CommandClient, *[]call) {
	calls := &[]call{}
	c := &CommandClient{
		dir: "test-dir",
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			*calls = append(*calls, call{name: name, args: args})
			return respond(name, args)
		},
	}
	return c, calls
}

func ok(name string, args []string) ([]byte, error) { return nil, nil }

func TestPushBranch(t *testing.T) {
	c, calls := newFakeClient(ok)
	if err := c.PushBranch(context.Background(), "task/T014-github-adapter"); err != nil {
		t.Fatalf("PushBranch failed: %v", err)
	}
	got := (*calls)[0]
	if got.name != "git" || strings.Join(got.args, " ") != "push -u origin task/T014-github-adapter" {
		t.Errorf("call = %s %v", got.name, got.args)
	}
}

func TestGetPullRequestParses(t *testing.T) {
	c, _ := newFakeClient(func(name string, args []string) ([]byte, error) {
		return []byte(`{"number":7,"url":"https://gh/pr/7","state":"OPEN","headRefName":"task/x","baseRefName":"main","mergeable":"MERGEABLE"}`), nil
	})
	pr, err := c.GetPullRequest(context.Background(), "task/x")
	if err != nil {
		t.Fatalf("GetPullRequest failed: %v", err)
	}
	if pr.Number != 7 || pr.URL != "https://gh/pr/7" || pr.State != "OPEN" || pr.Head != "task/x" || pr.Mergeable != "MERGEABLE" {
		t.Errorf("pr = %+v", pr)
	}
}

func TestCreatePullRequest(t *testing.T) {
	c, calls := newFakeClient(func(name string, args []string) ([]byte, error) {
		if len(args) > 1 && args[1] == "create" {
			return nil, nil
		}
		return []byte(`{"number":3,"url":"u","state":"OPEN","headRefName":"b","baseRefName":"main","mergeable":"MERGEABLE"}`), nil
	})
	pr, err := c.CreatePullRequest(context.Background(), CreateRequest{Base: "main", Head: "b", Title: "t", Body: "body"})
	if err != nil {
		t.Fatalf("CreatePullRequest failed: %v", err)
	}
	if pr.Number != 3 {
		t.Errorf("pr = %+v", pr)
	}
	create := (*calls)[0]
	if create.name != "gh" || create.args[1] != "create" || !contains(create.args, "main") || !contains(create.args, "b") {
		t.Errorf("create call = %v", create.args)
	}
}

func TestCreatePullRequestValidatesRequired(t *testing.T) {
	c, _ := newFakeClient(ok)
	if _, err := c.CreatePullRequest(context.Background(), CreateRequest{Base: "main"}); err == nil {
		t.Error("expected error for missing head/title")
	}
}

func TestChecksNormalizesStates(t *testing.T) {
	c, _ := newFakeClient(func(string, []string) ([]byte, error) {
		return []byte(`[{"name":"build","state":"SUCCESS","link":"l1"},{"name":"test","state":"FAILURE","link":"l2"},{"name":"lint","state":"QUEUED","link":"l3"}]`), nil
	})
	checks, err := c.Checks(context.Background(), 3)
	if err != nil {
		t.Fatalf("Checks failed: %v", err)
	}
	want := []CheckState{CheckPass, CheckFail, CheckPending}
	if len(checks) != 3 {
		t.Fatalf("got %d checks", len(checks))
	}
	for i, w := range want {
		if checks[i].State != w {
			t.Errorf("check %d state = %s, want %s", i, checks[i].State, w)
		}
	}
}

func TestChecksParsesEvenWhenCommandFails(t *testing.T) {
	// `gh pr checks` exits non-zero for failing/pending checks but still prints JSON.
	c, _ := newFakeClient(func(string, []string) ([]byte, error) {
		return []byte(`[{"name":"test","state":"FAILURE","link":"l"}]`), errors.New("exit 1")
	})
	checks, err := c.Checks(context.Background(), 3)
	if err != nil {
		t.Fatalf("Checks should still parse: %v", err)
	}
	if len(checks) != 1 || checks[0].State != CheckFail {
		t.Errorf("checks = %+v", checks)
	}
}

func TestMerge(t *testing.T) {
	c, calls := newFakeClient(ok)
	if err := c.Merge(context.Background(), 3, "squash"); err != nil {
		t.Fatalf("Merge failed: %v", err)
	}
	if got := strings.Join((*calls)[0].args, " "); got != "pr merge 3 --squash" {
		t.Errorf("merge args = %q", got)
	}
	if err := c.Merge(context.Background(), 3, "flatten"); err == nil {
		t.Error("expected error for unsupported merge method")
	}
}

func TestMalformedJSONIsError(t *testing.T) {
	c, _ := newFakeClient(func(string, []string) ([]byte, error) { return []byte("not json"), nil })
	if _, err := c.GetPullRequest(context.Background(), "b"); err == nil {
		t.Error("expected parse error")
	}
}

func TestCommandFailurePropagates(t *testing.T) {
	c, _ := newFakeClient(func(string, []string) ([]byte, error) { return nil, errors.New("gh missing") })
	if err := c.PushBranch(context.Background(), "task/x"); err == nil {
		t.Error("expected push failure to propagate")
	}
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
