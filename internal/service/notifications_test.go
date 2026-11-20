package service_test

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/observability"
	"github.com/RamizUnal/notification-system/internal/service"
)

// clock is a settable time source.
type clock struct{ t time.Time }

func (c *clock) Now() time.Time          { return c.t }
func (c *clock) Advance(d time.Duration) { c.t = c.t.Add(d) }

type fixture struct {
	mem   *memory
	clock *clock
	svc   *service.NotificationService
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	mem := newMemory()
	clk := &clock{t: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)}
	svc := service.NewNotificationService(
		mem.notificationRepo(), mem.batchRepo(), mem.templateRepo(),
		service.WithClock(clk.Now), service.WithMaxAttempts(4),
	)
	return &fixture{mem: mem, clock: clk, svc: svc}
}

func (f *fixture) addTemplate(name string, ch domain.Channel, body string) domain.Template {
	tpl := domain.Template{ID: uuid.New(), Name: name, Channel: ch, Body: body}
	f.mem.templates[tpl.ID] = tpl
	return tpl
}

func smsInput() service.NotificationInput {
	return service.NotificationInput{
		Channel:   domain.ChannelSMS,
		Recipient: "+905551234567",
		Content:   "Your verification code is 482910",
	}
}

func requireValidationFields(t *testing.T, err error, want ...string) *domain.ValidationError {
	t.Helper()
	var verr *domain.ValidationError
	require.ErrorAs(t, err, &verr)
	got := make([]string, 0, len(verr.Fields))
	for _, f := range verr.Fields {
		got = append(got, f.Field)
	}
	assert.ElementsMatch(t, want, got)
	return verr
}

func TestCreateStoresAQueuedNotification(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := observability.WithCorrelationID(context.Background(), "req-123")

	res, err := f.svc.Create(ctx, smsInput())

	require.NoError(t, err)
	assert.False(t, res.Replayed)
	n := res.Notification
	assert.Equal(t, domain.StatusQueued, n.Status)
	assert.Equal(t, domain.PriorityNormal, n.Priority)
	assert.Equal(t, 4, n.MaxAttempts)
	assert.Equal(t, "req-123", n.CorrelationID, "correlation ID is persisted for the worker")
	assert.Equal(t, f.clock.Now(), n.NextAttemptAt)
	assert.Equal(t, uuid.Version(7), n.ID.Version(), "IDs are time-ordered UUIDv7")
	assert.Len(t, f.mem.notifications, 1)
}

func TestCreateRejectsInvalidInputWithoutStoring(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	_, err := f.svc.Create(context.Background(), service.NotificationInput{Channel: domain.ChannelEmail, Recipient: "nope"})

	requireValidationFields(t, err, "recipient", "content")
	assert.Empty(t, f.mem.notifications)
}

func TestCreateIsIdempotent(t *testing.T) {
	t.Parallel()

	t.Run("same key and payload replays the original", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		in := smsInput()
		in.IdempotencyKey = "otp-482910"

		first, err := f.svc.Create(context.Background(), in)
		require.NoError(t, err)
		second, err := f.svc.Create(context.Background(), in)
		require.NoError(t, err)

		assert.True(t, second.Replayed)
		assert.Equal(t, first.Notification.ID, second.Notification.ID)
		assert.Len(t, f.mem.notifications, 1, "nothing new is stored")
	})

	t.Run("same key with a different payload conflicts", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		in := smsInput()
		in.IdempotencyKey = "otp-482910"
		_, err := f.svc.Create(context.Background(), in)
		require.NoError(t, err)

		in.Content = "A different message"
		_, err = f.svc.Create(context.Background(), in)

		require.ErrorIs(t, err, domain.ErrIdempotencyConflict)
		assert.Len(t, f.mem.notifications, 1)
	})

	t.Run("a concurrent insert with the same key is replayed", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		in := smsInput()
		in.IdempotencyKey = "otp-482910"
		first, err := f.svc.Create(context.Background(), in)
		require.NoError(t, err)

		f.mem.hideKeysFromLookup = true // the lookup misses, the insert collides
		second, err := f.svc.Create(context.Background(), in)

		require.NoError(t, err)
		assert.True(t, second.Replayed)
		assert.Equal(t, first.Notification.ID, second.Notification.ID)
	})

	t.Run("a retry is replayed even after its schedule has passed", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		in := smsInput()
		in.IdempotencyKey = "reminder-7"
		at := f.clock.Now().Add(time.Hour)
		in.ScheduledAt = &at

		first, err := f.svc.Create(context.Background(), in)
		require.NoError(t, err)
		require.Equal(t, domain.StatusScheduled, first.Notification.Status)

		f.clock.Advance(3 * time.Hour)
		second, err := f.svc.Create(context.Background(), in)

		require.NoError(t, err, "replay must not re-validate the schedule")
		assert.True(t, second.Replayed)
	})
}

