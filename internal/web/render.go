package web

import (
	"html/template"
	"io"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"sop-controller/internal/sopclient"
)

// Views renders the server-side HTML templates.
type Views struct {
	t *template.Template
}

// NewViews parses templates/*.html and templates/partials/*.html from fsys.
func NewViews(fsys fs.FS) (*Views, error) {
	funcs := template.FuncMap{
		"statusClass":            statusClass,
		"severityClass":          severityClass,
		"stageClass":             stageClass,
		"dispositionClass":       dispositionClass,
		"categoryClass":          categoryClass,
		"categoryLabel":          categoryLabel,
		"classificationCategory": classificationCategory,
		"levelClass":             levelClass,
		"activeTask":             activeTask,
		"since":                  since,
		"evidenceState":          evidenceState,
		"unavailable":            unavailable,
		"absentLiteral":          absentLiteral,
		"hasRetries":             hasRetries,
		"retryCount":             retryCount,
		"blocked":                blocked,
		"hasProviderModel":       hasProviderModel,
		"hasModelSelection":      hasModelSelection,
		"startedAt":              startedAt,
		"elapsed":                elapsed,
	}
	t, err := template.New("root").Funcs(funcs).ParseFS(fsys, "templates/*.html", "templates/partials/*.html")
	if err != nil {
		return nil, err
	}
	return &Views{t: t}, nil
}

// Render executes the named template.
func (v *Views) Render(w io.Writer, name string, data any) error {
	return v.t.ExecuteTemplate(w, name, data)
}

// statusClass maps SOP's task status to a badge style.
func statusClass(status string) string {
	switch sopclient.TaskState(status) {
	case "DONE":
		return "s-completed"
	case "BLOCKED":
		return "s-blocked"
	case "FAILED":
		return "s-failed"
	case "READY":
		return "s-ready"
	case "PLANNED":
		return "s-planned"
	default:
		return "s-running"
	}
}

// stageClass maps SOP's run stage to a badge style.
func stageClass(stage string) string {
	switch strings.ToUpper(stage) {
	case sopclient.StagePassed, "PASS":
		return "s-completed"
	case sopclient.StageFailed, "FAIL":
		return "s-failed"
	case sopclient.StageWaitingForHuman:
		return "s-blocked"
	case "":
		return "s-planned"
	default:
		return "s-running"
	}
}

// dispositionClass distinguishes SOP self-recovery (AUTO_FIX/CONTINUE/RETRY)
// from a human boundary.
func dispositionClass(disposition string) string {
	switch disposition {
	case sopclient.DispositionAutoFix:
		return "d-autofix"
	case sopclient.DispositionContinue:
		return "d-continue"
	case sopclient.DispositionRetry:
		return "d-retry"
	case sopclient.DispositionReplan:
		return "d-replan"
	case sopclient.DispositionNeedsHuman:
		return "d-human"
	default:
		return "s-planned"
	}
}

// classificationCategory delegates to sopclient (CTRL009).
func classificationCategory(c *sopclient.Classification) sopclient.Category {
	return c.Category()
}

// categoryLabel delegates to sopclient so the vocabulary cannot drift.
func categoryLabel(c sopclient.Category) string {
	return c.Label()
}

// categoryClass styles categories; budget never uses the human style, unknown
// is neutral.
func categoryClass(c sopclient.Category) string {
	switch c {
	case sopclient.CategoryProvider:
		return "c-provider"
	case sopclient.CategoryDeterministic:
		return "c-deterministic"
	case sopclient.CategoryBudget:
		return "c-budget"
	case sopclient.CategoryHuman:
		return "c-human"
	default:
		return "c-unknown"
	}
}

// levelClass: only an explicit PASS reads as complete.
func levelClass(level string) string {
	switch strings.ToUpper(level) {
	case "PASS":
		return "s-completed"
	case "FAIL", "ERROR":
		return "s-failed"
	default:
		return "s-planned"
	}
}

// notRunLabel is the CTRL008 wording for absent data; never a success word.
const notRunLabel = "not run"

// unavailable renders notRunLabel for a field SOP did not persist.
func unavailable() string { return notRunLabel }

// absentLiteral exposes sopclient.StatusUnknown to templates.
func absentLiteral() string { return sopclient.StatusUnknown }

// evidenceState returns the value, or notRunLabel when SOP persisted none.
func evidenceState(present bool, value string) string {
	if !present || strings.TrimSpace(value) == "" {
		return notRunLabel
	}
	return value
}

// activeTask wraps ProjectDetail.ActiveTask (templates can't take two returns).
func activeTask(p sopclient.ProjectDetail) string {
	id, _ := p.ActiveTask()
	return id
}

// hasRetries wraps TaskSummary.Retries (templates can't take two returns).
func hasRetries(t sopclient.TaskDetail) bool {
	_, ok := t.Retries()
	return ok
}

// retryCount is attempt-1, or 0; pair with hasRetries.
func retryCount(t sopclient.TaskDetail) int {
	n, _ := t.Retries()
	return n
}

// blocked: an explicit blocked status or incomplete dependencies.
func blocked(t sopclient.TaskDetail) bool {
	return t.Status == sopclient.StatusBlocked || len(t.BlockedBy) > 0
}

// hasProviderModel reports whether either half of provider/model is present.
func hasProviderModel(r sopclient.RunInfo) bool {
	return strings.TrimSpace(r.Provider) != "" || strings.TrimSpace(r.Model) != ""
}

// hasModelSelection reports whether SOP persisted a model-selection record
// (report.json "model_selection"). The controller displays what SOP selected;
// it never selects a model itself (docs/specs/AGENT-PROVIDER.md).
func hasModelSelection(r sopclient.RunInfo) bool {
	return r.ModelSelection.Present
}

// startedAt is the earliest attempt timestamp, or zero (CTRL008).
func startedAt(t sopclient.TaskDetail) time.Time {
	var earliest time.Time
	for _, a := range t.Attempts {
		if a.Timestamp.IsZero() {
			continue
		}
		if earliest.IsZero() || a.Timestamp.Before(earliest) {
			earliest = a.Timestamp
		}
	}
	return earliest
}

// elapsed is the time since startedAt, or notRunLabel.
func elapsed(t sopclient.TaskDetail) string {
	start := startedAt(t)
	if start.IsZero() {
		return notRunLabel
	}
	d := time.Since(start)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "< 1m"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + "d"
	}
}

func severityClass(sev string) string {
	switch strings.ToLower(sev) {
	case "critical":
		return "s-failed"
	case "high":
		return "s-blocked"
	case "medium":
		return "s-waiting"
	case "low":
		return "s-ready"
	default:
		return "s-planned"
	}
}

func since(t time.Time) string {
	if t.IsZero() {
		return "\u2014"
	}
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m ago"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h ago"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + "d ago"
	}
}
