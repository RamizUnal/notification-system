//go:build integration

package postgres_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/RamizUnal/notification-system/internal/domain"
)

// baseTime is truncated to microseconds, PostgreSQL's timestamp precision,
// so values read back compare equal to the ones written.
func baseTime() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

func recipientFor(ch domain.Channel) string {
	switch ch {
	case domain.ChannelEmail:
		return "ada@example.com"
	case domain.ChannelPush:
		return "device-token-0123456789"
	default:
		return "+905551234567"
	}
}

type draftOption func(*domain.Draft)

func withKey(key string) draftOption      { return func(d *domain.Draft) { d.IdempotencyKey = key } }
func scheduledAt(t time.Time) draftOption { return func(d *domain.Draft) { d.ScheduledAt = &t } }

func newNotification(tb testing.TB, ch domain.Channel, p domain.Priority, now time.Time, opts ...draftOption) domain.Notification {
	tb.Helper()
	d := domain.Draft{Channel: ch, Priority: p, Recipient: recipientFor(ch), Content: "hello from the tests"}
	for _, opt := range opts {
		opt(&d)
	}
	n, err := domain.NewNotification(uuid.Must(uuid.NewV7()), d, now, 3)
	require.NoError(tb, err)
	n.CorrelationID = "corr-" + n.ID.String()[:8]
	return n
}
