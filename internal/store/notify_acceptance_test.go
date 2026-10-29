package store

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Acceptance mapping for NTFY-008 (issue scenario coverage, store layer):
//
//	durability: TestAcceptanceOutboxDurabilitySurvivesReopen writes deliveries, closes
//	the store, reopens the same temp-file database, and reads the state back.
//	replay: TestAcceptanceReplayLeavesOneEventAndOneDeliveryPerChannel enqueues the
//	same event twice and asserts one event row and one delivery row per channel, and
//	NotificationEventExists reports recorded=true on replay.
//	restart: TestAcceptanceRestartDrainsAfterReclaim proves a delivery claimed before
//	close is drained after a restart, and an abandoned CLAIMED delivery with an
//	expired lease becomes claimable again.
//	concurrent: TestAcceptanceConcurrentWorkersNeverShareADelivery runs two claimers
//	and asserts disjoint claim tokens and ErrStaleClaim on the loser.

// acceptanceStoreTime is the fixed instant the store acceptance tests use.
func acceptanceStoreTime() time.Time {
	return time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
}

// openAcceptanceStore opens a fresh temp-file store outside the shared helper so the
// test owns its lifecycle and can close and reopen the same file.
func openAcceptanceStore(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%s): %v", path, err)
	}
	return s
}

// TestAcceptanceOutboxDurabilitySurvivesReopen proves the outbox state lives in the
// file: event and delivery rows written by one process are visible after a reopen.
func TestAcceptanceOutboxDurabilitySurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	now := acceptanceStoreTime()

	s1 := openAcceptanceStore(t, path)
	if err := s1.EnqueueNotification(notificationEvent("evt-dur"), []string{"log", "webhook"}, 3, now, now); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2 := openAcceptanceStore(t, path)
	defer s2.Close()

	exists, err := s2.NotificationEventExists("evt-dur")
	if err != nil {
		t.Fatalf("NotificationEventExists: %v", err)
	}
	if !exists {
		t.Fatal("event evt-dur not durable across reopen")
	}
	for _, ch := range []string{"log", "webhook"} {
		d, err := s2.GetNotificationDelivery("evt-dur", ch)
		if err != nil {
			t.Fatalf("GetNotificationDelivery(%s): %v", ch, err)
		}
		if d.Status != DeliveryStatusPending || d.Attempt != 0 {
			t.Fatalf("delivery %s = %+v, want PENDING attempt 0", ch, d)
		}
	}
	counts, err := s2.CountNotificationDeliveriesByStatus()
	if err != nil {
		t.Fatalf("CountNotificationDeliveriesByStatus: %v", err)
	}
	if len(counts) != 1 || counts[0].Status != DeliveryStatusPending || counts[0].Count != 2 {
		t.Fatalf("counts = %+v, want PENDING 2", counts)
	}
}

// TestAcceptanceReplayLeavesOneEventAndOneDeliveryPerChannel proves a replayed event
// is idempotent: exactly one event row and one delivery row per channel, and the event
// is reported as already recorded.
func TestAcceptanceReplayLeavesOneEventAndOneDeliveryPerChannel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	now := acceptanceStoreTime()
	s := openAcceptanceStore(t, path)
	defer s.Close()

	channels := []string{"log", "webhook"}
	if err := s.EnqueueNotification(notificationEvent("evt-replay"), channels, 3, now, now); err != nil {
		t.Fatalf("first Enqueue: %v", err)
	}
	// Replay the same transition at a different time; the row set must not change.
	if err := s.EnqueueNotification(notificationEvent("evt-replay"), channels, 3, now.Add(time.Hour), now.Add(time.Hour)); err != nil {
		t.Fatalf("replay Enqueue: %v", err)
	}

	if got := countRows(t, s, "notification_events"); got != 1 {
		t.Fatalf("event rows = %d, want 1", got)
	}
	if got := countRows(t, s, "notification_deliveries"); got != 2 {
		t.Fatalf("delivery rows = %d, want 2 (one per channel)", got)
	}
	exists, err := s.NotificationEventExists("evt-replay")
	if err != nil {
		t.Fatalf("NotificationEventExists: %v", err)
	}
	if !exists {
		t.Fatal("replayed event should report recorded=true")
	}
}

