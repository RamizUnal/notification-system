package httpapi_test

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/service"
)

// fakeNotifications lets each test script only the calls it cares about.
type fakeNotifications struct {
	create      func(service.NotificationInput) (service.CreateResult, error)
	createBatch func(service.BatchInput) (service.BatchResult, error)
	get         func(uuid.UUID) (domain.Notification, error)
	list        func(service.ListFilter) (service.Page[domain.Notification], error)
	cancel      func(uuid.UUID) (domain.Notification, error)
	retry       func(uuid.UUID) (domain.Notification, error)
	attempts    func(uuid.UUID) ([]domain.DeliveryAttempt, error)
	batch       func(uuid.UUID) (domain.BatchSummary, error)
	cancelBatch func(uuid.UUID) (int, error)
}

func notFound(kind string, id uuid.UUID) error { return domain.NotFound(kind, id.String()) }

func (f *fakeNotifications) Create(_ context.Context, in service.NotificationInput) (service.CreateResult, error) {
	return f.create(in)
}

func (f *fakeNotifications) CreateBatch(_ context.Context, in service.BatchInput) (service.BatchResult, error) {
	return f.createBatch(in)
}

func (f *fakeNotifications) Get(_ context.Context, id uuid.UUID) (domain.Notification, error) {
	if f.get == nil {
		return domain.Notification{}, notFound("notification", id)
	}
	return f.get(id)
}

func (f *fakeNotifications) List(_ context.Context, filter service.ListFilter) (service.Page[domain.Notification], error) {
	return f.list(filter)
}

func (f *fakeNotifications) Cancel(_ context.Context, id uuid.UUID) (domain.Notification, error) {
	return f.cancel(id)
}

func (f *fakeNotifications) Retry(_ context.Context, id uuid.UUID) (domain.Notification, error) {
	return f.retry(id)
}

func (f *fakeNotifications) Attempts(_ context.Context, id uuid.UUID) ([]domain.DeliveryAttempt, error) {
	return f.attempts(id)
}

func (f *fakeNotifications) Batch(_ context.Context, id uuid.UUID) (domain.BatchSummary, error) {
	if f.batch == nil {
		return domain.BatchSummary{}, notFound("batch", id)
	}
	return f.batch(id)
}

func (f *fakeNotifications) CancelBatch(_ context.Context, id uuid.UUID) (int, error) {
	return f.cancelBatch(id)
}

type fakeTemplates struct {
	created domain.Template
	err     error
}

func (f *fakeTemplates) Create(_ context.Context, d domain.TemplateDraft) (domain.Template, error) {
	if f.err != nil {
		return domain.Template{}, f.err
	}
	f.created = domain.Template{ID: uuid.New(), Name: d.Name, Channel: d.Channel, Body: d.Body}
	return f.created, nil
}

func (f *fakeTemplates) Update(context.Context, uuid.UUID, domain.TemplateDraft) (domain.Template, error) {
	return f.created, f.err
}

func (f *fakeTemplates) Get(_ context.Context, id uuid.UUID) (domain.Template, error) {
	return domain.Template{}, notFound("template", id)
}

func (f *fakeTemplates) List(context.Context) ([]domain.Template, error) {
	return []domain.Template{f.created}, nil
}

func (f *fakeTemplates) Delete(context.Context, uuid.UUID) error { return f.err }

func (f *fakeTemplates) Render(context.Context, uuid.UUID, map[string]string) (string, error) {
	return "Hi Ada", f.err
}

type fakeMetrics struct{ window time.Duration }

func (f *fakeMetrics) Snapshot(_ context.Context, window time.Duration) (service.MetricsSnapshot, error) {
	f.window = window
	return service.MetricsSnapshot{GeneratedAt: time.Now(), Window: window}, nil
}

func sampleNotification() domain.Notification {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	return domain.Notification{
		ID: uuid.MustParse("0193a0b2-7c1e-7d4f-9a51-2b3c4d5e6f70"), Channel: domain.ChannelSMS,
		Priority: domain.PriorityHigh, Recipient: "+905551234567", Content: "hello",
		Status: domain.StatusQueued, MaxAttempts: 5, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
		TraceParent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	}
}
