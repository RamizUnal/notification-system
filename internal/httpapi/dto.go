package httpapi

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/service"
)

// The types in this file are the API's wire format. They are kept separate
// from the domain model so the public contract (documented in
// api/openapi.yaml) only changes on purpose.

type notificationRequest struct {
	Channel        string            `json:"channel"`
	Recipient      string            `json:"recipient"`
	Content        string            `json:"content"`
	Priority       string            `json:"priority"`
	TemplateID     string            `json:"template_id"`
	Variables      map[string]string `json:"variables"`
	ScheduledAt    string            `json:"scheduled_at"`
	IdempotencyKey string            `json:"idempotency_key"`
}

// toInput converts the request, recording format problems under prefix.
func (req notificationRequest) toInput(verr *domain.ValidationError, prefix string) service.NotificationInput {
	in := service.NotificationInput{
		Channel:        domain.Channel(req.Channel),
		Priority:       domain.Priority(req.Priority),
		Recipient:      req.Recipient,
		Content:        req.Content,
		Variables:      req.Variables,
		IdempotencyKey: req.IdempotencyKey,
	}
	if req.TemplateID != "" {
		id, err := uuid.Parse(req.TemplateID)
		if err != nil {
			verr.Add(field(prefix, "template_id"), domain.CodeInvalidFormat, "template_id must be a UUID")
		} else {
			in.TemplateID = &id
		}
	}
	if req.ScheduledAt != "" {
		at, err := time.Parse(time.RFC3339, req.ScheduledAt)
		if err != nil {
			verr.Add(field(prefix, "scheduled_at"), domain.CodeInvalidFormat,
				"scheduled_at must be an RFC 3339 timestamp such as 2026-11-20T09:30:00Z")
		} else {
			in.ScheduledAt = &at
		}
	}
	return in
}

