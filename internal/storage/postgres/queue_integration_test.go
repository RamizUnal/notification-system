//go:build integration

package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RamizUnal/notification-system/internal/delivery"
	"github.com/RamizUnal/notification-system/internal/domain"
	pgstore "github.com/RamizUnal/notification-system/internal/storage/postgres"
	"github.com/RamizUnal/notification-system/internal/testutil/pgtest"
)

const leaseTTL = 30 * time.Second

func seed(t *testing.T, repo *pgstore.NotificationRepository, notifications ...domain.Notification) {
	t.Helper()
	for _, n := range notifications {
		_, _, err := repo.Create(context.Background(), n)
		require.NoError(t, err)
	}
}

func TestClaimHonoursPriorityChannelAndSchedule(t *testing.T) {
	t.Parallel()
	pool, _ := pgtest.New(t)
	repo := pgstore.NewNotificationRepository(pool)
	store := pgstore.NewDeliveryStore(pool)
	now := baseTime()

	low := newNotification(t, domain.ChannelSMS, domain.PriorityLow, now.Add(-3*time.Minute))
	normal := newNotification(t, domain.ChannelSMS, domain.PriorityNormal, now.Add(-2*time.Minute))
	high := newNotification(t, domain.ChannelSMS, domain.PriorityHigh, now.Add(-time.Minute))
	future := newNotification(t, domain.ChannelSMS, domain.PriorityHigh, now.Add(-time.Minute),
		scheduledAt(now.Add(time.Hour)))
	email := newNotification(t, domain.ChannelEmail, domain.PriorityHigh, now.Add(-time.Minute))
	seed(t, repo, low, normal, high, future, email)

	jobs, err := store.Claim(context.Background(), domain.ChannelSMS, 10, now, leaseTTL)
	require.NoError(t, err)

	require.Len(t, jobs, 3, "the scheduled and the email notification are not claimed")
	assert.Equal(t, []uuid.UUID{high.ID, normal.ID, low.ID},
		[]uuid.UUID{jobs[0].Notification.ID, jobs[1].Notification.ID, jobs[2].Notification.ID},
		"high priority first, even though it is the newest")
	for _, job := range jobs {
		assert.Equal(t, domain.StatusProcessing, job.Notification.Status)
		assert.Equal(t, 1, job.Notification.Attempts, "a claim counts as an attempt")
		assert.NotEqual(t, uuid.Nil, job.LeaseID)
		assert.WithinDuration(t, now.Add(leaseTTL), job.LeaseExpiresAt, time.Millisecond)
	}

	again, err := store.Claim(context.Background(), domain.ChannelSMS, 10, now, leaseTTL)
	require.NoError(t, err)
	assert.Empty(t, again, "leased notifications are not claimed twice")

	due, err := store.Claim(context.Background(), domain.ChannelSMS, 10, now.Add(2*time.Hour), leaseTTL)
	require.NoError(t, err)
	require.Len(t, due, 1, "the scheduled notification becomes due")
	assert.Equal(t, future.ID, due[0].Notification.ID)
}

