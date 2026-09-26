package taskrunner

import (
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/testrunner"
)

// maxOutput bounds the diagnostic text carried into agent requests and attempt
// records so raw logs are not persisted without limit.
const maxOutput = 4000

func truncate(s string) string {
	if len(s) <= maxOutput {
		return s
	}
	return s[:maxOutput] + "\n...[truncated]"
}

// failureEvidence is a bounded representation of a verification failure.
type failureEvidence struct {
	category string
	command  string
	exitCode int
	stdout   string
	stderr   string
}

func (e failureEvidence) String() string {
	parts := []string{
		"category: " + e.category,
		"command: " + e.command,
		fmt.Sprintf("exit code: %d", e.exitCode),
	}
	if e.stdout != "" {
		parts = append(parts, "stdout:\n"+e.stdout)
	}
	if e.stderr != "" {
		parts = append(parts, "stderr:\n"+e.stderr)
	}
	return strings.Join(parts, "\n")
}

func evidenceFrom(check testrunner.Check, result testrunner.Result) failureEvidence {
	return failureEvidence{
		category: string(check.Category),
		command:  check.Command,
		exitCode: result.ExitCode,
		stdout:   truncate(result.Stdout),
		stderr:   truncate(result.Stderr),
	}
}

func evidenceFromSuite(suite testrunner.SuiteResult) failureEvidence {
	for _, result := range suite.Results {
		if result.Status != testrunner.Pass {
			return failureEvidence{
				category: string(result.Category),
				command:  result.Command,
				exitCode: result.ExitCode,
				stdout:   truncate(result.Stdout),
				stderr:   truncate(result.Stderr),
			}
		}
	}
	return failureEvidence{}
}

func suiteSummary(suite testrunner.SuiteResult) string {
	var b strings.Builder
	for _, result := range suite.Results {
		fmt.Fprintf(&b, "%s %s %s\n", result.Category, result.Status, result.Command)
	}
	return truncate(b.String())
}

func taskContext(task *domain.Task) string {
	return fmt.Sprintf("Task %s: %s\nObjective: %s\nAcceptance criteria:\n%s",
		task.ID, task.Title, task.Objective, task.AcceptanceCriteria)
}

func joinContext(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "\n\n")
}

func designTestsRequest(task *domain.Task, contextText string) agent.Request {
	return agent.Request{
		Capability:         agent.DesignTests,
		Task:               fmt.Sprintf("Design tests for %s: %s", task.ID, task.Title),
		Input:              joinContext(taskContext(task), contextText),
		OutputRequirements: "Return the test cases and the files to create or change, in a form the configured work applier can apply.",
	}
}

func implementRequest(task *domain.Task, testDesign, contextText string) agent.Request {
	return agent.Request{
		Capability:         agent.Implement,
		Task:               fmt.Sprintf("Implement %s: %s", task.ID, task.Title),
		Input:              joinContext(taskContext(task), "Test design:\n"+testDesign, contextText),
		OutputRequirements: "Return the implementation changes, in a form the configured work applier can apply.",
	}
}

func diagnoseRequest(task *domain.Task, evidence failureEvidence, contextText string) agent.Request {
	return agent.Request{
		Capability:         agent.DiagnoseFailure,
		Task:               fmt.Sprintf("Diagnose verification failure for %s: %s", task.ID, task.Title),
		Input:              joinContext(taskContext(task), "Verification failure:\n"+evidence.String(), contextText),
		OutputRequirements: "Return the likely cause and the recommended corrective action.",
	}
}

func fixRequest(task *domain.Task, diagnosis string, evidence failureEvidence, contextText string) agent.Request {
	return agent.Request{
		Capability:         agent.Fix,
		Task:               fmt.Sprintf("Fix verification failure for %s: %s", task.ID, task.Title),
		Input:              joinContext(taskContext(task), "Diagnosis:\n"+diagnosis, "Failure evidence:\n"+evidence.String(), contextText),
		OutputRequirements: "Return the corrective changes, in a form the configured work applier can apply.",
	}
}
