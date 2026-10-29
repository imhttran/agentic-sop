package store

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Acceptance mapping for the notification outbox (NTFY-002):
//
//	criterion 1 (duplicate enqueue yields one event row and one delivery row per
//	channel): TestEnqueueIsIdempotent, TestEnqueueIsIdempotentAcrossReopen,
//	TestEnqueueIsIdempotentUnderConcurrency.
//	criterion 2 (a second worker cannot claim a held delivery and a stale claim
//	becomes claimable after the lease expires): TestClaimExcludesSecondWorker,
//	TestExpiredLeaseBecomesClaimable.
//	criterion 3 (attempt count, last error, and next attempt time round-trip and
//	survive reopen): TestDeliveryStateSurvivesReopen.
//	criterion 4 (an EXHAUSTED delivery is never claimed again):
//	TestExhaustedDeliveryIsTerminal.

func notificationEvent(id string) NotificationEvent {
	e := NotificationEvent{
		ID:         id,
		Version:    1,
		Kind:       "task.transition",
		Project:    "proj",
		TaskID:     "T001",
		Transition: "READY->IMPLEMENTING",
	}
	e.OccurredAt = fixedTime()
	return e
}

func countRows(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestSchemaVersionIsSeven(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != 7 {
		t.Fatalf("user_version = %d, want 7", version)
	}

	for _, table := range []string{"notification_events", "notification_deliveries"} {
		var name string
		if err := s.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil {
			t.Fatalf("table %s missing: %v", table, err)
		}
	}
}

func TestEnqueueCreatesOneEventAndOneDeliveryPerChannel(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	now := fixedTime()
	if err := s.EnqueueNotification(notificationEvent("evt-1"), []string{"log", "webhook"}, 3, now, now); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	if got := countRows(t, s, "notification_events"); got != 1 {
		t.Fatalf("event rows = %d, want 1", got)
	}
	if got := countRows(t, s, "notification_deliveries"); got != 2 {
		t.Fatalf("delivery rows = %d, want 2", got)
	}
}

func TestEnqueueIsIdempotent(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	now := fixedTime()
	channels := []string{"log", "webhook"}
	for i := 0; i < 2; i++ {
		if err := s.EnqueueNotification(notificationEvent("evt-1"), channels, 3, now, now); err != nil {
			t.Fatalf("Enqueue %d failed: %v", i, err)
		}
	}

	if got := countRows(t, s, "notification_events"); got != 1 {
		t.Fatalf("event rows = %d, want 1", got)
	}
	if got := countRows(t, s, "notification_deliveries"); got != 2 {
		t.Fatalf("delivery rows = %d, want 2", got)
	}
}

func TestEnqueueIsIdempotentAcrossReopen(t *testing.T) {
	dbPath := testDB(t)
	now := fixedTime()

	s1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := s1.EnqueueNotification(notificationEvent("evt-1"), []string{"log", "webhook"}, 3, now, now); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	defer s2.Close()

	if err := s2.EnqueueNotification(notificationEvent("evt-1"), []string{"log", "webhook"}, 3, now, now); err != nil {
		t.Fatalf("re-enqueue failed: %v", err)
	}
	if got := countRows(t, s2, "notification_events"); got != 1 {
		t.Fatalf("event rows after reopen = %d, want 1", got)
	}
	if got := countRows(t, s2, "notification_deliveries"); got != 2 {
		t.Fatalf("delivery rows after reopen = %d, want 2", got)
	}
}

func TestEnqueueIsIdempotentUnderConcurrency(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	now := fixedTime()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- s.EnqueueNotification(notificationEvent("evt-1"), []string{"log", "webhook"}, 3, now, now)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent enqueue failed: %v", err)
		}
	}

	if got := countRows(t, s, "notification_events"); got != 1 {
		t.Fatalf("event rows = %d, want 1", got)
	}
	if got := countRows(t, s, "notification_deliveries"); got != 2 {
		t.Fatalf("delivery rows = %d, want 2", got)
	}
}

