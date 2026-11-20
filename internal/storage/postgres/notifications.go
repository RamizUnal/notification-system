package postgres

import (
	"context"
	"errors"
	"fmt"
	stdstrings "strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/service"
)

// NotificationRepository implements service.NotificationRepository.
type NotificationRepository struct {
	pool *pgxpool.Pool
}

// NewNotificationRepository returns a repository using pool.
func NewNotificationRepository(pool *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{pool: pool}
}

var _ service.NotificationRepository = (*NotificationRepository)(nil)

// insertColumns are the columns written when a notification is created.
const insertColumns = `id, batch_id, template_id, channel, priority, recipient, content, status,
	idempotency_key, fingerprint, attempts, max_attempts, scheduled_at, next_attempt_at,
	correlation_id, trace_parent, created_at, updated_at`

// Create stores n, or returns the notification that already owns n's
// idempotency key. ON CONFLICT makes the check and the insert one atomic
// step, so concurrent requests with the same key cannot both insert.
func (r *NotificationRepository) Create(ctx context.Context, n domain.Notification) (domain.Notification, bool, error) {
	var (
		stored  domain.Notification
		created bool
	)
	err := inTx(ctx, r.pool, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO notifications (`+insertColumns+`)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 0, $11, $12, $13, $14, $15, $16, $16)
			ON CONFLICT (idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
			RETURNING `+notificationColumns,
			n.ID, n.BatchID, n.TemplateID, n.Channel, n.Priority, n.Recipient, n.Content, n.Status,
			nullable(n.IdempotencyKey), n.Fingerprint, n.MaxAttempts, n.ScheduledAt, n.NextAttemptAt,
			nullable(n.CorrelationID), nullable(n.TraceParent), n.CreatedAt)

		s, err := scanNotification(row)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// The key is taken; hand back the notification that owns it.
			existing, err := scanNotification(tx.QueryRow(ctx,
				`SELECT `+notificationColumns+` FROM notifications WHERE idempotency_key = $1`, n.IdempotencyKey))
			if err != nil {
				return fmt.Errorf("load notification for idempotency key: %w", err)
			}
			stored = existing
			return nil
		case err != nil:
			return fmt.Errorf("insert notification: %w", err)
		}
		stored, created = s, true
		return publish(ctx, tx, domain.NotificationEvent(domain.EventNotificationCreated, s))
	})
	return stored, created, err
}

// Get returns the notification with the given ID.
func (r *NotificationRepository) Get(ctx context.Context, id uuid.UUID) (domain.Notification, error) {
	n, err := scanNotification(r.pool.QueryRow(ctx,
		`SELECT `+notificationColumns+` FROM notifications WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Notification{}, domain.NotFound("notification", id.String())
	}
	if err != nil {
		return domain.Notification{}, fmt.Errorf("get notification: %w", err)
	}
	return n, nil
}

