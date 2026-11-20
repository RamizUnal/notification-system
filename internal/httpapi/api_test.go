package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/httpapi"
	"github.com/RamizUnal/notification-system/internal/realtime"
	"github.com/RamizUnal/notification-system/internal/service"
)

type harness struct {
	notifications *fakeNotifications
	templates     *fakeTemplates
	metrics       *fakeMetrics
	hub           *realtime.Hub
	ready         map[string]func(context.Context) error
	handler       http.Handler
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		notifications: &fakeNotifications{},
		templates:     &fakeTemplates{},
		metrics:       &fakeMetrics{},
		hub:           realtime.NewHub(),
		ready:         map[string]func(context.Context) error{},
	}
	h.handler = httpapi.NewRouter(httpapi.Config{
		Notifications:   h.notifications,
		Templates:       h.templates,
		Metrics:         h.metrics,
		Hub:             h.hub,
		ReadinessChecks: h.ready,
		OpenAPISpec:     []byte("openapi: 3.0.3"),
	})
	return h
}

func (h *harness) do(method, target, body string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &v), rec.Body.String())
	return v
}

func problem(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) httpapi.Problem {
	t.Helper()
	require.Equal(t, status, rec.Code, rec.Body.String())
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	p := decode[httpapi.Problem](t, rec)
	assert.Equal(t, code, p.Code)
	assert.Equal(t, status, p.Status)
	assert.Equal(t, http.StatusText(status), p.Title)
	assert.NotEmpty(t, p.RequestID, "every problem carries the request ID")
	return p
}

func TestCreateNotification(t *testing.T) {
	t.Parallel()

	t.Run("accepts and maps the request", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		var got service.NotificationInput
		h.notifications.create = func(in service.NotificationInput) (service.CreateResult, error) {
			got = in
			return service.CreateResult{Notification: sampleNotification()}, nil
		}

		rec := h.do(http.MethodPost, "/api/v1/notifications", `{
			"channel": "sms", "recipient": "+905551234567", "content": "hello", "priority": "high",
			"scheduled_at": "2026-11-21T09:00:00Z"
		}`, "Idempotency-Key", "otp-1")

		require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
		assert.Equal(t, "/api/v1/notifications/0193a0b2-7c1e-7d4f-9a51-2b3c4d5e6f70", rec.Header().Get("Location"))
		assert.Equal(t, domain.ChannelSMS, got.Channel)
		assert.Equal(t, domain.PriorityHigh, got.Priority)
		assert.Equal(t, "otp-1", got.IdempotencyKey, "the header becomes the idempotency key")
		assert.Equal(t, time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC), *got.ScheduledAt)

		body := decode[map[string]any](t, rec)
		assert.Equal(t, "queued", body["status"])
		assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", body["trace_id"], "the trace ID comes from the stored traceparent")
		assert.Equal(t, "2026-11-20T12:00:00Z", body["next_attempt_at"])
		assert.Nil(t, body["sent_at"], "absent values are explicit nulls")
	})

	t.Run("answers a replay with 200 and a marker header", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.notifications.create = func(service.NotificationInput) (service.CreateResult, error) {
			return service.CreateResult{Notification: sampleNotification(), Replayed: true}, nil
		}
		rec := h.do(http.MethodPost, "/api/v1/notifications", `{"channel":"sms"}`, "Idempotency-Key", "k")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "true", rec.Header().Get("Idempotent-Replayed"))
	})

	t.Run("reports malformed field values together", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		rec := h.do(http.MethodPost, "/api/v1/notifications",
			`{"template_id": "nope", "scheduled_at": "tomorrow", "idempotency_key": "a"}`, "Idempotency-Key", "b")
		p := problem(t, rec, http.StatusUnprocessableEntity, "validation_failed")
		fields := make([]string, 0, len(p.Errors))
		for _, e := range p.Errors {
			fields = append(fields, e.Field)
		}
		assert.ElementsMatch(t, []string{"template_id", "scheduled_at", "idempotency_key"}, fields)
	})
}

func TestRequestBodyErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		body        string
		contentType string
		status      int
		code        string
		detail      string
	}{
		{"malformed JSON", `{"channel": `, "application/json", 400, "invalid_json", "ends in the middle"},
		{"syntax error", `{"channel" "sms"}`, "application/json", 400, "invalid_json", "not valid JSON"},
		{"unknown field", `{"chanel": "sms"}`, "application/json", 400, "invalid_json", `Unknown field "chanel"`},
		{"wrong type", `{"recipient": 42}`, "application/json", 400, "invalid_json", `"recipient" must be a JSON string`},
		{"empty body", ``, "application/json", 400, "invalid_json", "empty"},
		{"two objects", `{} {}`, "application/json", 400, "invalid_json", "single JSON object"},
		{"form encoded", `channel=sms`, "application/x-www-form-urlencoded", 415, "unsupported_media_type", "Content-Type"},
		{"too large", `{"content": "` + strings.Repeat("x", 2<<20) + `"}`, "application/json", 413, "payload_too_large", "must not exceed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			rec := httptest.NewRecorder()
			h.handler.ServeHTTP(rec, req)

			p := problem(t, rec, tc.status, tc.code)
			assert.Contains(t, p.Detail, tc.detail)
			assert.Equal(t, "/api/v1/notifications", p.Instance)
		})
	}
}

func TestErrorMapping(t *testing.T) {
	t.Parallel()
	id := sampleNotification().ID

	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"validation", func() error {
			v := &domain.ValidationError{}
			v.Add("recipient", domain.CodeInvalidFormat, "recipient must be an E.164 phone number")
			return v
		}(), 422, "validation_failed"},
		{"not found", domain.NotFound("notification", id.String()), 404, "not_found"},
		{"not cancellable", fmt.Errorf("notification is processing: %w", domain.ErrNotCancellable), 409, "not_cancellable"},
		{"idempotency conflict", fmt.Errorf("key k: %w", domain.ErrIdempotencyConflict), 409, "idempotency_conflict"},
		{"unexpected", errors.New("pq: connection reset by peer"), 500, "internal_error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.notifications.cancel = func(uuid.UUID) (domain.Notification, error) { return domain.Notification{}, tc.err }

			rec := h.do(http.MethodPost, "/api/v1/notifications/"+id.String()+"/cancel", "")
			p := problem(t, rec, tc.status, tc.code)
			if tc.status == 500 {
				assert.NotContains(t, p.Detail, "connection reset", "internal details never leak")
			}
		})
	}
}

func TestBatchConflictListsTheOffendingItems(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	existing := uuid.New()
	h.notifications.createBatch = func(service.BatchInput) (service.BatchResult, error) {
		return service.BatchResult{}, fmt.Errorf("store batch: %w", &domain.IdempotencyConflictError{
			Conflicts: []domain.KeyConflict{{Index: 2, Key: "otp-1", ExistingID: existing}},
		})
	}

	rec := h.do(http.MethodPost, "/api/v1/batches", `{"notifications": [{}, {}, {}]}`)

	p := decode[map[string]any](t, rec)
	require.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, "idempotency_conflict", p["code"])
	assert.Equal(t, []any{map[string]any{
		"index": float64(2), "idempotency_key": "otp-1", "notification_id": existing.String(),
	}}, p["conflicts"])
}

func TestCreateBatchAccepts(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	batchID, first := uuid.New(), uuid.New()
	var got service.BatchInput
	h.notifications.createBatch = func(in service.BatchInput) (service.BatchResult, error) {
		got = in
		return service.BatchResult{
			Summary: domain.BatchSummary{
				Batch:  domain.Batch{ID: batchID, Size: 1},
				Counts: map[domain.Status]int{domain.StatusQueued: 1},
			},
			NotificationIDs: []uuid.UUID{first},
		}, nil
	}

	rec := h.do(http.MethodPost, "/api/v1/batches",
		`{"notifications": [{"channel": "push", "recipient": "device-token-0123456789", "content": "hi"}]}`,
		"Idempotency-Key", "campaign-1")

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	assert.Equal(t, "campaign-1", got.IdempotencyKey)
	require.Len(t, got.Items, 1)
	assert.Equal(t, domain.ChannelPush, got.Items[0].Channel)
	body := decode[map[string]any](t, rec)
	assert.Equal(t, []any{first.String()}, body["notification_ids"])
	assert.Equal(t, float64(1), body["pending"])
	assert.Equal(t, false, body["done"])
	assert.Equal(t, float64(0), body["status_counts"].(map[string]any)["sent"], "every status is present")
}

