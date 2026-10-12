package perf

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestRecorderInputKeepsLargestAndCounts proves the input record keeps the largest
// assembled sizes and counts each assembly.
func TestRecorderInputKeepsLargestAndCounts(t *testing.T) {
	r := NewRecorder("T1")
	r.InputAssembled(1000, 500)
	r.InputAssembled(800, 700)

	task := r.Task()
	if task.Input == nil {
		t.Fatal("Input = nil, want a record after two assemblies")
	}
	if task.Input.Assemblies != 2 {
		t.Errorf("Assemblies = %d, want 2", task.Input.Assemblies)
	}
	if task.Input.ContextBytes != 1000 {
		t.Errorf("ContextBytes = %d, want the largest 1000", task.Input.ContextBytes)
	}
	if task.Input.PromptBytes != 700 {
		t.Errorf("PromptBytes = %d, want the largest 700", task.Input.PromptBytes)
	}
}

// TestRecorderInputOmittedWhenNothingAssembled proves a task with no assembly carries
// no input record, so an existing report is unchanged.
func TestRecorderInputOmittedWhenNothingAssembled(t *testing.T) {
	task := NewRecorder("T2").Task()
	if task.Input != nil {
		t.Errorf("Input = %+v, want nil when nothing was assembled", task.Input)
	}
	if task.Measured() {
		t.Error("Measured() = true for an untouched recorder, want false")
	}
}

// TestWriteInputRendersBytes proves the input record renders its sizes, and renders
// nothing when absent.
func TestWriteInputRendersBytes(t *testing.T) {
	var buf bytes.Buffer
	WriteInput(&buf, &Input{Assemblies: 3, ContextBytes: 4096, PromptBytes: 8192, OutputBytes: 1024})
	out := buf.String()
	for _, want := range []string{"Input", "Assemblies: 3", "4096 bytes", "8192 bytes", "1024 bytes"} {
		if !strings.Contains(out, want) {
			t.Errorf("WriteInput output missing %q:\n%s", want, out)
		}
	}

	var empty bytes.Buffer
	WriteInput(&empty, nil)
	if empty.Len() != 0 {
		t.Errorf("WriteInput(nil) wrote %q, want nothing", empty.String())
	}
}

// TestRecorderOutputKeepsLargest proves the model output size keeps the largest value.
func TestRecorderOutputKeepsLargest(t *testing.T) {
	r := NewRecorder("T3")
	r.OutputProduced(120)
	r.OutputProduced(90)
	task := r.Task()
	if task.Input == nil || task.Input.OutputBytes != 120 {
		t.Fatalf("Input = %+v, want OutputBytes 120", task.Input)
	}
}

// TestRunAggregatesInput proves the run aggregate sums assemblies and keeps the
// largest context/prompt size, and stays nil when no task assembled input.
func TestRunAggregatesInput(t *testing.T) {
	run := NewRun(time.Now())
	run.Add(Task{ID: "a", Input: &Input{Assemblies: 1, ContextBytes: 1000, PromptBytes: 2000, OutputBytes: 300}})
	run.Add(Task{ID: "b", Input: &Input{Assemblies: 2, ContextBytes: 500, PromptBytes: 5000, OutputBytes: 100}})

	if run.Input == nil {
		t.Fatal("run Input = nil, want an aggregate")
	}
	if run.Input.Assemblies != 3 {
		t.Errorf("Assemblies = %d, want 3", run.Input.Assemblies)
	}
	if run.Input.ContextBytes != 1000 {
		t.Errorf("ContextBytes = %d, want the largest 1000", run.Input.ContextBytes)
	}
	if run.Input.PromptBytes != 5000 {
		t.Errorf("PromptBytes = %d, want the largest 5000", run.Input.PromptBytes)
	}
	if run.Input.OutputBytes != 300 {
		t.Errorf("OutputBytes = %d, want the largest 300", run.Input.OutputBytes)
	}

	empty := NewRun(time.Now())
	empty.Add(Task{ID: "c"})
	if empty.Input != nil {
		t.Errorf("run Input = %+v, want nil when no task assembled input", empty.Input)
	}
}
