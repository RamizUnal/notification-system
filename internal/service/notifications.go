package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/observability"
)

// Listing limits.
const (
	DefaultPageSize = 25
	MaxPageSize     = 100
)

// NotificationInput describes one notification to create. Exactly one of
// Content and TemplateID must be set; Variables fill the template's
// placeholders. When a template is used, Channel may be left empty and is
// taken from the template.
type NotificationInput struct {
	Channel        domain.Channel
	Priority       domain.Priority
	Recipient      string
	Content        string
	TemplateID     *uuid.UUID
	Variables      map[string]string
	ScheduledAt    *time.Time
	IdempotencyKey string
}

// CreateResult is the outcome of creating one notification.
type CreateResult struct {
	Notification domain.Notification
	// Replayed is true when an earlier request with the same idempotency key
	// already created the notification, so nothing new was stored.
	Replayed bool
}

// BatchInput describes a batch of notifications created together.
type BatchInput struct {
	Items []NotificationInput
	// IdempotencyKey makes retrying the whole batch request safe.
	IdempotencyKey string
}

// BatchResult is the outcome of creating a batch.
type BatchResult struct {
	Summary domain.BatchSummary
	// NotificationIDs are aligned with the request: the i-th ID belongs to
	// the i-th input.
	NotificationIDs []uuid.UUID
	// Replayed is true when an earlier request with the same idempotency key
	// already created the batch.
	Replayed bool
}

// NotificationService implements the notification use cases.
type NotificationService struct {
	settings
	notifications NotificationRepository
	batches       BatchRepository
	templates     TemplateRepository
}

// NewNotificationService wires the service to its repositories.
func NewNotificationService(
	notifications NotificationRepository,
	batches BatchRepository,
	templates TemplateRepository,
	opts ...Option,
) *NotificationService {
	return &NotificationService{
		settings:      newSettings(opts),
		notifications: notifications,
		batches:       batches,
		templates:     templates,
	}
}

// Create validates and stores one notification, which then waits in the
// queue until a worker delivers it.
//
// With an idempotency key, retrying the same request returns the
// notification created the first time (Replayed is set); reusing the key for
// a different payload fails with domain.ErrIdempotencyConflict.
func (s *NotificationService) Create(ctx context.Context, in NotificationInput) (CreateResult, error) {
	templates, err := s.templatesFor(ctx, []NotificationInput{in})
	if err != nil {
		return CreateResult{}, err
	}
	draft, err := resolveDraft(in, templates)
	if err != nil {
		return CreateResult{}, err
	}
	draft = draft.Normalize()

	// Answer a retried request from the original result before validating:
	// a schedule that was valid on the first attempt may lie in the past now.
	if draft.IdempotencyKey != "" {
		existing, found, err := s.notifications.FindByIdempotencyKey(ctx, draft.IdempotencyKey)
		if err != nil {
			return CreateResult{}, fmt.Errorf("look up idempotency key: %w", err)
		}
		if found {
			return replayNotification(existing, draft.Fingerprint())
		}
	}

	n, err := domain.NewNotification(s.newID(), draft, s.now(), s.maxAttempts)
	if err != nil {
		return CreateResult{}, err
	}
	n.CorrelationID = observability.CorrelationID(ctx)
	n.TraceParent = observability.TraceParent(ctx)

	stored, created, err := s.notifications.Create(ctx, n)
	if err != nil {
		return CreateResult{}, fmt.Errorf("store notification: %w", err)
	}
	if !created {
		// A concurrent request with the same key won the race.
		return replayNotification(stored, n.Fingerprint)
	}
	return CreateResult{Notification: stored}, nil
}

