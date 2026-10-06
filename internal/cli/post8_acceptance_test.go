package cli

import "testing"

const post8CfgOff = `project:
  name: x
validation:
  build:
    - "true"
  test:
    - "true"
`

const post8CfgOn = `project:
  name: x
validation:
  build:
    - "true"
  test:
    - "true"
context:
  decision_memory: true
`

// TestPost8AcceptanceGatePreservesLifecycle is the POST8-001 end-to-end acceptance for
// the live-path gate: with context.decision_memory off (the default) and on, sop run
// reaches the same terminal lifecycle stage. The feature changes context only, never
// lifecycle.
func TestPost8AcceptanceGatePreservesLifecycle(t *testing.T) {
	run := func(cfg string) string {
		dir := t.TempDir()
		writeFile(t, dir, "TASK.md", runTaskFile)
		writeConfig(t, dir, cfg)
		if code, _, se := runCLI(t, dir, "memory", "add", "--decision", "Postgres is canonical storage", "--reason", "single source of truth", "--scope", "architecture"); code != exitOK {
			t.Fatalf("memory add: %s", se)
		}
		a := &memoryAgent{plan: validPlanJSON}
		if code, stdout, se := runInjectedCLI(t, dir, evalDiff, a, "run", "--task", "TASK.md"); code != exitOK {
			t.Fatalf("run: %s (stdout=%s)", se, stdout)
		}
		return loadEvalTrace(t, dir, "T001").Termination.Stage
	}

	off := run(post8CfgOff)
	on := run(post8CfgOn)
	t.Logf("terminal stage: off=%q on=%q", off, on)
	if off == "" {
		t.Fatal("the run reached no terminal stage")
	}
	if off != on {
		t.Errorf("the terminal lifecycle stage must not depend on the gate: off=%q on=%q", off, on)
	}
}
