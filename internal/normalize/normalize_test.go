package normalize

import "testing"

// fence wraps body in a backtick code fence with an optional language token.
func fence(lang, body string) string {
	f := "```"
	nl := string(rune(10))
	return f + lang + nl + body + nl + f
}

// tildeFence wraps body in a tilde code fence.
func tildeFence(lang, body string) string {
	f := "~~~"
	nl := string(rune(10))
	return f + lang + nl + body + nl + f
}

// TestResponseClassification covers every normalized kind deterministically.
func TestResponseClassification(t *testing.T) {
	obj := `{"a":1}`
	cases := []struct {
		name     string
		in       string
		kind     Kind
		content  string
		wrappers int
	}{
		{"empty", "   " + string(rune(10)) + string(rune(9)), KindEmpty, "", 0},
		{"text", "hello world", KindText, "hello world", 0},
		{"json object", obj, KindJSON, obj, 0},
		{"json array", "[1,2,3]", KindJSON, "[1,2,3]", 0},
		{"trimmed json", "  " + obj + "  ", KindJSON, obj, 0},
		{"fenced json", fence("json", obj), KindJSON, obj, 1},
		{"fenced bare", fence("", "[1]"), KindJSON, "[1]", 1},
		{"tilde fenced", tildeFence("json", obj), KindJSON, obj, 1},
		{"fenced text", fence("", "just text"), KindText, "just text", 1},
		{"empty fence", "```" + string(rune(10)) + "```", KindEmpty, "", 1},
		{"unclosed fence", "```json" + string(rune(10)) + obj, KindText, "```json" + string(rune(10)) + obj, 0},
	}
	for _, tc := range cases {
		got := Response(tc.in)
		if got.Kind != tc.kind || got.Content != tc.content {
			t.Errorf("%s: Response = {%s,%q}, want {%s,%q}", tc.name, got.Kind, got.Content, tc.kind, tc.content)
		}
		if got.Raw != tc.in {
			t.Errorf("%s: raw = %q, want %q", tc.name, got.Raw, tc.in)
		}
		if len(got.Wrappers) != tc.wrappers {
			t.Errorf("%s: wrappers = %v, want %d", tc.name, got.Wrappers, tc.wrappers)
		}
	}
}

// TestMalformedFailsClosed proves structured output that does not parse is never repaired.
func TestMalformedFailsClosed(t *testing.T) {
	for _, in := range []string{"{", "[1,", `{"a":`} {
		r := Response(in)
		if r.Kind != KindMalformed || r.Err == nil {
			t.Errorf("Response(%q) = %+v, want malformed with error", in, r)
		}
		if r.Raw != in {
			t.Errorf("Response(%q) did not preserve raw", in)
		}
	}
}

// TestAmbiguousFenceIsText proves prose around a fence is not guessed at.
func TestAmbiguousFenceIsText(t *testing.T) {
	obj := `{"a":1}`
	in := "Here is the JSON:" + string(rune(10)) + fence("json", obj) + string(rune(10)) + "That is all."
	r := Response(in)
	if r.Kind != KindText || len(r.Wrappers) != 0 {
		t.Errorf("ambiguous fence = %+v, want text with no wrappers", r)
	}
	if data, err := JSON(in); err == nil {
		t.Errorf("ambiguous content must not extract JSON: %q", data)
	}
}

// TestJSONExtraction proves the JSON helper unwraps a fence but never repairs.
func TestJSONExtraction(t *testing.T) {
	obj := `{"a":1}`
	if data, err := JSON(fence("json", obj)); err != nil || string(data) != obj {
		t.Errorf("JSON(fenced) = %q,%v want %q", data, err, obj)
	}
	for _, in := range []string{"", "not json", `{"a":`} {
		if _, err := JSON(in); err == nil {
			t.Errorf("JSON(%q) must fail", in)
		}
	}
}

// TestResponseDeterministic proves repeated normalization is identical.
func TestResponseDeterministic(t *testing.T) {
	in := fence("json", `{"a":1}`)
	a, b := Response(in), Response(in)
	if a.Kind != b.Kind || a.Content != b.Content || len(a.Wrappers) != len(b.Wrappers) {
		t.Errorf("nondeterministic: %+v vs %+v", a, b)
	}
}
