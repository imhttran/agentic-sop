package criteria

import "testing"

// ParseBindingSpecs is the typed operator source; it authorizes bindings and is
// the path HARDEN-001-c uses to load bindings from operator configuration.
func TestParseBindingSpecsAuthorizes(t *testing.T) {
	bs, err := ParseBindingSpecs([]BindingSpec{{ID: "App Starts", Command: "true"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 1 || !bs[0].Authorized() || bs[0].CriterionID != "app starts" || bs[0].Digest() == "" {
		t.Fatalf("binding = %+v", bs)
	}
}

func TestParseBindingSpecsRejects(t *testing.T) {
	for _, specs := range [][]BindingSpec{
		{},
		{{ID: "c", Command: ""}},
		{{ID: "c", Command: "true"}, {ID: "c", Command: "false"}},
		{{ID: "c", Command: "rm -rf /"}},
	} {
		if _, err := ParseBindingSpecs(specs); err == nil {
			t.Errorf("expected rejection of specs %+v", specs)
		}
	}
}

func TestParseBindingSpecsUsesSharedCommandPolicy(t *testing.T) {
	if _, err := ParseBindingSpecs([]BindingSpec{{ID: "c", Command: "cat ../secret"}}); err == nil {
		t.Error("workspace-escaping command must be rejected via the shared policy")
	}
	if _, err := ParseBindingSpecs([]BindingSpec{{ID: "c", Command: "echo hi; rm -rf /"}}); err == nil {
		t.Error("shell metacharacters must be rejected via the shared policy")
	}
}
