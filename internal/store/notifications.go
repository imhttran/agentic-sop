package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Notification delivery status vocabulary. The same values are asserted by the
// CHECK constraint in notification_deliveries (schema version 7).
const (
	DeliveryStatusPending   = "PENDING"
	DeliveryStatusClaimed   = "CLAIMED"
	DeliveryStatusDelivered = "DELIVERED"
	DeliveryStatusExhausted = "EXHAUSTED"
)

// Delivery outcome results accepted by RecordDeliveryOutcome.
const (
	DeliveryResultDelivered = "DELIVERED"
	DeliveryResultRetry     = "RETRY"
	DeliveryResultExhausted = "EXHAUSTED"
)

// Package-level defaults for leased claims and retry policy.
const (
	// DefaultClaimLeaseDuration is how long a claimed delivery stays reserved
	// for one worker before another worker may claim it again.
	DefaultClaimLeaseDuration = 2 * time.Minute
	// DefaultClaimBatchSize is the maximum number of deliveries one claim
	// returns when the caller passes a non-positive limit.
	DefaultClaimBatchSize = 50
)

// ErrStaleClaim reports that an outcome referenced a delivery the caller no
// longer holds: the row moved on (another worker claimed it), it was already
// terminal, or no delivery carries the claim token.
var ErrStaleClaim = errors.New("stale notification claim")

// NotificationEvent is the durable record of one lifecycle transition.
type NotificationEvent struct {
	ID                string
	Version           int
	Kind              string
	Project           string
	JobID             string
	TaskID            string
	Attempt           int
	Transition        string
	Reason            string
	NextAction        string
	Outcome           string
	ApprovalRequestID string
	ResolutionCommand string
	OccurredAt        time.Time
}