func TestListNotificationsParsesQuery(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var got service.ListFilter
	next := &service.Cursor{CreatedAt: time.Date(2026, 11, 20, 12, 0, 0, 123456000, time.UTC), ID: uuid.New()}
	h.notifications.list = func(f service.ListFilter) (service.Page[domain.Notification], error) {
		got = f
		return service.Page[domain.Notification]{Items: []domain.Notification{sampleNotification()}, Next: next}, nil
	}

	rec := h.do(http.MethodGet,
		"/api/v1/notifications?status=queued,retrying&status=failed&channel=sms&priority=high"+
			"&created_from=2026-11-20T00:00:00Z&created_to=2026-11-21T00:00:00Z&limit=10&recipient=%2B905551234567", "")

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, []domain.Status{domain.StatusQueued, domain.StatusRetrying, domain.StatusFailed}, got.Statuses)
	assert.Equal(t, []domain.Channel{domain.ChannelSMS}, got.Channels)
	assert.Equal(t, []domain.Priority{domain.PriorityHigh}, got.Priorities)
	assert.Equal(t, 10, got.Limit)
	assert.Equal(t, "+905551234567", got.Recipient)
	assert.Equal(t, time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC), *got.CreatedFrom)

	body := decode[map[string]any](t, rec)
	assert.Equal(t, true, body["has_more"])
	cursor, ok := body["next_cursor"].(string)
	require.True(t, ok)

	// The opaque cursor round-trips exactly.
	h.do(http.MethodGet, "/api/v1/notifications?cursor="+cursor, "")
	require.NotNil(t, got.After)
	assert.Equal(t, *next, *got.After)
}

func TestListNotificationsRejectsBadParameters(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	rec := h.do(http.MethodGet, "/api/v1/notifications?status=delivered&channel=fax&limit=ten&cursor=xyz&created_from=yesterday", "")

	p := problem(t, rec, http.StatusBadRequest, "invalid_parameters")
	fields := make([]string, 0, len(p.Errors))
	for _, e := range p.Errors {
		fields = append(fields, e.Field)
	}
	assert.ElementsMatch(t, []string{"status", "channel", "limit", "cursor", "created_from"}, fields)
}

func TestRoutingProblems(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	problem(t, h.do(http.MethodGet, "/api/v1/nope", ""), http.StatusNotFound, "not_found")
	problem(t, h.do(http.MethodDelete, "/api/v1/notifications", ""), http.StatusMethodNotAllowed, "method_not_allowed")
	p := problem(t, h.do(http.MethodGet, "/api/v1/notifications/not-a-uuid", ""), http.StatusNotFound, "not_found")
	assert.Equal(t, "notification not-a-uuid not found", p.Detail)
	problem(t, h.do(http.MethodGet, "/api/v1/batches/"+uuid.NewString()+"/notifications", ""), http.StatusNotFound, "not_found")
}

func TestCorrelationIDs(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	rec := h.do(http.MethodGet, "/api/v1/nope", "", "X-Correlation-ID", "trace-me-42")
	assert.Equal(t, "trace-me-42", rec.Header().Get("X-Correlation-ID"))
	assert.Equal(t, "trace-me-42", decode[httpapi.Problem](t, rec).RequestID)

	rec = h.do(http.MethodGet, "/healthz", "", "X-Correlation-ID", "has spaces in it")
	assert.NoError(t, uuid.Validate(rec.Header().Get("X-Correlation-ID")), "unusable IDs are replaced")
}

