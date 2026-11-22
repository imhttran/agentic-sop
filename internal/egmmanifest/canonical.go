package egmmanifest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Canonical returns the canonical JSON bytes for v.
//
// Rules (implementation proposal, not operator-approved governance):
//   - UTF-8 JSON, compact, no insignificant whitespace.
//   - Object keys are sorted lexicographically; array order is preserved.
//   - HTML characters are not escaped.
//   - Numbers are preserved as their JSON literals (json.Number), so an integer is
//     emitted as an integer, not a float.
//   - null and [] are distinct; callers should use empty slices rather than nil when the
//     distinction does not matter.
//
// Two values that encode to the same JSON object produce identical bytes. A meaningful
// content change produces different bytes with overwhelming probability.
func Canonical(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var generic any
	if err := dec.Decode(&generic); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(generic); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Digest returns the lowercase hex SHA-256 of Canonical(v).
func Digest(v any) (string, error) {
	b, err := Canonical(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// CaseDigest returns the digest of a case with its sealed_sha256 field cleared, so the
// digest covers the record's content and not its own hash.
func CaseDigest(c Case) (string, error) {
	view := c
	view.SealedSHA256 = ""
	return Digest(view)
}

// Seal sets c.SealedSHA256 to CaseDigest(c) and returns it.
func Seal(c *Case) (string, error) {
	d, err := CaseDigest(*c)
	if err != nil {
		return "", err
	}
	c.SealedSHA256 = d
	return d, nil
}
