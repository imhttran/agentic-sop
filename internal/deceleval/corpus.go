// Package deceleval loads and validates the decision-model effectiveness corpus:
// independently labeled failure scenarios used to compare SOP's deterministic
// classification (baseline A) with the advisory decision layer (experimental B).
//
// It is tooling only. It defines no policy, calls no model, and never reads SOP
// state. Expected labels are authored from observable failure properties; the
// deterministic classifier, CLEF, and Nimble are never treated as ground truth.
package deceleval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Action is the expected governed action for a scenario. The set mirrors the
// decision evidence action vocabulary (internal/failure dispositions plus the
// advisory final action).
type Action string

const (
	AutoFix    Action = "AUTO_FIX"
	Continue   Action = "CONTINUE"
	Retry      Action = "RETRY"
	Replan     Action = "REPLAN"
	NeedsHuman Action = "NEEDS_HUMAN"
	Block      Action = "BLOCK"
)

// Categories are the fixed scenario families the corpus covers.
var categories = map[string]bool{
	"benign_impl":         true,
	"test_regression":     true,
	"build_dependency":    true,
	"missing_validation":  true,
	"security_sensitive":  true,
	"governance_boundary": true,
	"ambiguous":           true,
	"provider_fault":      true,
}

// Expected is the independently authored label: the governed action a scenario
// should produce and the rationale justifying it.
type Expected struct {
	Action    Action `json:"action"`
	Rationale string `json:"rationale"`
}

// Scenario is one labeled failure scenario.
type Scenario struct {
	ID string `json:"id"`
	// Category is one of categories.
	Category string `json:"category"`
	// Source is "synthetic" or "real:<run-id>" (a recorded failure artifact).
	Source string `json:"source"`
	// SeedKind is the recorded deterministic kind when the source is real. It is
	// reference only, never ground truth.
	SeedKind string `json:"seed_kind,omitempty"`
	// Observable is the failure's observable properties the label was authored from.
	Observable string   `json:"observable"`
	Expected   Expected `json:"expected"`
	// Split is "eval" or "tuning".
	Split string `json:"split"`
}

// Corpus is the frozen, labeled scenario set.
type Corpus struct {
	Version     int        `json:"version"`
	Description string     `json:"description"`
	Actions     []string   `json:"actions"`
	Scenarios   []Scenario `json:"scenarios"`
}

// Load reads and strictly decodes the corpus. Unknown keys are rejected so a
// mislabeled field is an explicit error rather than a silently ignored one.
func Load(path string) (*Corpus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	var c Corpus
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("deceleval: decode corpus: %w", err)
	}
	return &c, nil
}

// Validate reports the first corpus contract violation.
func (c Corpus) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("deceleval: unsupported corpus version %d", c.Version)
	}
	allowed := map[Action]bool{}
	for _, a := range c.Actions {
		allowed[Action(a)] = true
	}
	if len(allowed) == 0 {
		return fmt.Errorf("deceleval: corpus declares no actions")
	}
	if len(c.Scenarios) == 0 {
		return fmt.Errorf("deceleval: corpus has no scenarios")
	}
	seen := map[string]bool{}
	for i, s := range c.Scenarios {
		if strings.TrimSpace(s.ID) == "" {
			return fmt.Errorf("deceleval: scenario %d has no id", i)
		}
		if seen[s.ID] {
			return fmt.Errorf("deceleval: duplicate scenario id %q", s.ID)
		}
		seen[s.ID] = true
		if !categories[s.Category] {
			return fmt.Errorf("deceleval: scenario %q: unknown category %q", s.ID, s.Category)
		}
		if !allowed[s.Expected.Action] {
			return fmt.Errorf("deceleval: scenario %q: unknown expected action %q", s.ID, s.Expected.Action)
		}
		if strings.TrimSpace(s.Expected.Rationale) == "" {
			return fmt.Errorf("deceleval: scenario %q: expected rationale is required", s.ID)
		}
		if strings.TrimSpace(s.Observable) == "" {
			return fmt.Errorf("deceleval: scenario %q: observable is required", s.ID)
		}
		if s.Split != "eval" && s.Split != "tuning" {
			return fmt.Errorf("deceleval: scenario %q: split must be eval or tuning", s.ID)
		}
		if s.Source != "synthetic" && !strings.HasPrefix(s.Source, "real:") {
			return fmt.Errorf("deceleval: scenario %q: source must be synthetic or real:<run-id>", s.ID)
		}
	}
	return nil
}

// FreezeEntry is the recorded freeze: the sha256 of the corpus file bytes.
type FreezeEntry struct {
	SHA256 string
	Path   string
}

// FreezeOf returns the sha256 of a file's bytes.
func FreezeOf(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// LoadFreeze parses a `<sha256>  <path>` freeze file.
func LoadFreeze(path string) (FreezeEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return FreezeEntry{}, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return FreezeEntry{}, fmt.Errorf("deceleval: empty freeze file %s", path)
	}
	e := FreezeEntry{SHA256: fields[0]}
	if len(fields) > 1 {
		e.Path = fields[1]
	}
	return e, nil
}

// VerifyFreeze checks that the corpus file's current bytes match the frozen hash,
// so labels cannot change after they were frozen.
func VerifyFreeze(corpusPath, freezePath string) error {
	want, err := LoadFreeze(freezePath)
	if err != nil {
		return err
	}
	got, err := FreezeOf(corpusPath)
	if err != nil {
		return err
	}
	if got != want.SHA256 {
		return fmt.Errorf("deceleval: corpus freeze mismatch: file is %s, frozen %s", got, want.SHA256)
	}
	return nil
}

// Counts reports per-category, per-split, and per-source counts (exact
// denominators for reporting).
func (c Corpus) Counts() (byCategory, bySplit map[string]int, real, synthetic int) {
	byCategory = map[string]int{}
	bySplit = map[string]int{}
	for _, s := range c.Scenarios {
		byCategory[s.Category]++
		bySplit[s.Split]++
		if s.Source == "synthetic" {
			synthetic++
		} else {
			real++
		}
	}
	return byCategory, bySplit, real, synthetic
}
