package outcomefile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLLM09GateContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GateWithSourcesAndDigestsContext(ctx, []OutcomeRecord{{
		RecordID: "r", Complete: true, Authorized: true, Status: StatusPartial,
		SourceArtifacts: []string{"/nonexistent/source"},
	}}, FileSourceExists, FileSourceDigest)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestLLM09FileSourceDigestContextCanceled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := FileSourceDigestContext(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestLLM09ContextGateMatchesOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source")
	if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	digest, err := FileSourceDigest(path)
	if err != nil {
		t.Fatal(err)
	}
	rec := OutcomeRecord{
		RecordID: "r", Corpus: "outcome", Status: StatusPartial, ContentHash: "h",
		Complete: true, Authorized: true,
		SourceArtifacts: []string{path}, SourceDigests: map[string]string{path: digest},
	}
	records := []OutcomeRecord{rec}

	want := GateWithSourcesAndDigests(records, FileSourceExists, FileSourceDigest)
	got, gerr := GateWithSourcesAndDigestsContext(context.Background(), records, FileSourceExists, FileSourceDigest)
	if gerr != nil {
		t.Fatalf("context gate: %v", gerr)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("context gate = %+v, want %+v", got, want)
	}

	if err := os.WriteFile(path, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	want = GateWithSourcesAndDigests(records, FileSourceExists, FileSourceDigest)
	got, gerr = GateWithSourcesAndDigestsContext(context.Background(), records, FileSourceExists, FileSourceDigest)
	if gerr != nil {
		t.Fatalf("context gate after modification: %v", gerr)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("context gate after modification = %+v, want %+v", got, want)
	}
	if len(got) != 1 || got[0].Reason != ReasonSourceModified {
		t.Fatalf("modified source exclusions = %+v, want %s", got, ReasonSourceModified)
	}
}
