package config

import (
	"strings"
	"testing"
)

// TestTemplateCarriesTheGoValidationPolicy: the generated configuration must define
// the four required Go checks (build, test, vet, and a gofmt formatting check) so a
// Go implementation task has deterministic validation from the start. It must also
// parse cleanly, which proves the embedded shell-formatted lint command is valid YAML.
func TestTemplateCarriesTheGoValidationPolicy(t *testing.T) {
	tpl := Template("demo")

	for _, want := range []string{"go build ./...", "go test ./...", "go vet ./...", "gofmt -l ."} {
		if !strings.Contains(tpl, want) {
			t.Errorf("template is missing the required Go check %q", want)
		}
	}

	c, err := Parse([]byte(tpl))
	if err != nil {
		t.Fatalf("template does not parse: %v", err)
	}
	if len(c.Validation.Build) == 0 || len(c.Validation.Test) == 0 || len(c.Validation.Lint) == 0 {
		t.Fatalf("template validation must define build, test, and lint: %+v", c.Validation)
	}
	found := false
	for _, cmd := range c.Validation.Lint {
		if strings.Contains(cmd, "gofmt") {
			found = true
		}
	}
	if !found {
		t.Errorf("template lint must include a gofmt formatting check: %+v", c.Validation.Lint)
	}
}
