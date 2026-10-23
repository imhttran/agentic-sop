// Package hipcompare implements the HIP-003 opt-in comparison harness: a small,
// controlled A/B experiment that runs one task manifest twice (baseline and
// candidate instruction sets) from the same starting commit in separate
// disposable worktrees, then records the two arms as machine-readable results
// and a short Markdown comparison.
//
// It is deliberately OFF the default SOP execution path: nothing in the run,
// scheduler, or orchestration code calls it. It reuses existing SOP facilities
// (Git worktrees, the budget limits, the verification and acceptance commands)
// and builds no second orchestrator, no service, and no database.
package hipcompare

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FingerprintVersion identifies the fingerprint derivation. A change to the
// derivation must bump it so a stored fingerprint from an older harness can never
// be mistaken for a match.
const FingerprintVersion = "hip-003.fingerprint.v1"

// SchemaVersion is the manifest and instruction-set schema version.
const SchemaVersion = "hip-003.v1"

// TaskManifest is the task the comparison replays. It is the only description of
// the work both arms run.
type TaskManifest struct {
	// SchemaVersion must be SchemaVersion.
	SchemaVersion string `json:"schema_version"`
	// ID names the task (for example "HIP-003-PAIR-1"). It is required.
	ID string `json:"id"`
	// Title is a short human label. Optional.
	Title string `json:"title,omitempty"`
	// TaskFile is the repository-relative path of the SOP task file both arms run.
	TaskFile string `json:"task_file"`
	// TaskInputs are the caller-supplied observation strings handed to the agent.
	// They are part of the fingerprint, so both arms see identical text.
	TaskInputs map[string]string `json:"task_inputs,omitempty"`
	// VerificationCommands are the trusted acceptance checks both arms must run.
	// They must be non-empty: a comparison without a fixed check proves nothing.
	VerificationCommands []string `json:"verification_commands"`
	// AllowedCandidatePaths is the declared prompt or skill file list. The candidate
	// arm may edit only these paths; any other edit aborts the comparison. It is
	// required and must be non-empty.
	AllowedCandidatePaths []string `json:"allowed_candidate_paths"`
}

// InstructionSet is one arm's versioned instruction set. The baseline declares no
// edits; the candidate's files must be inside the manifest's allowed list.
type InstructionSet struct {
	// SchemaVersion must be SchemaVersion.
	SchemaVersion string `json:"schema_version"`
	// Name is a stable label for the set (for example "baseline" or "candidate").
	Name string `json:"name"`
	// Instructions is the versioned prompt or skill text under test.
	Instructions string `json:"instructions"`
	// Files records the repository-relative prompt or skill files this set carries.
	Files []InstructionFile `json:"files"`
}

// InstructionFile is one prompt or skill file in an instruction set.
type InstructionFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// LoadTaskManifest reads and validates a task manifest from path.
func LoadTaskManifest(path string) (*TaskManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read task manifest: %w", err)
	}
	var m TaskManifest
	if err := decodeStrict(data, &m); err != nil {
		return nil, fmt.Errorf("parse task manifest %s: %w", path, err)
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("task manifest %s: %w", path, err)
	}
	return &m, nil
}

// LoadInstructionSet reads and validates an instruction set from path.
func LoadInstructionSet(path string) (*InstructionSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read instruction set: %w", err)
	}
	var s InstructionSet
	if err := decodeStrict(data, &s); err != nil {
		return nil, fmt.Errorf("parse instruction set %s: %w", path, err)
	}
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("instruction set %s: %w", path, err)
	}
	return &s, nil
}

// decodeStrict decodes JSON, rejecting unknown fields so a typo is an error
// instead of a silently ignored setting.
func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}

// Validate checks a task manifest. Every required field must be present and no
// path may escape the repository.
func (m *TaskManifest) Validate() error {
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version %q is not %q", m.SchemaVersion, SchemaVersion)
	}
	if strings.TrimSpace(m.ID) == "" {
		return errors.New("id is required")
	}
	if strings.TrimSpace(m.TaskFile) == "" {
		return errors.New("task_file is required")
	}
	if err := checkRelPath("task_file", m.TaskFile); err != nil {
		return err
	}
	if len(m.VerificationCommands) == 0 {
		return errors.New("verification_commands is required and must not be empty")
	}
	for i, c := range m.VerificationCommands {
		if strings.TrimSpace(c) == "" {
			return fmt.Errorf("verification_commands[%d] is empty", i)
		}
	}
	if len(m.AllowedCandidatePaths) == 0 {
		return errors.New("allowed_candidate_paths is required: the candidate may edit only declared prompt or skill files")
	}
	for i, p := range m.AllowedCandidatePaths {
		if err := checkRelPath(fmt.Sprintf("allowed_candidate_paths[%d]", i), p); err != nil {
			return err
		}
	}
	for k := range m.TaskInputs {
		if strings.TrimSpace(k) == "" {
			return errors.New("task_inputs has an empty key")
		}
	}
	return nil
}

