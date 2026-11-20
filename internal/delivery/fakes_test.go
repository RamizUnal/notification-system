package delivery_test

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/RamizUnal/notification-system/internal/delivery"
	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/provider"
)

type completeCall struct {
	Job    delivery.Job
	Result delivery.Result
}

type releaseCall struct {
	Job         delivery.Job
	AvailableAt time.Time
}

// fakeStore is an in-memory delivery.Store with a simple FIFO queue.
type fakeStore struct {
	mu          sync.Mutex
	queue       []delivery.Job
	claimLimits []int
	claimErr    error
	completed   []completeCall
	released    []releaseCall
	completeErr error
	recoveries  int
}

func (s *fakeStore) Claim(_ context.Context, _ domain.Channel, limit int, _ time.Time, _ time.Duration) ([]delivery.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claimLimits = append(s.claimLimits, limit)
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	n := min(limit, len(s.queue))
	jobs := append([]delivery.Job(nil), s.queue[:n]...)
	s.queue = s.queue[n:]
	return jobs, nil
}

func (s *fakeStore) Complete(_ context.Context, job delivery.Job, result delivery.Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.completeErr != nil {
		return s.completeErr
	}
	s.completed = append(s.completed, completeCall{Job: job, Result: result})
	return nil
}

func (s *fakeStore) Release(_ context.Context, job delivery.Job, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.released = append(s.released, releaseCall{Job: job, AvailableAt: at})
	return nil
}

func (s *fakeStore) RecoverExpiredLeases(context.Context, time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recoveries++
	return 0, nil
}

func (s *fakeStore) snapshot() (completed []completeCall, released []releaseCall, limits []int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]completeCall(nil), s.completed...),
		append([]releaseCall(nil), s.released...),
		append([]int(nil), s.claimLimits...)
}

type senderFunc func(context.Context, provider.Message) (provider.Receipt, error)

func (f senderFunc) Send(ctx context.Context, m provider.Message) (provider.Receipt, error) {
	return f(ctx, m)
}

type fixedPolicy time.Duration

func (p fixedPolicy) Delay(int) time.Duration { return time.Duration(p) }

type unlimited struct{}

func (unlimited) Wait(context.Context, domain.Channel) error { return nil }

type failingLimiter struct {
	mu        sync.Mutex
	allowance int
}

func (l *failingLimiter) Wait(context.Context, domain.Channel) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.allowance == 0 {
		return errors.New("redis: connection refused")
	}
	l.allowance--
	return nil
}

type recordingObserver struct {
	mu        sync.Mutex
	delivered []domain.AttemptOutcome
	released  []string
}

func (o *recordingObserver) Delivered(_ domain.Channel, outcome domain.AttemptOutcome, _ time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.delivered = append(o.delivered, outcome)
}

func (o *recordingObserver) Released(_ domain.Channel, reason string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.released = append(o.released, reason)
}

func (o *recordingObserver) CircuitChanged(domain.Channel, delivery.CircuitState) {}
func (o *recordingObserver) LeasesRecovered(int)                                  {}

func newJob(attempt, maxAttempts int) delivery.Job {
	return delivery.Job{
		Notification: domain.Notification{
			ID:            uuid.New(),
			Channel:       domain.ChannelSMS,
			Priority:      domain.PriorityNormal,
			Recipient:     "+905551234567",
			Content:       "hello",
			Status:        domain.StatusProcessing,
			Attempts:      attempt,
			MaxAttempts:   maxAttempts,
			CorrelationID: "corr-1",
		},
		LeaseID: uuid.New(),
	}
}

func jobs(n int) []delivery.Job {
	out := make([]delivery.Job, n)
	for i := range out {
		out[i] = newJob(1, 5)
	}
	return out
}
