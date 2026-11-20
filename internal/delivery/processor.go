package delivery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/observability"
	"github.com/RamizUnal/notification-system/internal/provider"
)

const (
	// recordTimeout bounds writing an outcome to the store.
	recordTimeout = 5 * time.Second
	// maxRetryAfter caps how long a provider's Retry-After may postpone a
	// notification, so a misbehaving provider cannot park messages forever.
	maxRetryAfter = time.Hour
)

// Processor delivers one claimed notification and records the outcome.
type Processor struct {
	store       Store
	sender      provider.Sender
	breakers    map[domain.Channel]*CircuitBreaker
	policy      RetryPolicy
	observer    Observer
	logger      *slog.Logger
	tracer      trace.Tracer
	now         func() time.Time
	sendTimeout time.Duration
}

// ProcessorConfig wires a Processor.
type ProcessorConfig struct {
	Store       Store
	Sender      provider.Sender
	Breakers    map[domain.Channel]*CircuitBreaker
	Policy      RetryPolicy
	Observer    Observer     // optional
	Logger      *slog.Logger // optional
	Tracer      trace.Tracer // optional
	Clock       func() time.Time
	SendTimeout time.Duration
}

// NewProcessor returns a processor. Optional collaborators default to no-ops.
func NewProcessor(cfg ProcessorConfig) *Processor {
	p := &Processor{
		store:       cfg.Store,
		sender:      cfg.Sender,
		breakers:    cfg.Breakers,
		policy:      cfg.Policy,
		observer:    cfg.Observer,
		logger:      cfg.Logger,
		tracer:      cfg.Tracer,
		now:         cfg.Clock,
		sendTimeout: cfg.SendTimeout,
	}
	if p.observer == nil {
		p.observer = NopObserver{}
	}
	if p.logger == nil {
		p.logger = slog.New(slog.DiscardHandler)
	}
	if p.tracer == nil {
		p.tracer = noop.NewTracerProvider().Tracer("")
	}
	if p.now == nil {
		p.now = time.Now
	}
	if p.sendTimeout <= 0 {
		p.sendTimeout = 10 * time.Second
	}
	return p
}

// Process makes one delivery attempt for job.
//
// Once a send has started it is finished and recorded even if the worker is
// shutting down: the context's cancellation is deliberately ignored here, so
// an outcome the provider already knows about is never lost.
func (p *Processor) Process(ctx context.Context, job Job) {
	n := job.Notification
	ctx = context.WithoutCancel(ctx)
	ctx = observability.WithCorrelationID(ctx, n.CorrelationID)
	ctx = observability.ContextWithTraceParent(ctx, n.TraceParent)
	ctx, span := p.tracer.Start(ctx, "deliver "+string(n.Channel),
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("notification.id", n.ID.String()),
			attribute.String("notification.channel", string(n.Channel)),
			attribute.String("notification.priority", string(n.Priority)),
			attribute.Int("notification.attempt", n.Attempts),
		))
	defer span.End()

	logger := p.logger.With(
		slog.String("notification_id", n.ID.String()),
		slog.String("channel", string(n.Channel)),
		slog.String("priority", string(n.Priority)),
		slog.Int("attempt", n.Attempts),
		slog.Int("max_attempts", n.MaxAttempts),
	)

	breaker := p.breakers[n.Channel]
	if ok, wait := breaker.Allow(); !ok {
		span.AddEvent("circuit open; returned to queue")
		p.release(ctx, job, wait, "circuit_open", logger)
		return
	}

	started := p.now()
	sendCtx, cancel := context.WithTimeout(ctx, p.sendTimeout)
	receipt, sendErr := p.sender.Send(sendCtx, provider.Message{
		NotificationID: n.ID,
		Channel:        n.Channel,
		Recipient:      n.Recipient,
		Content:        n.Content,
		CorrelationID:  n.CorrelationID,
	})
	cancel()
	finished := p.now()

	result := p.decide(job, receipt, sendErr, started, finished)
	if result.Attempt.Outcome == domain.OutcomeRetryableFailure {
		breaker.Failure()
	} else {
		breaker.Success()
	}

	recordCtx, cancelRecord := context.WithTimeout(ctx, recordTimeout)
	defer cancelRecord()
	if err := p.store.Complete(recordCtx, job, result); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "recording the outcome failed")
		if errors.Is(err, domain.ErrLeaseLost) {
			logger.WarnContext(ctx, "lease expired before the outcome was recorded; another worker owns the notification now",
				slog.String("outcome", string(result.Attempt.Outcome)))
			return
		}
		// The lease will expire and the notification will be retried, so
		// delivery stays at-least-once.
		logger.ErrorContext(ctx, "recording delivery outcome failed", slog.Any("error", err))
		return
	}
	p.observer.Delivered(n.Channel, result.Attempt.Outcome, result.Attempt.Duration())

	attrs := []any{
		slog.String("outcome", string(result.Attempt.Outcome)),
		slog.String("status", string(result.Status)),
		slog.Float64("duration_ms", float64(result.Attempt.Duration().Microseconds())/1000),
	}
	span.SetAttributes(attribute.String("notification.outcome", string(result.Attempt.Outcome)))
	switch result.Status {
	case domain.StatusSent:
		logger.InfoContext(ctx, "notification delivered",
			append(attrs, slog.String("provider_message_id", result.ProviderMessageID))...)
	case domain.StatusRetrying:
		span.SetStatus(codes.Error, result.Error)
		logger.WarnContext(ctx, "delivery failed; retry scheduled",
			append(attrs, slog.String("error", result.Error), slog.Time("next_attempt_at", result.NextAttemptAt))...)
	default:
		span.SetStatus(codes.Error, result.Error)
		logger.ErrorContext(ctx, "delivery failed permanently", append(attrs, slog.String("error", result.Error))...)
	}
}

