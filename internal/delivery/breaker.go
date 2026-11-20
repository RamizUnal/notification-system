package delivery

import (
	"sync"
	"time"
)

// CircuitState is the state of a circuit breaker.
type CircuitState int

// Circuit breaker states.
const (
	// CircuitClosed lets every call through.
	CircuitClosed CircuitState = iota
	// CircuitOpen rejects calls until the cooldown has passed.
	CircuitOpen
	// CircuitHalfOpen lets a single probe through to test the provider.
	CircuitHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half_open"
	default:
		return "closed"
	}
}

// CircuitStatus is a snapshot of a breaker.
type CircuitStatus struct {
	State CircuitState
	// RetryAt is when an open breaker will admit its next probe.
	RetryAt time.Time
}

// probeWait is how long callers wait while a half-open probe is in flight.
const probeWait = time.Second

// CircuitBreaker stops calling a provider that keeps failing. During an
// outage, workers would otherwise burn every notification's attempt budget
// against a dead endpoint; with the breaker open, notifications simply wait
// in the queue and the provider gets room to recover.
//
//	closed ──(threshold consecutive failures)──▶ open
//	open ──(cooldown elapsed)──▶ half-open, admits one probe
//	half-open ──(probe succeeds)──▶ closed
//	half-open ──(probe fails)──▶ open
//
// Only failures that say something about the provider's health count: a
// rejected request (4xx) proves the provider is up and counts as a success.
type CircuitBreaker struct {
	mu        sync.Mutex
	state     CircuitState
	failures  int
	openedAt  time.Time
	probing   bool
	threshold int
	cooldown  time.Duration
	now       func() time.Time
	onChange  func(from, to CircuitState)
}

// BreakerOption configures a CircuitBreaker.
type BreakerOption func(*CircuitBreaker)

// WithBreakerClock replaces time.Now.
func WithBreakerClock(now func() time.Time) BreakerOption {
	return func(b *CircuitBreaker) { b.now = now }
}

// OnStateChange registers a callback for every state transition. It runs
// while the breaker's lock is held, so it must be fast and must not call
// back into the breaker.
func OnStateChange(fn func(from, to CircuitState)) BreakerOption {
	return func(b *CircuitBreaker) { b.onChange = fn }
}

// NewCircuitBreaker returns a closed breaker that opens after threshold
// consecutive failures and probes again after cooldown.
func NewCircuitBreaker(threshold int, cooldown time.Duration, opts ...BreakerOption) *CircuitBreaker {
	b := &CircuitBreaker{
		threshold: max(1, threshold),
		cooldown:  cooldown,
		now:       time.Now,
		onChange:  func(CircuitState, CircuitState) {},
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Allow reports whether a call may go ahead. When it may not, wait says how
// long until asking again is worthwhile.
func (b *CircuitBreaker) Allow() (ok bool, wait time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refresh()

	switch b.state {
	case CircuitOpen:
		return false, b.remainingCooldown()
	case CircuitHalfOpen:
		if b.probing {
			return false, probeWait
		}
		b.probing = true
		return true, 0
	default:
		return true, 0
	}
}

// Capacity reports how many calls the breaker would admit right now:
// -1 means no limit (closed), 1 a half-open probe, and 0 none, in which case
// wait says how long until that may change. Dispatchers use it to avoid
// claiming work that could not be sent.
func (b *CircuitBreaker) Capacity() (n int, wait time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refresh()

	switch {
	case b.state == CircuitOpen:
		return 0, b.remainingCooldown()
	case b.state == CircuitHalfOpen && b.probing:
		return 0, probeWait
	case b.state == CircuitHalfOpen:
		return 1, 0
	default:
		return -1, 0
	}
}

// Success records a call that reached a healthy provider.
func (b *CircuitBreaker) Success() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case CircuitClosed:
		b.failures = 0
	case CircuitHalfOpen:
		b.probing = false
		b.failures = 0
		b.transition(CircuitClosed)
	case CircuitOpen:
		// A straggler that started before the breaker opened; its success
		// alone does not prove recovery.
	}
}

// Failure records a call that failed because the provider is unhealthy.
func (b *CircuitBreaker) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case CircuitClosed:
		b.failures++
		if b.failures >= b.threshold {
			b.trip()
		}
	case CircuitHalfOpen:
		b.probing = false
		b.trip()
	case CircuitOpen:
		// Already open; stragglers do not extend the cooldown.
	}
}

// Status returns the breaker's state and, when open, when it will probe.
func (b *CircuitBreaker) Status() CircuitStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refresh()

	s := CircuitStatus{State: b.state}
	if b.state == CircuitOpen {
		s.RetryAt = b.openedAt.Add(b.cooldown)
	}
	return s
}

// refresh moves an open breaker whose cooldown has passed to half-open.
func (b *CircuitBreaker) refresh() {
	if b.state == CircuitOpen && b.remainingCooldown() <= 0 {
		b.transition(CircuitHalfOpen)
	}
}

func (b *CircuitBreaker) trip() {
	b.openedAt = b.now()
	b.failures = 0
	b.transition(CircuitOpen)
}

func (b *CircuitBreaker) remainingCooldown() time.Duration {
	return b.cooldown - b.now().Sub(b.openedAt)
}

func (b *CircuitBreaker) transition(to CircuitState) {
	from := b.state
	if from == to {
		return
	}
	b.state = to
	b.onChange(from, to)
}
