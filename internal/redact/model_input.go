package redact

// ModelInput removes known secret environment values and credential-shaped text
// from content that is about to become model-visible input. It is the same
// composition used for diagnostics, kept in the shared package so every adapter
// applies the same vocabulary and marker.
//
// It is deliberately bounded: it does not attempt general DLP and cannot
// recognise arbitrary confidential prose or unknown credential formats.
func ModelInput(content string, env []string) string {
	if content == "" {
		return content
	}
	content = Values(content, EnvSecrets(env), Fixed("[redacted]"))
	return ShapesValues(content, Fixed("[redacted]"))
}
