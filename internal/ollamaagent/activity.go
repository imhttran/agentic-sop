package ollamaagent

import (
	"context"
	"strings"

	"github.com/imhttran/agentic-sop/internal/activity"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// maxActivityDetail bounds a tool summary so activity stays one concise line.
const maxActivityDetail = 120

// recordToolActivity reports one executed tool call on the activity stream when a
// consumer is registered. It reports a short summary — a path, a search pattern,
// or a command — never raw arguments, file contents, model output, or environment
// values, and it never affects execution.
func recordToolActivity(ctx context.Context, name string, args map[string]any) {
	rec := activity.FromContext(ctx)
	if !rec.Enabled() {
		return
	}
	stage, action, detail := toolActivity(name, args)
	if stage == "" {
		return
	}
	rec.Emit(stage, action, detail)
}

// recordFinalizeActivity reports that a mutating capability has entered its
// tool-free finalization phase, so a long tail of denial turns is visible as a
// phase rather than noise.
func recordFinalizeActivity(ctx context.Context) {
	rec := activity.FromContext(ctx)
	if !rec.Enabled() {
		return
	}
	rec.Emit(activity.StageFinalize, "preparing outcome", "")
}

// toolActivity maps a tool call to its activity stage, verb, and detail. Reads and
// inspections are discovery, writes are changes, and commands are validation; the
// mapping is by tool type so the detail is always a meaningful summary rather than
// raw arguments.
func toolActivity(name string, args map[string]any) (stage, action, detail string) {
	path := argString(args, "path")
	switch name {
	case toolharness.ToolReadFile:
		return activity.StageDiscover, "reading", path
	case toolharness.ToolListFiles:
		return activity.StageDiscover, "listing", path
	case toolharness.ToolSearchFiles:
		return activity.StageDiscover, "searching", firstNonEmpty(argString(args, "pattern"), path)
	case toolharness.ToolGitStatus:
		return activity.StageDiscover, "inspecting", "git status"
	case toolharness.ToolGitDiff:
		return activity.StageDiscover, "inspecting", "git diff"
	case toolharness.ToolWriteFile:
		return activity.StageChange, "editing", path
	case toolharness.ToolCreateFile:
		return activity.StageChange, "creating", path
	case toolharness.ToolDeleteFile:
		return activity.StageChange, "deleting", path
	case toolharness.ToolRestoreFile:
		return activity.StageChange, "restoring", path
	case toolharness.ToolRunCommand:
		return activity.StageValidate, redactCommand(argString(args, "command")), ""
	default:
		return "", "", ""
	}
}

// argString returns a trimmed string tool argument, or "" when absent.
func argString(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// sensitiveKeys are substrings that mark a KEY=VALUE assignment as carrying a
// secret, so its value is masked before it can reach the activity stream.
var sensitiveKeys = []string{
	"TOKEN", "SECRET", "PASSWORD", "PASSWD", "APIKEY", "API_KEY",
	"ACCESS_KEY", "PRIVATE_KEY", "CREDENTIAL", "AUTH", "SESSION",
}

// redactCommand renders a command for activity: a single bounded line with the
// value of any obviously sensitive KEY=VALUE assignment masked, so a command can
// never leak a token through the stream. It is a display-only summary.
func redactCommand(command string) string {
	command = oneLine(command)
	if command == "" {
		return ""
	}
	parts := strings.Fields(command)
	for i, part := range parts {
		if eq := strings.IndexByte(part, '='); eq > 0 {
			key := strings.ToUpper(part[:eq])
			if containsSensitiveKey(key) {
				parts[i] = part[:eq+1] + "***"
			}
		}
	}
	return truncateActivity(strings.Join(parts, " "), maxActivityDetail)
}

func containsSensitiveKey(key string) bool {
	// Normalize separators so --api-key, api.key, and API_KEY all match.
	key = strings.NewReplacer("-", "_", ".", "_").Replace(key)
	for _, s := range sensitiveKeys {
		if strings.Contains(key, s) {
			return true
		}
	}
	return false
}

// oneLine collapses s to its first non-empty trimmed line.
func oneLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}

// truncateActivity bounds s to max runes, appending an ellipsis when it is cut.
func truncateActivity(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimRight(string(r[:max]), " ") + "…"
}
