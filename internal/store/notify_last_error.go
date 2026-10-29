package store

import (
	"database/sql"
	"errors"
)

// LastNotificationDeliveryError reports the most recent non-empty delivery
// last_error across the outbox, or an empty string when no delivery has recorded
// an error. It is a bounded, read-only observation for status surfaces: it
// performs no write and mutates no delivery state.
func (s *Store) LastNotificationDeliveryError() (string, error) {
	var lastError string
	err := s.db.QueryRow(`
		SELECT last_error
		FROM notification_deliveries
		WHERE last_error != ''
		ORDER BY updated_at DESC, event_id DESC, channel DESC
		LIMIT 1
	`).Scan(&lastError)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return lastError, nil
}

// CountNotificationEvents reports how many durable notification events the
// outbox holds. Status surfaces use it to prove a read created no event.
func (s *Store) CountNotificationEvents() (int, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM notification_events`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
