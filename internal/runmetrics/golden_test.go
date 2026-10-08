package runmetrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestGoldenOutput compares each fixture's aggregate JSON against a committed
// golden file. The golden test fails if any metric value, denominator, or
// coverage percentage drifts.
func TestGoldenOutput(t *testing.T) {
	for _, name := range []string{"present", "absent", "zero"} {
		agg, err := AggregateProject(filepath.Join("testdata", name))
		if err != nil {
			t.Fatalf("aggregate %s: %v", name, err)
		}
		got, err := json.MarshalIndent(agg, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, '\n')

		goldenPath := filepath.Join("testdata", "golden", name+".json")
		if os.Getenv("UPDATE_GOLDEN") != "" {
			if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}

		want, err := os.ReadFile(goldenPath)
		if err != nil {
			t.Fatalf("read golden %s: %v", goldenPath, err)
		}
		if string(got) != string(want) {
			t.Errorf("golden mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
		}
	}
}
