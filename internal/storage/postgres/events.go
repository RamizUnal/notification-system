package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RamizUnal/notification-system/internal/domain"
)

// eventsChannel is the LISTEN/NOTIFY channel carrying domain events.
const eventsChannel = "notification_events"

// maxEventErrorLength keeps payloads far below NOTIFY's 8000-byte limit.
const maxEventErrorLength = 300

// eventPayload is the wire format of an event inside a NOTIFY payload.
type eventPayload struct {
	Type           string     `json:"type"`
	NotificationID *uuid.UUID `json:"notification_id,omitempty"`
	BatchID        *uuid.UUID `json:"batch_id,omitempty"`
	Channel        string     `json:"channel,omitempty"`
	Priority       string     `json:"priority,omitempty"`
	Status         string     `json:"status,omitempty"`
	Attempts       int        `json:"attempts,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
	BatchSize      int        `json:"batch_size,omitempty"`
	OccurredAt     time.Time  `json:"occurred_at"`
}

// publish queues events for delivery to listeners when tx commits.
func publish(ctx context.Context, tx pgx.Tx, events ...domain.Event) error {
	if len(events) == 0 {
		return nil
	}
	payloads := make([]string, len(events))
	for i, e := range events {
		p := eventPayload{
			Type:       string(e.Type),
			BatchID:    e.BatchID,
			Channel:    string(e.Channel),
			Priority:   string(e.Priority),
			Status:     string(e.Status),
			Attempts:   e.Attempts,
			LastError:  truncateRunes(e.LastError, maxEventErrorLength),
			BatchSize:  e.BatchSize,
			OccurredAt: e.OccurredAt.UTC(),
		}
		if e.NotificationID != uuid.Nil {
			id := e.NotificationID
			p.NotificationID = &id
		}
		b, err := json.Marshal(p)
		if err != nil {
			return fmt.Errorf("encode event: %w", err)
		}
		payloads[i] = string(b)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_notify($1, payload) FROM unnest($2::text[]) AS payload`,
		eventsChannel, payloads); err != nil {
		return fmt.Errorf("publish events: %w", err)
	}
	return nil
}

func decodeEvent(payload string) (domain.Event, error) {
	var p eventPayload
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return domain.Event{}, fmt.Errorf("decode event: %w", err)
	}
	e := domain.Event{
		Type:       domain.EventType(p.Type),
		BatchID:    p.BatchID,
		Channel:    domain.Channel(p.Channel),
		Priority:   domain.Priority(p.Priority),
		Status:     domain.Status(p.Status),
		Attempts:   p.Attempts,
		LastError:  p.LastError,
		BatchSize:  p.BatchSize,
		OccurredAt: p.OccurredAt,
	}
	if p.NotificationID != nil {
		e.NotificationID = *p.NotificationID
	}
	return e, nil
}

// Listener streams committed domain events using LISTEN/NOTIFY.
//
// NOTIFY is fire-and-forget: events committed while the listener is
// reconnecting are not replayed. Consumers that need completeness (the
// console) refetch state after a reconnect instead of relying on the stream.
type Listener struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewListener returns a listener that opens its own dedicated connection,
// so a long-lived LISTEN never occupies a slot of the shared pool.
func NewListener(pool *pgxpool.Pool, logger *slog.Logger) *Listener {
	return &Listener{pool: pool, logger: logger}
}

// Listen delivers events to handle until ctx is cancelled, reconnecting with
// exponential backoff after connection failures.
func (l *Listener) Listen(ctx context.Context, handle func(domain.Event)) error {
	backoff := time.Second
	for {
		err := l.listen(ctx, handle)
		if ctx.Err() != nil {
			return nil
		}
		l.logger.WarnContext(ctx, "event listener disconnected; reconnecting",
			slog.Any("error", err), slog.Duration("backoff", backoff))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff = min(2*backoff, 30*time.Second)
	}
}

func (l *Listener) listen(ctx context.Context, handle func(domain.Event)) error {
	conn, err := pgx.ConnectConfig(ctx, l.pool.Config().ConnConfig.Copy())
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()

	if _, err := conn.Exec(ctx, "LISTEN "+eventsChannel); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	l.logger.InfoContext(ctx, "listening for notification events")

	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return fmt.Errorf("wait for notification: %w", err)
		}
		event, err := decodeEvent(n.Payload)
		if err != nil {
			l.logger.WarnContext(ctx, "dropping malformed event", slog.Any("error", err))
			continue
		}
		handle(event)
	}
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
