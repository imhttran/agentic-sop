package cli

import (
	"context"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	sopctx "github.com/imhttran/agentic-sop/internal/context"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

func enabledRAGEnv(t *testing.T) {
	t.Helper()
	t.Setenv(config.EnvRAGEnabled, "true")
	t.Setenv(config.EnvRAGExecutable, "/usr/local/bin/rag-retrieval")
	t.Setenv(config.EnvRAGCorpus, "kb")
	t.Setenv(config.EnvRAGAllowedCorpora, "kb")
}

func TestOptionalRAGDisabledDoesNotCallRetriever(t *testing.T) {
	t.Setenv(config.EnvRAGEnabled, "false")
	called := false
	d := deps{ragKnowledge: func(context.Context, *taskfile.Spec) ([]sopctx.Item, error) {
		called = true
		return nil, nil
	}}
	if got := optionalRAGKnowledge(context.Background(), config.Config{}, &taskfile.Spec{ID: "T1", Title: "t"}, d); got != nil {
		t.Fatalf("disabled RAG returned %v", got)
	}
	if called {
		t.Fatal("disabled RAG invoked the injected retriever")
	}
}

func TestOptionalRAGEnabledCallsRetriever(t *testing.T) {
	enabledRAGEnv(t)
	called := false
	item := sopctx.Item{Source: sopctx.SourceKnowledgeRAG, Identity: "kb/a.md", Priority: sopctx.PriorityKnowledgeRAG, Text: "evidence"}
	d := deps{ragKnowledge: func(context.Context, *taskfile.Spec) ([]sopctx.Item, error) {
		called = true
		return []sopctx.Item{item}, nil
	}}
	got := optionalRAGKnowledge(context.Background(), config.Config{}, &taskfile.Spec{ID: "T1", Title: "t"}, d)
	if !called || len(got) != 1 || got[0].Source != sopctx.SourceKnowledgeRAG {
		t.Fatalf("enabled RAG did not use the injected retriever: called=%v got=%v", called, got)
	}
}

func TestOptionalRAGSkipsRequiredEvidenceTasks(t *testing.T) {
	enabledRAGEnv(t)
	called := false
	d := deps{ragKnowledge: func(context.Context, *taskfile.Spec) ([]sopctx.Item, error) {
		called = true
		return nil, nil
	}}
	if got := optionalRAGKnowledge(context.Background(), config.Config{}, &taskfile.Spec{ID: "T1", RequiredEvidence: []string{"corpus:kb"}}, d); got != nil {
		t.Fatalf("required-evidence task received optional items: %v", got)
	}
	if called {
		t.Fatal("required-evidence task invoked optional retrieval")
	}
}
