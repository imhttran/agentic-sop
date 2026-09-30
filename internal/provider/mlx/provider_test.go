package mlx_test

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/provider"
	"github.com/imhttran/agentic-sop/internal/provider/mlx"
)

func TestMLXIdentity(t *testing.T) {
	p := mlx.New(mlx.DefaultBaseURL, 0)
	if p.ID() != provider.MLX {
		t.Fatalf("ID = %q", p.ID())
	}
	if mlx.EnvBaseURL != "SOP_MLX_BASE_URL" {
		t.Fatalf("EnvBaseURL = %q", mlx.EnvBaseURL)
	}
}