// FindByIdempotencyKey returns the notification that owns key, if any.
func (r *NotificationRepository) FindByIdempotencyKey(ctx context.Context, key string) (domain.Notification, bool, error) {
	n, err := scanNotification(r.pool.QueryRow(ctx,
		`SELECT `+notificationColumns+` FROM notifications WHERE idempotency_key = $1`, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Notification{}, false, nil
	}
	if err != nil {
		return domain.Notification{}, false, fmt.Errorf("find notification by idempotency key: %w", err)
	}
	return n, true, nil
}

// List returns notifications matching f, newest first. Conditions are added
// only for the filters that are set, so each query stays index-friendly;
// values always travel as bind parameters, never inside the SQL text.
func (r *NotificationRepository) List(ctx context.Context, f service.ListFilter) ([]domain.Notification, error) {
	var (
		where []string
		args  []any
	)
	add := func(condition string, values ...any) {
		placeholders := make([]any, len(values))
		for i, v := range values {
			args = append(args, v)
			placeholders[i] = len(args)
		}
		where = append(where, fmt.Sprintf(condition, placeholders...))
	}

	if len(f.Statuses) > 0 {
		add("status = ANY($%d)", strings(f.Statuses))
	}
	if len(f.Channels) > 0 {
		add("channel = ANY($%d)", strings(f.Channels))
	}
	if len(f.Priorities) > 0 {
		add("priority = ANY($%d)", strings(f.Priorities))
	}
	if f.BatchID != nil {
		add("batch_id = $%d", *f.BatchID)
	}
	if f.Recipient != "" {
		add("recipient = $%d", f.Recipient)
	}
	if f.CreatedFrom != nil {
		add("created_at >= $%d", *f.CreatedFrom)
	}
	if f.CreatedTo != nil {
		add("created_at < $%d", *f.CreatedTo)
	}
	if f.After != nil {
		add("(created_at, id) < ($%d, $%d)", f.After.CreatedAt, f.After.ID)
	}

	query := `SELECT ` + notificationColumns + ` FROM notifications`
	if len(where) > 0 {
		query += ` WHERE ` + stdstrings.Join(where, " AND ")
	}
	args = append(args, f.Limit)
	query += fmt.Sprintf(` ORDER BY created_at DESC, id DESC LIMIT $%d`, len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	items, err := pgx.CollectRows(rows, scanNotificationRow)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	return items, nil
}

// Cancel moves a waiting notification to cancelled. The status condition in
// the UPDATE makes this safe against a worker claiming the row at the same
// moment: exactly one of the two wins.
func (r *NotificationRepository) Cancel(ctx context.Context, id uuid.UUID, at time.Time) (domain.Notification, error) {
	var cancelled domain.Notification
	err := inTx(ctx, r.pool, func(tx pgx.Tx) error {
		n, err := scanNotification(tx.QueryRow(ctx, `
			UPDATE notifications
			SET status = 'cancelled', cancelled_at = $2, updated_at = $2
			WHERE id = $1 AND status IN ('scheduled', 'queued', 'retrying')
			RETURNING `+notificationColumns, id, at))
		if errors.Is(err, pgx.ErrNoRows) {
			return transitionError(ctx, tx, id, domain.ErrNotCancellable)
		}
		if err != nil {
			return fmt.Errorf("cancel notification: %w", err)
		}
		cancelled = n
		return publish(ctx, tx, domain.NotificationEvent(domain.EventNotificationUpdated, n))
	})
	return cancelled, err
}

// Requeue moves a failed notification back to the queue with extra attempts.
func (r *NotificationRepository) Requeue(ctx context.Context, id uuid.UUID, at time.Time, extraAttempts int) (domain.Notification, error) {
	var requeued domain.Notification
	err := inTx(ctx, r.pool, func(tx pgx.Tx) error {
		n, err := scanNotification(tx.QueryRow(ctx, `
			UPDATE notifications
			SET status = 'queued', next_attempt_at = $2, updated_at = $2,
			    max_attempts = attempts + $3, failed_at = NULL, last_error = NULL
			WHERE id = $1 AND status = 'failed'
			RETURNING `+notificationColumns, id, at, extraAttempts))
		if errors.Is(err, pgx.ErrNoRows) {
			return transitionError(ctx, tx, id, domain.ErrNotRetryable)
		}
		if err != nil {
			return fmt.Errorf("requeue notification: %w", err)
		}
		requeued = n
		return publish(ctx, tx, domain.NotificationEvent(domain.EventNotificationUpdated, n))
	})
	return requeued, err
}

// Attempts returns the delivery attempts of a notification, oldest first.
func (r *NotificationRepository) Attempts(ctx context.Context, id uuid.UUID) ([]domain.DeliveryAttempt, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, notification_id, attempt_number, channel, outcome, status_code, error,
		       provider_message_id, started_at, finished_at
		FROM delivery_attempts
		WHERE notification_id = $1
		ORDER BY attempt_number`, id)
	if err != nil {
		return nil, fmt.Errorf("list delivery attempts: %w", err)
	}
	attempts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.DeliveryAttempt, error) {
		var (
			a                   domain.DeliveryAttempt
			channel, outcome    string
			statusCode          *int
			errText, providerID *string
		)
		err := row.Scan(&a.ID, &a.NotificationID, &a.Number, &channel, &outcome, &statusCode, &errText,
			&providerID, &a.StartedAt, &a.FinishedAt)
		a.Channel, a.Outcome = domain.Channel(channel), domain.AttemptOutcome(outcome)
		if statusCode != nil {
			a.StatusCode = *statusCode
		}
		a.Error, a.ProviderMessageID = deref(errText), deref(providerID)
		a.StartedAt, a.FinishedAt = a.StartedAt.UTC(), a.FinishedAt.UTC()
		return a, err
	})
	if err != nil {
		return nil, fmt.Errorf("list delivery attempts: %w", err)
	}
	return attempts, nil
}

// transitionError explains why a conditional update matched no row: either
// the notification does not exist, or its status forbids the transition.
func transitionError(ctx context.Context, tx pgx.Tx, id uuid.UUID, forbidden error) error {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM notifications WHERE id = $1`, id).Scan(&status)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return domain.NotFound("notification", id.String())
	case err != nil:
		return fmt.Errorf("read notification status: %w", err)
	default:
		return fmt.Errorf("notification %s is %s: %w", id, status, forbidden)
	}
}
