package notify

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// FileCapabilities is the delivery guarantee of the local file channel. The
// channel honours the idempotency key: it records an event id at most once, so a
// retry of one event never appends a second record.
func FileCapabilities() Capabilities {
	return Capabilities{
		KeyHonored: true,
		Guarantee:  "duplicate-free: at most one durable append per event id",
	}
}

// FileChannel delivers events to a local file: one JSON-encoded event per line.
// The channel confirms delivery only after the append is durable (the file is
// synced), so a Confirmed result means the receiver content is parseable as the
// event payload.
//
// The channel honours the idempotency key: Send records an event id at most
// once. Before appending, Send scans the file for an existing record with the
// same event id; when one is present it returns AckConfirmed without appending
// again, so a retry of one event never creates a second record.
type FileChannel struct {
	// Path is the local file the channel appends to. A missing parent directory
	// is created on Send.
	Path string
}

// NewFileChannel returns a file channel that appends to path.
func NewFileChannel(path string) *FileChannel { return &FileChannel{Path: path} }

// Capabilities reports that the file channel honours the idempotency key.
func (c *FileChannel) Capabilities() Capabilities { return FileCapabilities() }

// Send appends e to the channel file and syncs it, at most once per event id.
//
// It returns AckConfirmed only after the append is durably synced. When the file
// already holds a record with this event id, it returns AckConfirmed without a
// second append, because the event was already applied. It returns AckRejected
// when the file cannot be opened or the path is not writable, since a retry of
// the same payload cannot be expected to succeed. It returns AckAmbiguous when a
// write or sync fails after the file was opened, because the on-disk outcome is
// then unknown.
func (c *FileChannel) Send(d Delivery) (Ack, error) {
	if err := d.Validate(); err != nil {
		return AckRejected, err
	}
	e := d.Event
	if c == nil || c.Path == "" {
		return AckRejected, errors.New("notify: file channel requires a path")
	}

	payload, err := json.Marshal(e)
	if err != nil {
		return AckRejected, fmt.Errorf("notify: encode event %q: %w", e.ID, err)
	}
	payload = append(payload, '\n')

	if dir := filepath.Dir(c.Path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return AckRejected, fmt.Errorf("notify: create channel directory %q: %w", dir, err)
		}
	}

	present, err := c.hasRecord(d.Key())
	if err != nil {
		return AckRejected, fmt.Errorf("notify: read channel file %q: %w", c.Path, err)
	}
	if present {
		// The event id is already recorded, so the append was applied once. Report
		// delivery without a second record.
		return AckConfirmed, nil
	}

	f, err := os.OpenFile(c.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return AckRejected, fmt.Errorf("notify: open channel file %q: %w", c.Path, err)
	}
	defer f.Close()

	return appendRecord(f, e, payload)
}

// dupSyncer is a writer that can also sync, so appendRecord can prove
// durability. *os.File satisfies it.
type dupSyncer interface {
	Write(p []byte) (int, error)
	Sync() error
}

// appendRecord writes payload to w and syncs it. It returns AckConfirmed only
// after both the write and the sync succeed. A write failure after a partial
// write leaves the on-disk outcome unknown, and a sync failure proves nothing
// about durability, so both return AckAmbiguous.
func appendRecord(w dupSyncer, e Event, payload []byte) (Ack, error) {
	if _, err := w.Write(payload); err != nil {
		return AckAmbiguous, fmt.Errorf("notify: append event %q: %w", e.ID, err)
	}
	if err := w.Sync(); err != nil {
		return AckAmbiguous, fmt.Errorf("notify: sync event %q: %w", e.ID, err)
	}
	return AckConfirmed, nil
}

// hasRecord reports whether the channel file already holds a record whose event
// id is id. A missing file holds no record.
func (c *FileChannel) hasRecord(id string) (bool, error) {
	data, err := os.ReadFile(c.Path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var rec struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(line, &rec); err != nil {
			// A line that is not a JSON record cannot name this event id.
			continue
		}
		if rec.ID == id {
			return true, nil
		}
	}
	return false, nil
}

// splitLines splits a file's bytes into lines without requiring a trailing
// newline.
func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
