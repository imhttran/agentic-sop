package continuation

import (
	"testing"

	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

func TestContinuationLimitSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	const max = 3

	first, err := runpkg.Open(dir, "SOP-004")
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		n, ok, err := Consume(first, max)
		if err != nil || !ok || n != i {
			t.Fatalf("consume %d = (%d, %v, %v), want (%d, true, nil)", i, n, ok, err, i)
		}
	}

	// A new process reopens the same run: the spent count is still there.
	second, err := runpkg.New(dir, "SOP-004")
	if err != nil {
		t.Fatal(err)
	}
	if n, ok, err := Consume(second, max); err != nil || !ok || n != 3 {
		t.Fatalf("after restart = (%d, %v, %v), want (3, true, nil)", n, ok, err)
	}

	third, err := runpkg.Open(dir, "SOP-004")
	if err != nil {
		t.Fatal(err)
	}
	if n, ok, err := Consume(third, max); err != nil || ok || n != 3 {
		t.Fatalf("over the limit = (%d, %v, %v), want (3, false, nil)", n, ok, err)
	}
}
