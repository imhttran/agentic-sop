package cli

import (
	"os"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/model"
)

// modelInputs builds the routing inputs for one CLI seam, including the endpoint
// resolver the model layer uses for origin-based locality inference.
//
// EndpointFor returns the endpoint the selected class would actually be served
// at: a class's SOP_MODEL_<CLASS>_BASE_URL override for the openai_compatible
// provider, otherwise the shared provider endpoint. It is a runtime-selection
// input only: it chooses which origin the inference compares, and it is never
// runtime evidence, never persisted, and never printed. Returning an endpoint for
// a non-openai provider is inert because only the openai_compatible origin has an
// inference rule.
func modelInputs(cfg config.Config, cliClass string) model.Inputs {
	return model.Inputs{
		Config:   cfg.Models,
		Lookup:   os.Getenv,
		CLIClass: cliClass,
		EndpointFor: func(provider string, class model.Class) string {
			if provider == agent.ProviderOpenAICompatible {
				return cfg.ResolveOpenAICompatibleEndpointForClass(class)
			}
			return effectiveEndpoint(provider, cfg)
		},
	}
}
