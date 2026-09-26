package quality

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/review"
)

// policy builds a config.Quality with an explicit test requirement.
func policy(requireTests bool, maxCycles int, failOn ...string) config.Quality {
	return config.Quality{RequireTests: &requireTests, MaxFixCycles: maxCycles, FailOn: failOn}
}

func TestEvaluate(t *testing.T) {
	cases := []struct {
		name       string
		policy     config.Quality
		in         Input
		want       Decision
		wantReason string
	}{
		{
			name:       "all required checks pass",
			policy:     policy(true, 3, "critical", "high"),
			in:         Input{BuildPassed: true, TestPassed: true},
			want:       Pass,
			wantReason: "all required checks passed",
		},
		{
			name:       "build failure fails",
			policy:     policy(true, 3, "critical"),
			in:         Input{BuildPassed: false, TestPassed: true},
			want:       Fail,
			wantReason: "build failed",
		},
		{
			name:       "test failure fails when required",
			policy:     policy(true, 3, "critical"),
			in:         Input{BuildPassed: true, TestPassed: false},
			want:       Fail,
			wantReason: "tests",
		},
		{
			name:       "test failure ignored when not required",
			policy:     policy(false, 3, "critical"),
			in:         Input{BuildPassed: true, TestPassed: false},
			want:       Pass,
			wantReason: "",
		},
		{
			name:       "lint failure fails when required",
			policy:     policy(true, 3, "critical"),
			in:         Input{BuildPassed: true, TestPassed: true, LintRequired: true, LintPassed: false},
			want:       Fail,
			wantReason: "lint failed",
		},
		{
			name:       "lint failure ignored when not required",
			policy:     policy(true, 3, "critical"),
			in:         Input{BuildPassed: true, TestPassed: true, LintRequired: false, LintPassed: false},
			want:       Pass,
			wantReason: "",
		},
		{
			name:       "blocking finding fails",
			policy:     policy(true, 3, "critical", "high"),
			in:         Input{BuildPassed: true, TestPassed: true, Unresolved: []review.Finding{{Severity: review.High}}},
			want:       Fail,
			wantReason: "unresolved finding",
		},
		{
			name:       "finding below threshold passes",
			policy:     policy(true, 3, "critical", "high"),
			in:         Input{BuildPassed: true, TestPassed: true, Unresolved: []review.Finding{{Severity: review.Medium}}},
			want:       Pass,
			wantReason: "",
		},
		{
			name:       "empty fail_on blocks nothing",
			policy:     policy(true, 3),
			in:         Input{BuildPassed: true, TestPassed: true, Unresolved: []review.Finding{{Severity: review.Critical}}},
			want:       Pass,
			wantReason: "",
		},
		{
			name:       "human required wins over a pass",
			policy:     policy(true, 3),
			in:         Input{BuildPassed: true, TestPassed: true, HumanRequired: true},
			want:       NeedsHuman,
			wantReason: "human",
		},
		{
			name:       "exhausted budget with a blocking finding needs a human",
			policy:     policy(true, 3, "critical"),
			in:         Input{BuildPassed: true, TestPassed: true, FixCycles: 3, Unresolved: []review.Finding{{Severity: review.Critical}}},
			want:       NeedsHuman,
			wantReason: "fix-loop limit",
		},
		{
			name:       "exhausted budget with a failing check needs a human",
			policy:     policy(true, 3, "critical"),
			in:         Input{BuildPassed: false, TestPassed: true, FixCycles: 3},
			want:       NeedsHuman,
			wantReason: "fix-loop limit",
		},
		{
			name:       "exhausted budget with a clean pass still passes",
			policy:     policy(true, 3, "critical"),
			in:         Input{BuildPassed: true, TestPassed: true, FixCycles: 3},
			want:       Pass,
			wantReason: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(tc.policy, tc.in)
			if got.Decision != tc.want {
				t.Errorf("Decision = %s, want %s (reasons: %v)", got.Decision, tc.want, got.Reasons)
			}
			if tc.wantReason != "" {
				if !strings.Contains(strings.Join(got.Reasons, "; "), tc.wantReason) {
					t.Errorf("reasons = %v, want one containing %q", got.Reasons, tc.wantReason)
				}
			}
			if len(got.Reasons) == 0 {
				t.Error("Result must always carry a reason")
			}
		})
	}
}
