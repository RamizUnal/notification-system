package service_test

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/service"
)

// memory is an in-memory implementation of the repository ports, just
// faithful enough to exercise the services' decisions. PostgreSQL behaviour
// itself is covered by the storage integration tests.
type memory struct {
	mu            sync.Mutex
	notifications map[uuid.UUID]domain.Notification
	keys          map[string]uuid.UUID
	batches       map[uuid.UUID]domain.Batch
	batchKeys     map[string]uuid.UUID
	templates     map[uuid.UUID]domain.Template

	// hideKeysFromLookup simulates a concurrent request that stores the same
	// idempotency key between the service's lookup and its insert.
	hideKeysFromLookup bool
	lastFilter         service.ListFilter
	requeueBudget      int
}

func newMemory() *memory {
	return &memory{
		notifications: map[uuid.UUID]domain.Notification{},
		keys:          map[string]uuid.UUID{},
		batches:       map[uuid.UUID]domain.Batch{},
		batchKeys:     map[string]uuid.UUID{},
		templates:     map[uuid.UUID]domain.Template{},
	}
}

func (m *memory) notificationRepo() *memNotifications { return &memNotifications{m} }
func (m *memory) batchRepo() *memBatches              { return &memBatches{m} }
func (m *memory) templateRepo() *memTemplates         { return &memTemplates{m} }

type memNotifications struct{ *memory }

func (r *memNotifications) Create(_ context.Context, n domain.Notification) (domain.Notification, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if id, taken := r.keys[n.IdempotencyKey]; n.IdempotencyKey != "" && taken {
		return r.notifications[id], false, nil
	}
	r.notifications[n.ID] = n
	if n.IdempotencyKey != "" {
		r.keys[n.IdempotencyKey] = n.ID
	}
	return n, true, nil
}

func (r *memNotifications) Get(_ context.Context, id uuid.UUID) (domain.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.notifications[id]
	if !ok {
		return domain.Notification{}, domain.NotFound("notification", id.String())
	}
	return n, nil
}

func (r *memNotifications) FindByIdempotencyKey(_ context.Context, key string) (domain.Notification, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.hideKeysFromLookup {
		return domain.Notification{}, false, nil
	}
	id, ok := r.keys[key]
	return r.notifications[id], ok, nil
}

func (r *memNotifications) List(_ context.Context, f service.ListFilter) ([]domain.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastFilter = f
	all := make([]domain.Notification, 0, len(r.notifications))
	for _, n := range r.notifications {
		all = append(all, n)
	}
	slices.SortFunc(all, func(a, b domain.Notification) int { return bytes.Compare(b.ID[:], a.ID[:]) })
	return all[:min(f.Limit, len(all))], nil
}

func (r *memNotifications) Cancel(_ context.Context, id uuid.UUID, at time.Time) (domain.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.notifications[id]
	switch {
	case !ok:
		return domain.Notification{}, domain.NotFound("notification", id.String())
	case !n.Status.IsWaiting():
		return domain.Notification{}, domain.ErrNotCancellable
	}
	n.Status, n.CancelledAt, n.UpdatedAt = domain.StatusCancelled, &at, at
	r.notifications[id] = n
	return n, nil
}

func (r *memNotifications) Requeue(_ context.Context, id uuid.UUID, at time.Time, extra int) (domain.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requeueBudget = extra
	n, ok := r.notifications[id]
	switch {
	case !ok:
		return domain.Notification{}, domain.NotFound("notification", id.String())
	case n.Status != domain.StatusFailed:
		return domain.Notification{}, domain.ErrNotRetryable
	}
	n.Status, n.MaxAttempts, n.NextAttemptAt = domain.StatusQueued, n.Attempts+extra, at
	r.notifications[id] = n
	return n, nil
}

func (r *memNotifications) Attempts(context.Context, uuid.UUID) ([]domain.DeliveryAttempt, error) {
	return nil, nil
}

type memBatches struct{ *memory }

func (r *memBatches) Create(_ context.Context, b domain.Batch, items []domain.Notification) (domain.Batch, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if id, taken := r.batchKeys[b.IdempotencyKey]; b.IdempotencyKey != "" && taken {
		return r.batches[id], false, nil
	}
	var conflicts []domain.KeyConflict
	for i, n := range items {
		if id, taken := r.keys[n.IdempotencyKey]; n.IdempotencyKey != "" && taken {
			conflicts = append(conflicts, domain.KeyConflict{Index: i, Key: n.IdempotencyKey, ExistingID: id})
		}
	}
	if len(conflicts) > 0 {
		return domain.Batch{}, false, &domain.IdempotencyConflictError{Conflicts: conflicts}
	}
	r.batches[b.ID] = b
	if b.IdempotencyKey != "" {
		r.batchKeys[b.IdempotencyKey] = b.ID
	}
	for _, n := range items {
		r.notifications[n.ID] = n
		if n.IdempotencyKey != "" {
			r.keys[n.IdempotencyKey] = n.ID
		}
	}
	return b, true, nil
}

func (r *memBatches) FindByIdempotencyKey(_ context.Context, key string) (domain.Batch, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.batchKeys[key]
	return r.batches[id], ok, nil
}

func (r *memBatches) Summary(_ context.Context, id uuid.UUID) (domain.BatchSummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.batches[id]
	if !ok {
		return domain.BatchSummary{}, domain.NotFound("batch", id.String())
	}
	counts := map[domain.Status]int{}
	for _, n := range r.notifications {
		if n.BatchID != nil && *n.BatchID == id {
			counts[n.Status]++
		}
	}
	return domain.BatchSummary{Batch: b, Counts: counts}, nil
}

func (r *memBatches) NotificationIDs(_ context.Context, id uuid.UUID) ([]uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []uuid.UUID
	for _, n := range r.notifications {
		if n.BatchID != nil && *n.BatchID == id {
			ids = append(ids, n.ID)
		}
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	return ids, nil
}

func (r *memBatches) Cancel(context.Context, uuid.UUID, time.Time) (int, error) { return 0, nil }

type memTemplates struct{ *memory }

func (r *memTemplates) Create(_ context.Context, t domain.Template) (domain.Template, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.templates {
		if existing.Name == t.Name {
			return domain.Template{}, domain.ErrTemplateNameTaken
		}
	}
	r.templates[t.ID] = t
	return t, nil
}

func (r *memTemplates) Get(_ context.Context, id uuid.UUID) (domain.Template, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.templates[id]
	if !ok {
		return domain.Template{}, domain.NotFound("template", id.String())
	}
	return t, nil
}

func (r *memTemplates) GetMany(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Template, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[uuid.UUID]domain.Template{}
	for _, id := range ids {
		if t, ok := r.templates[id]; ok {
			out[id] = t
		}
	}
	return out, nil
}

func (r *memTemplates) List(context.Context) ([]domain.Template, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.Template, 0, len(r.templates))
	for _, t := range r.templates {
		out = append(out, t)
	}
	return out, nil
}

func (r *memTemplates) Update(_ context.Context, t domain.Template) (domain.Template, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.templates[t.ID] = t
	return t, nil
}

func (r *memTemplates) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.templates, id)
	return nil
}
