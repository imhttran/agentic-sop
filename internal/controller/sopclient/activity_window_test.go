package sopclient

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/controller/config"
)

// An unreadable activity artifact is an error, not an empty timeline: the live
// window and the per-task view must say the read failed.
func TestActivityViewsSurfaceReadFailure(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root, `INSERT INTO tasks VALUES ('t1','T','o','a','PLANNED','',0,3,'`+now+`','`+now+`')`)
	if err := os.MkdirAll(filepath.Join(root, ".agent-sdlc", "runs", "t1", "activity.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}

	bin, _ := fakeSop(t)
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	pid := config.ProjectID(root)

	if _, err := c.ActivityWindow(ctx, pid, "", 10); err == nil {
		t.Fatal("ActivityWindow err = nil for an unreadable activity.jsonl; an empty timeline must not hide the failure")
	}
	if _, err := c.TaskActivityView(ctx, pid, "t1"); err == nil {
		t.Fatal("TaskActivityView err = nil for an unreadable activity.jsonl")
	}
}
