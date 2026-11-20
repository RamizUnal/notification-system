// Package delivery is the processing engine: it claims due notifications from
// the queue, paces them through per-channel rate limits, sends them through a
// provider guarded by a circuit breaker, and records each outcome, scheduling
// retries with exponential backoff when a failure is transient.
//
// The engine depends only on the ports declared here; PostgreSQL, Redis and
// HTTP adapters are plugged in by the application wiring.
package delivery

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/RamizUnal/notification-system/internal/domain"
)

// Job is a notification claimed by this worker.
type Job struct {
	Notification domain.Notification
	// LeaseID proves the claim. Recording an outcome requires the lease to
	// still be held, which fences off a worker whose lease expired and whose
	// notification was meanwhile taken over by another worker.
	LeaseID        uuid.UUID
	LeaseExpiresAt time.Time
}

// Result is the processor's decision after one delivery attempt.
type Result struct {
	// Status is the notification's next status: sent, retrying or failed.
	Status            domain.Status
	Attempt           domain.DeliveryAttempt
	NextAttemptAt     time.Time // when Status is retrying
	ProviderMessageID string
	Error             string // becomes last_error; empty when sent
	At                time.Time
}

// Store is the engine's persistence port.
type Store interface {
	// Claim leases up to limit due notifications of the channel, highest
	// priority first and oldest first within a priority. Each claim counts
	// as an attempt.
	Claim(ctx context.Context, ch domain.Channel, limit int, now time.Time, leaseTTL time.Duration) ([]Job, error)
	// Complete records the attempt and moves the notification to
	// result.Status. It fails with domain.ErrLeaseLost when the job's lease
	// is no longer held.
	Complete(ctx context.Context, job Job, result Result) error
	// Release returns a claimed notification to the queue, due at
	// availableAt, without counting the attempt.
	Release(ctx context.Context, job Job, availableAt time.Time) error
	// RecoverExpiredLeases requeues notifications whose worker stopped
	// before recording an outcome, and returns how many it recovered.
	RecoverExpiredLeases(ctx context.Context, now time.Time) (int, error)
}

// Heartbeats lets workers advertise that they are alive and how their
// circuit breakers stand.
type Heartbeats interface {
	Beat(ctx context.Context, hb Heartbeat) error
	Remove(ctx context.Context, workerID string) error
}

// Heartbeat is one worker's periodic status report.
type Heartbeat struct {
	WorkerID  string
	StartedAt time.Time
	SeenAt    time.Time
	InFlight  int
	Circuits  map[domain.Channel]CircuitStatus
}

// Limiter paces sends per channel.
type Limiter interface {
	// Wait blocks until one more message may be sent on ch.
	Wait(ctx context.Context, ch domain.Channel) error
}

// RetryPolicy decides how long to wait before retrying a failed attempt.
type RetryPolicy interface {
	// Delay returns the pause after the given (1-based) failed attempt.
	Delay(attempt int) time.Duration
}

// Observer is notified of engine activity, typically to update metrics.
// Implementations must be safe for concurrent use.
type Observer interface {
	Delivered(ch domain.Channel, outcome domain.AttemptOutcome, latency time.Duration)
	Released(ch domain.Channel, reason string)
	CircuitChanged(ch domain.Channel, state CircuitState)
	LeasesRecovered(n int)
}

// NopObserver ignores every event.
type NopObserver struct{}

func (NopObserver) Delivered(domain.Channel, domain.AttemptOutcome, time.Duration) {}
func (NopObserver) Released(domain.Channel, string)                                {}
func (NopObserver) CircuitChanged(domain.Channel, CircuitState)                    {}
func (NopObserver) LeasesRecovered(int)                                            {}
