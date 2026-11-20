// Package service implements the application's use cases: creating, querying,
// cancelling and retrying notifications, managing templates and reading
// metrics. It orchestrates the domain model and talks to storage only through
// the ports (interfaces) declared in this file, which keeps the use cases
// independent of PostgreSQL and testable with in-memory fakes.
package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/RamizUnal/notification-system/internal/domain"
)

// NotificationRepository persists notifications.
type NotificationRepository interface {
	// Create stores n. When another notification already uses n's
	// idempotency key, Create stores nothing and returns that notification
	// with created set to false.
	Create(ctx context.Context, n domain.Notification) (stored domain.Notification, created bool, err error)
	Get(ctx context.Context, id uuid.UUID) (domain.Notification, error)
	FindByIdempotencyKey(ctx context.Context, key string) (domain.Notification, bool, error)
	List(ctx context.Context, filter ListFilter) ([]domain.Notification, error)
	// Cancel moves a waiting notification to cancelled. It fails with
	// domain.ErrNotCancellable when the notification is no longer waiting.
	Cancel(ctx context.Context, id uuid.UUID, at time.Time) (domain.Notification, error)
	// Requeue moves a failed notification back to the queue and grants it
	// extraAttempts further delivery attempts. It fails with
	// domain.ErrNotRetryable when the notification has not failed.
	Requeue(ctx context.Context, id uuid.UUID, at time.Time, extraAttempts int) (domain.Notification, error)
	Attempts(ctx context.Context, id uuid.UUID) ([]domain.DeliveryAttempt, error)
}

// BatchRepository persists batches together with their notifications.
type BatchRepository interface {
	// Create stores the batch and all its items in one transaction. When
	// another batch already uses b's idempotency key, Create stores nothing
	// and returns that batch with created set to false. When an item's
	// idempotency key is taken, it fails with a
	// *domain.IdempotencyConflictError and stores nothing.
	Create(ctx context.Context, b domain.Batch, items []domain.Notification) (stored domain.Batch, created bool, err error)
	FindByIdempotencyKey(ctx context.Context, key string) (domain.Batch, bool, error)
	Summary(ctx context.Context, id uuid.UUID) (domain.BatchSummary, error)
	// NotificationIDs returns the batch's notification IDs in request order.
	NotificationIDs(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error)
	// Cancel cancels every waiting notification of the batch and returns
	// how many it cancelled.
	Cancel(ctx context.Context, id uuid.UUID, at time.Time) (int, error)
}

// TemplateRepository persists message templates.
type TemplateRepository interface {
	// Create fails with domain.ErrTemplateNameTaken for a duplicate name.
	Create(ctx context.Context, t domain.Template) (domain.Template, error)
	Get(ctx context.Context, id uuid.UUID) (domain.Template, error)
	// GetMany returns the templates that exist among ids, keyed by ID.
	GetMany(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Template, error)
	List(ctx context.Context) ([]domain.Template, error)
	// Update fails with domain.ErrTemplateNameTaken for a duplicate name.
	Update(ctx context.Context, t domain.Template) (domain.Template, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// ListFilter selects notifications. Zero-valued fields do not filter.
type ListFilter struct {
	Statuses    []domain.Status
	Channels    []domain.Channel
	Priorities  []domain.Priority
	BatchID     *uuid.UUID
	Recipient   string
	CreatedFrom *time.Time // inclusive
	CreatedTo   *time.Time // exclusive
	// After continues a previous listing: only notifications older than the
	// cursor position are returned.
	After *Cursor
	Limit int
}

// Cursor is a keyset pagination position: listings are ordered by creation
// time, newest first, with the ID breaking ties. Unlike offsets, a cursor
// stays correct while new notifications keep arriving.
type Cursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// Page is one page of a listing. Next is nil on the last page.
type Page[T any] struct {
	Items []T
	Next  *Cursor
}
