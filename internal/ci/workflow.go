// Package ci renders a project's GitHub Actions verification workflow from its
// configured verification commands (T008). It is language-independent: SOP
// hard-codes no target-language commands.
package ci

import (
	"errors"
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sdlc/internal/testrunner"
)

// Render produces a deterministic GitHub Actions workflow that runs the
// project's configured verification commands, in order, on every push and pull
// request.
func Render(commands testrunner.Commands) (string, error) {
	checks := commands.Checks()
	if len(checks) == 0 {
		return "", errors.New("ci: no verification commands configured")
	}
	for _, check := range checks {
		if strings.ContainsAny(check.Command, "\n\r") {
			return "", fmt.Errorf("ci: command for %s must not contain a newline", check.Category)
		}
	}

	var b strings.Builder
	b.WriteString("name: CI\n\n")
	b.WriteString("on:\n  push:\n  pull_request:\n\n")
	b.WriteString("jobs:\n  verify:\n    runs-on: ubuntu-latest\n    steps:\n")
	b.WriteString("      - uses: actions/checkout@v4\n")
	for _, check := range checks {
		fmt.Fprintf(&b, "      - name: %s\n        run: %s\n", stepName(check.Category), check.Command)
	}
	return b.String(), nil
}

func stepName(category testrunner.Category) string {
	return strings.ToLower(string(category))
}
