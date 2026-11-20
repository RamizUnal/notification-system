package postgres

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/service"
)

// BatchRepository implements service.BatchRepository.
type BatchRepository struct {
	pool *pgxpool.Pool
}

// NewBatchRepository returns a repository using pool.
func NewBatchRepository(pool *pgxpool.Pool) *BatchRepository {
	return &BatchRepository{pool: pool}
}

var _ service.BatchRepository = (*BatchRepository)(nil)

const batchColumns = `id, size, idempotency_key, fingerprint, created_at`

func scanBatch(row pgx.Row) (domain.Batch, error) {
	var (
		b   domain.Batch
		key *string
	)
	if err := row.Scan(&b.ID, &b.Size, &key, &b.Fingerprint, &b.CreatedAt); err != nil {
		return domain.Batch{}, err
	}
	b.IdempotencyKey = deref(key)
	b.CreatedAt = b.CreatedAt.UTC()
	return b, nil
}

// Create stores the batch and every item in one transaction. Items are
// inserted with a single INSERT ... SELECT FROM unnest(...) statement, so a
// batch of 1,000 costs one round trip instead of 1,000.
func (r *BatchRepository) Create(ctx context.Context, b domain.Batch, items []domain.Notification) (domain.Batch, bool, error) {
	var (
		stored  domain.Batch
		created bool
	)
	err := inTx(ctx, r.pool, func(tx pgx.Tx) error {
		s, err := scanBatch(tx.QueryRow(ctx, `
			INSERT INTO batches (`+batchColumns+`) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
			RETURNING `+batchColumns,
			b.ID, b.Size, nullable(b.IdempotencyKey), b.Fingerprint, b.CreatedAt))
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			existing, err := scanBatch(tx.QueryRow(ctx,
				`SELECT `+batchColumns+` FROM batches WHERE idempotency_key = $1`, b.IdempotencyKey))
			if err != nil {
				return fmt.Errorf("load batch for idempotency key: %w", err)
			}
			stored = existing
			return nil
		case err != nil:
			return fmt.Errorf("insert batch: %w", err)
		}

		inserted, err := insertItems(ctx, tx, b, items)
		if err != nil {
			return err
		}
		if inserted < len(items) {
			// Some item keys belong to existing notifications. Report them
			// and roll back: a batch is accepted whole or not at all.
			return keyConflicts(ctx, tx, b.ID, items)
		}

		stored, created = s, true
		return publish(ctx, tx, domain.Event{
			Type: domain.EventBatchCreated, BatchID: &s.ID, BatchSize: s.Size, OccurredAt: s.CreatedAt,
		})
	})
	return stored, created, err
}