func TestClaimExcludesSecondWorker(t *testing.T) {
	dbPath := testDB(t)
	now := fixedTime()

	s1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s1.Close()
	if err := s1.EnqueueNotification(notificationEvent("evt-1"), []string{"log"}, 3, now, now); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open second failed: %v", err)
	}
	defer s2.Close()

	first, err := s1.ClaimNotificationDeliveries(10, now)
	if err != nil {
		t.Fatalf("first claim failed: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first claim returned %d deliveries, want 1", len(first))
	}
	if first[0].ClaimToken == "" {
		t.Fatal("claimed delivery carries no claim token")
	}

	second, err := s2.ClaimNotificationDeliveries(10, now)
	if err != nil {
		t.Fatalf("second claim failed: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("second claim returned %d deliveries, want 0", len(second))
	}
}

func TestSecondWorkerClaimsAfterOutcome(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	now := fixedTime()
	if err := s.EnqueueNotification(notificationEvent("evt-1"), []string{"log"}, 3, now, now); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	claimed, err := s.ClaimNotificationDeliveries(1, now)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim = %d, %v", len(claimed), err)
	}
	if err := s.RecordDeliveryOutcome(claimed[0].ClaimToken, DeliveryResultRetry, "boom", now.Add(time.Minute), now); err != nil {
		t.Fatalf("record outcome failed: %v", err)
	}
	if got, err := s.ClaimNotificationDeliveries(1, now); err != nil || len(got) != 0 {
		t.Fatalf("claim before next attempt returned %d, %v; want 0", len(got), err)
	}
	got, err := s.ClaimNotificationDeliveries(1, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("claim after next attempt failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("claim after next attempt returned %d, want 1", len(got))
	}
}

func TestExpiredLeaseBecomesClaimable(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	now := fixedTime()
	if err := s.EnqueueNotification(notificationEvent("evt-1"), []string{"log"}, 3, now, now); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	first, err := s.ClaimNotificationDeliveries(1, now)
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim = %d, %v", len(first), err)
	}
	if got, err := s.ClaimNotificationDeliveries(1, now.Add(DefaultClaimLeaseDuration-time.Second)); err != nil || len(got) != 0 {
		t.Fatalf("claim inside lease returned %d, %v; want 0", len(got), err)
	}

	later := now.Add(DefaultClaimLeaseDuration + time.Second)
	reclaimed, err := s.ClaimNotificationDeliveries(1, later)
	if err != nil {
		t.Fatalf("reclaim failed: %v", err)
	}
	if len(reclaimed) != 1 {
		t.Fatalf("reclaim after lease expiry returned %d, want 1", len(reclaimed))
	}
	if reclaimed[0].ClaimToken == first[0].ClaimToken {
		t.Fatal("reclaim reused the stale claim token")
	}
}

func TestDeliveryStateSurvivesReopen(t *testing.T) {
	dbPath := testDB(t)
	now := fixedTime()

	s1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := s1.EnqueueNotification(notificationEvent("evt-1"), []string{"log"}, 3, now, now); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	claimed, err := s1.ClaimNotificationDeliveries(1, now)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim = %d, %v", len(claimed), err)
	}
	nextAttempt := now.Add(5 * time.Minute)
	if err := s1.RecordDeliveryOutcome(claimed[0].ClaimToken, DeliveryResultRetry, "smtp unavailable", nextAttempt, now); err != nil {
		t.Fatalf("record outcome failed: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	defer s2.Close()

	got, err := s2.GetNotificationDelivery("evt-1", "log")
	if err != nil {
		t.Fatalf("Get after reopen failed: %v", err)
	}
	if got.Attempt != 1 {
		t.Errorf("attempt = %d, want 1", got.Attempt)
	}
	if got.LastError != "smtp unavailable" {
		t.Errorf("last_error = %q, want %q", got.LastError, "smtp unavailable")
	}
	if !got.NextAttemptAt.Equal(nextAttempt) {
		t.Errorf("next_attempt_at = %v, want %v", got.NextAttemptAt, nextAttempt)
	}
	if got.Status != DeliveryStatusPending {
		t.Errorf("status = %q, want %q", got.Status, DeliveryStatusPending)
	}
}

func TestStaleClaimTokenRejected(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	now := fixedTime()
	if err := s.EnqueueNotification(notificationEvent("evt-1"), []string{"log"}, 5, now, now); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	claimed, err := s.ClaimNotificationDeliveries(1, now)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim = %d, %v", len(claimed), err)
	}

	err = s.RecordDeliveryOutcome("not-a-token", DeliveryResultDelivered, "", now, now)
	if !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("stale token outcome error = %v, want ErrStaleClaim", err)
	}
	got, err := s.GetNotificationDelivery("evt-1", "log")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Status != DeliveryStatusClaimed || got.Attempt != 0 {
		t.Fatalf("stale outcome changed the row: status=%q attempt=%d", got.Status, got.Attempt)
	}
}

