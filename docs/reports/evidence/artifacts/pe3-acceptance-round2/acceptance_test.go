package pe3probe

import (
	"os"
	"strings"
	"testing"
)

// TestAcceptedMarker is the operator-owned acceptance check: accepted.txt must contain
// exactly the planner's decision marker.
func TestAcceptedMarker(t *testing.T) {
	data, err := os.ReadFile("accepted.txt")
	if err != nil {
		t.Fatalf("read accepted.txt: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "accepted" {
		t.Fatalf("accepted.txt = %q, want %q", got, "accepted")
	}
}
