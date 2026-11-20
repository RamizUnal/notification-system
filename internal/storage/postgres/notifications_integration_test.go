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

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/service"
	pgstore "github.com/RamizUnal/notification-system/internal/storage/postgres"
	"github.com/RamizUnal/notification-system/internal/testutil/pgtest"
)

func TestNotificationRoundTrip(t *testing.T) {
	t.Parallel()
	pool, _ := pgtest.New(t)
	repo := pgstore.NewNotificationRepository(pool)
	ctx := context.Background()
	now := baseTime()

	n := newNotification(t, domain.ChannelEmail, domain.PriorityHigh, now,
		withKey("welcome-ada"), scheduledAt(now.Add(time.Hour)))
	n.TraceParent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

	stored, created, err := repo.Create(ctx, n)
	require.NoError(t, err)
	require.True(t, created)
	assert.Equal(t, n, stored, "every field survives the round trip")

	got, err := repo.Get(ctx, n.ID)
	require.NoError(t, err)
	assert.Equal(t, n, got)

	byKey, found, err := repo.FindByIdempotencyKey(ctx, "welcome-ada")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, n.ID, byKey.ID)

	_, err = repo.Get(ctx, uuid.New())
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestCreateIsIdempotentUnderConcurrency(t *testing.T) {
	t.Parallel()
	pool, _ := pgtest.New(t)
	repo := pgstore.NewNotificationRepository(pool)
	now := baseTime()

	const requests = 20
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		ids     = map[uuid.UUID]int{}
		created int
	)
	for range requests {
		wg.Go(func() {
			// Each request mints its own ID, as concurrent API calls would.
			n := newNotification(t, domain.ChannelSMS, domain.PriorityNormal, now, withKey("same-key"))
			stored, isNew, err := repo.Create(context.Background(), n)
			assert.NoError(t, err)
			mu.Lock()
			defer mu.Unlock()
			ids[stored.ID]++
			if isNew {
				created++
			}
		})
	}
	wg.Wait()

	assert.Equal(t, 1, created, "exactly one request inserts")
	assert.Len(t, ids, 1, "every request sees the same notification")

	var rows int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM notifications`).Scan(&rows))
	assert.Equal(t, 1, rows)
}

func TestListFiltersAndPaginates(t *testing.T) {
	t.Parallel()
	pool, _ := pgtest.New(t)
	repo := pgstore.NewNotificationRepository(pool)
	ctx := context.Background()
	start := baseTime()

	// 30 notifications, one per second, cycling through channels and priorities.
	var all []domain.Notification
	for i := range 30 {
		ch := domain.Channels()[i%3]
		p := domain.Priorities()[i%3]
		n := newNotification(t, ch, p, start.Add(time.Duration(i)*time.Second))
		_, _, err := repo.Create(ctx, n)
		require.NoError(t, err)
		all = append(all, n)
	}
	_, err := repo.Cancel(ctx, all[4].ID, start.Add(time.Minute))
	require.NoError(t, err)

	t.Run("newest first across pages without gaps or duplicates", func(t *testing.T) {
		t.Parallel()
		svc := service.NewNotificationService(repo, nil, nil)
		var seen []uuid.UUID
		filter := service.ListFilter{Limit: 7}
		for {
			page, err := svc.List(ctx, filter)
			require.NoError(t, err)
			for _, n := range page.Items {
				seen = append(seen, n.ID)
			}
			if page.Next == nil {
				break
			}
			filter.After = page.Next
		}
		require.Len(t, seen, 30)
		for i, id := range seen {
			assert.Equal(t, all[29-i].ID, id)
		}
	})

	tests := []struct {
		name   string
		filter service.ListFilter
		want   int
	}{
		{"by channel", service.ListFilter{Channels: []domain.Channel{domain.ChannelPush}}, 10},
		{"by several channels", service.ListFilter{Channels: []domain.Channel{domain.ChannelSMS, domain.ChannelEmail}}, 20},
		{"by priority", service.ListFilter{Priorities: []domain.Priority{domain.PriorityHigh}}, 10},
		{"by status", service.ListFilter{Statuses: []domain.Status{domain.StatusCancelled}}, 1},
		{"by recipient", service.ListFilter{Recipient: "ada@example.com"}, 10},
		{"by date range", service.ListFilter{
			CreatedFrom: ptr(start.Add(10 * time.Second)), CreatedTo: ptr(start.Add(20 * time.Second)),
		}, 10},
		{"combined", service.ListFilter{
			Channels: []domain.Channel{domain.ChannelSMS}, Statuses: []domain.Status{domain.StatusQueued},
		}, 10},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.filter.Limit = 100
			got, err := repo.List(ctx, tc.filter)
			require.NoError(t, err)
			assert.Len(t, got, tc.want)
		})
	}
}

func TestCancelAndRequeueTransitions(t *testing.T) {
	t.Parallel()
	pool, _ := pgtest.New(t)
	repo := pgstore.NewNotificationRepository(pool)
	ctx := context.Background()
	now := baseTime()

	n := newNotification(t, domain.ChannelSMS, domain.PriorityNormal, now)
	_, _, err := repo.Create(ctx, n)
	require.NoError(t, err)

	cancelled, err := repo.Cancel(ctx, n.ID, now.Add(time.Second))
	require.NoError(t, err)
	assert.Equal(t, domain.StatusCancelled, cancelled.Status)
	assert.Equal(t, now.Add(time.Second), *cancelled.CancelledAt)

	_, err = repo.Cancel(ctx, n.ID, now)
	require.ErrorIs(t, err, domain.ErrNotCancellable)
	assert.Contains(t, err.Error(), "is cancelled")

	_, err = repo.Requeue(ctx, n.ID, now, 3)
	require.ErrorIs(t, err, domain.ErrNotRetryable)

	_, err = repo.Cancel(ctx, uuid.New(), now)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// A failed notification goes back to the queue with a fresh budget.
	failed := newNotification(t, domain.ChannelSMS, domain.PriorityNormal, now)
	_, _, err = repo.Create(ctx, failed)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE notifications SET status = 'failed', attempts = 3, failed_at = $2,
		last_error = 'boom' WHERE id = $1`, failed.ID, now)
	require.NoError(t, err)

	requeued, err := repo.Requeue(ctx, failed.ID, now.Add(time.Minute), 3)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusQueued, requeued.Status)
	assert.Equal(t, 6, requeued.MaxAttempts)
	assert.Equal(t, now.Add(time.Minute), requeued.NextAttemptAt)
	assert.Nil(t, requeued.FailedAt)
	assert.Empty(t, requeued.LastError)
}

func ptr[T any](v T) *T { return &v }