// NotificationDelivery is the per-channel delivery state of one event.
type NotificationDelivery struct {
	EventID        string
	Channel        string
	Status         string
	Attempt        int
	MaxAttempts    int
	LastError      string
	NextAttemptAt  time.Time
	ClaimToken     string
	LeaseExpiresAt *time.Time
	DeliveredAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ClaimedDelivery is a delivery reserved by one worker for the lease window.
type ClaimedDelivery struct {
	NotificationDelivery
	LeaseExpiresAt time.Time
}

// NotificationStatusCount reports how many deliveries carry one status.
type NotificationStatusCount struct {
	Status string
	Count  int
}

// EnqueueNotification writes one event row and one delivery row per channel in
// a single transaction. Inserts ignore conflicts on the event primary key and
// the (event_id, channel) delivery key, so enqueuing the same event again —
// concurrently or after a restart — leaves exactly one event row and one
// delivery row per channel.
//
// An empty channels slice stores the event with no delivery rows. A non-positive
// maxAttempts defaults to 1, and a zero nextAttemptAt defaults to now.
func (s *Store) EnqueueNotification(event NotificationEvent, channels []string, maxAttempts int, nextAttemptAt time.Time, now time.Time) error {
	if event.ID == "" {
		return errors.New("enqueue notification: empty event id")
	}
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	now = now.UTC()
	if nextAttemptAt.IsZero() {
		nextAttemptAt = now
	}

	return withBusyRetry(func() error {
		return s.enqueueNotificationOnce(event, channels, maxAttempts, nextAttemptAt, now)
	})
}

// enqueueNotificationOnce performs the transactional insert for one enqueue
// attempt. It is retried by EnqueueNotification when another connection holds
// the SQLite write lock.
func (s *Store) enqueueNotificationOnce(event NotificationEvent, channels []string, maxAttempts int, nextAttemptAt, now time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		INSERT INTO notification_events
		(id, version, kind, project, job_id, task_id, attempt, transition, reason, next_action, outcome, approval_request_id, resolution_command, occurred_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, event.ID, event.Version, event.Kind, event.Project, event.JobID, event.TaskID, event.Attempt,
		event.Transition, event.Reason, event.NextAction, event.Outcome, event.ApprovalRequestID,
		event.ResolutionCommand, formatTime(event.OccurredAt), formatTime(now)); err != nil {
		return err
	}

	for _, channel := range channels {
		if _, err := tx.Exec(`
			INSERT INTO notification_deliveries
			(event_id, channel, status, attempt, max_attempts, last_error, next_attempt_at, created_at, updated_at)
			VALUES (?, ?, ?, 0, ?, '', ?, ?, ?)
			ON CONFLICT(event_id, channel) DO NOTHING
		`, event.ID, channel, DeliveryStatusPending, maxAttempts, formatTime(nextAttemptAt), formatTime(now), formatTime(now)); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ClaimNotificationDeliveries reserves up to limit claimable deliveries for one
// worker. A delivery is claimable when it is PENDING or CLAIMED, its next attempt
// time has arrived, and it carries no live lease. Each claimed row gets a fresh
// claim token and a lease expiry of now plus DefaultClaimLeaseDuration.
//
// Selection and update run in one transaction, and every update re-checks the
// row's prior claim token, so two workers never hold the same delivery at the
// same time. A non-positive limit uses DefaultClaimBatchSize. A delivery marked
// CLAIMED with an expired lease becomes claimable again, which recovers a
// delivery abandoned by a crashed worker. EXHAUSTED and DELIVERED rows are
// never selected.
func (s *Store) ClaimNotificationDeliveries(limit int, now time.Time) ([]ClaimedDelivery, error) {
	if limit <= 0 {
		limit = DefaultClaimBatchSize
	}
	now = now.UTC()
	nowStr := formatTime(now)
	leaseExpiry := now.Add(DefaultClaimLeaseDuration)
	leaseStr := formatTime(leaseExpiry)

	var claimed []ClaimedDelivery
	// A claim is a write transaction, so two workers on separate connections can
	// contend for the SQLite write lock. Retry the whole transaction while the lock
	// is held, so a concurrent claimer makes progress instead of surfacing
	// SQLITE_BUSY to the caller.
	if err := withBusyRetry(func() error {
		var err error
		claimed, err = s.claimNotificationDeliveriesOnce(limit, now, nowStr, leaseExpiry, leaseStr)
		return err
	}); err != nil {
		return nil, err
	}
	return claimed, nil
}

// claimNotificationDeliveriesOnce performs one claim transaction. It is retried
// by ClaimNotificationDeliveries when another connection holds the write lock.
func (s *Store) claimNotificationDeliveriesOnce(limit int, now time.Time, nowStr string, leaseExpiry time.Time, leaseStr string) ([]ClaimedDelivery, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`
		SELECT event_id, channel, claim_token
		FROM notification_deliveries
		WHERE status IN (?, ?)
		  AND next_attempt_at <= ?
		  AND (lease_expires_at IS NULL OR lease_expires_at <= ?)
		ORDER BY next_attempt_at, event_id, channel
		LIMIT ?
	`, DeliveryStatusPending, DeliveryStatusClaimed, nowStr, nowStr, limit)
	if err != nil {
		return nil, err
	}

	type candidate struct {
		eventID    string
		channel    string
		claimToken string
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.eventID, &c.channel, &c.claimToken); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	claimed := make([]ClaimedDelivery, 0, len(candidates))
	for _, c := range candidates {
		token, err := newClaimToken()
		if err != nil {
			return nil, err
		}
		res, err := tx.Exec(`
			UPDATE notification_deliveries
			SET status = ?, claim_token = ?, lease_expires_at = ?, updated_at = ?
			WHERE event_id = ? AND channel = ? AND claim_token = ?
			  AND status IN (?, ?)
		`, DeliveryStatusClaimed, token, leaseStr, nowStr, c.eventID, c.channel, c.claimToken,
			DeliveryStatusPending, DeliveryStatusClaimed)
		if err != nil {
			return nil, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}
		if n == 0 {
			continue
		}

		d, err := loadDelivery(tx, c.eventID, c.channel)
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, ClaimedDelivery{
			NotificationDelivery: *d,
			LeaseExpiresAt:       leaseExpiry,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return claimed, nil
}

// RecordDeliveryOutcome records the result of one delivery attempt. The update
// only applies when the delivery still carries token, so a worker whose lease
// was reassigned cannot overwrite the new owner's state; such a call returns a
// wrapped ErrStaleClaim. A row already marked EXHAUSTED is terminal and also
// rejects further outcomes.
//
// A DELIVERED result sets DELIVERED and stamps delivered_at. Any other result
// stores result-specific state: RETRY returns the row to PENDING so a later
// claim can pick it up at nextAttemptAt, and an exhausted attempt count (or an
// explicit EXHAUSTED result) sets the terminal EXHAUSTED status. The attempt
// count is incremented and last_error is stored on every outcome.
func (s *Store) RecordDeliveryOutcome(token, result, lastError string, nextAttemptAt, now time.Time) error {
	if token == "" {
		return fmt.Errorf("record delivery outcome: empty claim token: %w", ErrStaleClaim)
	}
	now = now.UTC()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var (
		eventID     string
		channel     string
		status      string
		attempt     int
		maxAttempts int
	)
	err = tx.QueryRow(`
		SELECT event_id, channel, status, attempt, max_attempts
		FROM notification_deliveries
		WHERE claim_token = ?
	`, token).Scan(&eventID, &channel, &status, &attempt, &maxAttempts)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("record delivery outcome: %w", ErrStaleClaim)
	}
	if err != nil {
		return err
	}
	if status == DeliveryStatusExhausted {
		return fmt.Errorf("record delivery outcome: delivery %s/%s is terminal: %w", eventID, channel, ErrStaleClaim)
	}

	nextAttempt := attempt + 1
	var newStatus string
	var leaseExpiry any
	var deliveredAt any

	switch result {
	case DeliveryResultDelivered:
		newStatus = DeliveryStatusDelivered
		deliveredAt = formatTime(now)
		nextAttemptAt = now
	case DeliveryResultExhausted:
		newStatus = DeliveryStatusExhausted
	default:
		// The worker owns the attempt bound: it decides whether an attempt is a retry
		// or the one that exhausts the budget, and records that decision in the result.
		// The store only counts the attempt performed, so the bound is evaluated in
		// exactly one place and cannot be applied twice (which would exhaust a delivery
		// one attempt early).
		newStatus = DeliveryStatusPending
	}
	if nextAttemptAt.IsZero() {
		nextAttemptAt = now
	}

	res, err := tx.Exec(`
		UPDATE notification_deliveries
		SET status = ?, attempt = ?, last_error = ?, next_attempt_at = ?,
		    claim_token = '', lease_expires_at = ?, delivered_at = COALESCE(?, delivered_at), updated_at = ?
		WHERE claim_token = ? AND status != ?
	`, newStatus, nextAttempt, lastError, formatTime(nextAttemptAt), leaseExpiry, deliveredAt, formatTime(now), token, DeliveryStatusExhausted)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("record delivery outcome: %w", ErrStaleClaim)
	}

	return tx.Commit()
}

// NotificationEventExists reports whether the outbox already holds the event id.
// The emitter uses it to report recorded=false for a replayed transition without
// a second event or delivery row.
func (s *Store) NotificationEventExists(id string) (bool, error) {
	if strings.TrimSpace(id) == "" {
		return false, nil
	}
	var exists int
	err := s.db.QueryRow("SELECT 1 FROM notification_events WHERE id = ?", id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// CountNotificationDeliveriesByStatus reports delivery counts grouped by status
// across every event. Entries are returned in a stable order (descending count,
// then status) and statuses with no rows are omitted.
func (s *Store) CountNotificationDeliveriesByStatus() ([]NotificationStatusCount, error) {
	return s.countDeliveriesByStatus("", false)
}

// CountNotificationDeliveriesByStatusForEvent reports delivery counts grouped by
// status for one event only, in the same stable order.
func (s *Store) CountNotificationDeliveriesByStatusForEvent(eventID string) ([]NotificationStatusCount, error) {
	return s.countDeliveriesByStatus(eventID, true)
}

func (s *Store) countDeliveriesByStatus(eventID string, scoped bool) ([]NotificationStatusCount, error) {
	query := "SELECT status, COUNT(*) FROM notification_deliveries"
	var args []any
	if scoped {
		query += " WHERE event_id = ?"
		args = append(args, eventID)
	}
	query += " GROUP BY status"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var counts []NotificationStatusCount
	for rows.Next() {
		var c NotificationStatusCount
		if err := rows.Scan(&c.Status, &c.Count); err != nil {
			return nil, err
		}
		counts = append(counts, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(counts, func(i, j int) bool {
		if counts[i].Count != counts[j].Count {
			return counts[i].Count > counts[j].Count
		}
		return counts[i].Status < counts[j].Status
	})
	return counts, nil
}

// GetNotificationDelivery reads one delivery row. It is used by tests and by
// operators inspecting the outbox.
func (s *Store) GetNotificationDelivery(eventID, channel string) (*NotificationDelivery, error) {
	d, err := loadDelivery(s.db, eventID, channel)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return d, nil
}

// querier is satisfied by *sql.DB and *sql.Tx so one loader serves both.
type querier interface {
	QueryRow(query string, args ...any) *sql.Row
}

// loadDelivery reads one delivery row through q.
func loadDelivery(q querier, eventID, channel string) (*NotificationDelivery, error) {
	d := &NotificationDelivery{}
	var nextAttemptAt, createdAt, updatedAt string
	var leaseExpiresAt, deliveredAt sql.NullString
	err := q.QueryRow(`
		SELECT event_id, channel, status, attempt, max_attempts, last_error, next_attempt_at,
		       claim_token, lease_expires_at, delivered_at, created_at, updated_at
		FROM notification_deliveries
		WHERE event_id = ? AND channel = ?
	`, eventID, channel).Scan(&d.EventID, &d.Channel, &d.Status, &d.Attempt, &d.MaxAttempts,
		&d.LastError, &nextAttemptAt, &d.ClaimToken, &leaseExpiresAt, &deliveredAt, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if d.NextAttemptAt, err = parseTimestamp(nextAttemptAt); err != nil {
		return nil, fmt.Errorf("parse next_attempt_at: %w", err)
	}
	if d.CreatedAt, err = parseTimestamp(createdAt); err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	if d.UpdatedAt, err = parseTimestamp(updatedAt); err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	if leaseExpiresAt.Valid {
		t, err := parseTimestamp(leaseExpiresAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse lease_expires_at: %w", err)
		}
		d.LeaseExpiresAt = &t
	}
	if deliveredAt.Valid {
		t, err := parseTimestamp(deliveredAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse delivered_at: %w", err)
		}
		d.DeliveredAt = &t
	}
	return d, nil
}

// busyRetryAttempts bounds how often a write retries while another connection
// holds the SQLite write lock.
const (
	busyRetryAttempts = 50
	busyRetryDelay    = 5 * time.Millisecond
)

// withBusyRetry runs fn again while it reports SQLITE_BUSY or SQLITE_LOCKED, so
// concurrent writers on separate connections make progress instead of failing.
// The store sets no busy timeout, so contention surfaces as an error and this
// helper paces the retry.
func withBusyRetry(fn func() error) error {
	var err error
	for i := 0; i < busyRetryAttempts; i++ {
		err = fn()
		if err == nil || !isBusyError(err) {
			return err
		}
		time.Sleep(busyRetryDelay)
	}
	return err
}

func isBusyError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "database table is locked") || strings.Contains(msg, "SQLITE_BUSY")
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// newClaimToken returns a random 128-bit hex token that identifies one lease.
func newClaimToken() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}