// CreateBatch validates and stores up to domain.MaxBatchSize notifications
// atomically: either every item is accepted or none is. Validation problems
// of all items are reported together, with field paths such as
// "notifications[3].recipient".
func (s *NotificationService) CreateBatch(ctx context.Context, in BatchInput) (BatchResult, error) {
	var verr domain.ValidationError
	switch {
	case len(in.Items) == 0:
		verr.Add("notifications", domain.CodeRequired, "a batch needs at least one notification")
	case len(in.Items) > domain.MaxBatchSize:
		verr.Add("notifications", domain.CodeOutOfRange, fmt.Sprintf(
			"a batch may hold at most %d notifications, got %d", domain.MaxBatchSize, len(in.Items)))
	}
	if in.IdempotencyKey != "" && !domain.ValidIdempotencyKey(in.IdempotencyKey) {
		verr.Add("Idempotency-Key", domain.CodeInvalidFormat, fmt.Sprintf(
			"the Idempotency-Key header must be 1-%d visible ASCII characters", domain.MaxIdempotencyKeyLength))
	}
	if err := verr.Err(); err != nil {
		return BatchResult{}, err
	}

	templates, err := s.templatesFor(ctx, in.Items)
	if err != nil {
		return BatchResult{}, err
	}

	drafts := make([]domain.Draft, len(in.Items))
	fingerprints := make([]string, len(in.Items))
	for i, item := range in.Items {
		draft, err := resolveDraft(item, templates)
		if err != nil {
			if !mergeValidation(&verr, itemPath(i), err) {
				return BatchResult{}, err
			}
			continue
		}
		drafts[i] = draft.Normalize()
		fingerprints[i] = drafts[i].Fingerprint()
	}
	if err := verr.Err(); err != nil {
		return BatchResult{}, err
	}
	fingerprint := domain.BatchFingerprint(fingerprints)

	if in.IdempotencyKey != "" {
		existing, found, err := s.batches.FindByIdempotencyKey(ctx, in.IdempotencyKey)
		if err != nil {
			return BatchResult{}, fmt.Errorf("look up batch idempotency key: %w", err)
		}
		if found {
			return s.replayBatch(ctx, existing, fingerprint)
		}
	}

	now := s.now()
	batchID := s.newID()
	ids := s.newIDs(len(drafts))
	correlationID, traceParent := observability.CorrelationID(ctx), observability.TraceParent(ctx)
	items := make([]domain.Notification, 0, len(drafts))
	keys := make(map[string]int)

	for i, draft := range drafts {
		n, err := domain.NewNotification(ids[i], draft, now, s.maxAttempts)
		if err != nil {
			if !mergeValidation(&verr, itemPath(i), err) {
				return BatchResult{}, err
			}
			continue
		}
		if key := n.IdempotencyKey; key != "" {
			if first, dup := keys[key]; dup {
				verr.Add(itemPath(i)+".idempotency_key", domain.CodeDuplicate,
					fmt.Sprintf("%s already uses this idempotency_key", itemPath(first)))
				continue
			}
			keys[key] = i
		}
		n.BatchID = &batchID
		n.CorrelationID = correlationID
		n.TraceParent = traceParent
		items = append(items, n)
	}
	if err := verr.Err(); err != nil {
		return BatchResult{}, err
	}

	batch := domain.Batch{
		ID:             batchID,
		Size:           len(items),
		IdempotencyKey: in.IdempotencyKey,
		Fingerprint:    fingerprint,
		CreatedAt:      now.UTC(),
	}
	stored, created, err := s.batches.Create(ctx, batch, items)
	if err != nil {
		return BatchResult{}, fmt.Errorf("store batch: %w", err)
	}
	if !created {
		return s.replayBatch(ctx, stored, fingerprint)
	}

	counts := make(map[domain.Status]int)
	for _, n := range items {
		counts[n.Status]++
	}
	return BatchResult{
		Summary:         domain.BatchSummary{Batch: stored, Counts: counts},
		NotificationIDs: ids,
	}, nil
}

// Get returns one notification.
func (s *NotificationService) Get(ctx context.Context, id uuid.UUID) (domain.Notification, error) {
	return s.notifications.Get(ctx, id)
}

