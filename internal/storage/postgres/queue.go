package postgres

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RamizUnal/notification-system/internal/delivery"
	"github.com/RamizUnal/notification-system/internal/domain"
)

// leaseLostError is recorded for notifications whose worker vanished.
const leaseLostError = "worker stopped before recording the outcome; the message may have been sent"

// heartbeatRetention is how long a silent worker's heartbeat row is kept.
const heartbeatRetention = 10 * time.Minute

// DeliveryStore implements delivery.Store: it is the work queue.
type DeliveryStore struct {
	pool *pgxpool.Pool
}

// NewDeliveryStore returns a store using pool.
func NewDeliveryStore(pool *pgxpool.Pool) *DeliveryStore {
	return &DeliveryStore{pool: pool}
}

var _ delivery.Store = (*DeliveryStore)(nil)

// Claim leases up to limit due notifications of one channel.
//
// FOR UPDATE SKIP LOCKED lets any number of workers claim concurrently
// without blocking each other or claiming the same row twice. The inner
// SELECT walks the partial queue index (channel, priority_rank,
// next_attempt_at) in order, so high priority always goes first.
func (s *DeliveryStore) Claim(ctx context.Context, ch domain.Channel, limit int, now time.Time, leaseTTL time.Duration) ([]delivery.Job, error) {
	rows, err := s.pool.Query(ctx, `
		WITH due AS (
			SELECT id FROM notifications
			WHERE channel = $1
			  AND status IN ('scheduled', 'queued', 'retrying')
			  AND next_attempt_at <= $2
			ORDER BY priority_rank, next_attempt_at
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		UPDATE notifications n
		SET status = 'processing',
		    attempts = n.attempts + 1,
		    lease_id = gen_random_uuid(),
		    lease_expires_at = $4,
		    updated_at = $2
		FROM due
		WHERE n.id = due.id
		RETURNING `+qualified("n", notificationColumns)+`, n.lease_id, n.lease_expires_at`,
		ch, now, limit, now.Add(leaseTTL))
	if err != nil {
		return nil, fmt.Errorf("claim notifications: %w", err)
	}
	jobs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (delivery.Job, error) {
		var job delivery.Job
		n, err := scanNotification(row, &job.LeaseID, &job.LeaseExpiresAt)
		job.Notification = n
		return job, err
	})
	if err != nil {
		return nil, fmt.Errorf("claim notifications: %w", err)
	}

	// UPDATE ... RETURNING does not preserve the CTE's order.
	slices.SortFunc(jobs, func(a, b delivery.Job) int {
		return cmp.Or(
			cmp.Compare(a.Notification.Priority.Rank(), b.Notification.Priority.Rank()),
			a.Notification.NextAttemptAt.Compare(b.Notification.NextAttemptAt),
		)
	})
	return jobs, nil
}

// Complete records an attempt and the resulting status, but only while the
// job's lease is still held. The lease ID acts as a fencing token: a worker
// that stalled past its lease cannot overwrite the work of the worker that
// took the notification over.
func (s *DeliveryStore) Complete(ctx context.Context, job delivery.Job, result delivery.Result) error {
	return inTx(ctx, s.pool, func(tx pgx.Tx) error {
		var next *time.Time
		if result.Status == domain.StatusRetrying {
			next = &result.NextAttemptAt
		}
		n, err := scanNotification(tx.QueryRow(ctx, `
			UPDATE notifications
			SET status = $3::text,
			    updated_at = $4,
			    lease_id = NULL,
			    lease_expires_at = NULL,
			    next_attempt_at = COALESCE($5, next_attempt_at),
			    last_error = $6,
			    provider_message_id = COALESCE($7, provider_message_id),
			    sent_at = CASE WHEN $3::text = 'sent' THEN $4 ELSE sent_at END,
			    failed_at = CASE WHEN $3::text = 'failed' THEN $4 ELSE failed_at END
			WHERE id = $1 AND lease_id = $2
			RETURNING `+notificationColumns,
			job.Notification.ID, job.LeaseID, result.Status, result.At, next,
			nullable(result.Error), nullable(result.ProviderMessageID)))
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("complete notification %s: %w", job.Notification.ID, domain.ErrLeaseLost)
		}
		if err != nil {
			return fmt.Errorf("complete notification: %w", err)
		}

		a := result.Attempt
		var statusCode *int
		if a.StatusCode != 0 {
			statusCode = &a.StatusCode
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO delivery_attempts (notification_id, attempt_number, channel, outcome, status_code,
			                               error, provider_message_id, duration_ms, started_at, finished_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			a.NotificationID, a.Number, a.Channel, a.Outcome, statusCode, nullable(a.Error),
			nullable(a.ProviderMessageID), float64(a.Duration().Microseconds())/1000, a.StartedAt, a.FinishedAt,
		); err != nil {
			return fmt.Errorf("record delivery attempt: %w", err)
		}
		return publish(ctx, tx, domain.NotificationEvent(domain.EventNotificationUpdated, n))
	})
}

// Release returns a claimed notification to the queue without counting the
// attempt, for example because the circuit opened after it was claimed.
func (s *DeliveryStore) Release(ctx context.Context, job delivery.Job, availableAt time.Time) error {
	return inTx(ctx, s.pool, func(tx pgx.Tx) error {
		n, err := scanNotification(tx.QueryRow(ctx, `
			UPDATE notifications
			SET status = CASE WHEN attempts > 1 THEN 'retrying' ELSE 'queued' END,
			    attempts = attempts - 1,
			    next_attempt_at = $3,
			    lease_id = NULL,
			    lease_expires_at = NULL,
			    updated_at = now()
			WHERE id = $1 AND lease_id = $2
			RETURNING `+notificationColumns,
			job.Notification.ID, job.LeaseID, availableAt))
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("release notification %s: %w", job.Notification.ID, domain.ErrLeaseLost)
		}
		if err != nil {
			return fmt.Errorf("release notification: %w", err)
		}
		return publish(ctx, tx, domain.NotificationEvent(domain.EventNotificationUpdated, n))
	})
}

