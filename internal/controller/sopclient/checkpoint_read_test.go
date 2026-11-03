package sopclient

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A damaged checkpoint is reported as damage, and the merge cache must not replay
// the last good value as if it were still current.
func TestDamagedCheckpointIsReportedNotReplayed(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root, `INSERT INTO tasks VALUES ('t1','T','o','a','PLANNED','',0,3,'`+now+`','`+now+`')`)
	writeArtifact(t, root, "t1", "checkpoint.json", `{"label":"CP1","analyzed":2,"total":5}`)

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	first, err := st.Task(ctx, "t1", ApprovalsListing{})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Checkpoint.Present || first.Checkpoint.Analyzed != 2 {
		t.Fatalf("checkpoint = %+v, want the reported record", first.Checkpoint)
	}
	if first.CheckpointError != "" {
		t.Fatalf("CheckpointError = %q, want empty", first.CheckpointError)
	}

	// Damage the file. The cached 2/5 must not be replayed as current.
	writeArtifact(t, root, "t1", "checkpoint.json", `{not json`)
	second, err := st.Task(ctx, "t1", ApprovalsListing{})
	if err != nil {
		t.Fatal(err)
	}
	if second.CheckpointError == "" {
		t.Fatal("CheckpointError empty for damaged checkpoint.json; damage must not look like absence")
	}
	if second.Checkpoint.Present {
		t.Fatalf("checkpoint = %+v, want absent after a read error (no stale replay)", second.Checkpoint)
	}
}

// A damaged activity.jsonl makes the checkpoint fallback an error, not an absence.
func TestDamagedCheckpointActivityIsReported(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root, `INSERT INTO tasks VALUES ('t1','T','o','a','PLANNED','',0,3,'`+now+`','`+now+`')`)
	if err := os.MkdirAll(filepath.Join(root, ".agent-sdlc", "runs", "t1", "activity.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	detail, err := st.Task(context.Background(), "t1", ApprovalsListing{})
	if err != nil {
		t.Fatal(err)
	}
	if detail.CheckpointError == "" {
		t.Fatal("CheckpointError empty for unreadable activity.jsonl; damage must not look like absence")
	}
	if detail.Checkpoint.Present {
		t.Fatalf("checkpoint = %+v, want absent after a read error", detail.Checkpoint)
	}
}
