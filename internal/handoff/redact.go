package handoff

import "github.com/imhttran/agentic-sop/internal/redact"

// Redact replaces occurrences of secret environment values in content. Which
// variables count as secret is decided by internal/redact, so a handoff capsule and
// a command diagnostic agree on what a credential is; very short values are left
// alone to avoid destroying unrelated text.
func Redact(content string, env []string) string {
	return redact.Values(content, redact.EnvSecrets(env), redact.Fixed("[redacted]"))
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