// RecoverExpiredLeases requeues notifications whose worker died while
// delivering them. Whether the provider got the message is unknown, so the
// lost attempt is recorded as a retryable failure; a notification that was
// on its last attempt fails instead of retrying forever.
func (s *DeliveryStore) RecoverExpiredLeases(ctx context.Context, now time.Time) (int, error) {
	var recovered int
	err := inTx(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			WITH expired AS (
				SELECT id FROM notifications
				WHERE status = 'processing' AND lease_expires_at < $1
				FOR UPDATE SKIP LOCKED
			)
			UPDATE notifications n
			SET status = CASE WHEN n.attempts >= n.max_attempts THEN 'failed' ELSE 'retrying' END,
			    failed_at = CASE WHEN n.attempts >= n.max_attempts THEN $1 ELSE n.failed_at END,
			    last_error = $2,
			    next_attempt_at = $1,
			    lease_id = NULL,
			    lease_expires_at = NULL,
			    updated_at = $1
			FROM expired
			WHERE n.id = expired.id
			RETURNING `+qualified("n", notificationColumns), now, leaseLostError)
		if err != nil {
			return fmt.Errorf("recover expired leases: %w", err)
		}
		updated, err := pgx.CollectRows(rows, scanNotificationRow)
		if err != nil {
			return fmt.Errorf("recover expired leases: %w", err)
		}
		if len(updated) == 0 {
			return nil
		}
		recovered = len(updated)

		ids := make([]uuid.UUID, len(updated))
		numbers := make([]int32, len(updated))
		channels := make([]string, len(updated))
		events := make([]domain.Event, len(updated))
		for i, n := range updated {
			ids[i], numbers[i], channels[i] = n.ID, int32(n.Attempts), string(n.Channel) //nolint:gosec // attempts are small
			events[i] = domain.NotificationEvent(domain.EventNotificationUpdated, n)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO delivery_attempts (notification_id, attempt_number, channel, outcome, error,
			                               duration_ms, started_at, finished_at)
			SELECT u.id, u.number, u.channel, 'retryable_failure', $4, 0, $5, $5
			FROM unnest($1::uuid[], $2::int[], $3::text[]) AS u(id, number, channel)
			ON CONFLICT (notification_id, attempt_number) DO NOTHING`,
			ids, numbers, channels, leaseLostError, now); err != nil {
			return fmt.Errorf("record lost attempts: %w", err)
		}
		return publish(ctx, tx, events...)
	})
	return recovered, err
}

// Heartbeats implements delivery.Heartbeats on the worker_heartbeats table.
type Heartbeats struct {
	pool *pgxpool.Pool
}

// NewHeartbeats returns a heartbeat store using pool.
func NewHeartbeats(pool *pgxpool.Pool) *Heartbeats {
	return &Heartbeats{pool: pool}
}

var _ delivery.Heartbeats = (*Heartbeats)(nil)

type circuitJSON struct {
	State   string     `json:"state"`
	RetryAt *time.Time `json:"retry_at,omitempty"`
}

// Beat upserts the worker's status and prunes workers that went silent long ago.
func (h *Heartbeats) Beat(ctx context.Context, hb delivery.Heartbeat) error {
	circuits := make(map[string]circuitJSON, len(hb.Circuits))
	for ch, c := range hb.Circuits {
		cj := circuitJSON{State: c.State.String()}
		if !c.RetryAt.IsZero() {
			at := c.RetryAt.UTC()
			cj.RetryAt = &at
		}
		circuits[string(ch)] = cj
	}
	encoded, err := json.Marshal(circuits)
	if err != nil {
		return fmt.Errorf("encode circuits: %w", err)
	}

	batch := &pgx.Batch{}
	batch.Queue(`
		INSERT INTO worker_heartbeats (worker_id, started_at, seen_at, in_flight, circuits)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (worker_id) DO UPDATE
		SET seen_at = EXCLUDED.seen_at, in_flight = EXCLUDED.in_flight, circuits = EXCLUDED.circuits`,
		hb.WorkerID, hb.StartedAt, hb.SeenAt, hb.InFlight, encoded)
	batch.Queue(`DELETE FROM worker_heartbeats WHERE seen_at < $1`, hb.SeenAt.Add(-heartbeatRetention))
	if err := h.pool.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("publish heartbeat: %w", err)
	}
	return nil
}

// Remove deletes a worker's heartbeat when it shuts down cleanly.
func (h *Heartbeats) Remove(ctx context.Context, workerID string) error {
	if _, err := h.pool.Exec(ctx, `DELETE FROM worker_heartbeats WHERE worker_id = $1`, workerID); err != nil {
		return fmt.Errorf("remove heartbeat: %w", err)
	}
	return nil
}

// qualified prefixes every column in a comma-separated list with alias.
func qualified(alias, columns string) string {
	var out []byte
	for i, col := range splitColumns(columns) {
		if i > 0 {
			out = append(out, ", "...)
		}
		out = append(out, alias...)
		out = append(out, '.')
		out = append(out, col...)
	}
	return string(out)
}

func splitColumns(columns string) []string {
	var (
		cols    []string
		current []byte
	)
	flush := func() {
		if len(current) > 0 {
			cols = append(cols, string(current))
			current = current[:0]
		}
	}
	for i := range len(columns) {
		switch c := columns[i]; c {
		case ',':
			flush()
		case ' ', '\n', '\t':
		default:
			current = append(current, c)
		}
	}
	flush()
	return cols
}
