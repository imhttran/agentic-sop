package sopclient

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Runs artifacts written by SOP live under <root>/.agent-sdlc/runs/<task>/.
// They carry validation (CI) and review output that isn't in the task tables.

type validationJSON struct {
	Status  string `json:"Status"`
	Results []struct {
		Category string `json:"Category"`
		Command  string `json:"Command"`
		ExitCode int    `json:"ExitCode"`
		Stdout   string `json:"Stdout"`
		Stderr   string `json:"Stderr"`
		Status   string `json:"Status"`
	} `json:"Results"`
}

type reviewJSON struct {
	Summary  string `json:"Summary"`
	Findings []struct {
		Severity string `json:"Severity"`
		Title    string `json:"Title"`
		Message  string `json:"Message"`
		Detail   string `json:"Detail"`
		Location string `json:"Location"`
		File     string `json:"File"`
		Line     int    `json:"Line"`
	} `json:"Findings"`
}

func (s *Store) runFile(taskID, name string) ([]byte, bool) {
	raw, err := os.ReadFile(filepath.Join(s.root, ".agent-sdlc", "runs", taskID, name))
	if err != nil {
		return nil, false
	}
	return raw, true
}

func (s *Store) readValidation(taskID string) []ValidationCheck {
	raw, ok := s.runFile(taskID, "validation.json")
	if !ok {
		return nil
	}
	var v validationJSON
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	out := make([]ValidationCheck, 0, len(v.Results))
	for _, r := range v.Results {
		out = append(out, ValidationCheck{
			Category: r.Category,
			Command:  r.Command,
			Status:   r.Status,
			ExitCode: r.ExitCode,
			Stderr:   r.Stderr,
		})
	}
	return out
}

func (s *Store) readReview(taskID string) Review {
	raw, ok := s.runFile(taskID, "review.json")
	if !ok {
		return Review{}
	}
	var r reviewJSON
	if err := json.Unmarshal(raw, &r); err != nil {
		return Review{}
	}
	rev := Review{Summary: r.Summary}
	for _, f := range r.Findings {
		rev.Findings = append(rev.Findings, ReviewFinding{
			Severity: f.Severity,
			Title:    firstNonEmpty(f.Title, f.Message),
			Detail:   f.Detail,
			Location: firstNonEmpty(f.Location, f.File),
		})
	}
	return rev
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
