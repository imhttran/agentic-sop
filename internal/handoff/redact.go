package handoff

import "strings"

// secretMarkers identify environment variable names whose values are secrets and
// must never reach a compressor.
var secretMarkers = []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "CREDENTIAL", "AUTH"}

// Redact replaces occurrences of secret environment values in content. Only
// values of variables whose names look sensitive are redacted, and very short
// values are left alone to avoid destroying unrelated text.
func Redact(content string, env []string) string {
	for _, kv := range env {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || len(value) < 4 || !sensitiveName(name) {
			continue
		}
		content = strings.ReplaceAll(content, value, "[redacted]")
	}
	return content
}

// RedactAll applies Redact to every artifact's content. It returns the input
// unchanged when there is no environment to redact.
func RedactAll(artifacts []Artifact, env []string) []Artifact {
	if len(env) == 0 {
		return artifacts
	}
	out := make([]Artifact, len(artifacts))
	for i, artifact := range artifacts {
		artifact.Content = Redact(artifact.Content, env)
		out[i] = artifact
	}
	return out
}

func redactCapsule(capsule Capsule, env []string) Capsule {
	if len(env) == 0 {
		return capsule
	}
	capsule.Summary = Redact(capsule.Summary, env)
	redactList(capsule.Changes, env)
	redactList(capsule.Decisions, env)
	redactList(capsule.Files, env)
	redactList(capsule.CarryForward, env)
	for i := range capsule.Verification {
		capsule.Verification[i].Check = Redact(capsule.Verification[i].Check, env)
		capsule.Verification[i].Status = Redact(capsule.Verification[i].Status, env)
	}
	return capsule
}

func redactList(items []string, env []string) {
	for i := range items {
		items[i] = Redact(items[i], env)
	}
}

func sensitiveName(name string) bool {
	upper := strings.ToUpper(strings.TrimSpace(name))
	for _, marker := range secretMarkers {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}