// TestRetryResultDoesNotExhaustEarly proves the attempt bound is evaluated once, by
// the worker: a RETRY outcome stays PENDING even when it is the last attempt the row's
// own bound would allow, so a delivery cannot be exhausted one attempt early.
func TestRetryResultDoesNotExhaustEarly(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	now := fixedTime()
	if err := s.EnqueueNotification(notificationEvent("evt-retry"), []string{"log"}, 1, now, now); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	claimed, err := s.ClaimNotificationDeliveries(1, now)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim = %d, %v", len(claimed), err)
	}
	if err := s.RecordDeliveryOutcome(claimed[0].ClaimToken, DeliveryResultRetry, "ambiguous", now.Add(time.Second), now); err != nil {
		t.Fatalf("record: %v", err)
	}
	got, err := s.GetNotificationDelivery("evt-retry", "log")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Status != DeliveryStatusPending {
		t.Fatalf("status = %q, want PENDING (the worker owns the bound)", got.Status)
	}
	if got.Attempt != 1 {
		t.Fatalf("attempt = %d, want 1 recorded attempt", got.Attempt)
	}
}

func TestExhaustedDeliveryIsTerminal(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	now := fixedTime()
	if err := s.EnqueueNotification(notificationEvent("evt-1"), []string{"log"}, 1, now, now); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	claimed, err := s.ClaimNotificationDeliveries(1, now)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim = %d, %v", len(claimed), err)
	}
	// The worker owns the attempt bound: it records EXHAUSTED on the attempt that
	// reaches the limit. The store must not re-evaluate the bound.
	if err := s.RecordDeliveryOutcome(claimed[0].ClaimToken, DeliveryResultExhausted, "permanent failure", now.Add(time.Minute), now); err != nil {
		t.Fatalf("record outcome failed: %v", err)
	}

	got, err := s.GetNotificationDelivery("evt-1", "log")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Status != DeliveryStatusExhausted {
		t.Fatalf("status = %q, want %q", got.Status, DeliveryStatusExhausted)
	}

	// Neither a retry after the lease nor a retry at the next attempt time may
	// return the terminal delivery.
	for _, at := range []time.Time{now.Add(time.Minute), now.Add(time.Hour), now.Add(24 * time.Hour)} {
		claim, err := s.ClaimNotificationDeliveries(10, at)
		if err != nil {
			t.Fatalf("claim at %v failed: %v", at, err)
		}
		if len(claim) != 0 {
			t.Fatalf("EXHAUSTED delivery claimed again at %v", at)
		}
	}

	if err := s.RecordDeliveryOutcome(claimed[0].ClaimToken, DeliveryResultDelivered, "", now, now); !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("outcome on exhausted delivery = %v, want ErrStaleClaim", err)
	}
}