func field(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

type batchRequest struct {
	Notifications []notificationRequest `json:"notifications"`
}

type templateRequest struct {
	Name    string `json:"name"`
	Channel string `json:"channel"`
	Body    string `json:"body"`
}

func (req templateRequest) toDraft() domain.TemplateDraft {
	return domain.TemplateDraft{Name: req.Name, Channel: domain.Channel(req.Channel), Body: req.Body}
}

type previewRequest struct {
	Variables map[string]string `json:"variables"`
}

type notificationResponse struct {
	ID                string     `json:"id"`
	BatchID           *string    `json:"batch_id"`
	TemplateID        *string    `json:"template_id"`
	Channel           string     `json:"channel"`
	Priority          string     `json:"priority"`
	Recipient         string     `json:"recipient"`
	Content           string     `json:"content"`
	Status            string     `json:"status"`
	IdempotencyKey    *string    `json:"idempotency_key"`
	Attempts          int        `json:"attempts"`
	MaxAttempts       int        `json:"max_attempts"`
	ScheduledAt       *time.Time `json:"scheduled_at"`
	NextAttemptAt     *time.Time `json:"next_attempt_at"`
	LastError         *string    `json:"last_error"`
	ProviderMessageID *string    `json:"provider_message_id"`
	CorrelationID     *string    `json:"correlation_id"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	SentAt            *time.Time `json:"sent_at"`
	FailedAt          *time.Time `json:"failed_at"`
	CancelledAt       *time.Time `json:"cancelled_at"`
}

func toNotificationResponse(n domain.Notification) notificationResponse {
	resp := notificationResponse{
		ID:                n.ID.String(),
		BatchID:           uuidString(n.BatchID),
		TemplateID:        uuidString(n.TemplateID),
		Channel:           string(n.Channel),
		Priority:          string(n.Priority),
		Recipient:         n.Recipient,
		Content:           n.Content,
		Status:            string(n.Status),
		IdempotencyKey:    optional(n.IdempotencyKey),
		Attempts:          n.Attempts,
		MaxAttempts:       n.MaxAttempts,
		ScheduledAt:       n.ScheduledAt,
		LastError:         optional(n.LastError),
		ProviderMessageID: optional(n.ProviderMessageID),
		CorrelationID:     optional(n.CorrelationID),
		CreatedAt:         n.CreatedAt,
		UpdatedAt:         n.UpdatedAt,
		SentAt:            n.SentAt,
		FailedAt:          n.FailedAt,
		CancelledAt:       n.CancelledAt,
	}
	// The next attempt only means something while the notification waits.
	if n.Status.IsWaiting() {
		at := n.NextAttemptAt
		resp.NextAttemptAt = &at
	}
	return resp
}

type listResponse[T any] struct {
	Data       []T     `json:"data"`
	NextCursor *string `json:"next_cursor"`
	HasMore    bool    `json:"has_more"`
}

func toListResponse(page service.Page[domain.Notification]) listResponse[notificationResponse] {
	data := make([]notificationResponse, len(page.Items))
	for i, n := range page.Items {
		data[i] = toNotificationResponse(n)
	}
	return listResponse[notificationResponse]{Data: data, NextCursor: encodeCursor(page.Next), HasMore: page.Next != nil}
}

type attemptResponse struct {
	Number            int       `json:"number"`
	Outcome           string    `json:"outcome"`
	StatusCode        *int      `json:"status_code"`
	Error             *string   `json:"error"`
	ProviderMessageID *string   `json:"provider_message_id"`
	DurationMS        float64   `json:"duration_ms"`
	StartedAt         time.Time `json:"started_at"`
	FinishedAt        time.Time `json:"finished_at"`
}

func toAttemptResponses(attempts []domain.DeliveryAttempt) []attemptResponse {
	out := make([]attemptResponse, len(attempts))
	for i, a := range attempts {
		out[i] = attemptResponse{
			Number:            a.Number,
			Outcome:           string(a.Outcome),
			Error:             optional(a.Error),
			ProviderMessageID: optional(a.ProviderMessageID),
			DurationMS:        float64(a.Duration().Microseconds()) / 1000,
			StartedAt:         a.StartedAt,
			FinishedAt:        a.FinishedAt,
		}
		if a.StatusCode != 0 {
			code := a.StatusCode
			out[i].StatusCode = &code
		}
	}
	return out
}

type batchResponse struct {
	ID              string         `json:"id"`
	Size            int            `json:"size"`
	CreatedAt       time.Time      `json:"created_at"`
	StatusCounts    map[string]int `json:"status_counts"`
	Pending         int            `json:"pending"`
	Done            bool           `json:"done"`
	NotificationIDs []string       `json:"notification_ids,omitempty"`
}

func toBatchResponse(s domain.BatchSummary, ids []uuid.UUID) batchResponse {
	counts := make(map[string]int, len(domain.Statuses()))
	for _, st := range domain.Statuses() {
		counts[string(st)] = s.Counts[st]
	}
	resp := batchResponse{
		ID: s.ID.String(), Size: s.Size, CreatedAt: s.CreatedAt,
		StatusCounts: counts, Pending: s.Pending(), Done: s.Done(),
	}
	if ids != nil {
		resp.NotificationIDs = make([]string, len(ids))
		for i, id := range ids {
			resp.NotificationIDs[i] = id.String()
		}
	}
	return resp
}

type templateResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Channel   string    `json:"channel"`
	Body      string    `json:"body"`
	Variables []string  `json:"variables"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toTemplateResponse(t domain.Template) templateResponse {
	vars := t.Variables()
	if vars == nil {
		vars = []string{}
	}
	return templateResponse{
		ID: t.ID.String(), Name: t.Name, Channel: string(t.Channel), Body: t.Body,
		Variables: vars, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

type metricsResponse struct {
	GeneratedAt   time.Time              `json:"generated_at"`
	WindowSeconds float64                `json:"window_seconds"`
	BucketSeconds float64                `json:"bucket_seconds"`
	Channels      []channelMetrics       `json:"channels"`
	StatusCounts  map[string]int         `json:"status_counts"`
	Throughput    []throughputPoint      `json:"throughput"`
	Workers       []workerStatusResponse `json:"workers"`
}

type channelMetrics struct {
	Channel            string          `json:"channel"`
	Queue              queueMetrics    `json:"queue"`
	Deliveries         deliveryMetrics `json:"deliveries"`
	RateLimitPerSecond int             `json:"rate_limit_per_second"`
	Circuit            string          `json:"circuit"`
}

type queueMetrics struct {
	Due        int            `json:"due"`
	Scheduled  int            `json:"scheduled"`
	Retrying   int            `json:"retrying"`
	Processing int            `json:"processing"`
	ByPriority map[string]int `json:"by_priority"`
}

type deliveryMetrics struct {
	Succeeded           int            `json:"succeeded"`
	RetryableFailures   int            `json:"retryable_failures"`
	PermanentFailures   int            `json:"permanent_failures"`
	SuccessRate         *float64       `json:"success_rate"`
	ThroughputPerSecond float64        `json:"throughput_per_second"`
	LatencyMS           latencyMetrics `json:"latency_ms"`
}

type latencyMetrics struct {
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
}

type throughputPoint struct {
	At        time.Time `json:"at"`
	Channel   string    `json:"channel"`
	Succeeded int       `json:"succeeded"`
	Failed    int       `json:"failed"`
}

type workerStatusResponse struct {
	ID        string                     `json:"id"`
	StartedAt time.Time                  `json:"started_at"`
	SeenAt    time.Time                  `json:"seen_at"`
	InFlight  int                        `json:"in_flight"`
	Circuits  map[string]circuitResponse `json:"circuits"`
}

type circuitResponse struct {
	State   string     `json:"state"`
	RetryAt *time.Time `json:"retry_at"`
}

func toMetricsResponse(s service.MetricsSnapshot) metricsResponse {
	resp := metricsResponse{
		GeneratedAt:   s.GeneratedAt,
		WindowSeconds: s.Window.Seconds(),
		BucketSeconds: s.Bucket.Seconds(),
		StatusCounts:  make(map[string]int, len(domain.Statuses())),
		Channels:      make([]channelMetrics, len(s.Channels)),
		Throughput:    make([]throughputPoint, len(s.Throughput)),
		Workers:       make([]workerStatusResponse, len(s.Workers)),
	}
	for _, st := range domain.Statuses() {
		resp.StatusCounts[string(st)] = s.StatusCounts[st]
	}
	for i, c := range s.Channels {
		byPriority := make(map[string]int, len(c.Waiting))
		for p, n := range c.Waiting {
			byPriority[string(p)] = n
		}
		resp.Channels[i] = channelMetrics{
			Channel: string(c.Channel),
			Queue: queueMetrics{
				Due: c.Due, Scheduled: c.Scheduled, Retrying: c.Retrying, Processing: c.Processing,
				ByPriority: byPriority,
			},
			Deliveries: deliveryMetrics{
				Succeeded:           c.Stats.Succeeded,
				RetryableFailures:   c.Stats.RetryableFailures,
				PermanentFailures:   c.Stats.PermanentFailures,
				SuccessRate:         c.SuccessRate,
				ThroughputPerSecond: c.Throughput,
				LatencyMS: latencyMetrics{
					P50: ms(c.Stats.Latency.P50), P95: ms(c.Stats.Latency.P95), P99: ms(c.Stats.Latency.P99),
				},
			},
			RateLimitPerSecond: c.RateLimit,
			Circuit:            c.Circuit,
		}
	}
	for i, p := range s.Throughput {
		resp.Throughput[i] = throughputPoint{At: p.At, Channel: string(p.Channel), Succeeded: p.Succeeded, Failed: p.Failed}
	}
	for i, w := range s.Workers {
		circuits := make(map[string]circuitResponse, len(w.Circuits))
		for ch, c := range w.Circuits {
			circuits[string(ch)] = circuitResponse{State: c.State, RetryAt: c.RetryAt}
		}
		resp.Workers[i] = workerStatusResponse{
			ID: w.ID, StartedAt: w.StartedAt, SeenAt: w.SeenAt, InFlight: w.InFlight, Circuits: circuits,
		}
	}
	return resp
}

// eventMessage is the WebSocket frame for one domain event.
type eventMessage struct {
	Type string    `json:"type"`
	Data eventData `json:"data"`
}

type eventData struct {
	NotificationID *string   `json:"notification_id,omitempty"`
	BatchID        *string   `json:"batch_id,omitempty"`
	Channel        string    `json:"channel,omitempty"`
	Priority       string    `json:"priority,omitempty"`
	Status         string    `json:"status,omitempty"`
	Attempts       int       `json:"attempts"`
	LastError      *string   `json:"last_error,omitempty"`
	BatchSize      int       `json:"batch_size,omitempty"`
	OccurredAt     time.Time `json:"occurred_at"`
}

func toEventMessage(e domain.Event) eventMessage {
	data := eventData{
		BatchID:    uuidString(e.BatchID),
		Channel:    string(e.Channel),
		Priority:   string(e.Priority),
		Status:     string(e.Status),
		Attempts:   e.Attempts,
		LastError:  optional(e.LastError),
		BatchSize:  e.BatchSize,
		OccurredAt: e.OccurredAt,
	}
	if e.NotificationID != uuid.Nil {
		data.NotificationID = uuidString(&e.NotificationID)
	}
	return eventMessage{Type: string(e.Type), Data: data}
}

func uuidString(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ms(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}

func itemPrefix(i int) string { return fmt.Sprintf("notifications[%d]", i) }
