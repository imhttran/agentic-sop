package deepseekagent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/commandpolicy"
)

// errCommandNotAllowed is the single rejection error for the command policy.
var errCommandNotAllowed = errors.New("command not allowed")

// runCommand executes an allow-listed command. It never uses a shell: the command
// string is tokenized here and passed as an argument vector, and both the
// characters and the program/subcommand are checked in code.
func (t *toolbox) runCommand(ctx context.Context, args map[string]any) (string, error) {
	command, _ := args["command"].(string)
	command = strings.TrimSpace(command)
	if command == "" {
		return "", errors.New(`missing required argument "command"`)
	}
	argv, err := splitCommand(command)
	if err != nil {
		return "", err
	}
	if err := t.allowCommand(argv); err != nil {
		return "", err
	}
	return t.exec(ctx, argv)
}

// splitCommand tokenizes a command string, rejecting shell metacharacters and
// globs so nothing can be reinterpreted by a shell. It is deliberately small: no
// variable expansion, no redirection, no command substitution.
func splitCommand(s string) ([]string, error) {
	if strings.ContainsAny(s, ";&|<>$`\n\r*?[]{}()~") {
		return nil, fmt.Errorf("%w: shell metacharacters or globs are not allowed in %q", errCommandNotAllowed, s)
	}

	var argv []string
	var cur strings.Builder
	var quote rune
	flush := func() {
		if cur.Len() > 0 {
			argv = append(argv, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("%w: unterminated quote", errCommandNotAllowed)
	}
	flush()
	if len(argv) == 0 {
		return nil, errors.New("command is empty")
	}
	return argv, nil
}

// allowCommand applies the command policy: no repository-location overrides, no
// absolute paths outside the repository, and only commands SOP classifies as
// SAFE (plus gofmt, which only rewrites Go source).
func (t *toolbox) allowCommand(argv []string) error {
	if len(argv) == 0 {
		return errors.New("command is empty")
	}
	if hasDirOverride(argv) {
		return fmt.Errorf("%w: -C/--git-dir/--work-tree overrides are not allowed", errCommandNotAllowed)
	}
	for _, a := range argv {
		if filepath.IsAbs(a) {
			if _, err := t.resolve(a, false); err != nil {
				return fmt.Errorf("%w: %v", errCommandNotAllowed, err)
			}
		}
	}

	if filepath.Base(argv[0]) == "gofmt" {
		return nil
	}
	class, err := commandpolicy.Classify(argv)
	if err != nil {
		return err
	}
	if class != commandpolicy.Safe {
		return fmt.Errorf("%w: %q is %s", errCommandNotAllowed, strings.Join(argv, " "), class)
	}
	return nil
}

// hasDirOverride reports whether argv would make a command operate outside the
// repository root.
func hasDirOverride(argv []string) bool {
	for _, a := range argv {
		switch a {
		case "-C", "--git-dir", "--work-tree", "--exec-path", "--namespace":
			return true
		}
		if strings.HasPrefix(a, "--git-dir=") || strings.HasPrefix(a, "--work-tree=") ||
			strings.HasPrefix(a, "--exec-path=") || strings.HasPrefix(a, "--namespace=") {
			return true
		}
	}
	return false
}

// exec runs argv in the repository root with a bounded timeout, capturing stdout
// and stderr. A non-zero exit is reported in the result (the model should see
// failing tests); only a timeout is a tool error.
func (t *toolbox) exec(ctx context.Context, argv []string) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, t.cfg.CommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	cmd.Dir = t.root
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	out := buf.String()
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return truncate(out, t.cfg.MaxOutputBytes), fmt.Errorf("command timed out after %s", t.cfg.CommandTimeout)
	}
	return fmt.Sprintf("%s\n%s", exitStatus(err), truncate(out, t.cfg.MaxOutputBytes)), nil
}

// exitStatus renders a command's exit state for the model.
func exitStatus(err error) string {
	if err == nil {
		return "exit 0"
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return fmt.Sprintf("exit %d", ee.ExitCode())
	}
	return fmt.Sprintf("error: %v", err)
}
