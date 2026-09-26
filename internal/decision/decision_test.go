package decision

import (
	"context"
	"testing"
)

func TestDeterministicProvider(t *testing.T) {
	p := DeterministicProvider{}
	cases := []struct {
		name string
		req  Request
		want Choice
	}{
		{"small", Request{Subject: "fix a typo"}, Low},
		{"moderate", Request{Signals: map[string]float64{"criteria": 3}}, Medium},
		{"risky keyword", Request{Subject: "database migration"}, High},
		{"large", Request{Signals: map[string]float64{"files": 5, "criteria": 1}}, High},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := p.Decide(context.Background(), tc.req)
			if err != nil {
				t.Fatalf("Decide failed: %v", err)
			}
			if got.Choice != tc.want {
				t.Errorf("Choice = %s, want %s", got.Choice, tc.want)
			}
			if got.Confidence <= 0 || got.Confidence > 1 {
				t.Errorf("Confidence = %v, want (0,1]", got.Confidence)
			}
		})
	}
}

func TestNewProvider(t *testing.T) {
	for _, name := range []string{"", "deterministic"} {
		p, err := NewProvider(name)
		if err != nil {
			t.Fatalf("NewProvider(%q) failed: %v", name, err)
		}
		if p.Name() != "deterministic" {
			t.Errorf("Name = %q", p.Name())
		}
	}
	if _, err := NewProvider("jev"); err == nil {
		t.Error("expected an error for the unimplemented jev provider")
	}
}

func TestRoute(t *testing.T) {
	thresholds := Thresholds{RouteToStrongModel: 0.7, RequireHuman: 0.4}
	cases := []struct {
		name string
		d    Decision
		want Target
	}{
		{"high choice needs a human", Decision{Choice: High, Confidence: 0.9}, HumanTarget},
		{"human choice", Decision{Choice: Human, Confidence: 0.5}, HumanTarget},
		{"low confidence needs a human", Decision{Choice: Low, Confidence: 0.3}, HumanTarget},
		{"medium choice uses the strong model", Decision{Choice: Medium, Confidence: 0.8}, StrongModel},
		{"medium confidence uses the strong model", Decision{Choice: Low, Confidence: 0.6}, StrongModel},
		{"confident low uses the small model", Decision{Choice: Low, Confidence: 0.95}, SmallModel},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Route(thresholds, tc.d); got != tc.want {
				t.Errorf("Route = %s, want %s", got, tc.want)
			}
		})
	}
}
