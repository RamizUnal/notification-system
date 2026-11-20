package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RamizUnal/notification-system/internal/domain"
)

var now = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

func validDraft() domain.Draft {
	return domain.Draft{
		Channel:   domain.ChannelSMS,
		Priority:  domain.PriorityHigh,
		Recipient: "+905551234567",
		Content:   "Your order has shipped.",
	}
}

func ptr[T any](v T) *T { return &v }

func TestNewNotificationAcceptsValidDrafts(t *testing.T) {
	t.Parallel()

	tests := map[string]domain.Draft{
		"sms":   validDraft(),
		"email": {Channel: domain.ChannelEmail, Recipient: "ada@example.com", Content: "Welcome!"},
		"push":  {Channel: domain.ChannelPush, Recipient: "fcm:APA91bH-device_token.01", Content: "Flash sale!"},
		"sms at exact limit": {
			Channel: domain.ChannelSMS, Recipient: "+905551234567",
			Content: strings.Repeat("a", 1600),
		},
		"multibyte characters count once": {
			Channel: domain.ChannelPush, Recipient: "device-token-0123456789",
			Content: strings.Repeat("ş", 500),
		},
	}
	for name, draft := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			n, err := domain.NewNotification(uuid.New(), draft, now, 5)
			require.NoError(t, err)
			assert.Equal(t, domain.StatusQueued, n.Status)
			assert.Equal(t, now, n.NextAttemptAt)
			assert.Equal(t, 5, n.MaxAttempts)
			assert.NotEmpty(t, n.Fingerprint)
		})
	}
}

func TestNewNotificationNormalizesInput(t *testing.T) {
	t.Parallel()

	d := validDraft()
	d.Recipient = "  +905551234567 \n"
	d.Priority = ""
	d.Content = "  keep my spaces  "

	n, err := domain.NewNotification(uuid.New(), d, now, 3)
	require.NoError(t, err)
	assert.Equal(t, "+905551234567", n.Recipient)
	assert.Equal(t, domain.PriorityNormal, n.Priority, "priority defaults to normal")
	assert.Equal(t, "  keep my spaces  ", n.Content, "content is stored verbatim")
}

func TestNewNotificationRejectsInvalidDrafts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*domain.Draft)
		field  string
		code   string
	}{
		{"missing channel", func(d *domain.Draft) { d.Channel = "" }, "channel", domain.CodeRequired},
		{"unknown channel", func(d *domain.Draft) { d.Channel = "fax" }, "channel", domain.CodeInvalidValue},
		{"unknown priority", func(d *domain.Draft) { d.Priority = "urgent" }, "priority", domain.CodeInvalidValue},
		{"missing recipient", func(d *domain.Draft) { d.Recipient = "  " }, "recipient", domain.CodeRequired},
		{"phone without plus", func(d *domain.Draft) { d.Recipient = "905551234567" }, "recipient", domain.CodeInvalidFormat},
		{"phone with letters", func(d *domain.Draft) { d.Recipient = "+90555ABC4567" }, "recipient", domain.CodeInvalidFormat},
		{"phone too long", func(d *domain.Draft) { d.Recipient = "+1234567890123456" }, "recipient", domain.CodeInvalidFormat},
		{"email with display name", func(d *domain.Draft) {
			d.Channel, d.Recipient = domain.ChannelEmail, "Ada <ada@example.com>"
		}, "recipient", domain.CodeInvalidFormat},
		{"email without domain dot", func(d *domain.Draft) {
			d.Channel, d.Recipient = domain.ChannelEmail, "ada@localhost"
		}, "recipient", domain.CodeInvalidFormat},
		{"push token too short", func(d *domain.Draft) {
			d.Channel, d.Recipient = domain.ChannelPush, "abc"
		}, "recipient", domain.CodeInvalidFormat},
		{"push token with spaces", func(d *domain.Draft) {
			d.Channel, d.Recipient = domain.ChannelPush, "device token with spaces"
		}, "recipient", domain.CodeInvalidFormat},
		{"empty content", func(d *domain.Draft) { d.Content = "" }, "content", domain.CodeRequired},
		{"whitespace content", func(d *domain.Draft) { d.Content = " \t\n" }, "content", domain.CodeRequired},
		{"content over sms limit", func(d *domain.Draft) { d.Content = strings.Repeat("a", 1601) }, "content", domain.CodeTooLong},
		{"content over push limit", func(d *domain.Draft) {
			d.Channel, d.Recipient, d.Content = domain.ChannelPush, "device-token-0123456789", strings.Repeat("ü", 501)
		}, "content", domain.CodeTooLong},
		{"NUL in content", func(d *domain.Draft) { d.Content = "a\x00b" }, "content", domain.CodeInvalidFormat},
		{"invalid UTF-8", func(d *domain.Draft) { d.Content = "a\xffb" }, "content", domain.CodeInvalidFormat},
		{"scheduled in the past", func(d *domain.Draft) { d.ScheduledAt = ptr(now.Add(-2 * time.Minute)) }, "scheduled_at", domain.CodeOutOfRange},
		{"scheduled too far ahead", func(d *domain.Draft) { d.ScheduledAt = ptr(now.Add(31 * 24 * time.Hour)) }, "scheduled_at", domain.CodeOutOfRange},
		{"idempotency key with space", func(d *domain.Draft) { d.IdempotencyKey = "order 42" }, "idempotency_key", domain.CodeInvalidFormat},
		{"idempotency key too long", func(d *domain.Draft) { d.IdempotencyKey = strings.Repeat("k", 256) }, "idempotency_key", domain.CodeInvalidFormat},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := validDraft()
			tc.mutate(&d)

			_, err := domain.NewNotification(uuid.New(), d, now, 5)

			var verr *domain.ValidationError
			require.ErrorAs(t, err, &verr)
			require.Len(t, verr.Fields, 1, "exactly one problem expected: %v", verr.Fields)
			assert.Equal(t, tc.field, verr.Fields[0].Field)
			assert.Equal(t, tc.code, verr.Fields[0].Code)
			assert.NotEmpty(t, verr.Fields[0].Message)
		})
	}
}