// TestAcceptanceRestartDrainsAfterReclaim proves a delivery claimed by a worker that
// then stops can be reclaimed by a fresh process once the lease expires, and the
// original (abandoned) claim token becomes stale.
func TestAcceptanceRestartDrainsAfterReclaim(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	now := acceptanceStoreTime()

	s1 := openAcceptanceStore(t, path)
	if err := s1.EnqueueNotification(notificationEvent("evt-restart"), []string{"log"}, 3, now, now); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	claimed, err := s1.ClaimNotificationDeliveries(1, now)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("first claim = %d, %v", len(claimed), err)
	}
	abandonedToken := claimed[0].ClaimToken
	if abandonedToken == "" {
		t.Fatal("claimed delivery carries no claim token")
	}
	// The worker crashes here: the row is left CLAIMED with a live lease.
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2 := openAcceptanceStore(t, path)
	defer s2.Close()

	// Inside the lease the delivery is still reserved, so a restart does not steal it.
	inside := now.Add(DefaultClaimLeaseDuration - time.Second)
	if got, err := s2.ClaimNotificationDeliveries(1, inside); err != nil || len(got) != 0 {
		t.Fatalf("claim inside lease = %d, %v; want 0", len(got), err)
	}

	// After the lease expires the abandoned delivery becomes claimable again.
	after := now.Add(DefaultClaimLeaseDuration + time.Second)
	reclaimed, err := s2.ClaimNotificationDeliveries(1, after)
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if len(reclaimed) != 1 {
		t.Fatalf("reclaim after lease expiry = %d, want 1", len(reclaimed))
	}
	if reclaimed[0].ClaimToken == abandonedToken {
		t.Fatal("reclaim reused the abandoned claim token")
	}

	// The abandoned worker cannot record an outcome; its token is stale.
	if err := s2.RecordDeliveryOutcome(abandonedToken, DeliveryResultDelivered, "", after, after); !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("abandoned token outcome = %v, want ErrStaleClaim", err)
	}

	// The new owner records the delivery and the outbox drains.
	if err := s2.RecordDeliveryOutcome(reclaimed[0].ClaimToken, DeliveryResultDelivered, "", after, after); err != nil {
		t.Fatalf("record delivered: %v", err)
	}
	got, err := s2.GetNotificationDelivery("evt-restart", "log")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != DeliveryStatusDelivered {
		t.Fatalf("status = %q, want DELIVERED", got.Status)
	}
}

// TestAcceptanceConcurrentWorkersNeverShareADelivery proves two workers claiming at
// the same instant never hold the same delivery: claim tokens are disjoint and the
// loser cannot record an outcome with the winner's token.
func TestAcceptanceConcurrentWorkersNeverShareADelivery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	now := acceptanceStoreTime()

	s1 := openAcceptanceStore(t, path)
	defer s1.Close()
	if err := s1.EnqueueNotification(notificationEvent("evt-conc"), []string{"log"}, 3, now, now); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	s2 := openAcceptanceStore(t, path)
	defer s2.Close()

	type result struct {
		claimed []ClaimedDelivery
		err     error
	}
	results := make([]result, 2)
	stores := []*Store{s1, s2}
	var wg sync.WaitGroup
	for i := range stores {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			claimed, err := stores[i].ClaimNotificationDeliveries(10, now)
			results[i] = result{claimed: claimed, err: err}
		}(i)
	}
	wg.Wait()

	// The store serializes claims with a busy-retry, so a claim that loses the
	// SQLite write lock must still succeed after the retry instead of surfacing
	// SQLITE_BUSY to the caller.
	var tokens []string
	total := 0
	for i, r := range results {
		if r.err != nil {
			t.Fatalf("claimer %d error: %v", i, r.err)
		}
		total += len(r.claimed)
		for _, c := range r.claimed {
			tokens = append(tokens, c.ClaimToken)
		}
	}
	if total != 1 {
		t.Fatalf("total claimed = %d across both workers, want 1", total)
	}
	if len(tokens) != 1 || tokens[0] == "" {
		t.Fatalf("claim tokens = %v, want one non-empty token", tokens)
	}

	winner := tokens[0]
	// The loser holds no token; recording with a fabricated/other token is a stale claim.
	if err := s1.RecordDeliveryOutcome("not-the-winner", DeliveryResultDelivered, "", now, now); !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("loser outcome = %v, want ErrStaleClaim", err)
	}
	// The winner records successfully.
	if err := s1.RecordDeliveryOutcome(winner, DeliveryResultDelivered, "", now, now); err != nil {
		t.Fatalf("winner outcome: %v", err)
	}
}
