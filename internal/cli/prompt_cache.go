package cli

import (
	"context"
	"os"
	"path/filepath"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/prompt"
	"github.com/imhttran/agentic-sop/internal/promptcache"
	"github.com/imhttran/agentic-sop/internal/workitem"
)

// promptGenerate runs a read-only prompt, serving an identical request from the CTX-008
// Prompt Result Cache when --cache is set. Only read-only capabilities are cached: a
// mutating capability never is, and a cache hit is only a replay of model output, not
// task success. The cache key combines the compiled prompt, the model identity, and the
// compiler version, so a changed prompt, model, or compiler is a miss.
func promptGenerate(ctx context.Context, dir string, a agent.Agent, wi workitem.WorkItem, sel model.Selection, useCache bool) (string, error) {
	req := agent.Request{
		Capability:         wi.Capability,
		Task:               wi.Content,
		OutputRequirements: promptOutputRequirements(wi.Capability),
	}
	if !useCache || !promptcache.Eligible(wi.Capability) {
		resp, err := a.Generate(ctx, req)
		if err != nil {
			return "", err
		}
		return resp.Content, nil
	}

	compiled := prompt.Compile(prompt.Input{
		Capability:         wi.Capability,
		Task:               wi.Content,
		OutputRequirements: req.OutputRequirements,
	})
	id := promptcache.Identity{
		Schema:     promptcache.SchemaVersion,
		Capability: string(wi.Capability),
		Prompt:     compiled.Digest,
		Context:    "",
		Model:      sel.Provider + "/" + sel.Model,
		Parameters: "",
		Compiler:   "prompt-compiler/1",
	}
	cache := loadPromptCache(dir)
	if hit := cache.Lookup(id); hit.Hit {
		return hit.Entry.Content, nil
	}

	resp, err := a.Generate(ctx, req)
	if err != nil {
		return "", err
	}
	cache.Store(id, resp.Content, "agent")
	_ = savePromptCache(dir, cache)
	return resp.Content, nil
}

func promptCachePath(dir string) string {
	return filepath.Join(dir, stateDirName, "context", "prompt-cache.json")
}

func loadPromptCache(dir string) *promptcache.Cache {
	data, err := os.ReadFile(promptCachePath(dir))
	if err != nil {
		return promptcache.New()
	}
	cache, err := promptcache.Unmarshal(data)
	if err != nil {
		return promptcache.New()
	}
	return cache
}

func savePromptCache(dir string, cache *promptcache.Cache) error {
	data, err := cache.Marshal()
	if err != nil {
		return err
	}
	path := promptCachePath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