func TestNewNotificationReportsEveryProblemAtOnce(t *testing.T) {
	t.Parallel()

	_, err := domain.NewNotification(uuid.New(), domain.Draft{Channel: "fax", Priority: "urgent"}, now, 5)

	var verr *domain.ValidationError
	require.ErrorAs(t, err, &verr)
	fields := make([]string, 0, len(verr.Fields))
	for _, f := range verr.Fields {
		fields = append(fields, f.Field)
	}
	assert.ElementsMatch(t, []string{"channel", "priority", "recipient", "content"}, fields)
}

func TestNewNotificationScheduling(t *testing.T) {
	t.Parallel()

	t.Run("future time schedules the notification", func(t *testing.T) {
		t.Parallel()
		d := validDraft()
		at := now.Add(time.Hour)
		d.ScheduledAt = &at

		n, err := domain.NewNotification(uuid.New(), d, now, 5)
		require.NoError(t, err)
		assert.Equal(t, domain.StatusScheduled, n.Status)
		assert.Equal(t, at, n.NextAttemptAt)
		require.NotNil(t, n.ScheduledAt)
		assert.Equal(t, at, *n.ScheduledAt)
	})

	t.Run("time within clock skew means deliver now", func(t *testing.T) {
		t.Parallel()
		d := validDraft()
		d.ScheduledAt = ptr(now.Add(-30 * time.Second))

		n, err := domain.NewNotification(uuid.New(), d, now, 5)
		require.NoError(t, err)
		assert.Equal(t, domain.StatusQueued, n.Status)
		assert.Equal(t, now, n.NextAttemptAt)
		assert.Nil(t, n.ScheduledAt)
	})
}

func TestDraftFingerprint(t *testing.T) {
	t.Parallel()

	a := validDraft().Normalize()
	b := validDraft().Normalize()
	assert.Equal(t, a.Fingerprint(), b.Fingerprint(), "identical payloads share a fingerprint")

	b.Content += "!"
	assert.NotEqual(t, a.Fingerprint(), b.Fingerprint(), "content is part of the fingerprint")

	c := validDraft().Normalize()
	c.ScheduledAt = ptr(now.Add(time.Hour))
	assert.NotEqual(t, a.Fingerprint(), c.Fingerprint(), "schedule is part of the fingerprint")
}

func TestValidationErrorHelpers(t *testing.T) {
	t.Parallel()

	var empty *domain.ValidationError
	require.NoError(t, empty.Err(), "nil collector yields a nil error")
	require.NoError(t, (&domain.ValidationError{}).Err(), "empty collector yields a nil error")

	inner := &domain.ValidationError{}
	inner.Add("recipient", domain.CodeRequired, "recipient is required")
	inner.Add("", domain.CodeInvalidValue, "item is invalid")

	outer := &domain.ValidationError{}
	outer.Merge("notifications[3]", inner)
	require.Error(t, outer.Err())
	assert.Equal(t, "notifications[3].recipient", outer.Fields[0].Field)
	assert.Equal(t, "notifications[3]", outer.Fields[1].Field)
	assert.Equal(t, "validation failed: 2 invalid fields", outer.Error())
}

func TestNotFoundErrorMatchesSentinel(t *testing.T) {
	t.Parallel()

	err := domain.NotFound("notification", "42")
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Equal(t, "notification 42 not found", err.Error())
	assert.False(t, errors.Is(err, domain.ErrNotCancellable))
}