func TestConcurrentClaimsNeverShareANotification(t *testing.T) {
	t.Parallel()
	pool, _ := pgtest.New(t)
	repo := pgstore.NewNotificationRepository(pool)
	store := pgstore.NewDeliveryStore(pool)
	now := baseTime()

	const total = 300
	for range total {
		seed(t, repo, newNotification(t, domain.ChannelPush, domain.PriorityNormal, now.Add(-time.Second)))
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		claimed = map[uuid.UUID]int{}
	)
	for range 8 {
		wg.Go(func() {
			for {
				jobs, err := store.Claim(context.Background(), domain.ChannelPush, 7, now, leaseTTL)
				if !assert.NoError(t, err) || len(jobs) == 0 {
					return
				}
				mu.Lock()
				for _, j := range jobs {
					claimed[j.Notification.ID]++
				}
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	assert.Len(t, claimed, total, "every notification is claimed")
	for id, n := range claimed {
		assert.Equalf(t, 1, n, "notification %s was claimed %d times", id, n)
	}
}

func TestCompleteRecordsTheOutcomeUnderTheLease(t *testing.T) {
	t.Parallel()
	pool, _ := pgtest.New(t)
	repo := pgstore.NewNotificationRepository(pool)
	store := pgstore.NewDeliveryStore(pool)
	ctx := context.Background()
	now := baseTime()

	seed(t, repo, newNotification(t, domain.ChannelSMS, domain.PriorityNormal, now))
	jobs, err := store.Claim(ctx, domain.ChannelSMS, 1, now, leaseTTL)
	require.NoError(t, err)
	job := jobs[0]

	t.Run("a stale lease is rejected", func(t *testing.T) {
		stale := job
		stale.LeaseID = uuid.New()
		err := store.Complete(ctx, stale, delivery.Result{Status: domain.StatusSent, At: now})
		require.ErrorIs(t, err, domain.ErrLeaseLost)
	})

	t.Run("a retry is scheduled", func(t *testing.T) {
		next := now.Add(10 * time.Second)
		err := store.Complete(ctx, job, delivery.Result{
			Status:        domain.StatusRetrying,
			NextAttemptAt: next,
			Error:         "provider responded 503: down",
			At:            now.Add(time.Second),
			Attempt: domain.DeliveryAttempt{
				NotificationID: job.Notification.ID, Number: 1, Channel: domain.ChannelSMS,
				Outcome: domain.OutcomeRetryableFailure, StatusCode: 503, Error: "provider responded 503: down",
				StartedAt: now, FinishedAt: now.Add(250 * time.Millisecond),
			},
		})
		require.NoError(t, err)

		n, err := repo.Get(ctx, job.Notification.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.StatusRetrying, n.Status)
		assert.Equal(t, next, n.NextAttemptAt)
		assert.Equal(t, "provider responded 503: down", n.LastError)

		early, err := store.Claim(ctx, domain.ChannelSMS, 1, now.Add(5*time.Second), leaseTTL)
		require.NoError(t, err)
		assert.Empty(t, early, "not due before next_attempt_at")
	})

	t.Run("the retry succeeds", func(t *testing.T) {
		jobs, err := store.Claim(ctx, domain.ChannelSMS, 1, now.Add(11*time.Second), leaseTTL)
		require.NoError(t, err)
		require.Len(t, jobs, 1)
		retry := jobs[0]
		assert.Equal(t, 2, retry.Notification.Attempts)

		sentAt := now.Add(12 * time.Second)
		require.NoError(t, store.Complete(ctx, retry, delivery.Result{
			Status: domain.StatusSent, ProviderMessageID: "msg-42", At: sentAt,
			Attempt: domain.DeliveryAttempt{
				NotificationID: retry.Notification.ID, Number: 2, Channel: domain.ChannelSMS,
				Outcome: domain.OutcomeSuccess, StatusCode: 202, ProviderMessageID: "msg-42",
				StartedAt: sentAt.Add(-80 * time.Millisecond), FinishedAt: sentAt,
			},
		}))

		n, err := repo.Get(ctx, retry.Notification.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.StatusSent, n.Status)
		assert.Equal(t, sentAt, *n.SentAt)
		assert.Equal(t, "msg-42", n.ProviderMessageID)
		assert.Empty(t, n.LastError)

		attempts, err := repo.Attempts(ctx, n.ID)
		require.NoError(t, err)
		require.Len(t, attempts, 2)
		assert.Equal(t, domain.OutcomeRetryableFailure, attempts[0].Outcome)
		assert.Equal(t, 503, attempts[0].StatusCode)
		assert.Equal(t, domain.OutcomeSuccess, attempts[1].Outcome)
		assert.Equal(t, 80*time.Millisecond, attempts[1].Duration())
	})
}

func TestReleaseReturnsTheClaimWithoutCountingIt(t *testing.T) {
	t.Parallel()
	pool, _ := pgtest.New(t)
	repo := pgstore.NewNotificationRepository(pool)
	store := pgstore.NewDeliveryStore(pool)
	ctx := context.Background()
	now := baseTime()

	seed(t, repo, newNotification(t, domain.ChannelEmail, domain.PriorityNormal, now))
	jobs, err := store.Claim(ctx, domain.ChannelEmail, 1, now, leaseTTL)
	require.NoError(t, err)

	require.NoError(t, store.Release(ctx, jobs[0], now.Add(30*time.Second)))

	n, err := repo.Get(ctx, jobs[0].Notification.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusQueued, n.Status)
	assert.Zero(t, n.Attempts)
	assert.Equal(t, now.Add(30*time.Second), n.NextAttemptAt)
	assert.ErrorIs(t, store.Release(ctx, jobs[0], now), domain.ErrLeaseLost, "a lease can be released once")
}

func TestRecoverExpiredLeases(t *testing.T) {
	t.Parallel()
	pool, _ := pgtest.New(t)
	repo := pgstore.NewNotificationRepository(pool)
	store := pgstore.NewDeliveryStore(pool)
	ctx := context.Background()
	now := baseTime()

	first := newNotification(t, domain.ChannelSMS, domain.PriorityHigh, now)
	last := newNotification(t, domain.ChannelSMS, domain.PriorityNormal, now)
	seed(t, repo, first, last)
	_, err := pool.Exec(ctx, `UPDATE notifications SET attempts = 2 WHERE id = $1`, last.ID) // max_attempts is 3
	require.NoError(t, err)

	jobs, err := store.Claim(ctx, domain.ChannelSMS, 10, now, time.Second)
	require.NoError(t, err)
	require.Len(t, jobs, 2)

	recovered, err := store.RecoverExpiredLeases(ctx, now.Add(500*time.Millisecond))
	require.NoError(t, err)
	assert.Zero(t, recovered, "live leases are left alone")

	recovered, err = store.RecoverExpiredLeases(ctx, now.Add(2*time.Second))
	require.NoError(t, err)
	assert.Equal(t, 2, recovered)

	retried, err := repo.Get(ctx, first.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusRetrying, retried.Status)
	assert.Contains(t, retried.LastError, "may have been sent")

	exhausted, err := repo.Get(ctx, last.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusFailed, exhausted.Status, "a lost final attempt fails instead of looping")

	attempts, err := repo.Attempts(ctx, first.ID)
	require.NoError(t, err)
	require.Len(t, attempts, 1, "the lost attempt stays in the audit trail")
	assert.Equal(t, domain.OutcomeRetryableFailure, attempts[0].Outcome)

	err = store.Complete(ctx, jobs[0], delivery.Result{Status: domain.StatusSent, At: now})
	assert.ErrorIs(t, err, domain.ErrLeaseLost, "the vanished worker cannot overwrite the recovery")
}

func TestCancelLosesToAClaim(t *testing.T) {
	t.Parallel()
	pool, _ := pgtest.New(t)
	repo := pgstore.NewNotificationRepository(pool)
	store := pgstore.NewDeliveryStore(pool)
	ctx := context.Background()
	now := baseTime()

	n := newNotification(t, domain.ChannelPush, domain.PriorityNormal, now)
	seed(t, repo, n)
	_, err := store.Claim(ctx, domain.ChannelPush, 1, now, leaseTTL)
	require.NoError(t, err)

	_, err = repo.Cancel(ctx, n.ID, now)
	require.ErrorIs(t, err, domain.ErrNotCancellable)
	assert.Contains(t, err.Error(), "is processing")
}