// Validate checks an instruction set against its own schema. The candidate's
// files are checked against the manifest by ValidateFor.
func (s *InstructionSet) Validate() error {
	if s.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version %q is not %q", s.SchemaVersion, SchemaVersion)
	}
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(s.Instructions) == "" {
		return errors.New("instructions is required")
	}
	seen := map[string]bool{}
	for i, f := range s.Files {
		if err := checkRelPath(fmt.Sprintf("files[%d].path", i), f.Path); err != nil {
			return err
		}
		if seen[f.Path] {
			return fmt.Errorf("files[%d].path %q is duplicated", i, f.Path)
		}
		seen[f.Path] = true
	}
	return nil
}

// ValidateFor checks that a candidate instruction set declares only files in the
// manifest's allowed list.
func (s *InstructionSet) ValidateFor(m *TaskManifest) error {
	if err := s.Validate(); err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, p := range m.AllowedCandidatePaths {
		allowed[filepath.Clean(p)] = true
	}
	for _, f := range s.Files {
		if !allowed[filepath.Clean(f.Path)] {
			return fmt.Errorf("candidate instruction file %q is not in allowed_candidate_paths", f.Path)
		}
	}
	return nil
}

// checkRelPath rejects an absolute path and any path that escapes the repository.
func checkRelPath(field, p string) error {
	if strings.TrimSpace(p) == "" {
		return fmt.Errorf("%s is empty", field)
	}
	clean := filepath.Clean(p)
	if filepath.IsAbs(p) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s %q escapes the repository", field, p)
	}
	return nil
}

// FingerprintInputs are the deterministic inputs the comparison identity is
// derived from. Any change to a listed component changes the fingerprint, so a
// resume can never silently mix incomparable runs.
type FingerprintInputs struct {
	// TaskInputs is a canonical rendering of the manifest and its inputs.
	TaskInputs string
	// StartingCommit is the Git commit both arms start from.
	StartingCommit string
	// InstructionHashes are the per-set content hashes, keyed by set name.
	InstructionHashes map[string]string
	// Executor, Model, Settings describe the executor and its settings, identical
	// for both arms.
	Executor string
	Model    string
	Settings string
	// ToolPermissions is a canonical rendering of the tool permissions both arms
	// receive.
	ToolPermissions string
	// VerificationCommands are the trusted checks both arms run.
	VerificationCommands []string
}

// Fingerprint is the derived, deterministic comparison identity.
type Fingerprint struct {
	Version   string            `json:"version"`
	Digest    string            `json:"digest"`
	Parts     map[string]string `json:"parts"`
	Baseline  string            `json:"baseline_instruction_hash"`
	Candidate string            `json:"candidate_instruction_hash"`
}

// DeriveFingerprint computes the comparison fingerprint. It is deterministic:
// identical inputs produce an identical digest, and every named component is
// included in its own part and in the digest.
func DeriveFingerprint(in FingerprintInputs) Fingerprint {
	parts := map[string]string{
		"task_inputs":           hashString(in.TaskInputs),
		"starting_commit":       hashString(in.StartingCommit),
		"executor":              hashString(in.Executor),
		"model":                 hashString(in.Model),
		"settings":              hashString(in.Settings),
		"tool_permissions":      hashString(in.ToolPermissions),
		"verification_commands": hashString(strings.Join(in.VerificationCommands, "\x00")),
	}
	for name, h := range in.InstructionHashes {
		parts["instructions:"+name] = h
	}
	keys := make([]string, 0, len(parts))
	for k := range parts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(FingerprintVersion)
	for _, k := range keys {
		b.WriteString("\n")
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(parts[k])
	}
	sum := sha256.Sum256([]byte(b.String()))
	return Fingerprint{
		Version:   FingerprintVersion,
		Digest:    hex.EncodeToString(sum[:]),
		Parts:     parts,
		Baseline:  in.InstructionHashes["baseline"],
		Candidate: in.InstructionHashes["candidate"],
	}
}

// InstructionHash is the content hash of one instruction set: the name, then each
// declared file path and content, in sorted path order.
func InstructionHash(s *InstructionSet) string {
	files := make([]InstructionFile, len(s.Files))
	copy(files, s.Files)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	var b strings.Builder
	b.WriteString(s.Name)
	b.WriteString("\n")
	b.WriteString(s.Instructions)
	for _, f := range files {
		b.WriteString("\n--file--\n")
		b.WriteString(f.Path)
		b.WriteString("\n")
		b.WriteString(f.Content)
	}
	return hashString(b.String())
}

func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