func TestCreateFromTemplate(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	tpl := f.addTemplate("order-shipped", domain.ChannelSMS, "Hi {{name}}, order {{order}} has shipped.")

	t.Run("renders content and infers the channel", func(t *testing.T) {
		res, err := f.svc.Create(context.Background(), service.NotificationInput{
			Recipient:  "+905551234567",
			TemplateID: &tpl.ID,
			Variables:  map[string]string{"name": "Ada", "order": "#42"},
		})
		require.NoError(t, err)
		assert.Equal(t, "Hi Ada, order #42 has shipped.", res.Notification.Content)
		assert.Equal(t, domain.ChannelSMS, res.Notification.Channel)
		assert.Equal(t, &tpl.ID, res.Notification.TemplateID)
	})

	unknown := uuid.New()
	tests := []struct {
		name  string
		input service.NotificationInput
		field string
		code  string
	}{
		{"missing variables", service.NotificationInput{
			Recipient: "+905551234567", TemplateID: &tpl.ID, Variables: map[string]string{"name": "Ada"},
		}, "variables", domain.CodeMissingVariables},
		{"unknown template", service.NotificationInput{
			Recipient: "+905551234567", TemplateID: &unknown,
		}, "template_id", domain.CodeInvalidValue},
		{"channel mismatch", service.NotificationInput{
			Channel: domain.ChannelEmail, Recipient: "ada@example.com", TemplateID: &tpl.ID,
		}, "template_id", domain.CodeChannelMismatch},
		{"content and template together", service.NotificationInput{
			Recipient: "+905551234567", TemplateID: &tpl.ID, Content: "hello",
		}, "content", domain.CodeInvalidValue},
		{"variables without template", service.NotificationInput{
			Channel: domain.ChannelSMS, Recipient: "+905551234567", Content: "hi", Variables: map[string]string{"a": "b"},
		}, "variables", domain.CodeInvalidValue},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.svc.Create(context.Background(), tc.input)
			verr := requireValidationFields(t, err, tc.field)
			assert.Equal(t, tc.code, verr.Fields[0].Code)
		})
	}
}

func batchOf(n int) []service.NotificationInput {
	items := make([]service.NotificationInput, n)
	for i := range items {
		items[i] = smsInput()
		items[i].Content = fmt.Sprintf("message %d", i)
	}
	return items
}

