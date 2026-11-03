package sopclient

import (
	"errors"
	"fmt"
	"io/fs"
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

// runFile reads one run artifact. found is false when it does not exist, so a
// missing file stays distinct from an unreadable one.
func (s *Store) runFile(taskID, name string) (raw []byte, found bool, err error) {
	raw, err = os.ReadFile(filepath.Join(s.root, ".agent-sdlc", "runs", taskID, name))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("%s: %w", name, err)
	}
	return raw, true, nil
}

// readValidation decodes SOP's validation artifact. A missing artifact is no checks
// and no error; a corrupt one is an error, never "validation never ran".
func (s *Store) readValidation(taskID string) ([]ValidationCheck, error) {
	var v validationJSON
	found, err := readJSON(filepath.Join(s.runDir(taskID), "validation.json"), &v)
	if err != nil || !found {
		return nil, err
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
	return out, nil
}

// readReview decodes SOP's review artifact, with the same absence/damage split as
// readValidation.
func (s *Store) readReview(taskID string) (Review, error) {
	var r reviewJSON
	found, err := readJSON(filepath.Join(s.runDir(taskID), "review.json"), &r)
	if err != nil || !found {
		return Review{}, err
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
	return rev, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