func insertItems(ctx context.Context, tx pgx.Tx, b domain.Batch, items []domain.Notification) (int, error) {
	var (
		ids          = make([]uuid.UUID, len(items))
		templateIDs  = make([]*uuid.UUID, len(items))
		channels     = make([]string, len(items))
		priorities   = make([]string, len(items))
		recipients   = make([]string, len(items))
		contents     = make([]string, len(items))
		statuses     = make([]string, len(items))
		keys         = make([]*string, len(items))
		fingerprints = make([]string, len(items))
		maxAttempts  = make([]int32, len(items))
		scheduled    = make([]*time.Time, len(items))
		nextAttempts = make([]time.Time, len(items))
	)
	for i, n := range items {
		ids[i], templateIDs[i] = n.ID, n.TemplateID
		channels[i], priorities[i] = string(n.Channel), string(n.Priority)
		recipients[i], contents[i], statuses[i] = n.Recipient, n.Content, string(n.Status)
		keys[i], fingerprints[i] = nullable(n.IdempotencyKey), n.Fingerprint
		maxAttempts[i] = int32(n.MaxAttempts) //nolint:gosec // bounded by configuration
		scheduled[i], nextAttempts[i] = n.ScheduledAt, n.NextAttemptAt
	}

	var correlationID, traceParent string
	if len(items) > 0 {
		correlationID, traceParent = items[0].CorrelationID, items[0].TraceParent
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO notifications (`+insertColumns+`)
		SELECT u.id, $1, u.template_id, u.channel, u.priority, u.recipient, u.content, u.status,
		       u.idempotency_key, u.fingerprint, 0, u.max_attempts, u.scheduled_at, u.next_attempt_at,
		       $2, $3, $4, $4
		FROM unnest($5::uuid[], $6::uuid[], $7::text[], $8::text[], $9::text[], $10::text[], $11::text[],
		            $12::text[], $13::text[], $14::int[], $15::timestamptz[], $16::timestamptz[])
		     AS u(id, template_id, channel, priority, recipient, content, status,
		          idempotency_key, fingerprint, max_attempts, scheduled_at, next_attempt_at)
		ON CONFLICT (idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING`,
		b.ID, nullable(correlationID), nullable(traceParent), b.CreatedAt,
		ids, templateIDs, channels, priorities, recipients, contents, statuses,
		keys, fingerprints, maxAttempts, scheduled, nextAttempts)
	if err != nil {
		return 0, fmt.Errorf("insert batch items: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// keyConflicts builds the error listing every item whose idempotency key
// belongs to a notification outside this batch.
func keyConflicts(ctx context.Context, tx pgx.Tx, batchID uuid.UUID, items []domain.Notification) error {
	indexByKey := make(map[string]int)
	var keys []string
	for i, n := range items {
		if n.IdempotencyKey != "" {
			indexByKey[n.IdempotencyKey] = i
			keys = append(keys, n.IdempotencyKey)
		}
	}

	rows, err := tx.Query(ctx, `
		SELECT idempotency_key, id FROM notifications
		WHERE idempotency_key = ANY($1) AND batch_id IS DISTINCT FROM $2`, keys, batchID)
	if err != nil {
		return fmt.Errorf("find conflicting idempotency keys: %w", err)
	}
	conflicts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.KeyConflict, error) {
		var c domain.KeyConflict
		err := row.Scan(&c.Key, &c.ExistingID)
		c.Index = indexByKey[c.Key]
		return c, err
	})
	if err != nil {
		return fmt.Errorf("find conflicting idempotency keys: %w", err)
	}
	slices.SortFunc(conflicts, func(a, b domain.KeyConflict) int { return cmp.Compare(a.Index, b.Index) })
	return &domain.IdempotencyConflictError{Conflicts: conflicts}
}

// FindByIdempotencyKey returns the batch that owns key, if any.
func (r *BatchRepository) FindByIdempotencyKey(ctx context.Context, key string) (domain.Batch, bool, error) {
	b, err := scanBatch(r.pool.QueryRow(ctx, `SELECT `+batchColumns+` FROM batches WHERE idempotency_key = $1`, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Batch{}, false, nil
	}
	if err != nil {
		return domain.Batch{}, false, fmt.Errorf("find batch by idempotency key: %w", err)
	}
	return b, true, nil
}

// Summary returns the batch with the number of its notifications per status.
func (r *BatchRepository) Summary(ctx context.Context, id uuid.UUID) (domain.BatchSummary, error) {
	b, err := scanBatch(r.pool.QueryRow(ctx, `SELECT `+batchColumns+` FROM batches WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.BatchSummary{}, domain.NotFound("batch", id.String())
	}
	if err != nil {
		return domain.BatchSummary{}, fmt.Errorf("get batch: %w", err)
	}

	rows, err := r.pool.Query(ctx, `SELECT status, count(*) FROM notifications WHERE batch_id = $1 GROUP BY status`, id)
	if err != nil {
		return domain.BatchSummary{}, fmt.Errorf("count batch statuses: %w", err)
	}
	counts := make(map[domain.Status]int)
	var (
		status string
		count  int
	)
	_, err = pgx.ForEachRow(rows, []any{&status, &count}, func() error {
		counts[domain.Status(status)] = count
		return nil
	})
	if err != nil {
		return domain.BatchSummary{}, fmt.Errorf("count batch statuses: %w", err)
	}
	return domain.BatchSummary{Batch: b, Counts: counts}, nil
}

// NotificationIDs returns the batch's notification IDs in request order.
// The service mints them in ascending order, so ID order is request order.
func (r *BatchRepository) NotificationIDs(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `SELECT id FROM notifications WHERE batch_id = $1 ORDER BY id`, id)
	if err != nil {
		return nil, fmt.Errorf("list batch notification IDs: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, fmt.Errorf("list batch notification IDs: %w", err)
	}
	return ids, nil
}

// Cancel cancels every waiting notification of the batch.
func (r *BatchRepository) Cancel(ctx context.Context, id uuid.UUID, at time.Time) (int, error) {
	var cancelled int
	err := inTx(ctx, r.pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM batches WHERE id = $1)`, id).Scan(&exists); err != nil {
			return fmt.Errorf("check batch: %w", err)
		}
		if !exists {
			return domain.NotFound("batch", id.String())
		}

		rows, err := tx.Query(ctx, `
			UPDATE notifications
			SET status = 'cancelled', cancelled_at = $2, updated_at = $2
			WHERE batch_id = $1 AND status IN ('scheduled', 'queued', 'retrying')
			RETURNING `+notificationColumns, id, at)
		if err != nil {
			return fmt.Errorf("cancel batch: %w", err)
		}
		updated, err := pgx.CollectRows(rows, scanNotificationRow)
		if err != nil {
			return fmt.Errorf("cancel batch: %w", err)
		}

		cancelled = len(updated)
		events := make([]domain.Event, len(updated))
		for i, n := range updated {
			events[i] = domain.NotificationEvent(domain.EventNotificationUpdated, n)
		}
		return publish(ctx, tx, events...)
	})
	return cancelled, err
}