func TestStatusCounts(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	now := fixedTime()
	if err := s.EnqueueNotification(notificationEvent("evt-1"), []string{"log", "webhook"}, 3, now, now); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	counts, err := s.CountNotificationDeliveriesByStatus()
	if err != nil {
		t.Fatalf("counts failed: %v", err)
	}
	if len(counts) != 1 || counts[0].Status != DeliveryStatusPending || counts[0].Count != 2 {
		t.Fatalf("counts after enqueue = %+v, want PENDING 2", counts)
	}

	claimed, err := s.ClaimNotificationDeliveries(1, now)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim = %d, %v", len(claimed), err)
	}
	if err := s.RecordDeliveryOutcome(claimed[0].ClaimToken, DeliveryResultDelivered, "", now, now); err != nil {
		t.Fatalf("record outcome failed: %v", err)
	}

	byEvent, err := s.CountNotificationDeliveriesByStatusForEvent("evt-1")
	if err != nil {
		t.Fatalf("scoped counts failed: %v", err)
	}
	got := map[string]int{}
	for _, c := range byEvent {
		got[c.Status] = c.Count
	}
	if got[DeliveryStatusDelivered] != 1 || got[DeliveryStatusPending] != 1 {
		t.Fatalf("scoped counts = %+v, want DELIVERED 1 PENDING 1", got)
	}

	if err := s.EnqueueNotification(notificationEvent("evt-2"), []string{"log"}, 3, now, now); err != nil {
		t.Fatalf("Enqueue evt-2 failed: %v", err)
	}
	other, err := s.CountNotificationDeliveriesByStatusForEvent("evt-2")
	if err != nil {
		t.Fatalf("scoped counts evt-2 failed: %v", err)
	}
	if len(other) != 1 || other[0].Status != DeliveryStatusPending || other[0].Count != 1 {
		t.Fatalf("evt-2 counts = %+v, want PENDING 1", other)
	}
}

func TestMigrationSixToSevenKeepsTasks(t *testing.T) {
	dbPath := testDB(t)
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	saveDependency(t, s, "T001")

	// Simulate a version-6 database: the notification tables do not exist and
	// the recorded version is 6.
	if _, err := s.db.Exec("DROP TABLE notification_deliveries"); err != nil {
		t.Fatalf("drop deliveries: %v", err)
	}
	if _, err := s.db.Exec("DROP TABLE notification_events"); err != nil {
		t.Fatalf("drop events: %v", err)
	}
	if _, err := s.db.Exec("PRAGMA user_version = 6"); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen (migrate 6->7) failed: %v", err)
	}
	defer s2.Close()

	if _, err := s2.Get("T001"); err != nil {
		t.Fatalf("T001 lost in migration: %v", err)
	}
	for _, name := range []string{
		"notification_events",
		"notification_deliveries",
		"idx_notification_deliveries_claim",
		"idx_notification_deliveries_status",
	} {
		var found string
		if err := s2.db.QueryRow("SELECT name FROM sqlite_master WHERE name=?", name).Scan(&found); err != nil {
			t.Fatalf("%s missing after migration: %v", name, err)
		}
	}
}

func TestDuplicateDeliveryRowRejected(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	now := fixedTime()
	if err := s.EnqueueNotification(notificationEvent("evt-1"), []string{"log"}, 3, now, now); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	if _, err := s.db.Exec(`
		INSERT INTO notification_deliveries (event_id, channel, next_attempt_at, created_at, updated_at)
		VALUES ('evt-1', 'log', ?, ?, ?)
	`, formatTime(now), formatTime(now), formatTime(now)); err == nil {
		t.Fatal("duplicate delivery row accepted, want unique-key rejection")
	}
	if _, err := s.db.Exec(`
		INSERT INTO notification_events (id, version, kind, project, transition, occurred_at, created_at)
		VALUES ('evt-1', 1, 'k', 'p', 't', ?, ?)
	`, formatTime(now), formatTime(now)); err == nil {
		t.Fatal("duplicate event row accepted, want primary-key rejection")
	}
}
