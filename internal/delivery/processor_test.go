package delivery_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RamizUnal/notification-system/internal/delivery"
	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/provider"
)

type processorFixture struct {
	store    *fakeStore
	clock    *manualClock
	breaker  *delivery.CircuitBreaker
	observer *recordingObserver
	sent     int
	proc     *delivery.Processor
}

func newProcessorFixture(t *testing.T, send func(context.Context, provider.Message) (provider.Receipt, error)) *processorFixture {
	t.Helper()
	f := &processorFixture{store: &fakeStore{}, clock: newManualClock(), observer: &recordingObserver{}}
	f.breaker = delivery.NewCircuitBreaker(3, time.Minute, delivery.WithBreakerClock(f.clock.Now))
	f.proc = delivery.NewProcessor(delivery.ProcessorConfig{
		Store: f.store,
		Sender: senderFunc(func(ctx context.Context, m provider.Message) (provider.Receipt, error) {
			f.sent++
			return send(ctx, m)
		}),
		Breakers: map[domain.Channel]*delivery.CircuitBreaker{domain.ChannelSMS: f.breaker},
		Policy:   fixedPolicy(30 * time.Second),
		Observer: f.observer,
		Clock:    f.clock.Now,
	})
	return f
}

func (f *processorFixture) only(t *testing.T) delivery.Result {
	t.Helper()
	completed, _, _ := f.store.snapshot()
	require.Len(t, completed, 1)
	return completed[0].Result
}

func accepted(context.Context, provider.Message) (provider.Receipt, error) {
	return provider.Receipt{MessageID: "msg-1", Status: "accepted", StatusCode: http.StatusAccepted}, nil
}

func failWith(err error) func(context.Context, provider.Message) (provider.Receipt, error) {
	return func(context.Context, provider.Message) (provider.Receipt, error) { return provider.Receipt{}, err }
}

var (
	errUnavailable = &provider.Error{StatusCode: http.StatusServiceUnavailable, Retryable: true, Message: "down"}
	errBadRequest  = &provider.Error{StatusCode: http.StatusBadRequest, Message: "invalid recipient"}
)

func TestProcessRecordsASuccessfulDelivery(t *testing.T) {
	t.Parallel()
	var got provider.Message
	f := newProcessorFixture(t, func(ctx context.Context, m provider.Message) (provider.Receipt, error) {
		got = m
		return accepted(ctx, m)
	})
	job := newJob(1, 5)

	f.proc.Process(context.Background(), job)

	assert.Equal(t, job.Notification.ID, got.NotificationID, "the ID doubles as the provider idempotency key")
	assert.Equal(t, "corr-1", got.CorrelationID)
	res := f.only(t)
	assert.Equal(t, domain.StatusSent, res.Status)
	assert.Equal(t, "msg-1", res.ProviderMessageID)
	assert.Empty(t, res.Error)
	assert.Equal(t, domain.DeliveryAttempt{
		NotificationID:    job.Notification.ID,
		Number:            1,
		Channel:           domain.ChannelSMS,
		Outcome:           domain.OutcomeSuccess,
		StatusCode:        http.StatusAccepted,
		ProviderMessageID: "msg-1",
		StartedAt:         f.clock.Now(),
		FinishedAt:        f.clock.Now(),
	}, res.Attempt)
	assert.Equal(t, []domain.AttemptOutcome{domain.OutcomeSuccess}, f.observer.delivered)
}

func TestProcessDecidesWhatHappensAfterAFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		attempt   int
		status    domain.Status
		outcome   domain.AttemptOutcome
		nextAfter time.Duration
		errPrefix string
	}{
		{
			name: "transient failure is retried with backoff", err: errUnavailable, attempt: 1,
			status: domain.StatusRetrying, outcome: domain.OutcomeRetryableFailure, nextAfter: 30 * time.Second,
		},
		{
			name: "provider Retry-After wins when longer",
			err: &provider.Error{
				StatusCode: http.StatusTooManyRequests, Retryable: true, RetryAfter: 2 * time.Minute, Message: "slow down",
			},
			attempt: 2, status: domain.StatusRetrying, outcome: domain.OutcomeRetryableFailure, nextAfter: 2 * time.Minute,
		},
		{
			name: "unclassified errors are treated as transient", err: errors.New("connection reset"), attempt: 1,
			status: domain.StatusRetrying, outcome: domain.OutcomeRetryableFailure, nextAfter: 30 * time.Second,
		},
		{
			name: "last attempt gives up", err: errUnavailable, attempt: 5,
			status: domain.StatusFailed, outcome: domain.OutcomeRetryableFailure, errPrefix: "gave up after 5 attempts",
		},
		{
			name: "permanent failure is not retried", err: errBadRequest, attempt: 1,
			status: domain.StatusFailed, outcome: domain.OutcomePermanentFailure,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newProcessorFixture(t, failWith(tc.err))

			f.proc.Process(context.Background(), newJob(tc.attempt, 5))

			res := f.only(t)
			assert.Equal(t, tc.status, res.Status)
			assert.Equal(t, tc.outcome, res.Attempt.Outcome)
			assert.Equal(t, tc.attempt, res.Attempt.Number)
			assert.Equal(t, provider.StatusCode(tc.err), res.Attempt.StatusCode)
			assert.Contains(t, res.Error, tc.errPrefix)
			if tc.status == domain.StatusRetrying {
				assert.Equal(t, f.clock.Now().Add(tc.nextAfter), res.NextAttemptAt)
			} else {
				assert.True(t, res.NextAttemptAt.IsZero())
			}
		})
	}
}

func TestProcessOpensTheCircuitOnlyForProviderFailures(t *testing.T) {
	t.Parallel()

	t.Run("rejections prove the provider is up", func(t *testing.T) {
		t.Parallel()
		f := newProcessorFixture(t, failWith(errBadRequest))
		for range 5 {
			f.proc.Process(context.Background(), newJob(1, 5))
		}
		assert.Equal(t, delivery.CircuitClosed, f.breaker.Status().State)
	})

	t.Run("outages open it and later jobs wait in the queue", func(t *testing.T) {
		t.Parallel()
		f := newProcessorFixture(t, failWith(errUnavailable))
		for range 3 {
			f.proc.Process(context.Background(), newJob(1, 5))
		}
		require.Equal(t, delivery.CircuitOpen, f.breaker.Status().State)

		f.clock.Advance(15 * time.Second)
		job := newJob(2, 5)
		f.proc.Process(context.Background(), job)

		assert.Equal(t, 3, f.sent, "no request is made while the circuit is open")
		_, released, _ := f.store.snapshot()
		require.Len(t, released, 1)
		assert.Equal(t, job.Notification.ID, released[0].Job.Notification.ID)
		assert.Equal(t, f.clock.Now().Add(45*time.Second), released[0].AvailableAt, "due again when the circuit probes")
		assert.Equal(t, []string{"circuit_open"}, f.observer.released)
	})
}

func TestProcessToleratesALostLease(t *testing.T) {
	t.Parallel()
	f := newProcessorFixture(t, accepted)
	f.store.completeErr = fmt.Errorf("complete: %w", domain.ErrLeaseLost)

	f.proc.Process(context.Background(), newJob(1, 5))

	assert.Empty(t, f.observer.delivered, "an outcome that was not recorded is not reported")
}

func TestProcessFinishesTheSendDuringShutdown(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := newProcessorFixture(t, func(ctx context.Context, m provider.Message) (provider.Receipt, error) {
		if err := ctx.Err(); err != nil {
			return provider.Receipt{}, err
		}
		return accepted(ctx, m)
	})

	f.proc.Process(ctx, newJob(1, 5))

	assert.Equal(t, domain.StatusSent, f.only(t).Status)
}
