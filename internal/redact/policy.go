package redact

import (
	"regexp"
	"strings"
)

// Policy selects the bounded C1 model-input redaction rules. The zero value is
// exactly the previous ModelInput behavior: known secret-named environment
// values plus credential shapes. An enabled policy adds the explicit inventory
// values, contextual credential-key detection, and the approved known formats.
//
// The policy is deliberately not general DLP. It does not claim to detect
// arbitrary confidential prose, unknown formats, encoded values, or values that
// are not listed in the inventory and have no credential-like key or known
// format.
type Policy struct {
	// Enabled turns on the C1 additions. It defaults to false so an omitted
	// configuration preserves existing behavior.
	Enabled bool
	// Values are exact sensitive values loaded from the operator's external
	// inventory. They are never logged or persisted by this package.
	Values []string
}

// ModelInputWithPolicy is ModelInput plus the optional C1 rules. When policy is
// disabled it is byte-for-byte the previous composition. When enabled, exact
// inventory values are added to the existing environment-value redaction, and
// the bounded contextual and known-format rules run after the existing shapes.
// The marker is always [redacted], matching the model-input boundary.
func ModelInputWithPolicy(content string, env []string, policy Policy) string {
	if content == "" {
		return content
	}
	secrets := EnvSecrets(env)
	if policy.Enabled {
		secrets = append(secrets, policy.Values...)
	}
	content = Values(content, secrets, Fixed("[redacted]"))
	if !policy.Enabled {
		content = ShapesValues(content, Fixed("[redacted]"))
		return content
	}
	// The C1 path uses a precise assignment rule: the default detector's broad
	// suffix also redacts ordinary words such as "secretary", which the approved
	// false-positive corpus requires to survive. The default ModelInput path is
	// untouched.
	content = renderWithRules(content, Fixed("[redacted]"), true, modelInputRules())
	content = contextualValues(content, Fixed("[redacted]"))
	content = knownFormats(content, Fixed("[redacted]"))
	return content
}

// preciseAssignmentRule is the assignment rule used only when the C1 policy is
// enabled. It requires a separator after the credential word (or a compound
// key such as api_key/secret_key), so "secretary=..." is not a credential while
// api_key, token, password, secret_key, access_key_id, and authorization remain
// covered.
var preciseAssignmentRule = rule{
	re:    regexp.MustCompile("(?i)\\b(api[\\s_-]?key|access[_-]?key|private[_-]?key|secret(?:[\\s_-]?key)?|token|password|passwd|credential|authorization)(?:[\\s_-]+[a-z0-9_-]*)?[\"']?\\s*[:=]\\s*[\"']?([^\\s\"',;}\\[\\]`]+)"),
	kind:  KindAssignment,
	value: 2,
}

func modelInputRules() []rule {
	out := make([]rule, len(rules))
	copy(out, rules)
	out[0] = preciseAssignmentRule
	return out
}

// credentialKeySegments are the key-name segments that make an assignment value
// credential-bearing. "key" is included for api_key/access_key/private_key, but
// public/public-key names are excluded below.
var credentialKeySegments = map[string]bool{
	"secret":        true,
	"secrets":       true,
	"token":         true,
	"tokens":        true,
	"password":      true,
	"passwd":        true,
	"credential":    true,
	"credentials":   true,
	"auth":          true,
	"authorization": true,
	"apikey":        true,
	"accesskey":     true,
	"privatekey":    true,
	"clientsecret":  true,
	"secretkey":     true,
	"key":           true,
}

// publicKeySegments suppress redaction for public key material, which is not a
// credential. A name with one of these segments is left unchanged.
var publicKeySegments = map[string]bool{
	"public":    true,
	"pub":       true,
	"pubkey":    true,
	"publickey": true,
}

