//go:build integration

package postgres_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RamizUnal/notification-system/internal/delivery"
	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/service"
	pgstore "github.com/RamizUnal/notification-system/internal/storage/postgres"
	"github.com/RamizUnal/notification-system/internal/testutil/pgtest"
)

func TestMigrationsRoundTrip(t *testing.T) {
	t.Parallel()
	dsn := pgtest.NewEmpty(t)

	version, err := pgstore.MigrateUp(dsn)
	require.NoError(t, err)
	assert.EqualValues(t, 4, version)

	require.NoError(t, pgstore.MigrateDown(dsn), "every migration has a working down step")

	version, err = pgstore.MigrateUp(dsn)
	require.NoError(t, err)
	assert.EqualValues(t, 4, version, "and the schema can be rebuilt afterwards")
}

func TestMetricsAggregates(t *testing.T) {
	t.Parallel()
	pool, _ := pgtest.New(t)
	repo := pgstore.NewNotificationRepository(pool)
	store := pgstore.NewDeliveryStore(pool)
	metrics := pgstore.NewMetricsRepository(pool)
	ctx := context.Background()
	now := baseTime()

	seed(t, repo,
		newNotification(t, domain.ChannelSMS, domain.PriorityHigh, now),
		newNotification(t, domain.ChannelSMS, domain.PriorityHigh, now),
		newNotification(t, domain.ChannelSMS, domain.PriorityLow, now, scheduledAt(now.Add(time.Hour))),
		newNotification(t, domain.ChannelEmail, domain.PriorityNormal, now),
	)

	// Deliver the two high-priority SMS: one success (100 ms), one failure (300 ms).
	jobs, err := store.Claim(ctx, domain.ChannelSMS, 2, now, leaseTTL)
	require.NoError(t, err)
	require.Len(t, jobs, 2)
	outcomes := []struct {
		status  domain.Status
		outcome domain.AttemptOutcome
		latency time.Duration
	}{
		{domain.StatusSent, domain.OutcomeSuccess, 100 * time.Millisecond},
		{domain.StatusFailed, domain.OutcomePermanentFailure, 300 * time.Millisecond},
	}
	for i, o := range outcomes {
		require.NoError(t, store.Complete(ctx, jobs[i], delivery.Result{
			Status: o.status, At: now,
			Attempt: domain.DeliveryAttempt{
				NotificationID: jobs[i].Notification.ID, Number: 1, Channel: domain.ChannelSMS, Outcome: o.outcome,
				StartedAt: now.Add(-o.latency), FinishedAt: now,
			},
		}))
	}

	svc := service.NewMetricsService(metrics, 100, service.WithClock(func() time.Time { return now.Add(time.Second) }))
	snap, err := svc.Snapshot(ctx, 5*time.Minute)
	require.NoError(t, err)

	sms := snap.Channels[0]
	require.Equal(t, domain.ChannelSMS, sms.Channel)
	assert.Equal(t, 1, sms.Scheduled)
	assert.Zero(t, sms.Due, "the only waiting SMS is scheduled for later")
	assert.Equal(t, 1, sms.Waiting[domain.PriorityLow])
	assert.Equal(t, 1, sms.Stats.Succeeded)
	assert.Equal(t, 1, sms.Stats.PermanentFailures)
	require.NotNil(t, sms.SuccessRate)
	assert.InDelta(t, 0.5, *sms.SuccessRate, 0.001)
	assert.Equal(t, 200*time.Millisecond, sms.Stats.Latency.P50, "interpolated median of 100 and 300")
	assert.Equal(t, 100, sms.RateLimit)
	assert.Equal(t, "unknown", sms.Circuit, "no worker has reported yet")

	email := snap.Channels[1]
	assert.Equal(t, 1, email.Due)
	assert.Nil(t, email.SuccessRate, "no attempts, no rate")

	assert.Equal(t, map[domain.Status]int{
		domain.StatusSent: 1, domain.StatusFailed: 1, domain.StatusScheduled: 1, domain.StatusQueued: 1,
	}, snap.StatusCounts)

	var succeeded, failed int
	for _, p := range snap.Throughput {
		succeeded += p.Succeeded
		failed += p.Failed
	}
	assert.Equal(t, 1, succeeded)
	assert.Equal(t, 1, failed)
	assert.Equal(t, time.Second, snap.Bucket, "short windows use one-second buckets")
	assert.Len(t, snap.Throughput, 301*3, "a point per bucket and channel, gaps filled with zeros")
}

func TestHeartbeatsFeedWorkerStatus(t *testing.T) {
	t.Parallel()
	pool, _ := pgtest.New(t)
	hb := pgstore.NewHeartbeats(pool)
	metrics := pgstore.NewMetricsRepository(pool)
	ctx := context.Background()
	now := baseTime()

	require.NoError(t, hb.Beat(ctx, delivery.Heartbeat{
		WorkerID: "worker-a", StartedAt: now.Add(-time.Minute), SeenAt: now, InFlight: 3,
		Circuits: map[domain.Channel]delivery.CircuitStatus{
			domain.ChannelSMS:   {State: delivery.CircuitOpen, RetryAt: now.Add(20 * time.Second)},
			domain.ChannelEmail: {State: delivery.CircuitClosed},
		},
	}))
	require.NoError(t, hb.Beat(ctx, delivery.Heartbeat{WorkerID: "worker-b", StartedAt: now, SeenAt: now.Add(-time.Hour)}))

	workers, err := metrics.Workers(ctx, now.Add(-15*time.Second))
	require.NoError(t, err)
	require.Len(t, workers, 1, "silent workers are not reported")
	w := workers[0]
	assert.Equal(t, "worker-a", w.ID)
	assert.Equal(t, 3, w.InFlight)
	assert.Equal(t, "open", w.Circuits[domain.ChannelSMS].State)
	assert.Equal(t, now.Add(20*time.Second), *w.Circuits[domain.ChannelSMS].RetryAt)

	require.NoError(t, hb.Remove(ctx, "worker-a"))
	workers, err = metrics.Workers(ctx, now.Add(-15*time.Second))
	require.NoError(t, err)
	assert.Empty(t, workers)
}

func TestListenerDeliversCommittedEvents(t *testing.T) {
	t.Parallel()
	pool, _ := pgtest.New(t)
	repo := pgstore.NewNotificationRepository(pool)
	listener := pgstore.NewListener(pool, slog.New(slog.DiscardHandler))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan domain.Event, 100)
	go func() { _ = listener.Listen(ctx, func(e domain.Event) { events <- e }) }()

	// The listener connects asynchronously and NOTIFY does not queue for
	// absent listeners, so keep committing changes until one is heard.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		n := newNotification(t, domain.ChannelEmail, domain.PriorityHigh, baseTime())
		_, _, err := repo.Create(ctx, n)
		require.NoError(t, err)

		select {
		case e := <-events:
			assert.Equal(t, domain.EventNotificationCreated, e.Type)
			assert.Equal(t, domain.ChannelEmail, e.Channel)
			assert.Equal(t, domain.PriorityHigh, e.Priority)
			assert.Equal(t, domain.StatusQueued, e.Status)
			assert.NotZero(t, e.NotificationID)
			return
		case <-time.After(200 * time.Millisecond):
		}
	}
	t.Fatal("no event received within 10s")
}
