package notify

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/redact"
)

// TestNewEventAppliesC1PolicyToDescriptiveText checks that an enabled C1 policy
// removes an inventory value and a contextual credential assignment from
// notification text, while the zero policy leaves the same text as before and the
// event id does not depend on the policy.
func TestNewEventAppliesC1PolicyToDescriptiveText(t *testing.T) {
	const inventoryValue = "opaque-inventory-value-77"
	base := Params{
		Kind:       KindFailed,
		Project:    "p",
		Attempt:    1,
		Transition: "FAILED",
		Reason:     "step saw " + inventoryValue,
		NextAction: "retry with password=hunter2hunter2",
	}
	control, err := NewEvent(base)
	if err != nil {
		t.Fatalf("control: %v", err)
	}
	if !strings.Contains(control.Reason, inventoryValue) {
		t.Fatalf("zero policy must not remove an inventory value: %q", control.Reason)
	}

	withPolicy := base
	withPolicy.Policy = redact.Policy{Enabled: true, Values: []string{inventoryValue}}
	e, err := NewEvent(withPolicy)
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	if strings.Contains(e.Reason, inventoryValue) {
		t.Fatalf("C1 inventory value reached the notification: %q", e.Reason)
	}
	if strings.Contains(e.NextAction, "hunter2hunter2") {
		t.Fatalf("C1 contextual credential reached the notification: %q", e.NextAction)
	}
	if e.ID != control.ID {
		t.Fatalf("policy changed the event id: %s != %s", e.ID, control.ID)
	}
}