func TestCreateBatch(t *testing.T) {
	t.Parallel()

	t.Run("accepts the whole batch and aligns IDs with the request", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)

		res, err := f.svc.CreateBatch(context.Background(), service.BatchInput{Items: batchOf(50)})

		require.NoError(t, err)
		assert.Equal(t, 50, res.Summary.Size)
		assert.Equal(t, map[domain.Status]int{domain.StatusQueued: 50}, res.Summary.Counts)
		require.Len(t, res.NotificationIDs, 50)
		assert.True(t, slices.IsSortedFunc(res.NotificationIDs, func(a, b uuid.UUID) int {
			return bytes.Compare(a[:], b[:])
		}), "IDs ascend so that ordering by ID reproduces the request order")
		for i, id := range res.NotificationIDs {
			n := f.mem.notifications[id]
			assert.Equal(t, fmt.Sprintf("message %d", i), n.Content)
			assert.Equal(t, res.Summary.ID, *n.BatchID)
		}
	})

	t.Run("rejects empty and oversized batches", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)

		_, err := f.svc.CreateBatch(context.Background(), service.BatchInput{})
		requireValidationFields(t, err, "notifications")

		_, err = f.svc.CreateBatch(context.Background(), service.BatchInput{Items: batchOf(domain.MaxBatchSize + 1)})
		verr := requireValidationFields(t, err, "notifications")
		assert.Equal(t, domain.CodeOutOfRange, verr.Fields[0].Code)
	})

	t.Run("reports every invalid item and stores nothing", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		items := batchOf(5)
		items[1].Recipient = "not-a-phone"
		items[3].Content = ""
		items[4].Channel = "fax"

		_, err := f.svc.CreateBatch(context.Background(), service.BatchInput{Items: items})

		requireValidationFields(t, err,
			"notifications[1].recipient", "notifications[3].content", "notifications[4].channel")
		assert.Empty(t, f.mem.notifications, "batches are atomic")
	})

	t.Run("rejects duplicate idempotency keys inside the batch", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		items := batchOf(3)
		items[0].IdempotencyKey = "same"
		items[2].IdempotencyKey = "same"

		_, err := f.svc.CreateBatch(context.Background(), service.BatchInput{Items: items})

		verr := requireValidationFields(t, err, "notifications[2].idempotency_key")
		assert.Equal(t, domain.CodeDuplicate, verr.Fields[0].Code)
	})

	t.Run("surfaces item keys that already exist", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		single := smsInput()
		single.IdempotencyKey = "taken"
		existing, err := f.svc.Create(context.Background(), single)
		require.NoError(t, err)

		items := batchOf(2)
		items[1].IdempotencyKey = "taken"
		_, err = f.svc.CreateBatch(context.Background(), service.BatchInput{Items: items})

		var conflict *domain.IdempotencyConflictError
		require.ErrorAs(t, err, &conflict)
		require.ErrorIs(t, err, domain.ErrIdempotencyConflict)
		assert.Equal(t, []domain.KeyConflict{{Index: 1, Key: "taken", ExistingID: existing.Notification.ID}}, conflict.Conflicts)
	})

	t.Run("replays a retried batch and rejects a changed one", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		in := service.BatchInput{Items: batchOf(3), IdempotencyKey: "campaign-2026-11-20"}

		first, err := f.svc.CreateBatch(context.Background(), in)
		require.NoError(t, err)
		again, err := f.svc.CreateBatch(context.Background(), in)
		require.NoError(t, err)

		assert.True(t, again.Replayed)
		assert.Equal(t, first.Summary.ID, again.Summary.ID)
		assert.Equal(t, first.NotificationIDs, again.NotificationIDs)
		assert.Len(t, f.mem.notifications, 3)

		in.Items[0].Content = "changed"
		_, err = f.svc.CreateBatch(context.Background(), in)
		require.ErrorIs(t, err, domain.ErrIdempotencyConflict)
	})

	t.Run("validates the batch idempotency key", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		_, err := f.svc.CreateBatch(context.Background(), service.BatchInput{
			Items: batchOf(1), IdempotencyKey: strings.Repeat("k", 300),
		})
		requireValidationFields(t, err, "Idempotency-Key")
	})
}

func TestList(t *testing.T) {
	t.Parallel()

	t.Run("pages with a cursor at the last item", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		_, err := f.svc.CreateBatch(context.Background(), service.BatchInput{Items: batchOf(5)})
		require.NoError(t, err)

		page, err := f.svc.List(context.Background(), service.ListFilter{Limit: 2})
		require.NoError(t, err)

		require.Len(t, page.Items, 2)
		require.NotNil(t, page.Next)
		assert.Equal(t, page.Items[1].ID, page.Next.ID)
		assert.Equal(t, 3, f.mem.lastFilter.Limit, "one extra row reveals whether more pages exist")

		last, err := f.svc.List(context.Background(), service.ListFilter{Limit: 5})
		require.NoError(t, err)
		assert.Len(t, last.Items, 5)
		assert.Nil(t, last.Next)
	})

	t.Run("defaults and validates its parameters", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)

		_, err := f.svc.List(context.Background(), service.ListFilter{})
		require.NoError(t, err)
		assert.Equal(t, service.DefaultPageSize+1, f.mem.lastFilter.Limit)

		_, err = f.svc.List(context.Background(), service.ListFilter{Limit: 500})
		requireValidationFields(t, err, "limit")

		from := f.clock.Now()
		to := from.Add(-time.Hour)
		_, err = f.svc.List(context.Background(), service.ListFilter{CreatedFrom: &from, CreatedTo: &to})
		requireValidationFields(t, err, "created_from")
	})
}

func TestRetryGrantsAFreshAttemptBudget(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	res, err := f.svc.Create(context.Background(), smsInput())
	require.NoError(t, err)
	n := res.Notification
	n.Status, n.Attempts = domain.StatusFailed, 4
	f.mem.notifications[n.ID] = n

	retried, err := f.svc.Retry(context.Background(), n.ID)

	require.NoError(t, err)
	assert.Equal(t, domain.StatusQueued, retried.Status)
	assert.Equal(t, 4, f.mem.requeueBudget)
	assert.Equal(t, 8, retried.MaxAttempts)
}

func TestAttemptsOfUnknownNotification(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	_, err := f.svc.Attempts(context.Background(), uuid.New())
	require.ErrorIs(t, err, domain.ErrNotFound)
}