// List returns one page of notifications, newest first.
func (s *NotificationService) List(ctx context.Context, f ListFilter) (Page[domain.Notification], error) {
	var verr domain.ValidationError
	if f.Limit == 0 {
		f.Limit = DefaultPageSize
	}
	if f.Limit < 1 || f.Limit > MaxPageSize {
		verr.Add("limit", domain.CodeOutOfRange, fmt.Sprintf("limit must be between 1 and %d", MaxPageSize))
	}
	if f.CreatedFrom != nil && f.CreatedTo != nil && !f.CreatedFrom.Before(*f.CreatedTo) {
		verr.Add("created_from", domain.CodeOutOfRange, "created_from must be earlier than created_to")
	}
	if err := verr.Err(); err != nil {
		return Page[domain.Notification]{}, err
	}

	// Fetch one extra row to learn whether another page follows.
	limit := f.Limit
	f.Limit = limit + 1
	items, err := s.notifications.List(ctx, f)
	if err != nil {
		return Page[domain.Notification]{}, fmt.Errorf("list notifications: %w", err)
	}

	page := Page[domain.Notification]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		last := page.Items[limit-1]
		page.Next = &Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

// Cancel stops a notification that is still waiting for delivery.
func (s *NotificationService) Cancel(ctx context.Context, id uuid.UUID) (domain.Notification, error) {
	return s.notifications.Cancel(ctx, id, s.now())
}

// Retry puts a failed notification back in the queue with a fresh budget of
// delivery attempts. This is how operators redrive the dead-letter state.
func (s *NotificationService) Retry(ctx context.Context, id uuid.UUID) (domain.Notification, error) {
	return s.notifications.Requeue(ctx, id, s.now(), s.maxAttempts)
}

// Attempts returns the delivery attempt history of a notification.
func (s *NotificationService) Attempts(ctx context.Context, id uuid.UUID) ([]domain.DeliveryAttempt, error) {
	if _, err := s.notifications.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.notifications.Attempts(ctx, id)
}

// Batch returns a batch with the number of its notifications per status.
func (s *NotificationService) Batch(ctx context.Context, id uuid.UUID) (domain.BatchSummary, error) {
	return s.batches.Summary(ctx, id)
}

// CancelBatch cancels every notification of the batch that is still
// waiting and returns how many were cancelled.
func (s *NotificationService) CancelBatch(ctx context.Context, id uuid.UUID) (int, error) {
	return s.batches.Cancel(ctx, id, s.now())
}

func (s *NotificationService) replayBatch(ctx context.Context, existing domain.Batch, fingerprint string) (BatchResult, error) {
	if existing.Fingerprint != fingerprint {
		return BatchResult{}, fmt.Errorf("batch idempotency key %q: %w", existing.IdempotencyKey, domain.ErrIdempotencyConflict)
	}
	summary, err := s.batches.Summary(ctx, existing.ID)
	if err != nil {
		return BatchResult{}, fmt.Errorf("load replayed batch: %w", err)
	}
	ids, err := s.batches.NotificationIDs(ctx, existing.ID)
	if err != nil {
		return BatchResult{}, fmt.Errorf("load replayed batch items: %w", err)
	}
	return BatchResult{Summary: summary, NotificationIDs: ids, Replayed: true}, nil
}

// templatesFor loads every template the inputs reference with one query.
func (s *NotificationService) templatesFor(ctx context.Context, inputs []NotificationInput) (map[uuid.UUID]domain.Template, error) {
	var ids []uuid.UUID
	seen := make(map[uuid.UUID]bool)
	for _, in := range inputs {
		if in.TemplateID != nil && !seen[*in.TemplateID] {
			seen[*in.TemplateID] = true
			ids = append(ids, *in.TemplateID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	templates, err := s.templates.GetMany(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load templates: %w", err)
	}
	return templates, nil
}

func replayNotification(existing domain.Notification, fingerprint string) (CreateResult, error) {
	if existing.Fingerprint != fingerprint {
		return CreateResult{}, fmt.Errorf("idempotency key %q: %w", existing.IdempotencyKey, domain.ErrIdempotencyConflict)
	}
	return CreateResult{Notification: existing, Replayed: true}, nil
}

// resolveDraft turns an input into a draft, rendering the referenced template
// if there is one. Input problems come back as a *domain.ValidationError.
func resolveDraft(in NotificationInput, templates map[uuid.UUID]domain.Template) (domain.Draft, error) {
	draft := domain.Draft{
		Channel:        in.Channel,
		Priority:       in.Priority,
		Recipient:      in.Recipient,
		Content:        in.Content,
		ScheduledAt:    in.ScheduledAt,
		IdempotencyKey: in.IdempotencyKey,
		TemplateID:     in.TemplateID,
	}

	var verr domain.ValidationError
	if in.TemplateID == nil {
		if len(in.Variables) > 0 {
			verr.Add("variables", domain.CodeInvalidValue, "variables can only be used together with template_id")
		}
		return draft, verr.Err()
	}
	if in.Content != "" {
		verr.Add("content", domain.CodeInvalidValue, "set either content or template_id, not both")
		return draft, verr.Err()
	}

	tpl, ok := templates[*in.TemplateID]
	switch {
	case !ok:
		verr.Add("template_id", domain.CodeInvalidValue, fmt.Sprintf("template %s does not exist", in.TemplateID))
	case in.Channel != "" && in.Channel != tpl.Channel:
		verr.Add("template_id", domain.CodeChannelMismatch,
			fmt.Sprintf("template %q is for %s, not %s", tpl.Name, tpl.Channel, in.Channel))
	}
	if err := verr.Err(); err != nil {
		return draft, err
	}

	content, err := tpl.Render(in.Variables)
	if err != nil {
		var missing *domain.MissingVariablesError
		if errors.As(err, &missing) {
			verr.Add("variables", domain.CodeMissingVariables, err.Error())
			return draft, verr.Err()
		}
		return draft, fmt.Errorf("render template %s: %w", tpl.ID, err)
	}
	draft.Channel = tpl.Channel
	draft.Content = content
	return draft, nil
}

// mergeValidation copies the problems of a validation error into verr under
// prefix and reports whether err was a validation error at all.
func mergeValidation(verr *domain.ValidationError, prefix string, err error) bool {
	var itemErr *domain.ValidationError
	if !errors.As(err, &itemErr) {
		return false
	}
	verr.Merge(prefix, itemErr)
	return true
}

func itemPath(i int) string { return fmt.Sprintf("notifications[%d]", i) }
