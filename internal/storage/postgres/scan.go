package postgres

import (
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/RamizUnal/notification-system/internal/domain"
)

// notificationColumns is the column list every notification query returns,
// in the order scanNotification expects.
const notificationColumns = `id, batch_id, template_id, channel, priority, recipient, content, status,
	idempotency_key, fingerprint, attempts, max_attempts, scheduled_at, next_attempt_at, last_error,
	provider_message_id, correlation_id, trace_parent, created_at, updated_at, sent_at, failed_at, cancelled_at`

// scanNotification reads one row of notificationColumns (optionally
// followed by extra destinations, such as lease columns).
func scanNotification(row pgx.Row, extra ...any) (domain.Notification, error) {
	var (
		n                                                 domain.Notification
		channel, priority, status                         string
		key, lastErr, providerID, correlationID, traceCtx *string
	)
	dest := []any{
		&n.ID, &n.BatchID, &n.TemplateID, &channel, &priority, &n.Recipient, &n.Content, &status,
		&key, &n.Fingerprint, &n.Attempts, &n.MaxAttempts, &n.ScheduledAt, &n.NextAttemptAt, &lastErr,
		&providerID, &correlationID, &traceCtx, &n.CreatedAt, &n.UpdatedAt, &n.SentAt, &n.FailedAt, &n.CancelledAt,
	}
	if err := row.Scan(append(dest, extra...)...); err != nil {
		return domain.Notification{}, err
	}

	n.Channel = domain.Channel(channel)
	n.Priority = domain.Priority(priority)
	n.Status = domain.Status(status)
	n.IdempotencyKey = deref(key)
	n.LastError = deref(lastErr)
	n.ProviderMessageID = deref(providerID)
	n.CorrelationID = deref(correlationID)
	n.TraceParent = deref(traceCtx)
	n.NextAttemptAt = n.NextAttemptAt.UTC()
	n.CreatedAt, n.UpdatedAt = n.CreatedAt.UTC(), n.UpdatedAt.UTC()
	n.ScheduledAt, n.SentAt = utcPtr(n.ScheduledAt), utcPtr(n.SentAt)
	n.FailedAt, n.CancelledAt = utcPtr(n.FailedAt), utcPtr(n.CancelledAt)
	return n, nil
}

func scanNotificationRow(row pgx.CollectableRow) (domain.Notification, error) {
	return scanNotification(row)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// nullable maps the empty string to SQL NULL.
func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func strings[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}
