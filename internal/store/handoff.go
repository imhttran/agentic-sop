package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/imhttran/agentic-sop/internal/handoff"
)

// SaveHandoff upserts a handoff record keyed by task id, so regenerating a
// handoff after a restart replaces the record instead of creating a duplicate.
func (s *Store) SaveHandoff(record handoff.Record) error {
	capsuleJSON, err := json.Marshal(record.Capsule)
	if err != nil {
		return fmt.Errorf("marshal capsule: %w", err)
	}
	referencesJSON, err := json.Marshal(record.References)
	if err != nil {
		return fmt.Errorf("marshal references: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO handoffs
		(task_id, capsule_json, status, content, references_json, compression_error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			capsule_json = excluded.capsule_json,
			status = excluded.status,
			content = excluded.content,
			references_json = excluded.references_json,
			compression_error = excluded.compression_error,
			created_at = excluded.created_at
	`, record.TaskID, string(capsuleJSON), string(record.Status), record.Content,
		string(referencesJSON), record.CompressionError, record.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

// GetHandoff returns the handoff record for a task, or ErrNotFound.
func (s *Store) GetHandoff(taskID string) (handoff.Record, error) {
	row := s.db.QueryRow("SELECT "+handoffColumns+" FROM handoffs WHERE task_id = ?", taskID)
	record, err := scanHandoff(row)
	if errors.Is(err, sql.ErrNoRows) {
		return handoff.Record{}, ErrNotFound
	}
	return record, err
}

// Handoffs returns every stored handoff ordered by task id.
func (s *Store) Handoffs() ([]handoff.Record, error) {
	rows, err := s.db.Query("SELECT " + handoffColumns + " FROM handoffs ORDER BY task_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []handoff.Record
	for rows.Next() {
		record, err := scanHandoff(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

const handoffColumns = `task_id, capsule_json, status, content, references_json, compression_error, created_at`

func scanHandoff(row rowScanner) (handoff.Record, error) {
	var (
		record                      handoff.Record
		capsuleJSON, referencesJSON string
		status, content, compErr    string
		createdAt                   string
	)
	if err := row.Scan(&record.TaskID, &capsuleJSON, &status, &content, &referencesJSON, &compErr, &createdAt); err != nil {
		return handoff.Record{}, err
	}

	if err := json.Unmarshal([]byte(capsuleJSON), &record.Capsule); err != nil {
		return handoff.Record{}, fmt.Errorf("unmarshal capsule: %w", err)
	}
	if err := json.Unmarshal([]byte(referencesJSON), &record.References); err != nil {
		return handoff.Record{}, fmt.Errorf("unmarshal references: %w", err)
	}
	record.Status = handoff.Status(status)
	record.Content = content
	record.CompressionError = compErr

	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return handoff.Record{}, fmt.Errorf("parse handoff created_at: %w", err)
	}
	record.CreatedAt = created
	return record, nil
}