// decide turns the provider's answer into the notification's next state.
func (p *Processor) decide(job Job, receipt provider.Receipt, sendErr error, started, finished time.Time) Result {
	n := job.Notification
	attempt := domain.DeliveryAttempt{
		NotificationID: n.ID,
		Number:         n.Attempts,
		Channel:        n.Channel,
		StartedAt:      started,
		FinishedAt:     finished,
	}

	if sendErr == nil {
		attempt.Outcome = domain.OutcomeSuccess
		attempt.StatusCode = receipt.StatusCode
		attempt.ProviderMessageID = receipt.MessageID
		return Result{Status: domain.StatusSent, Attempt: attempt, ProviderMessageID: receipt.MessageID, At: finished}
	}

	attempt.Error = sendErr.Error()
	attempt.StatusCode = provider.StatusCode(sendErr)
	result := Result{Attempt: attempt, Error: sendErr.Error(), At: finished}

	switch {
	case !provider.IsRetryable(sendErr):
		result.Attempt.Outcome = domain.OutcomePermanentFailure
		result.Status = domain.StatusFailed
	case n.Attempts >= n.MaxAttempts:
		result.Attempt.Outcome = domain.OutcomeRetryableFailure
		result.Status = domain.StatusFailed
		result.Error = fmt.Sprintf("gave up after %d attempts: %s", n.Attempts, sendErr)
	default:
		result.Attempt.Outcome = domain.OutcomeRetryableFailure
		result.Status = domain.StatusRetrying
		delay := max(p.policy.Delay(n.Attempts), min(provider.RetryAfter(sendErr), maxRetryAfter))
		result.NextAttemptAt = finished.Add(delay)
	}
	return result
}

func (p *Processor) release(ctx context.Context, job Job, wait time.Duration, reason string, logger *slog.Logger) {
	releaseCtx, cancel := context.WithTimeout(ctx, recordTimeout)
	defer cancel()
	if err := p.store.Release(releaseCtx, job, p.now().Add(wait)); err != nil {
		logger.WarnContext(ctx, "returning notification to the queue failed; lease recovery will requeue it",
			slog.String("reason", reason), slog.Any("error", err))
		return
	}
	p.observer.Released(job.Notification.Channel, reason)
	logger.DebugContext(ctx, "notification returned to the queue", slog.String("reason", reason), slog.Duration("wait", wait))
}
