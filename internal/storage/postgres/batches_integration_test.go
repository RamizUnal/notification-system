//go:build integration

package postgres_test

import (
	"context"
	"fmt"
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

func TestBatchOfAThousandIsStoredAtomically(t *testing.T) {
	t.Parallel()
	pool, _ := pgtest.New(t)
	svc := service.NewNotificationService(
		pgstore.NewNotificationRepository(pool), pgstore.NewBatchRepository(pool), pgstore.NewTemplateRepository(pool))
	ctx := context.Background()

	items := make([]service.NotificationInput, domain.MaxBatchSize)
	for i := range items {
		items[i] = service.NotificationInput{
			Channel: domain.Channels()[i%3], Recipient: recipientFor(domain.Channels()[i%3]),
			Content: fmt.Sprintf("flash sale #%d", i),
		}
	}

	res, err := svc.CreateBatch(ctx, service.BatchInput{Items: items, IdempotencyKey: "flash-sale-1"})
	require.NoError(t, err)
	assert.Equal(t, domain.MaxBatchSize, res.Summary.Size)

	batches := pgstore.NewBatchRepository(pool)
	ids, err := batches.NotificationIDs(ctx, res.Summary.ID)
	require.NoError(t, err)
	assert.Equal(t, res.NotificationIDs, ids, "stored order is request order")

	notifications := pgstore.NewNotificationRepository(pool)
	n, err := notifications.Get(ctx, ids[777])
	require.NoError(t, err)
	assert.Equal(t, "flash sale #777", n.Content)

	summary, err := batches.Summary(ctx, res.Summary.ID)
	require.NoError(t, err)
	assert.Equal(t, map[domain.Status]int{domain.StatusQueued: domain.MaxBatchSize}, summary.Counts)

	replay, err := svc.CreateBatch(ctx, service.BatchInput{Items: items, IdempotencyKey: "flash-sale-1"})
	require.NoError(t, err)
	assert.True(t, replay.Replayed)
	assert.Equal(t, res.NotificationIDs, replay.NotificationIDs)

	cancelled, err := batches.Cancel(ctx, res.Summary.ID, time.Now())
	require.NoError(t, err)
	assert.Equal(t, domain.MaxBatchSize, cancelled)
	summary, err = batches.Summary(ctx, res.Summary.ID)
	require.NoError(t, err)
	assert.True(t, summary.Done())

	_, err = batches.Cancel(ctx, uuid.New(), time.Now())
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestBatchItemKeyConflictStoresNothing(t *testing.T) {
	t.Parallel()
	pool, _ := pgtest.New(t)
	notifications := pgstore.NewNotificationRepository(pool)
	svc := service.NewNotificationService(notifications, pgstore.NewBatchRepository(pool), pgstore.NewTemplateRepository(pool))
	ctx := context.Background()

	existing, err := svc.Create(ctx, service.NotificationInput{
		Channel: domain.ChannelSMS, Recipient: "+905551234567", Content: "otp 1", IdempotencyKey: "otp-1",
	})
	require.NoError(t, err)

	_, err = svc.CreateBatch(ctx, service.BatchInput{Items: []service.NotificationInput{
		{Channel: domain.ChannelSMS, Recipient: "+905551234567", Content: "a", IdempotencyKey: "fresh"},
		{Channel: domain.ChannelSMS, Recipient: "+905551234567", Content: "b"},
		{Channel: domain.ChannelSMS, Recipient: "+905551234567", Content: "otp 1", IdempotencyKey: "otp-1"},
	}})

	var conflict *domain.IdempotencyConflictError
	require.ErrorAs(t, err, &conflict)
	assert.Equal(t, []domain.KeyConflict{{Index: 2, Key: "otp-1", ExistingID: existing.Notification.ID}}, conflict.Conflicts)

	var rows, batches int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM notifications`).Scan(&rows))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM batches`).Scan(&batches))
	assert.Equal(t, 1, rows, "the batch was rolled back")
	assert.Zero(t, batches)
}

func TestTemplateRepository(t *testing.T) {
	t.Parallel()
	pool, _ := pgtest.New(t)
	repo := pgstore.NewTemplateRepository(pool)
	svc := service.NewTemplateService(repo)
	ctx := context.Background()

	otp, err := svc.Create(ctx, domain.TemplateDraft{Name: "otp", Channel: domain.ChannelSMS, Body: "Code: {{code}}"})
	require.NoError(t, err)
	welcome, err := svc.Create(ctx, domain.TemplateDraft{Name: "welcome", Channel: domain.ChannelEmail, Body: "Hi {{name}}"})
	require.NoError(t, err)

	_, err = svc.Create(ctx, domain.TemplateDraft{Name: "otp", Channel: domain.ChannelSMS, Body: "dup"})
	require.ErrorIs(t, err, domain.ErrTemplateNameTaken)
	_, err = svc.Update(ctx, welcome.ID, domain.TemplateDraft{Name: "otp", Channel: domain.ChannelEmail, Body: "x"})
	require.ErrorIs(t, err, domain.ErrTemplateNameTaken, "renaming onto a taken name fails too")

	list, err := svc.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "otp", list[0].Name, "ordered by name")

	many, err := repo.GetMany(ctx, []uuid.UUID{otp.ID, uuid.New()})
	require.NoError(t, err)
	assert.Len(t, many, 1)

	// Deleting a template keeps the notifications rendered from it.
	notifications := pgstore.NewNotificationRepository(pool)
	ns := service.NewNotificationService(notifications, pgstore.NewBatchRepository(pool), repo)
	created, err := ns.Create(ctx, service.NotificationInput{
		Recipient: "+905551234567", TemplateID: &otp.ID, Variables: map[string]string{"code": "1234"},
	})
	require.NoError(t, err)
	require.NoError(t, svc.Delete(ctx, otp.ID))

	n, err := notifications.Get(ctx, created.Notification.ID)
	require.NoError(t, err)
	assert.Nil(t, n.TemplateID)
	assert.Equal(t, "Code: 1234", n.Content)

	assert.ErrorIs(t, svc.Delete(ctx, otp.ID), domain.ErrNotFound)
}