// credentialKey reports whether key names a credential-bearing assignment. The
// key is split on non-alphanumeric separators, so opaque_secret, api_key,
// auth_token, and client_secret match, while secretary, keyboard, monkey, and
// public_key do not.
func credentialKey(key string) bool {
	lower := strings.ToLower(strings.Trim(strings.TrimSpace(key), "\"'"))
	if lower == "" {
		return false
	}
	segments := strings.FieldsFunc(lower, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	hasPublic := false
	hasCredential := false
	for _, segment := range segments {
		if publicKeySegments[segment] {
			hasPublic = true
		}
		if credentialKeySegments[segment] {
			hasCredential = true
		}
	}
	if hasPublic {
		return false
	}
	return hasCredential
}

// contextualValues applies the bounded key-based rule line by line. It preserves
// the key and the separator and replaces only the value. It handles shell
// assignments, JSON key/value pairs, and diff lines (the leading + or - is
// outside the key match and is preserved).
func contextualValues(text string, m Marker) string {
	if text == "" {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = contextualLine(line, m)
	}
	return strings.Join(lines, "\n")
}

func contextualLine(line string, m Marker) string {
	if line == "" {
		return line
	}
	var b strings.Builder
	last := 0
	i := 0
	for i < len(line) {
		sep := nextAssignmentSeparator(line, i)
		if sep < 0 {
			break
		}
		keyEnd := sep
		for keyEnd > i && (line[keyEnd-1] == ' ' || line[keyEnd-1] == '\t') {
			keyEnd--
		}
		keyStart := keyEnd
		for keyStart > i && isKeyByte(line[keyStart-1]) {
			keyStart--
		}
		key := strings.Trim(line[keyStart:keyEnd], " \t\"'+-")
		if !credentialKey(key) {
			i = sep + 1
			continue
		}

		valueStart := sep + 1
		for valueStart < len(line) && (line[valueStart] == ' ' || line[valueStart] == '\t') {
			valueStart++
		}
		if valueStart >= len(line) {
			break
		}
		if line[valueStart] == '"' || line[valueStart] == '\'' {
			quote := line[valueStart]
			close := valueStart + 1
			for close < len(line) {
				if line[close] == '\\' && close+1 < len(line) {
					close += 2
					continue
				}
				if line[close] == quote {
					break
				}
				close++
			}
			if close >= len(line) {
				i = sep + 1
				continue
			}
			value := line[valueStart+1 : close]
			if value == "" || value == "[redacted]" || isPlaceholderValue(value) {
				i = close + 1
				continue
			}
			b.WriteString(line[last : valueStart+1])
			b.WriteString(m(KindAssignment))
			last = close
			i = close + 1
			continue
		}

		end := valueStart
		for end < len(line) && !isValueDelimiter(line[end]) {
			end++
		}
		if end == valueStart {
			i = sep + 1
			continue
		}
		value := line[valueStart:end]
		if strings.HasPrefix(value, "[redacted") || isSchemeWord(value) || isPlaceholderValue(value) {
			i = end
			continue
		}
		b.WriteString(line[last:valueStart])
		b.WriteString(m(KindAssignment))
		last = end
		i = end
	}
	if last == 0 {
		return line
	}
	b.WriteString(line[last:])
	return b.String()
}

func nextAssignmentSeparator(line string, from int) int {
	for i := from; i < len(line); i++ {
		if line[i] == '=' || line[i] == ':' {
			return i
		}
	}
	return -1
}

func isKeyByte(b byte) bool {
	return b >= 'a' && b <= 'z' ||
		b >= 'A' && b <= 'Z' ||
		b >= '0' && b <= '9' ||
		b == '_' || b == '-' || b == '.' || b == '+' || b == '"' || b == '\''
}

// isPlaceholderValue exempts the obvious template placeholders used by
// .env.example and .env.sample. It is deliberately narrow: a real value is not
// likely to be "replace-me", "changeme", or a <...> placeholder, and inventory
// exact values are still redacted before this rule runs.
func isPlaceholderValue(value string) bool {
	v := strings.ToLower(strings.Trim(value, "\"'"))
	switch v {
	case "", "replace-me", "replace_me", "replaceme", "change-me", "changeme",
		"example", "placeholder", "your-key-here", "your_api_key",
		"your-api-key", "redacted", "[redacted]":
		return true
	}
	return strings.HasPrefix(v, "your-") || strings.HasPrefix(v, "your_") ||
		strings.HasPrefix(v, "xxx") || strings.HasPrefix(v, "<")
}

func isValueDelimiter(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n', '"', '\'', ',', ';', '}', ']', ')', '>', '|', '&':
		return true
	}
	return false
}

// knownFormatRules are the approved known formats that the existing shape
// vocabulary does not cover. They are gated by Policy.Enabled and are deliberately
// narrow: each requires a provider-specific prefix and a minimum token length.
var knownFormatRules = []*regexp.Regexp{
	regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}\b`),
	regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`),
	regexp.MustCompile(`\bpypi-[A-Za-z0-9_-]{16,}\b`),
	regexp.MustCompile(`\bshpat_[A-Za-z0-9]{32,}\b`),
	regexp.MustCompile(`\bghs_[A-Za-z0-9]{20,}\b`),
	regexp.MustCompile(`\bghu_[A-Za-z0-9]{20,}\b`),
	regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`),
	regexp.MustCompile(`\bSG\.[A-Za-z0-9_-]{16,}\.[A-Za-z0-9_-]{16,}\b`),
	regexp.MustCompile(`\b(?:sk|rk|pk)_live_[A-Za-z0-9]{16,}\b`),
}

func knownFormats(text string, m Marker) string {
	if text == "" {
		return text
	}
	marker := m(KindToken)
	for _, rule := range knownFormatRules {
		text = rule.ReplaceAllString(text, marker)
	}
	return text
}