func TestPanicsBecomeProblems(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.notifications.get = func(uuid.UUID) (domain.Notification, error) { panic("nil map write") }

	rec := h.do(http.MethodGet, "/api/v1/notifications/"+uuid.NewString(), "")

	p := problem(t, rec, http.StatusInternalServerError, "internal_error")
	assert.NotContains(t, p.Detail, "nil map")
}

func TestHealthEndpoints(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.ready["postgres"] = func(context.Context) error { return nil }

	assert.Equal(t, http.StatusOK, h.do(http.MethodGet, "/healthz", "").Code)
	rec := h.do(http.MethodGet, "/readyz", "")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"ready","checks":{"postgres":"ok"}}`, rec.Body.String())

	h.ready["redis"] = func(context.Context) error { return errors.New("dial tcp: refused") }
	rec = h.do(http.MethodGet, "/readyz", "")
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.JSONEq(t, `{"status":"unavailable","checks":{"postgres":"ok","redis":"unavailable"}}`, rec.Body.String())
}

func TestMetricsWindow(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	rec := h.do(http.MethodGet, "/api/v1/metrics?window=15m", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 15*time.Minute, h.metrics.window)

	problem(t, h.do(http.MethodGet, "/api/v1/metrics?window=soon", ""), http.StatusBadRequest, "invalid_parameters")
}

func TestTemplateEndpoints(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	rec := h.do(http.MethodPost, "/api/v1/templates", `{"name": "otp", "channel": "sms", "body": "Code {{code}}"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	body := decode[map[string]any](t, rec)
	assert.Equal(t, []any{"code"}, body["variables"])
	assert.Equal(t, "/api/v1/templates/"+h.templates.created.ID.String(), rec.Header().Get("Location"))

	rec = h.do(http.MethodPost, "/api/v1/templates/"+uuid.NewString()+"/preview", `{"variables": {"name": "Ada"}}`)
	assert.JSONEq(t, `{"content": "Hi Ada", "length": 6}`, rec.Body.String())

	assert.Equal(t, http.StatusNoContent, h.do(http.MethodDelete, "/api/v1/templates/"+uuid.NewString(), "").Code)

	h.templates.err = fmt.Errorf("template %q: %w", "otp", domain.ErrTemplateNameTaken)
	problem(t, h.do(http.MethodPost, "/api/v1/templates", `{"name": "otp", "channel": "sms", "body": "x"}`),
		http.StatusConflict, "template_name_taken")
}

func TestEventStream(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	srv := httptest.NewServer(h.handler)
	t.Cleanup(srv.Close)
	batch := uuid.New()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/api/v1/events?batch_id="+batch.String(), nil)
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()

	read := func() map[string]any {
		_, data, err := conn.Read(ctx)
		require.NoError(t, err)
		var msg map[string]any
		require.NoError(t, json.Unmarshal(data, &msg))
		return msg
	}
	assert.Equal(t, "ready", read()["type"])

	other := uuid.New()
	h.hub.Publish(domain.Event{Type: domain.EventNotificationUpdated, NotificationID: uuid.New(), BatchID: &other})
	id := uuid.New()
	h.hub.Publish(domain.Event{
		Type: domain.EventNotificationUpdated, NotificationID: id, BatchID: &batch,
		Channel: domain.ChannelSMS, Status: domain.StatusSent, Attempts: 1,
	})

	msg := read()
	assert.Equal(t, "notification.updated", msg["type"])
	data := msg["data"].(map[string]any)
	assert.Equal(t, id.String(), data["notification_id"], "only the subscribed batch is streamed")
	assert.Equal(t, "sent", data["status"])
}

func TestEventStreamRejectsBadFilters(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	rec := h.do(http.MethodGet, "/api/v1/events?batch_id=nope", "")
	problem(t, rec, http.StatusBadRequest, "invalid_parameters")
	_, _ = io.Copy(io.Discard, rec.Body)
}
